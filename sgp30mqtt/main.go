package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"time"

	"github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439"
	"github.com/operationspark/aethernet-firmware/mqttsensor/mqtt"
	"github.com/operationspark/aethernet-firmware/mqttsensor/ntp"
	"github.com/operationspark/aethernet-firmware/mqttsensor/weather"
	"github.com/operationspark/aethernet-firmware/sgp30mqtt/warmup"
	"tinygo.org/x/drivers/dht"
	"tinygo.org/x/drivers/sgp30"
)

// Linker-injected via Makefile.
var (
	mqttServerAddr string
	mqttDeviceSlug string
	mqttTopic      string
	mqttUsername   string
	mqttPassword   string
)

const (
	startupDelay          = 2 * time.Second
	i2cFrequencyHz        = 100000
	mqttTimeout           = 5 * time.Second
	mqttTCPBufferSize     = 2030
	mqttHeartbeatInterval = 45 * time.Second

	sensorReadingBufferSize = 10
	sgp30SampleBufferSize   = 1
	sgp30SampleInterval     = time.Second
	sgp30WarmupDuration     = 15 * time.Second

	sgp30ErrorBlinkDelay = 75 * time.Millisecond
	publishBlinkDuration = 100 * time.Millisecond
	stackIdlePollDelay   = 5 * time.Millisecond
	fatalErrorLogDelay   = 10 * time.Second
)

type sgp30Sample struct {
	ECO2            uint32
	TVOC            uint32
	WarmupElapsed   time.Duration
	WarmupRemaining time.Duration
	Ready           bool
	Valid           bool
}

func main() {
	// Give USB serial + sensors time to come up before first prints/IO.
	time.Sleep(startupDelay)

	start := time.Now()
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	debugLED := machine.GP21
	debugLED.Configure(machine.PinConfig{Mode: machine.PinOutput})

	// DHT11 on GPIO0 (matches mqttsensor wiring).
	weatherSensor := weather.New(machine.GPIO0, dht.F)
	//weatherSensor.TempOffset = -11.0

	// I2C0 on GP4 (SDA) / GP5 (SCL) at 100 kHz — match the freq used by i2cscan.
	err := machine.I2C0.Configure(machine.I2CConfig{
		SDA:       machine.GP4,
		SCL:       machine.GP5,
		Frequency: i2cFrequencyHz,
	})
	if err != nil {
		printErrForever(logger, "configure I2C", slog.Any("reason", err))
	}

	sgp := sgp30.New(machine.I2C0)
	// Connected() does a CRC-checked serial-ID read and can be flaky on cold start.
	// Log the result but don't bail — Configure/Update below are the real test.
	if !sgp.Connected() {
		logger.Warn("sgp30:connected check failed, attempting configure anyway")
	}
	if err := sgp.Configure(sgp30.Config{}); err != nil {
		printErrForever(logger, "sgp30 configure", slog.Any("reason", err))
	}
	sgp30WarmupStartedAt := time.Now()
	logger.Info("sgp30:configured", slog.Int64("warmupDurationMS", sgp30WarmupDuration.Milliseconds()))

	// SGP30 is the priority for this firmware. Prove that command/response
	// reads work before starting WiFi/MQTT so we don't hide sensor failures
	// behind successful network publishes of default 0/0 values.
	waitForFirstSGP30Reading(logger, debugLED, sgp)

	sgp30Samples := make(chan sgp30Sample, sgp30SampleBufferSize)
	go sampleSGP30Forever(logger, debugLED, sgp, sgp30WarmupStartedAt, sgp30Samples)

	mqttC := &mqtt.Client{
		ID:                "tinygo-sgp30",
		DeviceSlug:        mqttDeviceSlug,
		Topic:             mqttTopic,
		Logger:            logger,
		Timeout:           mqttTimeout,
		TCPBufSize:        mqttTCPBufferSize,
		Username:          mqttUsername,
		Password:          mqttPassword,
		HeartbeatInterval: mqttHeartbeatInterval,
	}

	sensorReadings := make(chan mqtt.SensorReading, sensorReadingBufferSize)

	cystack, err := cyw43439.NewConfiguredPicoWithStack(
		cyw43439.SSID(),
		cyw43439.Password(),
		cyw43439.DefaultWifiConfig(),
		cyw43439.StackConfig{
			Hostname:    mqttC.ID,
			MaxTCPPorts: 1,
			Logger:      logger,
		},
	)
	if err != nil {
		printErrForever(logger, "wifi stack setup", slog.Any("reason", err))
	}

	go loopForeverStack(cystack)

	dhcpResults, err := cystack.SetupWithDHCP(cyw43439.DHCPConfig{
		RequestedAddr: netip.AddrFrom4([4]byte{192, 168, 1, 100}),
		Hostname:      mqttC.ID,
	})
	if err != nil {
		printErrForever(logger, "DHCP setup", slog.Any("reason", err))
	}
	logger.Info("dhcp complete", slog.String("ip", dhcpResults.AssignedAddr.String()))

	if err := ntp.SyncTime(cystack.LnetoStack(), logger); err != nil {
		logger.Error("ntp sync failed", slog.String("reason", err.Error()))
	} else {
		mqttC.TimeSyncedAt = time.Now()
		logger.Info("ntp:success", slog.Time("time", mqttC.TimeSyncedAt))
	}

	go func() {
		err := mqttC.ConnectAndPublish(cystack, mqttServerAddr, sensorReadings, nil)
		if err != nil {
			printErrForever(logger, "connect to MQTT broker", slog.Any("reason", err))
		}
	}()

	loggedFirstPublishAfterWarmup := false
	for sample := range sgp30Samples {
		if !sample.Valid {
			logger.Warn("mqtt publish skipped: invalid sgp30 sample")
			continue
		}
		if !sample.Ready {
			logger.Info("sgp30 warm-up active; publishing provisional reading",
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("remainingMS", sample.WarmupRemaining.Milliseconds()),
				slog.Int64("warmupDurationMS", sgp30WarmupDuration.Milliseconds()),
				slog.Uint64("eco2", uint64(sample.ECO2)),
				slog.Uint64("tvoc", uint64(sample.TVOC)),
			)
		}

		temp, humidity, isCached, err := weatherSensor.ReadMeasurements()
		if err != nil {
			if isCached {
				logger.Info("dht11 read failed, using cached values", slog.Any("reason", err))
			} else {
				logger.Error("dht11 read failed, no cached data", slog.Any("reason", err))
			}
		}

		reading := mqtt.SensorReading{
			Temperature: temp,
			Humidity:    humidity,
			ECO2:        sample.ECO2,
			TVOC:        sample.TVOC,
			SinceBootNS: time.Since(start),
		}
		if !mqttC.TimeSyncedAt.IsZero() {
			reading.Timestamp = time.Now()
		}

		enqueued := false
		select {
		case sensorReadings <- reading:
			enqueued = true
		default:
			logger.Warn("mqtt reading channel full; dropping sgp30 reading")
		}

		if enqueued && !loggedFirstPublishAfterWarmup {
			logger.Info("sgp30 warm-up elapsed; queued first MQTT publish",
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("warmupDurationMS", sgp30WarmupDuration.Milliseconds()),
				slog.Uint64("eco2", uint64(sample.ECO2)),
				slog.Uint64("tvoc", uint64(sample.TVOC)),
			)
			loggedFirstPublishAfterWarmup = true
		}

		debugLED.High()
		time.Sleep(publishBlinkDuration)
		debugLED.Low()
	}
}

func sampleSGP30Forever(logger *slog.Logger, debugLED machine.Pin, sgp *sgp30.Device, warmupStartedAt time.Time, samples chan sgp30Sample) {
	logger.Info("sgp30:sampler started",
		slog.Int("sampleIntervalSec", int(sgp30SampleInterval/time.Second)),
		slog.Int64("warmupDurationMS", sgp30WarmupDuration.Milliseconds()),
	)

	nextSampleTime := time.Now().Add(sgp30SampleInterval)
	sampleCount := 0

	for {
		now := time.Now()
		if now.Before(nextSampleTime) {
			time.Sleep(nextSampleTime.Sub(now))
		}
		nextSampleTime = nextSampleTime.Add(sgp30SampleInterval)

		// SGP30 requires Update() called once per second for its internal baseline.
		// On failure, publish an invalid status so an older successful sample cannot
		// be mistaken for the latest sensor state while WiFi/MQTT setup is blocking.
		if err := sgp.Update(0); err != nil {
			logger.Error("sgp30 update failed; skipping publish", slog.Any("reason", err))
			blink(debugLED, 2, sgp30ErrorBlinkDelay)
			publishLatestSGP30Sample(samples, sgp30Sample{})
			continue
		}

		sampleCount++
		warmupStatus := warmup.Status(warmupStartedAt, time.Now(), sgp30WarmupDuration)
		sample := sgp30Sample{
			ECO2:            sgp.CO2(),
			TVOC:            sgp.TVOC(),
			WarmupElapsed:   warmupStatus.Elapsed,
			WarmupRemaining: warmupStatus.Remaining,
			Valid:           true,
			Ready:           warmupStatus.Ready,
		}

		if !sample.Ready {
			logger.Info("sgp30 warm-up active",
				slog.Int("sample", sampleCount),
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("remainingMS", sample.WarmupRemaining.Milliseconds()),
				slog.Int64("warmupDurationMS", sgp30WarmupDuration.Milliseconds()),
				slog.Uint64("eco2", uint64(sample.ECO2)),
				slog.Uint64("tvoc", uint64(sample.TVOC)),
			)
			publishLatestSGP30Sample(samples, sample)
			continue
		}

		logger.Info("sgp30 read",
			slog.Uint64("eco2", uint64(sample.ECO2)),
			slog.Uint64("tvoc", uint64(sample.TVOC)),
		)
		publishLatestSGP30Sample(samples, sample)
	}
}

func publishLatestSGP30Sample(samples chan sgp30Sample, sample sgp30Sample) {
	select {
	case samples <- sample:
		return
	default:
	}

	select {
	case <-samples:
	default:
	}

	select {
	case samples <- sample:
	default:
	}
}

func waitForFirstSGP30Reading(logger *slog.Logger, debugLED machine.Pin, sgp *sgp30.Device) {
	logger.Info("sgp30:first-read waiting before WiFi/MQTT")
	for attempt := uint64(1); ; attempt++ {
		time.Sleep(sgp30SampleInterval)
		if err := sgp.Update(0); err != nil {
			logger.Error("sgp30:first-read failed",
				slog.Uint64("attempt", attempt),
				slog.Any("reason", err),
			)
			blink(debugLED, 2, sgp30ErrorBlinkDelay)
			continue
		}

		logger.Info("sgp30:first-read success",
			slog.Uint64("attempt", attempt),
			slog.Uint64("eco2", uint64(sgp.CO2())),
			slog.Uint64("tvoc", uint64(sgp.TVOC())),
		)
		return
	}
}

func blink(pin machine.Pin, count int, delay time.Duration) {
	for i := 0; i < count; i++ {
		pin.High()
		time.Sleep(delay)
		pin.Low()
		time.Sleep(delay)
	}
}

func loopForeverStack(stack *cyw43439.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(stackIdlePollDelay)
		}
	}
}

func printErrForever(logger *slog.Logger, msg string, args ...any) {
	for {
		logger.Error(msg, args...)
		time.Sleep(fatalErrorLogDelay)
	}
}

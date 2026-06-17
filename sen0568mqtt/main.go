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
	mqttTimeout           = 5 * time.Second
	mqttTCPBufferSize     = 2030
	mqttHeartbeatInterval = 45 * time.Second

	sensorReadingBufferSize = 10
	sampleBufferSize        = 1
	sampleInterval          = time.Second

	// SEN0568 is a MEMS H2S sensor and needs a preheat before its analog
	// output is trustworthy. The datasheet recommends minutes for full
	// sensitivity; this gate just keeps the first noisy readings flagged as
	// provisional. Bump it up if the workshop has time for a longer warm-up.
	warmupDuration = 60 * time.Second

	// ADC math. The Pico's onboard ADC reports a 16-bit value (0..65535) over
	// the 0..3.3V logic range.
	max16Bit uint16  = 65535
	sysV     float32 = 3.3

	// SEN0568 breakout load resistor (ohms). The sensor is a resistive MOS
	// element in a divider: VCC - Rs - AOUT - RL - GND. VCC is tied to the
	// 3.3V rail (= ADC reference), so the reading is ratiometric.
	rLoad float32 = 4700

	// Burst sampling smooths out ADC noise: discard the first read, then
	// average a burst.
	burstSize          = 32
	interSampleDelayUs = 500

	errorBlinkDelay      = 75 * time.Millisecond
	publishBlinkDuration = 100 * time.Millisecond
	stackIdlePollDelay   = 5 * time.Millisecond
	fatalErrorLogDelay   = 10 * time.Second
)

type h2sSample struct {
	Raw             uint16
	Voltage         float32
	Rs              float32 // Sensor resistance (ohms), derived from the divider.
	Ratio           float32 // Relative H2S index R0/Rs: ~1 in clean air, rises with H2S. 0 until baseline captured.
	WarmupElapsed   time.Duration
	WarmupRemaining time.Duration
	Ready           bool
	Valid           bool
}

// burstSample averages a burst of ADC reads into a single value.
//
//  1. Discard first read (sample & hold warm-up)
//  2. Take burstSize samples with an inter-sample delay
//  3. Return arithmetic mean
func burstSample(sensor machine.ADC) uint16 {
	_ = sensor.Get()

	var sum uint32
	for i := 0; i < burstSize; i++ {
		sum += uint32(sensor.Get())
		if i < burstSize-1 {
			time.Sleep(interSampleDelayUs * time.Microsecond)
		}
	}
	return uint16(sum / burstSize)
}

func rawToVoltage(raw uint16) float32 {
	return (float32(raw) / float32(max16Bit)) * sysV
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

	// DHT11 on GPIO0 (matches mqttsensor/sgp30mqtt wiring).
	weatherSensor := weather.New(machine.GPIO0, dht.F)

	// SEN0568 AOUT -> GP26 (ADC0). VCC -> 3.3V, GND -> GND.
	machine.InitADC()
	sensor := machine.ADC{Pin: machine.ADC0}
	sensor.Configure(machine.ADCConfig{})

	warmupStartedAt := time.Now()
	logger.Info("sen0568:configured", slog.Int64("warmupDurationMS", warmupDuration.Milliseconds()))

	// Prove the ADC reads before bringing up WiFi/MQTT so we don't hide a
	// disconnected sensor behind successful network publishes.
	waitForFirstReading(logger, debugLED, sensor)

	samples := make(chan h2sSample, sampleBufferSize)
	go sampleForever(logger, debugLED, sensor, warmupStartedAt, samples)

	mqttC := &mqtt.Client{
		ID:                "tinygo-sen0568",
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
	for sample := range samples {
		if !sample.Valid {
			logger.Warn("mqtt publish skipped: invalid sen0568 sample")
			continue
		}
		if !sample.Ready {
			logger.Info("sen0568 warm-up active; publishing provisional reading",
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("remainingMS", sample.WarmupRemaining.Milliseconds()),
				slog.Int64("warmupDurationMS", warmupDuration.Milliseconds()),
				slog.Uint64("raw", uint64(sample.Raw)),
				slog.Float64("rsOhms", float64(sample.Rs)),
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
			Voltage:     sample.Voltage,
			RawUInt16:   sample.Raw,
			Ratio:       sample.Ratio,
			Temperature: temp,
			Humidity:    humidity,
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
			logger.Warn("mqtt reading channel full; dropping sen0568 reading")
		}

		if enqueued && !loggedFirstPublishAfterWarmup {
			logger.Info("sen0568 warm-up elapsed; queued first MQTT publish",
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("warmupDurationMS", warmupDuration.Milliseconds()),
				slog.Uint64("raw", uint64(sample.Raw)),
				slog.Float64("ratio", float64(sample.Ratio)),
			)
			loggedFirstPublishAfterWarmup = true
		}

		debugLED.High()
		time.Sleep(publishBlinkDuration)
		debugLED.Low()
	}
}

// rawToRs converts an ADC reading into the sensor resistance Rs using the
// SEN0568 divider (VCC - Rs - AOUT - RL - GND):
//
//	Vout = VCC * RL / (Rs + RL)  =>  Rs = RL * (VCC - Vout) / Vout
//
// The GM-602B is a resistive MOS element with a nonlinear, per-part response,
// so absolute ppm needs a per-sensor calibration against a known H2S source we
// don't have in the workshop. Instead we publish the relative index R0/Rs (see
// sampleForever), which the datasheet says needs no calibration.
//
// Returns 0 for a non-positive reading (open/disconnected pin).
func rawToRs(raw uint16) float32 {
	v := rawToVoltage(raw)
	if v <= 0 {
		return 0
	}
	return rLoad * (sysV - v) / v
}

func sampleForever(logger *slog.Logger, debugLED machine.Pin, sensor machine.ADC, warmupStartedAt time.Time, samples chan h2sSample) {
	logger.Info("sen0568:sampler started",
		slog.Int("sampleIntervalSec", int(sampleInterval/time.Second)),
		slog.Int64("warmupDurationMS", warmupDuration.Milliseconds()),
	)

	nextSampleTime := time.Now().Add(sampleInterval)
	sampleCount := 0
	var r0 float32 // Clean-air baseline resistance, captured once warm-up completes.

	for {
		now := time.Now()
		if now.Before(nextSampleTime) {
			time.Sleep(nextSampleTime.Sub(now))
		}
		nextSampleTime = nextSampleTime.Add(sampleInterval)

		raw := burstSample(sensor)
		sampleCount++

		warmupStatus := warmup.Status(warmupStartedAt, time.Now(), warmupDuration)
		rs := rawToRs(raw)

		// Capture the clean-air baseline R0 the moment warm-up completes; we
		// assume the bench air is clean at that point. The relative index is
		// undefined (0) until then.
		if warmupStatus.Ready && r0 == 0 && rs > 0 {
			r0 = rs
			logger.Info("sen0568 baseline captured", slog.Float64("r0Ohms", float64(r0)))
		}

		var ratio float32
		if r0 > 0 && rs > 0 {
			ratio = r0 / rs
		}

		sample := h2sSample{
			Raw:             raw,
			Voltage:         rawToVoltage(raw),
			Rs:              rs,
			Ratio:           ratio,
			WarmupElapsed:   warmupStatus.Elapsed,
			WarmupRemaining: warmupStatus.Remaining,
			Valid:           true,
			Ready:           warmupStatus.Ready,
		}

		if !sample.Ready {
			logger.Info("sen0568 warm-up active",
				slog.Int("sample", sampleCount),
				slog.Int64("elapsedMS", sample.WarmupElapsed.Milliseconds()),
				slog.Int64("remainingMS", sample.WarmupRemaining.Milliseconds()),
				slog.Int64("warmupDurationMS", warmupDuration.Milliseconds()),
				slog.Uint64("raw", uint64(sample.Raw)),
				slog.Float64("rsOhms", float64(sample.Rs)),
			)
			publishLatestSample(samples, sample)
			continue
		}

		logger.Info("sen0568 read",
			slog.Uint64("raw", uint64(sample.Raw)),
			slog.Float64("rsOhms", float64(sample.Rs)),
			slog.Float64("ratio", float64(sample.Ratio)),
		)
		publishLatestSample(samples, sample)
	}
}

func publishLatestSample(samples chan h2sSample, sample h2sSample) {
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

func waitForFirstReading(logger *slog.Logger, debugLED machine.Pin, sensor machine.ADC) {
	logger.Info("sen0568:first-read waiting before WiFi/MQTT")
	for attempt := uint64(1); ; attempt++ {
		time.Sleep(sampleInterval)
		raw := burstSample(sensor)

		// A dead/disconnected analog pin tends to read a flat 0. Treat that as
		// a failure so we don't bring up the network around a missing sensor.
		if raw == 0 {
			logger.Error("sen0568:first-read got 0 (check AOUT->GP26 wiring)",
				slog.Uint64("attempt", attempt),
			)
			blink(debugLED, 2, errorBlinkDelay)
			continue
		}

		logger.Info("sen0568:first-read success",
			slog.Uint64("attempt", attempt),
			slog.Uint64("raw", uint64(raw)),
			slog.Float64("rsOhms", float64(rawToRs(raw))),
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

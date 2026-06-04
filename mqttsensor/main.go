package main

import (
	"errors"
	"log/slog"
	"machine"
	"net/netip"
	"strconv"
	"time"

	"github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439"
	"github.com/operationspark/aethernet-firmware/mqttsensor/lcd"
	"github.com/operationspark/aethernet-firmware/mqttsensor/mqtt"
	"github.com/operationspark/aethernet-firmware/mqttsensor/ntp"
	"github.com/operationspark/aethernet-firmware/mqttsensor/weather"
	"tinygo.org/x/drivers/dht"
	"tinygo.org/x/drivers/hd44780i2c"
)

// mqttServerAddr is the address to the MQTT broker.
// It can be passed via linker flags.
//
// Ex: "10.0.0.9:1883"
// make flash/mqtt WIFI_SSID=spacecataz WIFI_PASS=foreigner
// tinygo build -ldflags="-X 'main.mqttServerAddr=10.0.0.9:1883'
var mqttServerAddr string

// mqttUsername is the MQTT broker username for authentication.
// Optional - if empty, anonymous connection is attempted.
// Can be passed via linker flags.
var mqttUsername string

// mqttPassword is the MQTT broker password for authentication.
// Optional - only used if mqttUsername is also set.
// Can be passed via linker flags.
var mqttPassword string

const (
	max16Bit uint16  = 65535 // Max ADC value. The Pico has an onboard 16-bit ADC.
	sysV     float32 = 3.3   // Logic level in volts. Pico runs at 3.3VDC.

	// Burst sampling configuration
	sampleIntervalSec  = 1   // ADC sampling interval in seconds.
	burstSize          = 32  // Number of samples per burst
	interSampleDelayUs = 500 // Delay between samples (microseconds)
)

// burstSample takes a burst of samples from the ADC and
// averages them into a single value.
//
//  1. Discard first ADC read (sample & hold warm-up)
//  2. Take n samples with inter-sample delay
//  3. Return arithmetic mean
func burstSample(sensor machine.ADC) uint16 {
	// Step 1: Discard first read
	_ = sensor.Get()

	// Step 2: Stack-allocated array for burst samples
	var samples [burstSize]uint16

	for i := 0; i < burstSize; i++ {
		samples[i] = sensor.Get()
		if i < burstSize-1 { // Don't delay after last sample
			time.Sleep(interSampleDelayUs * time.Microsecond)
		}
	}

	// Step 3: Compute arithmetic mean
	// Use uint32 to avoid overflow: 32 * 65535 = 2,097,120
	var sum uint32
	for i := 0; i < burstSize; i++ {
		sum += uint32(samples[i])
	}

	return uint16(sum / burstSize)
}

func main() {
	start := time.Now()
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	debugLED := machine.GP21
	debugLED.Configure(machine.PinConfig{Mode: machine.PinOutput})

	btn := machine.GPIO22
	btn.Configure(machine.PinConfig{Mode: machine.PinInputPullup})

	machine.InitADC()
	sensor := machine.ADC{Pin: machine.ADC0}
	sensor.Configure(machine.ADCConfig{})

	// Setup DHT11 temperature/humidity sensor
	weatherSensor := weather.New(machine.GPIO0, dht.F)

	// Setup LCD display over I2C
	err := machine.I2C0.Configure(machine.I2CConfig{
		SDA: machine.GP4,
		SCL: machine.GP5,
	})
	if err != nil {
		printErrForever(logger, "configure I2C", slog.Any("reason", err))
	}

	lcdDev, err := configureLCD(machine.I2C0)
	if err != nil {
		for {
			println(err.Error())
			time.Sleep(time.Second)
		}
	}

	lcdDev.BacklightOn(false)
	lcdDev.ClearDisplay()

	// Create channel and start LCD handler goroutine
	lcdMessages := make(chan lcd.Message, 10)
	handler := lcd.NewHandler(lcdDev, lcdMessages, logger)
	go handler.Run()

	mqttC := &mqtt.Client{
		ID:                "tinygo-mqtt",
		Logger:            logger,
		Timeout:           5 * time.Second,
		TCPBufSize:        2030, // MTU - ethhdr - iphdr - tcphdr
		Username:          mqttUsername,
		Password:          mqttPassword,
		HeartbeatInterval: 45 * time.Second,
	}

	// Buffered channel of 10 readings. We may need to adjust depending
	// on the sensor read frequency and network availability
	sensorReadings := make(chan mqtt.SensorReading, 10)

	// ------------------------------------------------------------------
	// Network initialization (WiFi, DHCP, NTP) - done before MQTT goroutine
	// ------------------------------------------------------------------

	// 1. Create WiFi stack
	lcd.Send(lcdMessages, "Connecting to", "WiFi...")
	cystack, err := cyw43439.NewConfiguredPicoWithStack(
		cyw43439.SSID(),
		cyw43439.Password(),
		cyw43439.DefaultWifiConfig(),
		cyw43439.StackConfig{
			Hostname:    mqttC.ID,
			MaxTCPPorts: 1,
			Logger:      logger,
			// Fallback DNS server (Google Public DNS) for networks whose
			// DHCP server does not advertise one. DHCP-provided DNS, if any,
			// takes precedence.
			DNSServer: netip.AddrFrom4([4]byte{8, 8, 8, 8}),
		},
	)
	if err != nil {
		printErrForever(logger, "wifi stack setup", slog.Any("reason", err))
	}

	// 2. Start background packet processing (REQUIRED)
	go loopForeverStack(cystack)

	// 3. DHCP
	lcd.Send(lcdMessages, "Getting IP", "via DHCP...")
	// Request 0.0.0.0 so the stack performs a real DHCP lease on whatever
	// subnet it joins (correct gateway + DNS). A hardcoded address here forces
	// a static-IP fallback that breaks on any network with a different subnet.
	dhcpResults, err := cystack.SetupWithDHCP(cyw43439.DHCPConfig{
		RequestedAddr: netip.AddrFrom4([4]byte{0, 0, 0, 0}),
		Hostname:      mqttC.ID,
	})
	if err != nil {
		printErrForever(logger, "DHCP setup", slog.Any("reason", err))
	}
	logger.Info("dhcp complete", slog.String("ip", dhcpResults.AssignedAddr.String()))

	// 4. NTP sync (before starting MQTT goroutine)
	lcd.Send(lcdMessages, "Syncing time", "via NTP...")
	err = ntp.SyncTime(cystack.LnetoStack(), logger)
	if err != nil {
		logger.Error("ntp sync failed", slog.String("reason", err.Error()))
		lcd.Send(lcdMessages, "NTP sync failed", "Continuing...")
		time.Sleep(2 * time.Second)
	} else {
		mqttC.TimeSyncedAt = time.Now()
		lcd.Send(lcdMessages, "Time synced", mqttC.TimeSyncedAt.Format("15:04:05"))
		logger.Info("ntp:success", slog.Time("time", mqttC.TimeSyncedAt))
		time.Sleep(2 * time.Second)
	}

	// 5. Start MQTT in goroutine (pass stack)
	go func() {
		err := mqttC.ConnectAndPublish(cystack, mqttServerAddr, sensorReadings, lcdMessages)
		if err != nil {
			// Print error in a loop in case the serial monitor is not
			// ready before the initial messages
			printErrForever(logger, "connect to MQTT broker", slog.Any("reason", err))
		}
	}()

	// Read sensor, display readings on LCD and send off to MQTT broker
	// _________________________________________________________________

	// Buffer for LCD characters (16x2)
	// We need a preallocated buffer so the heap isn't exhausted
	// by many calls to fmt functions.
	line1 := make([]byte, 0, 20)
	line2 := make([]byte, 0, 20)
	const floatNoExp = 'f'

	// NTP is now complete (or failed) at this point - no need to wait
	logger.Info("sample interval", slog.Int("v", sampleIntervalSec))

	// Initialize next sample time for interval-based sampling
	nextSampleTime := time.Now().Add(time.Duration(sampleIntervalSec) * time.Second)

	for {
		// reslice the buffers to zero-length so append continues to work
		line1 = line1[:0]
		line2 = line2[:0]

		// Wait until next sampling interval
		now := time.Now()
		if now.Before(nextSampleTime) {
			time.Sleep(nextSampleTime.Sub(now))
		}

		// Perform burst sampling
		val := burstSample(sensor)

		// Read temperature and humidity from DHT11
		// Note: ReadMeasurements uses throttling/caching, so it returns cached values
		// when called too frequently (< 2s interval) or on sensor error
		temp, humidity, isCached, err := weatherSensor.ReadMeasurements()
		if err != nil {
			if isCached {
				// Using cached values - log at INFO level (less critical)
				logger.Info("dht11 read failed, using cached values", slog.Any("reason", err))
			} else {
				// No cached values available - log at ERROR level (critical)
				logger.Error("dht11 read failed, no cached data", slog.Any("reason", err))
			}
		}

		// Update next sample time (before processing to maintain consistent intervals)
		nextSampleTime = nextSampleTime.Add(time.Duration(sampleIntervalSec) * time.Second)
		percentage := (float32(val) / float32(max16Bit))
		voltage := percentage * sysV

		// Ex line1: "ADC: 3.2v, 63452"
		line1 = append(line1, "ADC: "...)
		line1 = strconv.AppendFloat(line1, float64(voltage), floatNoExp, 1, 32)
		line1 = append(line1, "v, "...)
		line1 = strconv.AppendUint(line1, uint64(val), 10)

		// Ex line2: "Temp:22.5C H:45%"
		line2 = append(line2, "Temp:"...)
		line2 = strconv.AppendFloat(line2, float64(temp), floatNoExp, 1, 32)
		line2 = append(line2, "F H:"...)
		line2 = strconv.AppendInt(line2, int64(humidity), 10)
		line2 = append(line2, "%"...)

		// Non-blocking send to LCD
		select {
		case lcdMessages <- lcd.Message{Line1: line1, Line2: line2}:
		default:
			// Channel full - drop message to keep sensor loop responsive
		}

		// Non-blocking send to prevent main loop from blocking when channel is full
		reading := mqtt.SensorReading{
			Voltage:     voltage,
			RawUInt16:   val,
			Temperature: temp,
			Humidity:    humidity,
			SinceBootNS: time.Since(start),
		}
		// Only set Timestamp if NTP sync succeeded
		if !mqttC.TimeSyncedAt.IsZero() {
			reading.Timestamp = time.Now()
		}

		select {
		case sensorReadings <- reading:
		default:
			// Channel full - drop this reading to keep LCD responsive
		}

		// Single LED blink to indicate burst sampling completed
		debugLED.High()
		time.Sleep(100 * time.Millisecond)
		debugLED.Low()
	}
}

// loopForeverStack runs the network stack's send/receive loop.
// This must run in a background goroutine for networking to function.
func loopForeverStack(stack *cyw43439.Stack) {
	for {
		send, recv, _ := stack.RecvAndSend()
		if send == 0 && recv == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

// configureLCD takes a preconfigured I2C peripheral and attempts to
// initialize the HD44780 LCD display. If no LCD found on the commond I2C
// addresses (0x27, 0x3F), an error is returned.
func configureLCD(i2c *machine.I2C) (hd44780i2c.Device, error) {
	// Try common addresses (0x27 then 0x3F)
	addrs := []uint8{0x27, 0x3F}
	var lcd hd44780i2c.Device
	found := false

	for _, a := range addrs {
		println("checking I2C address...")
		dev := hd44780i2c.New(i2c, a)
		dev.Configure(hd44780i2c.Config{
			Width:  16,
			Height: 2,
		})

		lcd = dev
		found = true
		break
	}

	if !found {
		return lcd, errors.New("LCD not found on addresses: 0x27, 0x3f")
	}
	return lcd, nil
}

// printError prints a string to serial @ 1hz. It
// blocks forever.
func printErrForever(logger *slog.Logger, msg string, args ...any) {
	for {
		logger.Error(msg, args...)
		time.Sleep(time.Second * 10)
	}
}

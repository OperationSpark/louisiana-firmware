package main

import (
	"fmt"
	"time"

	"github.com/operationspark/aethernet-firmware/internal/board"
	"github.com/operationspark/aethernet-firmware/internal/sensors"
)

const (
	nodeID          = "field-01"
	startupDelay    = 5 * time.Second // ponytail: USB CDC needs time to enumerate after reset; raise if boot lines still drop
	readingInterval = 2 * time.Second

	// ponytail: SGP41 compensation defaults when no BME280 is attached.
	// These are Sensirion's own defaults; swap in a DHT reading when one lands.
	defaultTemperatureC = 25.0
	defaultHumidityPct  = 50.0

	// Real STEMMA QT chains carry a handful of devices; more means a bus fault.
	maxPlausibleDevices = 8
)

func main() {
	bootedAt := time.Now()
	time.Sleep(startupDelay)

	// Short writes with gaps: the ESP32-S3 USB-Serial/JTAG FIFO is 64 bytes,
	// and a single larger write loses everything past the first packet.
	fmt.Println("BOOT field sensor test")
	time.Sleep(150 * time.Millisecond)
	fmt.Println("[BOOT] TinyGo environmental node")
	time.Sleep(150 * time.Millisecond)
	fmt.Println("[I2C] configuring SDA=D4/GPIO5 SCL=D5/GPIO6 frequency=100000Hz")
	time.Sleep(150 * time.Millisecond)
	if err := board.ConfigureI2C(); err != nil {
		stop("[I2C] ERROR: configure failed: ", err)
	}
	fmt.Println("[I2C] configure returned")
	time.Sleep(150 * time.Millisecond)
	fmt.Println("[I2C] configured; scanning 0x08 through 0x77")
	time.Sleep(150 * time.Millisecond)

	var found [0x78]bool
	deviceCount := 0
	probe := []byte{0}
	for address := uint16(0x08); address <= 0x77; address++ {
		if err := board.I2C.Tx(address, probe, nil); err == nil {
			found[address] = true
			deviceCount++
			fmt.Printf("I2C device found: 0x%02X\n", address)
		}
	}

	fmt.Println("[I2C] scan complete")
	// A dozen-plus hits means SDA is stuck low, so every address reads as an ACK.
	// The usual cause is no breakout attached: the pullups live on the sensor boards.
	if deviceCount > maxPlausibleDevices {
		fmt.Printf("[I2C] ERROR: %d devices is a stuck-low SDA, not a real bus\n", deviceCount)
		stop("[I2C] no breakout attached means no pullups: ", nil)
	}
	if !found[sensors.SGP41Address] {
		stop("[I2C] ERROR: SGP41 missing at 0x59: ", nil)
	}
	fmt.Println("SGP41 detected")

	bmeAddress := uint16(0)
	if found[sensors.BME280AddressHigh] {
		bmeAddress = sensors.BME280AddressHigh
	} else if found[sensors.BME280AddressLow] {
		bmeAddress = sensors.BME280AddressLow
	}

	// Let devices discard the scanner's harmless one-byte probe before commands.
	time.Sleep(150 * time.Millisecond)

	// ponytail: BME280 is optional; a nil bme means fixed-compensation mode.
	var bme *sensors.BME280
	reading := sensors.EnvironmentalReading{
		TemperatureC: defaultTemperatureC,
		HumidityPct:  defaultHumidityPct,
	}
	if bmeAddress == 0 {
		fmt.Println("[BME280] WARN: not found at 0x76 or 0x77")
		time.Sleep(150 * time.Millisecond)
		fmt.Printf("[BME280] running without it: %.1fC %.0f%%RH assumed\n", defaultTemperatureC, defaultHumidityPct)
	} else if candidate, err := sensors.NewBME280(board.I2C, bmeAddress); err != nil {
		fmt.Printf("[BME280] WARN: init failed, running without it: %v\n", err)
	} else if first, err := candidate.Read(); err != nil {
		fmt.Printf("[BME280] WARN: first read failed, running without it: %v\n", err)
	} else {
		bme, reading = candidate, first
		fmt.Printf("[BME280] ready address=0x%02X\n", bmeAddress)
	}
	time.Sleep(150 * time.Millisecond)

	sgp := sensors.NewSGP41(board.I2C)
	serial, err := sgp.SerialNumber()
	if err != nil {
		stop("[SGP41] ERROR: serial number read failed: ", err)
	}
	fmt.Printf("[SGP41] serial=%012X\n", serial)
	fmt.Println("[SGP41] self-test")
	if err := sgp.SelfTest(); err != nil {
		stop("[SGP41] ERROR: ", err)
	}

	fmt.Println("[SGP41] conditioning for 10 seconds")
	conditioningStarted := time.Now()
	for second := 1; second <= 10; second++ {
		if bme != nil {
			if current, readErr := bme.Read(); readErr != nil {
				fmt.Printf("[BME280] ERROR: conditioning compensation read failed; using last reading: %v\n", readErr)
			} else {
				reading = current
			}
		}
		if _, err := sgp.ExecuteConditioning(reading.TemperatureC, reading.HumidityPct); err != nil {
			stop("[SGP41] ERROR: conditioning failed: ", err)
		}
		fmt.Printf("[SGP41] conditioning %d/10\n", second)
		time.Sleep(time.Until(conditioningStarted.Add(time.Duration(second) * time.Second)))
	}
	fmt.Println("[SGP41] ready")
	fmt.Println("[APP] starting environmental loop")

	var sequence uint32
	for {
		cycleStarted := time.Now()
		if bme != nil {
			current, err := bme.Read()
			if err != nil {
				fmt.Printf("[BME280] ERROR: reading failed; retrying next cycle: %v\n", err)
				sleepUntil(cycleStarted.Add(readingInterval))
				continue
			}
			reading = current
		}
		sequence++
		uptimeMS := uint64(time.Since(bootedAt).Milliseconds())
		srawVOC, srawNOx, gasErr := sgp.MeasureRaw(reading.TemperatureC, reading.HumidityPct)

		// Absent hardware reports null rather than an assumed value, so downstream
		// never mistakes the compensation default for a real measurement.
		tempField, rhField, pressureField := "null", "null", "null"
		if bme != nil {
			tempField = fmt.Sprintf("%.2f", reading.TemperatureC)
			rhField = fmt.Sprintf("%.2f", reading.HumidityPct)
			pressureField = fmt.Sprintf("%.2f", reading.PressureHPa)
		}
		vocField, noxField := "null", "null"
		if gasErr == nil {
			vocField = fmt.Sprintf("%d", srawVOC)
			noxField = fmt.Sprintf("%d", srawNOx)
		}

		fmt.Printf("\nEnvironmental reading #%d\n\n", sequence)
		if bme == nil {
			fmt.Println("temperature_c: unavailable")
			fmt.Println("humidity_pct: unavailable")
			fmt.Println("pressure_hpa: unavailable")
		} else {
			fmt.Printf("temperature_c: %s\n", tempField)
			fmt.Printf("humidity_pct: %s\n", rhField)
			fmt.Printf("pressure_hpa: %s\n", pressureField)
		}
		if gasErr != nil {
			fmt.Printf("[SGP41] ERROR: measurement failed; retrying next cycle: %v\n", gasErr)
		}
		fmt.Printf("sraw_voc: %s\n", vocField)
		fmt.Printf("sraw_nox: %s\n", noxField)
		fmt.Printf("{\"node\":\"%s\",\"seq\":%d,\"temp_c\":%s,\"rh_pct\":%s,\"pressure_hpa\":%s,\"sraw_voc\":%s,\"sraw_nox\":%s,\"uptime_ms\":%d}\n",
			nodeID, sequence, tempField, rhField, pressureField, vocField, noxField, uptimeMS)

		sleepUntil(cycleStarted.Add(readingInterval))
	}
}

func sleepUntil(deadline time.Time) {
	if delay := time.Until(deadline); delay > 0 {
		time.Sleep(delay)
	}
}

func stop(message string, err error) {
	if err != nil {
		fmt.Printf("%s%s\n", message, err.Error())
	} else {
		fmt.Printf("%scheck power and STEMMA QT wiring\n", message)
	}
	time.Sleep(100 * time.Millisecond)
	for {
		time.Sleep(time.Hour)
	}
}

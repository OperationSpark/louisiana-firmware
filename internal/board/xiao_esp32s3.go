// Package board centralizes the XIAO ESP32S3 hardware mapping.
package board

import (
	"machine"
	"time"
)

const I2CFrequencyHz = 100_000

var (
	I2C    = machine.I2C0
	I2CSDA = machine.GPIO5 // XIAO D4
	I2CSCL = machine.GPIO6 // XIAO D5
)

// PullupsPresent drives each line low, releases it as a plain input with no
// internal pull, and samples. The 10k pullups on a STEMMA QT breakout snap the
// line back high in about a microsecond; with nothing attached the line has only
// pin leakage to charge it and still reads low. Reading with the internal pullup
// enabled cannot tell those apart, since both read high.
func PullupsPresent() (sda, scl bool) {
	return pullupPresent(I2CSDA), pullupPresent(I2CSCL)
}

func pullupPresent(pin machine.Pin) bool {
	pin.Configure(machine.PinConfig{Mode: machine.PinOutput})
	pin.Low()
	time.Sleep(time.Millisecond)
	pin.Configure(machine.PinConfig{Mode: machine.PinInput})
	// A 5k-10k pullup charges the bus in about a microsecond. Sampling in the same
	// breath as the release reads the line before it has risen and reports no pullup
	// on a perfectly good bus, so give it time it cannot possibly need.
	time.Sleep(time.Millisecond)
	return pin.Get()
}

func ConfigureI2C() error {
	return I2C.Configure(machine.I2CConfig{
		SDA:       I2CSDA,
		SCL:       I2CSCL,
		Frequency: I2CFrequencyHz,
	})
}

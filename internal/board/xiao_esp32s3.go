// Package board centralizes the XIAO ESP32S3 hardware mapping.
package board

import "machine"

const I2CFrequencyHz = 100_000

var (
	I2C    = machine.I2C0
	I2CSDA = machine.GPIO5 // XIAO D4
	I2CSCL = machine.GPIO6 // XIAO D5
)

func ConfigureI2C() error {
	return I2C.Configure(machine.I2CConfig{
		SDA:       I2CSDA,
		SCL:       I2CSCL,
		Frequency: I2CFrequencyHz,
	})
}

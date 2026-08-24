package sensors

import (
	"errors"

	"tinygo.org/x/drivers"
	"tinygo.org/x/drivers/bme280"
)

const (
	BME280AddressLow  = 0x76
	BME280AddressHigh = 0x77
)

type EnvironmentalReading struct {
	TemperatureC float32
	HumidityPct  float32
	PressureHPa  float32
}

type BME280 struct {
	device bme280.Device
}

func NewBME280(bus drivers.I2C, address uint16) (*BME280, error) {
	if address != BME280AddressLow && address != BME280AddressHigh {
		return nil, errors.New("BME280 address must be 0x76 or 0x77")
	}

	device := bme280.New(bus)
	device.Address = address
	if !device.Connected() {
		return nil, errors.New("BME280 chip ID mismatch")
	}
	device.Configure()

	return &BME280{device: device}, nil
}

func (b *BME280) Read() (EnvironmentalReading, error) {
	temperature, err := b.device.ReadTemperature()
	if err != nil {
		return EnvironmentalReading{}, err
	}
	humidity, err := b.device.ReadHumidity()
	if err != nil {
		return EnvironmentalReading{}, err
	}
	pressure, err := b.device.ReadPressure()
	if err != nil {
		return EnvironmentalReading{}, err
	}

	return EnvironmentalReading{
		TemperatureC: float32(temperature) / 1_000,
		HumidityPct:  float32(humidity) / 100,
		PressureHPa:  float32(pressure) / 100_000,
	}, nil
}

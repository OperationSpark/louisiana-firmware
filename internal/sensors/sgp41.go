package sensors

import (
	"errors"
	"fmt"
	"time"

	"tinygo.org/x/drivers"
)

const SGP41Address = 0x59

const (
	sgp41ConditioningCommand = 0x2612
	sgp41MeasureCommand      = 0x2619
	sgp41SelfTestCommand     = 0x280e
	sgp41SerialCommand       = 0x3682
	sgp41SelfTestPassed      = 0xd400
)

type SGP41 struct {
	bus drivers.I2C
}

func NewSGP41(bus drivers.I2C) *SGP41 {
	return &SGP41{bus: bus}
}

func (s *SGP41) SerialNumber() (uint64, error) {
	if err := s.writeCommand(sgp41SerialCommand); err != nil {
		return 0, err
	}
	time.Sleep(time.Millisecond)

	var response [9]byte
	if err := s.bus.Tx(SGP41Address, nil, response[:]); err != nil {
		return 0, err
	}
	if err := checkWords(response[:]); err != nil {
		return 0, err
	}

	return uint64(response[0])<<40 |
		uint64(response[1])<<32 |
		uint64(response[3])<<24 |
		uint64(response[4])<<16 |
		uint64(response[6])<<8 |
		uint64(response[7]), nil
}

func (s *SGP41) SelfTest() error {
	if err := s.writeCommand(sgp41SelfTestCommand); err != nil {
		return err
	}
	time.Sleep(320 * time.Millisecond)

	var response [3]byte
	if err := s.bus.Tx(SGP41Address, nil, response[:]); err != nil {
		return err
	}
	if err := checkWords(response[:]); err != nil {
		return err
	}
	result := uint16(response[0])<<8 | uint16(response[1])
	if result != sgp41SelfTestPassed {
		return fmt.Errorf("SGP41 self-test failed: result=0x%04x", result)
	}
	return nil
}

func (s *SGP41) ExecuteConditioning(temperatureC, humidityPct float32) (uint16, error) {
	if err := s.writeCompensatedCommand(sgp41ConditioningCommand, temperatureC, humidityPct); err != nil {
		return 0, err
	}
	time.Sleep(50 * time.Millisecond)

	var response [3]byte
	if err := s.bus.Tx(SGP41Address, nil, response[:]); err != nil {
		return 0, err
	}
	if err := checkWords(response[:]); err != nil {
		return 0, err
	}
	return uint16(response[0])<<8 | uint16(response[1]), nil
}

func (s *SGP41) MeasureRaw(temperatureC, humidityPct float32) (srawVOC, srawNOx uint16, err error) {
	if err = s.writeCompensatedCommand(sgp41MeasureCommand, temperatureC, humidityPct); err != nil {
		return 0, 0, err
	}
	time.Sleep(50 * time.Millisecond)

	var response [6]byte
	if err = s.bus.Tx(SGP41Address, nil, response[:]); err != nil {
		return 0, 0, err
	}
	if err = checkWords(response[:]); err != nil {
		return 0, 0, err
	}
	return uint16(response[0])<<8 | uint16(response[1]),
		uint16(response[3])<<8 | uint16(response[4]), nil
}

func (s *SGP41) writeCommand(command uint16) error {
	data := [2]byte{byte(command >> 8), byte(command)}
	return s.bus.Tx(SGP41Address, data[:], nil)
}

func (s *SGP41) writeCompensatedCommand(command uint16, temperatureC, humidityPct float32) error {
	humidity := RelativeHumidityTicks(humidityPct)
	temperature := TemperatureTicks(temperatureC)
	data := [8]byte{
		byte(command >> 8), byte(command),
		byte(humidity >> 8), byte(humidity), 0,
		byte(temperature >> 8), byte(temperature), 0,
	}
	data[4] = SensirionCRC8(data[2:4])
	data[7] = SensirionCRC8(data[5:7])
	return s.bus.Tx(SGP41Address, data[:], nil)
}

func RelativeHumidityTicks(relativeHumidityPct float32) uint16 {
	if relativeHumidityPct <= 0 {
		return 0
	}
	if relativeHumidityPct >= 100 {
		return 0xffff
	}
	return uint16(relativeHumidityPct*65535/100 + 0.5)
}

func TemperatureTicks(temperatureC float32) uint16 {
	if temperatureC <= -45 {
		return 0
	}
	if temperatureC >= 130 {
		return 0xffff
	}
	return uint16((temperatureC+45)*65535/175 + 0.5)
}

func SensirionCRC8(data []byte) byte {
	crc := byte(0xff)
	for _, value := range data {
		crc ^= value
		for bit := uint8(0); bit < 8; bit++ {
			if crc&0x80 != 0 {
				crc = crc<<1 ^ 0x31
			} else {
				crc <<= 1
			}
		}
	}
	return crc
}

func checkWords(data []byte) error {
	if len(data)%3 != 0 {
		return errors.New("SGP41 response length is not a whole number of words")
	}
	for i := 0; i < len(data); i += 3 {
		if SensirionCRC8(data[i:i+2]) != data[i+2] {
			return fmt.Errorf("SGP41 CRC mismatch at word %d", i/3)
		}
	}
	return nil
}

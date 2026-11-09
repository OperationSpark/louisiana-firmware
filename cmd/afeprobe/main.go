// afeprobe: bring-up check for Pico W -> Grove 5 -> SmartCitizen ASB (2x ADS1115) -> Alphasense AFE.
// Scans I2C0 on GP8/GP9, then prints single-ended voltage of all 8 ADC channels every 2 s.
package main

import (
	"fmt"
	"machine"
	"time"
)

var i2c = machine.I2C0

// readADS does one single-shot conversion of AINch vs GND at PGA ±4.096 V, 128 SPS.
func readADS(addr uint16, ch uint8) (float32, error) {
	cfg := []byte{0x01, 0xC3 | (ch << 4), 0x83} // reg, OS|MUX=1xx|PGA=001|single, DR=128|comp off
	if err := i2c.Tx(addr, cfg, nil); err != nil {
		return 0, err
	}
	time.Sleep(10 * time.Millisecond) // 128 SPS = 7.8 ms
	var b [2]byte
	if err := i2c.Tx(addr, []byte{0x00}, b[:]); err != nil {
		return 0, err
	}
	return float32(int16(uint16(b[0])<<8|uint16(b[1]))) * 4.096 / 32768, nil
}

func main() {
	time.Sleep(3 * time.Second) // let USB serial attach
	if err := i2c.Configure(machine.I2CConfig{SDA: machine.GP8, SCL: machine.GP9, Frequency: 100_000}); err != nil {
		println("i2c configure:", err.Error())
	}

	for {
		print("scan:")
		for a := uint16(0x08); a < 0x78; a++ {
			var b [1]byte
			if i2c.Tx(a, nil, b[:]) == nil {
				fmt.Printf(" 0x%02X", a)
			}
		}
		println()

		for _, addr := range []uint16{0x48, 0x49} {
			fmt.Printf("0x%02X", addr)
			for ch := uint8(0); ch < 4; ch++ {
				v, err := readADS(addr, ch)
				if err != nil {
					fmt.Printf("  A%d=ERR", ch)
					continue
				}
				fmt.Printf("  A%d=%.4fV", ch, v)
			}
			println()
		}
		time.Sleep(2 * time.Second)
	}
}

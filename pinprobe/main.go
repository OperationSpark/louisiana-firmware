package main

import (
	"machine"
	"time"
)

// Pinprobe scans GP0..GP28 and reports external connections.
//
// Method: read each pin twice — once with internal pulldown, once with pullup.
//   pulldown=HIGH  -> externally tied HIGH (likely to VCC, e.g. pull-up resistor or VCC rail)
//   pullup=LOW     -> externally tied LOW  (likely to GND)
//   pulldown=LOW & pullup=HIGH -> floating (nothing connected)
//   pulldown=HIGH & pullup=LOW -> conflicting (rare; weak source on both sides)

var skip = map[int]bool{
	23: true, // CYW43 power
	24: true, // CYW43 IRQ
	25: true, // onboard LED (CYW43)
	29: true, // VSYS sense
}

func main() {
	time.Sleep(2 * time.Second) // let serial monitor attach

	println("=== pinprobe: GP0..GP28 ===")
	println("legend: H=high L=low FLOAT=nothing TIED_HIGH=ext pullup/VCC TIED_LOW=ext gnd")
	println()

	for i := 0; i <= 28; i++ {
		if skip[i] {
			continue
		}
		pin := machine.Pin(i)

		pin.Configure(machine.PinConfig{Mode: machine.PinInputPulldown})
		time.Sleep(2 * time.Millisecond)
		pd := pin.Get()

		pin.Configure(machine.PinConfig{Mode: machine.PinInputPullup})
		time.Sleep(2 * time.Millisecond)
		pu := pin.Get()

		var state string
		switch {
		case pd && !pu:
			state = "CONFLICT"
		case pd && pu:
			state = "TIED_HIGH"
		case !pd && !pu:
			state = "TIED_LOW"
		default:
			state = "FLOAT"
		}

		printGP(i)
		print(" pulldown=")
		printBit(pd)
		print(" pullup=")
		printBit(pu)
		print(" -> ")
		println(state)
	}

	println()
	println("=== done ===")

	for {
		time.Sleep(time.Hour)
	}
}

func printGP(n int) {
	print("GP")
	if n < 10 {
		print(" ")
	}
	print(n)
}

func printBit(b bool) {
	if b {
		print("H")
	} else {
		print("L")
	}
}

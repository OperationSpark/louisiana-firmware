package main

import (
	"net"
	"time"

	"github.com/soypat/cyw43439"
)

// Macprobe prints the Pico W's WiFi MAC address over serial. Use the lowercased
// value as MQTT_DEVICE when flashing the sensor firmware. No WiFi creds needed —
// the MAC is readable right after chip Init.
func main() {
	time.Sleep(2 * time.Second) // let serial monitor attach

	dev := cyw43439.NewPicoWDevice()
	if err := dev.Init(cyw43439.DefaultWifiConfig()); err != nil {
		println("wifi init failed:", err.Error())
		halt()
	}

	mac, err := dev.HardwareAddr6()
	if err != nil {
		println("get hardware address:", err.Error())
		halt()
	}

	println("=== Pico W MAC ===")
	println(net.HardwareAddr(mac[:]).String())
	println("set MQTT_DEVICE to the lowercased value above")
	halt()
}

func halt() {
	for {
		time.Sleep(time.Hour)
	}
}

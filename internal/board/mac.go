package board

import (
	"unsafe"

	"runtime/volatile"
)

// The factory MAC lives in eFuse BLK1. The controller latches every block into
// shadow registers during boot, so a plain read is enough: no clock gating or
// read-command sequence like the ADC calibration fuses need.
//
// Field layout is IDF's ESP_EFUSE_MAC table for the ESP32-S3, which stores the
// address most-significant byte last: mac[5] is bits 0..7, mac[0] is bits 40..47.
const (
	efuseRdMac0Reg = uintptr(0x60007044)
	efuseRdMac1Reg = uintptr(0x60007048)
)

// MAC returns the factory base MAC address burned into eFuse BLK1. On the
// ESP32-S3 the station WiFi interface uses this address unchanged, so it is the
// same value esptool reports and the same one a broker sees on connect.
func MAC() [6]byte {
	low := (*volatile.Register32)(unsafe.Pointer(efuseRdMac0Reg)).Get()
	high := (*volatile.Register32)(unsafe.Pointer(efuseRdMac1Reg)).Get()
	return decodeMAC(low, high)
}

func decodeMAC(low, high uint32) [6]byte {
	return [6]byte{
		byte(high >> 8),
		byte(high),
		byte(low >> 24),
		byte(low >> 16),
		byte(low >> 8),
		byte(low),
	}
}

// MACString formats a MAC as the lowercase colon-separated form used by esptool
// and most broker logs.
func MACString(mac [6]byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 17)
	for i, b := range mac {
		if i > 0 {
			out = append(out, ':')
		}
		out = append(out, hex[b>>4], hex[b&0x0f])
	}
	return string(out)
}

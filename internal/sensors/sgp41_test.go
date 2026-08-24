package sensors

import "testing"

func TestSensirionCRC8(t *testing.T) {
	tests := []struct {
		data []byte
		want byte
	}{
		{[]byte{0x80, 0x00}, 0xa2},
		{[]byte{0x66, 0x66}, 0x93},
		{[]byte{0xbe, 0xef}, 0x92},
	}
	for _, test := range tests {
		if got := SensirionCRC8(test.data); got != test.want {
			t.Fatalf("CRC8(%x) = 0x%02x, want 0x%02x", test.data, got, test.want)
		}
	}
}

func TestCheckWordsRejectsCorruption(t *testing.T) {
	valid := []byte{0xbe, 0xef, 0x92}
	if err := checkWords(valid); err != nil {
		t.Fatalf("valid response rejected: %v", err)
	}
	valid[2] ^= 1
	if err := checkWords(valid); err == nil {
		t.Fatal("corrupted response accepted")
	}
}

func TestCompensationTicks(t *testing.T) {
	if got := RelativeHumidityTicks(50); got != 0x8000 {
		t.Fatalf("50%% RH = 0x%04x, want 0x8000", got)
	}
	if got := TemperatureTicks(25); got != 0x6666 {
		t.Fatalf("25 C = 0x%04x, want 0x6666", got)
	}
}

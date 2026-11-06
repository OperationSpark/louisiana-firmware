package sensors

import (
	"math"
	"testing"
)

func TestAlphasensePPB(t *testing.T) {
	c := AlphasenseCal{WEe: 300, AEe: 310, SensMVPerP: 0.4, NT: 1}
	// WE +20 mV, AE +4 mV over zero → 16 mV net / 0.4 = 40 ppb.
	got, ok := c.PPB(320, 314)
	if !ok || math.Abs(got-40) > 1e-9 {
		t.Fatalf("PPB = %v, %v; want 40, true", got, ok)
	}
	if _, ok := (AlphasenseCal{}).PPB(320, 314); ok {
		t.Fatal("uncalibrated cal must return ok=false")
	}
}

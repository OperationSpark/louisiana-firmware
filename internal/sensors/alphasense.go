package sensors

// AlphasenseCal holds factory-certificate values for one Alphasense A4 sensor on an AFE.
// All voltages are AFE output millivolts.
type AlphasenseCal struct {
	Serial     string
	WEe        float64 // WE electronic zero (mV)
	AEe        float64 // AE electronic zero (mV)
	SensMVPerP float64 // WE sensitivity in mV/ppb (cert nA/ppb × PCB gain mV/nA). 0 = not calibrated.
	NT         float64 // Algorithm 1 temperature factor nT. ponytail: fixed value, swap for the Alphasense nT table once a temperature source is wired.
}

// PPB applies Alphasense Algorithm 1: ((WE - WEe) - nT·(AE - AEe)) / sensitivity.
// ok is false when no sensitivity has been entered, so callers never publish an uncalibrated ppb.
func (c AlphasenseCal) PPB(weMV, aeMV float64) (ppb float64, ok bool) {
	if c.SensMVPerP == 0 {
		return 0, false
	}
	return ((weMV - c.WEe) - c.NT*(aeMV-c.AEe)) / c.SensMVPerP, true
}

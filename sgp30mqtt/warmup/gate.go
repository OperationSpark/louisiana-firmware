package warmup

import "time"

// ReadingStatus describes whether SGP30 readings have passed the configured
// warm-up duration and are eligible for publishing.
type ReadingStatus struct {
	Ready     bool
	Elapsed   time.Duration
	Remaining time.Duration
}

// Status reports warm-up readiness based on elapsed wall time since SGP30
// warm-up started. Negative elapsed time is clamped so a clock adjustment does
// not produce confusing logs.
func Status(startedAt, now time.Time, duration time.Duration) ReadingStatus {
	elapsed := now.Sub(startedAt)
	if elapsed < 0 {
		elapsed = 0
	}

	if duration <= 0 || elapsed >= duration {
		return ReadingStatus{
			Ready:   true,
			Elapsed: elapsed,
		}
	}

	return ReadingStatus{
		Ready:     false,
		Elapsed:   elapsed,
		Remaining: duration - elapsed,
	}
}

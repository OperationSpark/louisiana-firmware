package warmup

import (
	"testing"
	"time"
)

func TestStatusBeforeDurationIsNotReady(t *testing.T) {
	startedAt := time.Unix(100, 0)
	status := Status(startedAt, startedAt.Add(15*time.Second-time.Nanosecond), 15*time.Second)

	if status.Ready {
		t.Fatal("expected warm-up to still be active before the configured duration elapses")
	}
	if status.Elapsed != 15*time.Second-time.Nanosecond {
		t.Fatalf("elapsed = %v, want %v", status.Elapsed, 15*time.Second-time.Nanosecond)
	}
	if status.Remaining != time.Nanosecond {
		t.Fatalf("remaining = %v, want %v", status.Remaining, time.Nanosecond)
	}
}

func TestStatusAtDurationIsReady(t *testing.T) {
	startedAt := time.Unix(100, 0)
	status := Status(startedAt, startedAt.Add(15*time.Second), 15*time.Second)

	if !status.Ready {
		t.Fatal("expected warm-up to be ready once the configured duration elapses")
	}
	if status.Elapsed != 15*time.Second {
		t.Fatalf("elapsed = %v, want %v", status.Elapsed, 15*time.Second)
	}
	if status.Remaining != 0 {
		t.Fatalf("remaining = %v, want 0", status.Remaining)
	}
}

func TestStatusClampsClockSkewAndZeroDuration(t *testing.T) {
	startedAt := time.Unix(100, 0)
	status := Status(startedAt, startedAt.Add(-time.Second), 15*time.Second)
	if status.Elapsed != 0 {
		t.Fatalf("elapsed with backward clock = %v, want 0", status.Elapsed)
	}
	if status.Remaining != 15*time.Second {
		t.Fatalf("remaining with backward clock = %v, want %v", status.Remaining, 15*time.Second)
	}

	status = Status(startedAt, startedAt, 0)
	if !status.Ready {
		t.Fatal("expected zero warm-up duration to be ready immediately")
	}
	if status.Remaining != 0 {
		t.Fatalf("remaining with zero duration = %v, want 0", status.Remaining)
	}
}

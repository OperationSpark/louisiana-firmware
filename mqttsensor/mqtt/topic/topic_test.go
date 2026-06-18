package topic

import (
	"strings"
	"testing"
)

func TestResolveDefaultTopicRequiresDeviceSlug(t *testing.T) {
	_, err := Resolve("", "gateway", "")
	if err == nil {
		t.Fatal("expected missing MQTT_DEVICE to reject default LouisianAir topic")
	}
	if !strings.Contains(err.Error(), "MQTT_DEVICE") {
		t.Fatalf("error = %q, want MQTT_DEVICE guidance", err.Error())
	}
}

func TestResolveDefaultTopic(t *testing.T) {
	got, err := Resolve("", "gateway", "picopuff")
	if err != nil {
		t.Fatal(err)
	}
	want := "louisianair/dev/gateway/picopuff/telemetry"
	if got != want {
		t.Fatalf("topic = %q, want %q", got, want)
	}
}

func TestResolveLowercasesDeviceSlug(t *testing.T) {
	got, err := Resolve("", "gateway", "AA:BB:CC:DD:EE:20")
	if err != nil {
		t.Fatal(err)
	}
	want := "louisianair/dev/gateway/aa:bb:cc:dd:ee:20/telemetry"
	if got != want {
		t.Fatalf("topic = %q, want %q", got, want)
	}
}

func TestResolveTopicOverrideSkipsDefaultRequirements(t *testing.T) {
	got, err := Resolve("custom/topic", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got != "custom/topic" {
		t.Fatalf("topic = %q, want custom/topic", got)
	}
}

func TestResolveRejectsPathSeparators(t *testing.T) {
	tests := []struct {
		username   string
		deviceSlug string
	}{
		{username: "bad/user", deviceSlug: "device"},
		{username: "gateway", deviceSlug: "bad/device"},
	}
	for _, tt := range tests {
		if _, err := Resolve("", tt.username, tt.deviceSlug); err == nil {
			t.Fatalf("Resolve(%q, %q) error = nil, want slash validation error", tt.username, tt.deviceSlug)
		}
	}
}

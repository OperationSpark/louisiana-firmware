package topic

import (
	"errors"
	"strings"
)

const prefix = "louisianair/dev"

func Resolve(overrideTopic, username, deviceSlug string) (string, error) {
	if overrideTopic != "" {
		return overrideTopic, nil
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return "", errors.New("mqtt username is required to build LouisianAir telemetry topic")
	}
	if strings.Contains(username, "/") {
		return "", errors.New("mqtt username must not contain '/'")
	}

	// Backend stores devices.source_device_key as the lowercased MAC, and
	// ingestion resolves the device by the topic slug verbatim. Normalize here
	// so MQTT_DEVICE matches regardless of how the MAC was typed.
	deviceSlug = strings.ToLower(strings.TrimSpace(deviceSlug))
	if deviceSlug == "" {
		return "", errors.New("mqtt device slug is required to build LouisianAir telemetry topic; set MQTT_DEVICE to the backend devices.source_device_key")
	}
	if strings.Contains(deviceSlug, "/") {
		return "", errors.New("mqtt device slug must not contain '/'")
	}

	return prefix + "/" + username + "/" + deviceSlug + "/telemetry", nil
}

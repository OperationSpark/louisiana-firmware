# MQTT Sensor

TinyGo firmware for Raspberry Pi Pico W that reads sensor data and publishes LouisianAir backend-compatible telemetry to MQTT.

## Features

- Reads analog sensor values from ADC0 (16-bit resolution)
- Reads DHT11 temperature and humidity on `GP0`
- Displays readings on 16x2 LCD display (HD44780 over I2C)
- Publishes canonical telemetry JSON over MQTT

## Hardware

- Raspberry Pi Pico W
- Analog sensor connected to `ADC0` / `GP26`
- DHT11 data connected to `GP0`
- 16x2 LCD display (HD44780 with I2C adapter)
  - `SDA`: `GP4`
  - `SCL`: `GP5`
- Debug LED on `GP21`

## Configuration

Configure WiFi and MQTT via environment variables injected by TinyGo linker flags:

- `WIFI_SSID` - WiFi network name
- `WIFI_PASS` - WiFi password
- `MQTT_ADDR` - MQTT broker address as `host:port`, for example `os-mqtt-gateway.fly.dev:1883`
- `MQTT_USER` - MQTT username; required unless `MQTT_TOPIC` is set
- `MQTT_PASS` - MQTT password
- `MQTT_DEVICE` - backend device slug; required unless `MQTT_TOPIC` is set; must match `devices.source_device_key`
- `MQTT_TOPIC` - optional full topic override; normally leave blank

Default publish topic:

```text
louisianair/dev/<MQTT_USER>/<MQTT_DEVICE>/telemetry
```

## Flashing

```bash
make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MQTT_ADDR" \
  MQTT_USER="$MQTT_USER" MQTT_PASS="$MQTT_PASS" \
  MQTT_DEVICE="$MQTT_DEVICE" MQTT_TOPIC="$MQTT_TOPIC"
```

For the shared Fly broker, use `MQTT_ADDR="os-mqtt-gateway.fly.dev:1883"` without a `tcp://` prefix.

## Verifying MQTT output

```bash
mosquitto_sub -h os-mqtt-gateway.fly.dev -p 1883 \
  -u "$MQTT_USER" -P "$MQTT_PASS" \
  -t "louisianair/dev/$MQTT_USER/$MQTT_DEVICE/telemetry" -v
```

Expected payload:

```json
{
  "source_reading_key": "picopuff:...",
  "recorded_at": "2026-06-08T15:00:00Z",
  "measurements": {
    "temperature": 19.4,
    "humidity": 45
  }
}
```

## Sensor Readings

Published readings use the backend community-device ingest contract:

- `source_reading_key` - unique reading key generated from device slug, timestamp, and uptime
- `recorded_at` - RFC3339 UTC timestamp; publish is skipped until NTP time is available
- `measurements.temperature` - DHT11 temperature converted to °C
- `measurements.humidity` - DHT11 relative humidity percentage

Readings are buffered before publishing to handle temporary network latency.

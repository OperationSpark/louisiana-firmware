# aethernet-firmware

TinyGo firmware for the Raspberry Pi Pico W. Reads temperature, humidity, and sensor data and publishes backend-compatible telemetry to MQTT over WiFi.

## Hardware connections

| Component | Pin |
|-----------|-----|
| Analog sensor | `ADC0` / `GP26` |
| DHT11 data | `GP0` |
| LCD SDA | `GP4` |
| LCD SCL | `GP5` |
| Debug LED | `GP21` |

## Setup

### TODO 1: Copy and fill in your environment

```bash
cp .env.example .env
```

Edit `.env` with your WiFi credentials and MQTT broker settings:

```bash
WIFI_SSID=your_network_name
WIFI_PASS=your_wifi_password
MQTT_ADDR=os-mqtt-gateway.fly.dev:1883   # host:port only, no tcp:// prefix
MQTT_USER=picopuff                       # MQTT account/user segment in the topic
MQTT_PASS=your_mqtt_password
MQTT_DEVICE=your_pico_mac_address        # will get from the Pico in TODO 2; must match devices.source_device_key
MQTT_TOPIC=                              # optional full topic override; normally blank
```

The default publish topic is:

```text
louisianair/dev/<MQTT_USER>/<MQTT_DEVICE>/telemetry
```

The firmware now fails fast if `MQTT_USER` or `MQTT_DEVICE` is missing and no `MQTT_TOPIC`
override is supplied, so it will not silently publish backend-bound readings under a firmware
default like `tinygo-sen0568`.

### TODO 2: Get the Pico's MAC address

Hold the **BOOTSEL** button, plug in the Pico, then release. Flash the MAC probe and watch the serial output:

```bash
make flash/macprobe
```

It prints the Pico W's MAC address over serial. Set `MQTT_DEVICE` in `.env` to the **lowercased** MAC:

```bash
MQTT_DEVICE=28cdc1xxxxxx   # lowercased MAC from macprobe; must match devices.source_device_key
```

### TODO 3: Flash the Pico W

Hold the **BOOTSEL** button, plug in the Pico, then release. Run:

If you are flashing the LCD/ADC firmware, use `flash/mqttsensor` instead with the same variables (including `MQTT_DEVICE`):

```bash
source .env && make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MQTT_ADDR" \
  MQTT_USER="$MQTT_USER" MQTT_PASS="$MQTT_PASS" \
  MQTT_DEVICE="$MQTT_DEVICE" MQTT_TOPIC="$MQTT_TOPIC"
```

### TODO 4: Verify data is flowing

Subscribe to the exact backend topic:

```bash
mosquitto_sub -h os-mqtt-gateway.fly.dev -p 1883 \
  -u "$MQTT_USER" -P "$MQTT_PASS" \
  -t "louisianair/dev/$MQTT_USER/$MQTT_DEVICE/telemetry" -v
```

Expected payload shape:

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

## Published data fields

| Field | Description |
|-------|-------------|
| `source_reading_key` | Unique key generated from device slug, timestamp, and uptime |
| `recorded_at` | RFC3339 UTC timestamp; publish is skipped until NTP succeeds |
| `measurements.temperature` | DHT11 temperature converted to °C |
| `measurements.humidity` | Relative humidity (%) |

The backend must have a matching `devices.source_device_key` row for `MQTT_DEVICE` under the `community_device` source.

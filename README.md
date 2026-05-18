# aethernet-firmware

TinyGo firmware for the Raspberry Pi Pico W. Reads temperature, humidity, and analog sensor data, displays it on an LCD, and publishes it to an MQTT broker over WiFi.

## Hardware connections

| Component | Pin |
|-----------|-----|
| Analog sensor | `ADC0` / `GP26` |
| DHT11 data | `GP0` |
| LCD SDA | `GP4` |
| LCD SCL | `GP5` |
| Debug LED | `GP21` |

## Setup

### 1. Copy and fill in your environment

```bash
cp .env.example .env
```

Edit `.env` with your WiFi credentials and MQTT broker address:

```bash
WIFI_SSID=your_network_name   # 2.4 GHz only — the Pico W doesn't support 5 GHz
WIFI_PASS=your_wifi_password
MQTT_ADDR=192.168.1.x:1883    # your broker's IP (see step 2 if you need a local one)
MQTT_USER=                    # leave blank for anonymous/local Mosquitto
MQTT_PASS=
```

### 2. Start a local MQTT broker (if you don't have one)

Open a dedicated terminal and run:

```bash
printf 'listener 1883 0.0.0.0\nallow_anonymous true\nlog_type all\n' > /tmp/aethernet-mosquitto.conf
mosquitto -c /tmp/aethernet-mosquitto.conf -v
```

> If macOS prompts for firewall permission, allow incoming connections.
> `allow_anonymous true` is fine for local lab use — don't use it on public networks.

Then get your Mac's WiFi IP to use as `MQTT_ADDR`:

```bash
ipconfig getifaddr en0   # try en1 if this prints nothing
```

### 3. Flash the Pico W

Hold the **BOOTSEL** button, plug in the Pico, then release. Run:

```bash
source .env && make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MQTT_ADDR" \
  MQTT_USER="$MQTT_USER" MQTT_PASS="$MQTT_PASS"
```

### 4. Verify data is flowing

In a new terminal, subscribe to the sensor topic:

```bash
mosquitto_sub -h "$MAC_IP" -p 1883 -t 'tests/#' -v
```

You should see readings arrive every second:

```text
tests/43 {"Voltage":1.6,"RawUInt16":32210,"Temperature":67.0,"Humidity":45,...}
```

## Published data fields

| Field | Description |
|-------|-------------|
| `Voltage` | ADC voltage (0–3.3 V) |
| `RawUInt16` | Raw 16-bit ADC value (0–65535) |
| `Temperature` | DHT11 temperature in °F |
| `Humidity` | Relative humidity (%) |
| `SinceBootNS` | Nanoseconds since the Pico booted |
| `Timestamp` | Wall-clock time (populated after NTP sync) |

Readings are published to the topic `tests/43`.

## Viewing data in MQTTX

[MQTTX](https://mqttx.app/) is a free GUI MQTT client. Create a new connection with:

| Setting | Value |
|---------|-------|
| Host | your `MAC_IP` |
| Port | `1883` |
| Protocol | `mqtt://` |
| SSL/TLS | off |
| Username / Password | leave blank |

After connecting, subscribe to `tests/#` to see live readings.

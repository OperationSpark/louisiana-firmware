# MQTT Sensor

TinyGo firmware for Raspberry Pi Pico W that reads sensor data and publishes it to an MQTT broker.

## Features

- Reads analog sensor values from ADC0 (16-bit resolution)
- Reads DHT11 temperature and humidity on `GP0`
- Displays readings on 16x2 LCD display (HD44780 over I2C)
- Publishes sensor data to MQTT broker over WiFi

## Hardware

- Raspberry Pi Pico W
- Analog sensor connected to `ADC0` / `GP26`
- DHT11 data connected to `GP0`
- 16x2 LCD display (HD44780 with I2C adapter)
  - `SDA`: `GP4`
  - `SCL`: `GP5`
- Debug LED on `GP21`

## Configuration

Configure WiFi credentials and MQTT broker address via environment variables:

- `WIFI_SSID` - WiFi network name
- `WIFI_PASS` - WiFi password
- `MQTT_ADDR` - MQTT broker address, for example `192.168.87.23:1883`
- `MQTT_USER` - MQTT username, optional
- `MQTT_PASS` - MQTT password, optional

These are injected at build time using TinyGo linker flags.

## Flashing

```bash
make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MQTT_ADDR" \
  MQTT_USER="$MQTT_USER" MQTT_PASS="$MQTT_PASS"
```

The firmware currently publishes sensor readings to:

```text
tests/43
```

## Local MQTT Broker Setup

Use this when the shared/remote broker blocks topics by ACL or when you want to test locally.

### 1. Install Mosquitto

```bash
brew install mosquitto
```

### 2. Start a local broker

```bash
printf 'listener 1883 0.0.0.0\nallow_anonymous true\nlog_type all\n' > /tmp/aethernet-mosquitto.conf
mosquitto -c /tmp/aethernet-mosquitto.conf -v
```

Leave this terminal running. If macOS asks for firewall permission, allow incoming connections.

> Note: `allow_anonymous true` is for local lab testing only. Do not use this on public or untrusted networks.

### 3. Get your Mac WiFi IP

```bash
MAC_IP=$(ipconfig getifaddr en0)
echo "$MAC_IP"
```

If that prints nothing, try:

```bash
MAC_IP=$(ipconfig getifaddr en1)
echo "$MAC_IP"
```

### 4. Verify the broker with Mosquitto CLI

Terminal A - subscribe:

```bash
mosquitto_sub -h "$MAC_IP" -p 1883 -t 'tests/#' -v
```

Terminal B - publish a test message:

```bash
mosquitto_pub -h "$MAC_IP" -p 1883 -t 'tests/43' -m '{"test":"hello"}'
```

Terminal A should show:

```text
tests/43 {"test":"hello"}
```

### 5. Flash Pico W to publish to the local broker

```bash
MAC_IP=$(ipconfig getifaddr en0)

make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MAC_IP:1883" \
  MQTT_USER="" MQTT_PASS=""
```

Watch the subscriber terminal. Expected output:

```text
tests/43 {"Voltage":...,"RawUInt16":...,"Temperature":...,"Humidity":...}
```

## MQTTX Local Connection

Create a new MQTTX connection with:

```text
Name: local aethernet
Host: <your MAC_IP>
Port: 1883
Protocol: mqtt://
SSL/TLS: off
Username: blank
Password: blank
Client ID: mqttx-local-anything
Clean Start: true
Keep Alive: 60
```

After connecting, add a subscription:

```text
tests/#
```

To verify MQTTX receives messages, publish from a terminal:

```bash
mosquitto_pub -h "$MAC_IP" -p 1883 -t 'tests/43' -m '{"test":"mqttx"}'
```

## Sensor Readings

Published readings include:

- `Voltage` - ADC voltage, 0-3.3V
- `RawUInt16` - raw 16-bit ADC value, 0-65535
- `Temperature` - DHT11 temperature
- `Humidity` - DHT11 relative humidity percentage
- `SinceBootNS` - time since boot
- `Timestamp` - wall-clock time if NTP sync succeeded

Readings are buffered before publishing to handle temporary network latency.

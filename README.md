# aethernet-firmware

TinyGo firmware for Raspberry Pi Pico W that reads analog air quality sensor data and publishes it to an MQTT broker.

## Features

- Reads analog sensor values from ADC0 (16-bit resolution)
- Displays readings on 16x2 LCD display (HD44780 over I2C)
- Publishes sensor data to MQTT broker over WiFi
- Shows voltage, percentage, and raw ADC values

## Hardware

- Raspberry Pi Pico W
- Analog sensor connected to `ADC0`
- 16x2 LCD display (HD44780 with I2C adapter)
  - `SDA`: `GP4`
  - `SCL`: `GP5`
- Debug LED on `GP21`

## Configuration

Configure WiFi credentials and MQTT broker address via environment variables:

- `WIFI_SSID` - WiFi network name
- `WIFI_PASS` - WiFi password
- `MQTT_ADDR` - MQTT broker address (e.g., "10.0.0.9:1883")

These are [injected at build time using linker flags](https://tinygo.org/docs/guides/tips-n-tricks/).

## Flashing

```
make flash/mqttsensor
```

which is equivalent to:

```bash
tinygo flash -target=pico-w -stack-size=16kb \
  -monitor \
  -ldflags="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
  -X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
  -X 'main.serverAddrStr=${MQTT_ADDR}'" \
  ./mqttsensor/...
```

## Sensor Readings

The sensor reads at 500ms intervals (250ms high, 250ms low) and publishes:

- Voltage (0-3.3V)
- Raw 16-bit value (0-65535)
- Time since boot

Readings are buffered in a channel (capacity: 10) to handle network latency.

# Air Quality Sensor Build Guide

A complete guide for building and deploying a battery-powered air quality sensor with WiFi connectivity and MQTT publishing.

## Overview

This project creates an IoT air quality monitoring station using a Raspberry Pi Pico W (or Pico 2 W). The sensor reads analog air quality data, temperature, and humidity, displays readings on an LCD, and publishes data to an MQTT broker over WiFi.

### Features

- **Analog air quality sensing** via MQ-style sensors (MQ-135, MQ-7, etc.)
- **Temperature and humidity** from DHT11 sensor
- **16x2 LCD display** showing real-time readings
- **MQTT publishing** over WiFi for remote monitoring
- **NTP time synchronization** for accurate timestamps
- **Burst sampling** with averaging for stable readings

### Architecture

```
┌─────────────────┐     ┌──────────────────┐
│  MQ Sensor      │────▶│                  │
│ (5v in,         │     │                  │
│ 3.3V analog out)│     │                  │     ┌─────────────────┐
└─────────────────┘     │   Pico W /       │     │   MQTT Broker   │
                        │   Pico 2 W       │────▶│   (WiFi)        │
┌─────────────────┐     │                  │     └─────────────────┘
│   DHT11         │────▶│                  │
│   (Temp/Humid)  │     │                  │
└─────────────────┘     └────────┬─────────┘
                                 │
                        ┌────────▼─────────┐
                        │   HD44780 LCD    │
                        │   (I2C, 16x2)    │
                        └──────────────────┘
```

---

## Phase 1: Parts & Materials

### Bill of Materials

| Component                       | Description                             | Notes                         |
| ------------------------------- | --------------------------------------- | ----------------------------- |
| Raspberry Pi Pico W or Pico 2 W | Microcontroller with WiFi               | RP2040 or RP2350 based        |
| MQ-style analog sensor          | Air quality sensor (MQ-135, MQ-7, etc.) | 5V module with analog output  |
| DHT11                           | Temperature/humidity sensor             | Digital, single-wire protocol |
| HD44780 16x2 LCD                | Character display                       | With I2C backpack (PCF8574)   |
| 20kΩ resistor                   | Voltage divider (top)                   | 1/4W, 5% tolerance            |
| 27kΩ resistor                   | Voltage divider (bottom)                | 1/4W, 5% tolerance            |
| 4.7kΩ resistor                  | DHT11 pull-up                           | Can use 10kΩ as alternative   |
| 0.1µF capacitor                 | ADC noise filtering                     | Optional but recommended      |
| Breadboard                      | Prototyping                             | Full or half size             |
| Jumper wires                    | Connections                             | Male-to-male, male-to-female  |
| Battery pack                    | Power supply                            | See Phase 7 for options       |

### About MQ-Style Sensors

MQ sensors are metal-oxide semiconductor gas sensors commonly used for air quality monitoring:

- **MQ-135**: General air quality (NH3, NOx, alcohol, benzene, smoke, CO2)
- **MQ-7**: Carbon monoxide (CO)
- **MQ-2**: Combustible gases and smoke
- **MQ-4**: Methane and natural gas

**Important calibration notes:**

1. MQ sensors require a **burn-in period** of 24-48 hours on first use
2. Readings are relative and require calibration against known concentrations
3. Temperature and humidity affect readings (the DHT11 data can help compensate)
4. Response curves are logarithmic - consult the datasheet for your specific sensor

---

## Phase 2: Hardware Assembly

### Wiring Diagram

Refer to `sensor-lcd.fzz` (Fritzing file) in this directory for the complete wiring diagram.

<!-- TODO: Add exported wiring diagram image here -->
<!-- ![Wiring Diagram](./sensor-lcd.png) -->

### Pin Connections

| Pico Pin    | Function     | Connected To                      |
| ----------- | ------------ | --------------------------------- |
| GP26 (ADC0) | Analog input | Voltage divider output            |
| GPIO0       | DHT11 data   | DHT11 data pin                    |
| GP4         | I2C SDA      | LCD SDA                           |
| GP5         | I2C SCL      | LCD SCL                           |
| GP21        | Debug LED    | LED (optional)                    |
| GP22        | Button       | Button (TODO for display cycling) |
| 3V3(OUT)    | 3.3V power   | DHT11 VCC, pull-up resistors      |
| VSYS        | System power | Battery input (1.8V-5.5V)         |
| GND         | Ground       | Common ground                     |

### ADC Voltage Divider Circuit

The MQ sensor outputs 0-5V, but the Pico's ADC only accepts 0-3.3V. A voltage divider scales the signal safely.

```
MQ Sensor AO ──┬── 20kΩ ──┬── GP26 (ADC0)
               │          │
               │          ├── 0.1µF ── GND (optional)
               │          │
               └──────────┴── 27kΩ ── GND
```

**Divider calculation:**

- Ratio: 27kΩ / (20kΩ + 27kΩ) ≈ 0.574
- 5V input → ~2.87V at ADC (safely under 3.3V)
- Maximum safe input: ~5.75V

**Critical:** Verify the resistor orientation:

- 20kΩ is the **top** resistor (AO → ADC node)
- 27kΩ is the **bottom** resistor (ADC node → GND)

### DHT11 Wiring

```
DHT11 Pin 1 (VCC) ── 3.3V
DHT11 Pin 2 (Data) ─┬── GPIO0
                    │
                    └── 4.7kΩ ── 3.3V (pull-up)
DHT11 Pin 4 (GND) ── GND
```

Note: Some DHT11 modules have a built-in pull-up resistor. Check your module before adding an external one. Double-check pinout as some modules have different layouts.

### I2C LCD Connections

```
LCD I2C Backpack    Pico
─────────────────────────
VCC              ── 3V3(OUT)
GND              ── GND
SDA              ── GP4
SCL              ── GP5
```

**Important:** Power the I2C backpack at 3.3V, not 5V. The backpack's I2C lines have pull-ups to VCC, so powering at 5V would put 5V on the Pico's GPIO pins, which are not 5V tolerant and could be damaged.

The PCF8574 chip works fine at 3.3V. LCD contrast may be slightly reduced compared to 5V operation, but is typically still readable. If you need 5V for better contrast, use a bidirectional I2C level shifter between the Pico and the backpack.

The LCD backpack typically has address `0x27` or `0x3F`. The firmware auto-detects both.

### Power Distribution (Battery Operation)

For battery power, connect your power source to the Pico's VSYS pin:

- VSYS accepts 1.8V to 5.5V input
- The onboard regulator provides 3.3V to the system
- See Phase 7 for battery pack options

---

## Phase 3: Software Setup

### Install TinyGo

TinyGo is required to compile and flash the firmware. Install it following the official guide:

```bash
# macOS (Homebrew)
brew tap tinygo-org/tools
brew install tinygo

# Verify installation
tinygo version
```

For other platforms, see: https://tinygo.org/getting-started/install/

### Clone the Repository

```bash
git clone https://github.com/operationspark/aethernet-firmware.git
cd picoplayground
```

### Project Structure

```
mqttsensor/
├── main.go           # Application entry point
├── cyw43439/         # WiFi driver configuration
├── lcd/              # LCD display handler
├── mqtt/             # MQTT client implementation
├── ntp/              # NTP time synchronization
├── weather/          # DHT11 sensor wrapper
├── docs/             # Documentation (you are here)
├── sensor-lcd.fzz    # Fritzing wiring diagram
└── README.md         # Quick reference
```

---

## Phase 4: Configuration

### Environment Variables

The firmware is configured at build time via environment variables:

| Variable    | Required | Description              | Example              |
| ----------- | -------- | ------------------------ | -------------------- |
| `WIFI_SSID` | Yes      | WiFi network name        | `MyNetwork`          |
| `WIFI_PASS` | Yes      | WiFi password            | `secretpass`         |
| `MQTT_ADDR` | Yes      | MQTT broker address:port | `192.168.1.100:1883` |
| `MQTT_USER` | No       | MQTT username            | `sensor1`            |
| `MQTT_PASS` | No       | MQTT password            | `sensorpass`         |

Set these in your shell before building:

```bash
export WIFI_SSID="YourNetworkName"
export WIFI_PASS="YourPassword"
export MQTT_ADDR="192.168.1.100:1883"

# Optional authentication
export MQTT_USER="username"
export MQTT_PASS="password"
```

### MQTT Broker Setup

You'll need an MQTT broker on your network. Options include:

- **Mosquitto** (self-hosted): `sudo apt install mosquitto`
- **Home Assistant** (built-in broker)
- **Cloud services**: HiveMQ, CloudMQTT, AWS IoT

The sensor publishes readings that include:

- Voltage (0-3.3V scaled from ADC)
- Raw 16-bit unsigned int ADC value (`0`-`65535`)
- Temperature (°F)
- Humidity (%)
- Timestamp (if NTP sync succeeded)

### Sensor Calibration Notes

MQ sensors output a voltage that corresponds to gas concentration via a logarithmic response curve. To get meaningful PPM values:

1. **Consult your sensor's datasheet** for the Rs/R0 vs PPM curve
2. **Measure R0** (sensor resistance in clean air) after burn-in
3. **Apply the response curve formula** in your data processing pipeline

This firmware outputs raw voltage/ADC values. Calibration and conversion to PPM should be done in your data collection system (e.g., Home Assistant, Node-RED, or a custom service).

---

## Phase 5: Build & Flash

### For Raspberry Pi Pico W

```bash
# Ensure environment variables are set (see Phase 4)

# Flash and open serial monitor
make flash/mqttsensor
```

### For Raspberry Pi Pico 2 W

```bash
# Flash to Pico 2 W (RP2350-based)
make flash/mqttsensor/pico2w
```

### Manual Flash Command

If you prefer not to use the Makefile:

```bash
tinygo flash -target=pico-w -stack-size=16kb -monitor \
  -ldflags="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
  -X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
  -X 'main.mqttServerAddr=${MQTT_ADDR}' \
  -X 'main.mqttUsername=${MQTT_USER}' \
  -X 'main.mqttPassword=${MQTT_PASS}'" \
  ./mqttsensor/...
```

### Flashing Process

1. Connect the Pico to your computer via USB
2. If the Pico doesn't appear as a mass storage device, hold BOOTSEL while connecting
3. Run the flash command
4. The `-monitor` flag opens a serial console after flashing

---

## Phase 6: Testing & Calibration

### Initial Boot Sequence

After flashing, the LCD should display status messages:

1. `Connecting to WiFi...`
2. `Getting IP via DHCP...`
3. `Syncing time via NTP...`
4. `Time synced HH:MM:SS` (or `NTP sync failed`)
5. Sensor readings: `ADC: X.Xv, XXXXX` / `Temp:XX.XF H:XX%`

### Verify LCD Output

- Check that the display shows updating values
- The debug LED (GP21) blinks once per sampling cycle
- If the LCD shows nothing, check I2C connections and address

### Verify Serial Output

With the `-monitor` flag, you'll see logs on the serial console:

```
level=INFO msg="dhcp complete" ip=192.168.1.xxx
level=INFO msg="ntp:success" time=2024-01-15T10:30:00Z
level=INFO msg="sample interval" v=1
```

### Verify MQTT Messages

Use an MQTT client to subscribe to your broker and verify messages:

```bash
# Using mosquitto_sub
mosquitto_sub -h your-broker-ip -t "#" -v
```

You should see JSON payloads with sensor data.

### Sensor Calibration Guidance

1. **Burn-in**: Let the MQ sensor run continuously for 24-48 hours before trusting readings
2. **Baseline**: Record readings in clean air as your baseline
3. **Response test**: Introduce a known stimulus (e.g., lighter near MQ-135) and observe the change
4. **Document**: Note your baseline values and environmental conditions for future reference

---

## Phase 7: Deployment

### Battery Power Options

#### Option A: 3x AA Batteries (Simplest)

- **Voltage**: 4.5V (3 × 1.5V)
- **Capacity**: ~2000-2500mAh (alkaline)
- **Pros**: Widely available, no charging circuit needed, safe
- **Cons**: Not rechargeable, voltage drops as batteries drain
- **Connection**: Battery holder → VSYS and GND

```
3x AA Holder (+) ── VSYS
3x AA Holder (-) ── GND
```

#### Option B: LiPo + Boost Converter (Rechargeable)

- **Voltage**: 3.7V LiPo → 5V boosted
- **Capacity**: 1000-3000mAh typical
- **Pros**: Rechargeable, compact, stable voltage output
- **Cons**: Requires boost converter, charging circuit, more complex
- **Components needed**: LiPo battery, boost converter (to 5V), charging module (TP4056)

```
LiPo ── Boost Converter (5V out) ── VSYS
                                 └── GND
```

**Safety note**: LiPo batteries require proper handling. Use a battery with built-in protection circuit.

### Power Consumption Notes

Estimated current draw:

- Pico W (WiFi active): ~50-100mA
- MQ sensor heater: ~150mA (continuous)
- DHT11: <1mA (during read)
- LCD with backlight: ~20-30mA

**Total**: ~220-280mA typical

With 3x AA batteries (2500mAh), expect approximately 9-11 hours of continuous operation. For longer battery life, consider:

- Turning off the LCD backlight
- Reducing sampling frequency
- Using sleep modes between readings (requires firmware changes)

### Enclosure Suggestions

- Use a ventilated enclosure to allow air flow to the sensors
- Keep the MQ sensor accessible (don't seal it)
- Consider 3D printing a custom case
- A simple project box with drilled ventilation holes works well

### Placement Recommendations

For accurate air quality monitoring:

- Place at breathing height (~1.5m / 5ft)
- Avoid direct sunlight (affects temperature readings)
- Keep away from windows and HVAC vents
- Allow air circulation around the MQ sensor
- Avoid placing near cooking areas if monitoring general air quality

---

## Appendix

### Troubleshooting

| Problem             | Possible Cause              | Solution                                                                   |
| ------------------- | --------------------------- | -------------------------------------------------------------------------- |
| LCD blank           | I2C address mismatch        | Check wiring; firmware tries 0x27 and 0x3F; check contrast pot on backpack |
| LCD blank           | Wrong I2C pins              | Verify SDA→GP4, SCL→GP5                                                    |
| WiFi won't connect  | Wrong credentials           | Double-check WIFI_SSID and WIFI_PASS                                       |
| WiFi won't connect  | 5GHz network                | Pico W only supports 2.4GHz                                                |
| MQTT not publishing | Broker unreachable          | Verify MQTT_ADDR and broker is running                                     |
| ADC reads 0 or max  | Voltage divider wired wrong | Check resistor values and orientation                                      |
| DHT11 read errors   | Missing pull-up             | Add 4.7kΩ pull-up to data line                                             |
| DHT11 read errors   | Polling too fast            | Firmware handles this with caching                                         |
| Erratic readings    | Electrical noise            | Add 0.1µF capacitor on ADC input                                           |
| Sensor unresponsive | Needs burn-in               | Run continuously for 24-48 hours                                           |

### Serial Debugging

Connect via serial monitor at 115200 baud to see debug output:

```bash
# macOS
screen /dev/tty.usbmodem* 115200

# Linux
screen /dev/ttyACM0 115200

# Or use the TinyGo monitor
tinygo monitor
```

### References

- [README.md](../README.md) - Quick start and overview
- [TODO.md](../TODO.md) - Planned features and improvements
- [pico-w-plan.md](../pico-w-plan.md) - ADC sampling strategy details
- [TinyGo Documentation](https://tinygo.org/docs/)
- [Raspberry Pi Pico Datasheet](https://datasheets.raspberrypi.com/pico/pico-datasheet.pdf)
- MQ sensor datasheets (search for your specific model)

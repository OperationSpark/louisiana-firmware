# Aethernet firmware

TinyGo firmware for a two-node environmental sensor system. The current runnable milestone proves USB serial, the XIAO ESP32S3 I²C bus, BME280 measurements, and compensated SGP41 raw measurements. LoRa is intentionally gated on a successful physical sensor run; Wi-Fi/MQTT comes after LoRa RX.

## Current hardware

### Field node

- Seeed Studio XIAO ESP32S3
- Adafruit SGP41 at I²C address `0x59`
- Adafruit BME280 at I²C address `0x76` or `0x77`
- SGP41 and BME280 daisy-chained with STEMMA QT
- USB-C for power, flashing, and serial diagnostics

### I²C wiring

| Signal | XIAO pin | ESP32-S3 GPIO |
|---|---:|---:|
| SDA | D4 | GPIO5 |
| SCL | D5 | GPIO6 |
| Power | 3V3 | — |
| Ground | GND | — |

The Adafruit STEMMA QT boards include I²C pull-ups. Power the sensors from `3V3`, not `5V`, during bring-up.

## Dependencies

- Go version from `go.mod`
- [TinyGo](https://tinygo.org/getting-started/install/) 0.41.1 or newer
- `tinygo.org/x/drivers/bme280` for the BME280
- Local minimal SGP41 driver in `internal/sensors/sgp41.go`

No maintained SGP41 package was found in the current TinyGo drivers module. The local implementation follows Sensirion's official embedded driver commands and timing: CRC-8 polynomial `0x31`, serial-number read, 320 ms self-test, ten one-second conditioning cycles, and compensated raw measurement.

## Install TinyGo

Follow the TinyGo installation instructions, then verify:

```bash
go version
tinygo version
tinygo targets | grep xiao-esp32s3
```

The locally verified toolchain was:

```text
go version go1.26.2 darwin/arm64
tinygo version 0.41.1 darwin/arm64
```

## Build and test

Run host-side protocol tests:

```bash
make test
```

Compile for the XIAO without touching hardware:

```bash
make build-sensor-test
```

Equivalent command:

```bash
mkdir -p .build
tinygo build -target=xiao-esp32s3 -size=short -o .build/sensor-test.elf ./cmd/sensor-test
```

## Flash and monitor

1. Connect the XIAO ESP32S3 over USB-C.
2. Connect both sensors before powering the board.
3. If normal flashing does not find the board, place the XIAO in bootloader mode using its BOOT/RESET controls.
4. Run:

```bash
make sensor-test
```

Equivalent command:

```bash
tinygo flash -target=xiao-esp32s3 -size=short -monitor ./cmd/sensor-test
```

The firmware waits two seconds so the serial monitor can attach.

## Expected output

A BME280 may appear at either supported address:

```text
BOOT field sensor test
[BOOT] TinyGo environmental node
[I2C] configuring SDA=D4/GPIO5 SCL=D5/GPIO6 frequency=100000Hz
I2C device found: 0x59
I2C device found: 0x77
SGP41 detected
BME280 detected
[BME280] ready address=0x77
[SGP41] serial=0123456789AB
[SGP41] self-test
[SGP41] conditioning for 10 seconds
[SGP41] conditioning 1/10
...
[SGP41] conditioning 10/10
[SGP41] ready
[APP] starting environmental loop

Environmental reading #1

temperature_c: 24.61
humidity_pct: 53.20
pressure_hpa: 1012.84
sraw_voc: 22187
sraw_nox: 13422
{"node":"field-01","seq":1,"temp_c":24.61,"rh_pct":53.20,"pressure_hpa":1012.84,"sraw_voc":22187,"sraw_nox":13422,"uptime_ms":12345}
```

`sraw_voc` and `sraw_nox` are raw sensor ticks, not ppm or air-quality index values. Temperature and humidity from the BME280 are converted to Sensirion compensation ticks for every SGP41 command.

A failed runtime reading is logged and retried on the next cycle. BME280 failures do not panic. An SGP41 runtime failure produces valid JSON with `null` raw gas fields.

## Source layout

```text
cmd/sensor-test/main.go          I²C scan and physical sensor bring-up
internal/board/xiao_esp32s3.go   XIAO pins and bus configuration
internal/sensors/bme280.go       BME280 address selection and unit conversion
internal/sensors/sgp41.go        Minimal SGP41 protocol
internal/sensors/sgp41_test.go   CRC, corruption, and compensation tests
internal/sensors/alphasense.go   Alphasense A4 WE/AE → ppb (Algorithm 1)
cmd/afeprobe/main.go             Pico W ADS1115/AFE bring-up probe
so2mqtt/main.go                  Pico W SO₂ AFE → MQTT firmware
```

Older Raspberry Pi Pico W experiments remain in `mqttsensor/`, `sgp30mqtt/`, `i2cscan/`, and `pinprobe/`; they are not used by the XIAO sensor test.

## Pico W + Alphasense AFE (SO₂)

Hardware chain, per the [LouisianAir connection diagram](https://asp2131.github.io/louisianair/connection-diagram.html):

```text
Pico W (Maker Pi Pico, Grove 5: GP8 SDA / GP9 SCL / 3.3 V)
  → SmartCitizen 8-ch ASB (J2)       ADS1115 at 0x48 and 0x49
  → 2×5 ribbon (J5)                  Alphasense 2-way AFE
  → SN1: SO2-A4, serial 134160063    SN2: empty (needs an H2S-A4)
```

### Channel map (measured 2026-11-05)

| ADS1115 | Channel | Signal |
|---|---|---|
| 0x48 | A0 | SN1 WE (SO₂), ~546 mV at zero |
| 0x48 | A1 | SN1 AE (SO₂), ~574 mV at zero |
| 0x48 | A2 / A3 | SN2 WE / AE (empty slot, ~326 / ~310 mV) |
| 0x49 | A0–A3 | Not sensor signals (rails / 2.5 V reference) |

Readings are single-shot, PGA ±4.096 V, 128 SPS. Noise is about ±2 mV per sample; the firmware averages 16 samples per channel.

### Bench findings

- **Ribbon orientation matters.** When the J5 ribbon is reversed, only 0x49 A0/A1 carry signal (±20 mV noise) and 0x48 reads ~3 mV. Correct orientation gives the map above.
- **The AFE needs 4-pin A4 sensors.** The 3-pin `H2S AE` (serial 29640601) is a 3-electrode part. In SN2 it pins WE2/AE2 at 41.6 / 1.0 mV. Do not use it on this AFE.
- **Don't hold BOOTSEL to probe.** It boots the Pico into the `RPI-RP2` drive, so the firmware doesn't run. Plug in normally; `tinygo flash` resets into the bootloader on its own.
- **Give sensors time to settle.** After a reseat or power loss, A4 sensors can take minutes to hours to stabilise.

### Firmware

| Path | Purpose |
|---|---|
| `cmd/afeprobe/main.go` | Scans I²C on GP8/GP9 and prints all 8 ADS1115 channels every 2 s |
| `so2mqtt/main.go` | Averages SO₂ WE/AE and publishes them over WiFi/MQTT every 10 s |
| `internal/sensors/alphasense.go` | Alphasense Algorithm 1: `ppb = ((WE − WEe) − nT·(AE − AEe)) / sensitivity` |

```bash
tinygo flash -target=pico-w ./cmd/afeprobe     # bring-up probe
make flash/so2mqtt                              # with .env values exported
```

`so2mqtt` publishes `so2_we_mv` and `so2_ae_mv` in the canonical telemetry `measurements` map. It adds `so2_ppb` only when the certificate values are passed in at flash time. Set them in `.env` next to the WiFi/MQTT values:

```bash
SO2_WEE_MV=          # WE electronic zero (mV)
SO2_AEE_MV=          # AE electronic zero (mV)
SO2_SENS_MV_PPB=     # WE sensitivity, mV/ppb = cert nA/ppb × PCB gain mV/nA
```

```bash
set -a; . ./.env; set +a; make flash/so2mqtt
```

The values are compiled into the firmware, so reflash after editing `.env`. At boot the serial log shows `so2: certificate loaded` with the parsed numbers. If the values are blank or invalid, it logs a warning and publishes raw mV only.

`nT` is fixed at 1 until a temperature source (AFE PT1000 or BME280) is wired, so expect temperature drift. `mqtt.SensorReading.Measurements` lets devices without a DHT11 supply their own measurement set instead of publishing a fake temperature/humidity.

## Radio plan (not yet enabled)

The Wio-SX1262 must have its antenna attached before any TX firmware is run. The commonly reported XIAO ESP32S3 B2B mapping is shown only as an inspection checklist and has not yet been committed as a working board definition:

| SX1262 signal | Candidate GPIO |
|---|---:|
| SCK | 7 |
| MISO | 8 |
| MOSI | 9 |
| NSS/CS | 41 |
| RESET | 42 |
| BUSY | 40 |
| DIO1 | 39 |
| RF_SW | 38 |
| TCXO | 1.8 V |

Before implementing LoRa, confirm the installed Wio-SX1262 hardware revision, B2B schematic, RF-switch control, and 1.8 V TCXO requirements. Radio defaults will then be centralized and shared by TX/RX.

## Bring-up sequence

```text
A. flash sensor-test
B. verify 0x59 + 0x76/0x77
C. verify BME280 and SGP41 measurements
D. verify the installed Wio-SX1262 revision and schematic
E. flash radio-test-tx to node A
F. flash radio-test-rx to node B
G. verify hello packets
H. flash field-node
I. flash base-node
J. verify environmental telemetry
K. enable Wi-Fi/MQTT
```

Steps D–K are deliberately not implemented until A–C pass on physical hardware.

## Troubleshooting

Bench findings from I²C bring-up — phantom scan results, pull-up testing, and the
SGP41 command set — are recorded in [docs/i2c-bringup.md](docs/i2c-bringup.md).

### No serial output

- Confirm the USB cable carries data, not power only.
- Retry with the board in bootloader mode.
- Keep `-monitor` in the flash command.
- Reset the board after the monitor opens; boot logs are delayed by two seconds.

### Neither I²C device appears

- Confirm SDA is D4/GPIO5 and SCL is D5/GPIO6.
- Confirm common ground and 3.3 V sensor power.
- Reseat both STEMMA QT cables and test one sensor at a time.
- Avoid adding extra pull-ups unless the bus is measured and needs them.

### Only one device appears

- Swap the two STEMMA QT cables or reverse the daisy-chain order.
- The BME280 is valid at `0x76` or `0x77`; no code change is needed.
- The SGP41 must be at `0x59`.

### BME280 detected but initialization fails

A device ACK is not proof that it is a BME280. The driver also validates Bosch chip ID `0x60`. Check the breakout part number and power integrity.

### SGP41 CRC or self-test fails

- Shorten the I²C wiring.
- Disconnect other I²C devices and retry.
- Power-cycle the sensor before rerunning conditioning.
- Do not modify conditioning to exceed ten seconds; Sensirion warns this can damage the sensing material.

## References

- [TinyGo XIAO ESP32S3 target](https://tinygo.org/docs/reference/microcontrollers/xiao-esp32s3/)
- [TinyGo BME280 driver](https://pkg.go.dev/tinygo.org/x/drivers/bme280)
- [Sensirion SGP41 datasheet](https://sensirion.com/media/documents/5FE8673C/61E96F50/Sensirion_Gas_Sensors_Datasheet_SGP41.pdf)
- [Sensirion embedded SGP41 driver](https://github.com/Sensirion/embedded-i2c-sgp41)

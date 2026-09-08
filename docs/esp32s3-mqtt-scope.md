# Scoping MQTT uplink for the XIAO ESP32-S3

Status: option 2 (Arduino C++) is implemented in `esp32s3-mqtt/`. The TinyGo
firmware `cmd/sensor-test` remains the serial-only bring-up tool and still
prints readings and the factory MAC. The other MQTT paths in this repo
(`mqttsensor/`, `sgp30mqtt/`) target the Pico W and are unchanged.

## Why the Pico W code does not port

`mqttsensor/mqtt` is transport-agnostic: it speaks natiu-mqtt over a
`net.Conn` handed to it by the lneto/xnet stack. The stack is fed raw ethernet
frames by `mqttsensor/cyw43439`, which drives the Pico W's CYW43439 radio.

Swapping the radio is the whole job, and on the ESP32-S3 there is no radio
driver to swap in. TinyGo 0.41 ships no WiFi support for any ESP32 part: the
WiFi MAC is undocumented and Espressif only exposes it through a closed-source
blob (`libnet80211.a`, `libpp.a`) linked by ESP-IDF. Nobody has reimplemented
it, so there is no pure-Go path from a XIAO ESP32-S3 to an access point.

## Options, cheapest first

### 1. Move the SGP41 + BME280 rig back to the Pico W

`internal/sensors` only depends on `tinygo.org/x/drivers.I2C`, so both drivers
run unchanged on the Pico W. The work is a new `cmd/` entry point that wires
them to the existing `mqttsensor/cyw43439` + `mqttsensor/mqtt` stack, plus a
Makefile target mirroring `flash/sgp30mqtt`. Roughly a day. Keeps one language
for the workshop and reuses the broker plumbing that already works.

Cost: the XIAO becomes a bench/bring-up board only, and the STEMMA QT wiring
students learned moves to Pico W pins GP4/GP5.

### 2. Flash ESP-IDF or Arduino firmware onto the XIAO for the uplink (chosen)

Espressif's own stack has WiFi and `esp-mqtt` in the box. This is the only way
the XIAO itself reaches the broker today. It means a second toolchain, a second
language, and porting the SGP41 driver logic in `internal/sensors/sgp41.go`
(the CRC handling and conditioning sequence are the non-trivial part) to C.

Meshtastic is a packaged instance of this option, not a shortcut around it: it
is ESP-IDF/Arduino C++ firmware for ESP32 boards with an MQTT uplink built in.
Adopting it means the node speaks Meshtastic's LoRa mesh protocol and publishes
mesh packets to its own topic scheme, so it fits if the goal is a mesh of
sensor nodes and someone maintains a Meshtastic module for the SGP41/BME280
pair. It does not fit if the goal is teaching TinyGo firmware or publishing
plain JSON to `louisianair/dev/...`, since neither survives the switch.

### 3. Bridge the XIAO over serial to a networked host

`cmd/sensor-test` already prints structured lines. A host-side reader that
parses them and publishes to the broker takes an afternoon and needs no
firmware change. Fine for a demo table where a laptop is present; useless for a
deployed field node.

## Recommendation

Option 1 for anything that has to ship this term. Option 3 if the only near
goal is showing the broker receiving XIAO data at a workshop. Option 2 only if
the XIAO must be standalone, and budget the C rewrite of the SGP41 driver.

## Gas index

The firmware samples the SGP41 at 1 Hz and publishes every tenth sample, because
Sensirion's Gas Index Algorithm is a recursive filter tied to a fixed cadence and
its NOx variant is hard-wired to 1 s. It publishes `sensirion_voc_index` and
`sensirion_nox_index` alongside the raw ticks. For what the index does and does
not mean when shown to residents, see `aethernet/docs/sensirion-index-display.md`.

## Groundwork already done

`board.MAC()` reads the factory MAC from eFuse BLK1 and `cmd/sensor-test`
prints it at boot as `[BOOT] mac=... node=...`. That is the value the ESP32-S3
station interface uses, so it is the identifier to key devices by whichever
option lands. Verify it against `esptool.py --port <port> read_mac`.

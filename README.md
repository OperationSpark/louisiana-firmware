# louisianAir-firmware

TinyGo firmware for the Raspberry Pi Pico W. Reads temperature, humidity, and analog sensor data, displays it on an LCD, and publishes it to an MQTT broker over WiFi.

## Hardware connections

| Component | Pin |
|-----------|-----|
| Analog sensor | `ADC0` / `GP26` |
| DHT11 data | `GP0` |
| LCD SDA | `GP4` |
| LCD SCL | `GP5` |
| Debug LED | `GP21` |

## Prerequisites

You'll need four tools installed: **Go**, **TinyGo** (compiles Go for microcontrollers), **Mosquitto** (the MQTT broker + CLI clients), and optionally **MQTTX** (a GUI MQTT client).

### Go

Download page: <https://go.dev/dl/> · Install docs: <https://go.dev/doc/install>

| OS | Command |
|----|---------|
| macOS | `brew install go` |
| Linux | `sudo rm -rf /usr/local/go && wget -qO- https://go.dev/dl/go1.22.0.linux-amd64.tar.gz \| sudo tar -C /usr/local -xz` then add `/usr/local/go/bin` to your `PATH` |
| Windows | Download and run the `.msi` from <https://go.dev/dl/> |

Verify: `go version`

### TinyGo

Install docs: <https://tinygo.org/getting-started/install/>

| OS | Command |
|----|---------|
| macOS | `brew tap tinygo-org/tools && brew install tinygo` |
| Linux (Debian/Ubuntu) | `wget https://github.com/tinygo-org/tinygo/releases/latest/download/tinygo_amd64.deb && sudo dpkg -i tinygo_amd64.deb` |
| Linux (other) | Download a release tarball from <https://github.com/tinygo-org/tinygo/releases> and add its `bin` to your `PATH` |
| Windows | `scoop install tinygo` (needs [Scoop](https://scoop.sh/)) — see <https://tinygo.org/getting-started/install/windows/> |

Verify: `tinygo version`

> TinyGo needs Go installed first.

### Mosquitto (broker + `mosquitto_sub`/`mosquitto_pub`)

Download page: <https://mosquitto.org/download/>

| OS | Command |
|----|---------|
| macOS | `brew install mosquitto` |
| Linux (Debian/Ubuntu) | `sudo apt update && sudo apt install -y mosquitto mosquitto-clients` |
| Windows | `choco install mosquitto` (needs [Chocolatey](https://chocolatey.org/)) or run the installer from <https://mosquitto.org/download/> |

Verify: `mosquitto --help`

### MQTTX (optional GUI client)

Download page: <https://mqttx.app/>

| OS | Command |
|----|---------|
| macOS | `brew install --cask mqttx` |
| Linux | Download the `.AppImage`/`.deb`/`.rpm` from <https://mqttx.app/downloads> |
| Windows | `choco install mqttx` or download the installer from <https://mqttx.app/downloads> |

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
printf 'listener 1883 0.0.0.0\nallow_anonymous true\nlog_type all\n' > /tmp/louisianAir-mosquitto.conf
mosquitto -c /tmp/louisianAir-mosquitto.conf -v
```

> If macOS prompts for firewall permission, allow incoming connections.
> `allow_anonymous true` is fine for local lab use — don't use it on public networks.

Then find your computer's LAN IP and save it as `MAC_IP` (used in step 4 and in MQTTX). Use the broker machine's IP — the one the Pico W will reach over WiFi.

**macOS** (run in the same terminal you'll use for the rest of the steps):

```bash copy
export MAC_IP=$(ipconfig getifaddr en0 || ipconfig getifaddr en1)
echo "$MAC_IP"
```

**Linux:**

```bash copy
export MAC_IP=$(hostname -I | awk '{print $1}')
echo "$MAC_IP"
```

**Windows (PowerShell):**

```powershell
$env:MAC_IP = (Get-NetIPAddress -AddressFamily IPv4 |
  Where-Object { $_.InterfaceAlias -match 'Wi-Fi|Ethernet' -and $_.IPAddress -notlike '169.*' } |
  Select-Object -First 1 -ExpandProperty IPAddress)
echo $env:MAC_IP
```

Then set `MQTT_ADDR=$MAC_IP:1883` in your `.env` (for example `MQTT_ADDR=192.168.1.42:1883`).

> `export`/`$env:` only lasts for the current terminal session. If you open a new terminal, re-run the command above before step 4.

### 3. Flash the Pico W

Hold the **BOOTSEL** button, plug in the Pico, then release. Run:

```bash copy
source .env && make flash/mqttsensor \
  WIFI_SSID="$WIFI_SSID" WIFI_PASS="$WIFI_PASS" \
  MQTT_ADDR="$MQTT_ADDR" \
  MQTT_USER="$MQTT_USER" MQTT_PASS="$MQTT_PASS"
```

### 4. Verify data is flowing

In a new terminal, subscribe to the sensor topic:

```bash copy
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
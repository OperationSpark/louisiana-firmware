# I²C bring-up notes — XIAO ESP32S3, SGP41 + BME280

Findings from the 2026-09-02 bench session on the Grove Base for XIAO (8-connector
version). Each finding is marked with how far it was actually verified, because
several plausible-sounding diagnoses in this session turned out to be wrong.

## Hardware as wired

- Seeed XIAO ESP32S3 on the Grove Base for XIAO, in a printed enclosure
- Adafruit SGP41 → Grove-to-STEMMA-QT cable (Adafruit 4528) → **ext-half top-right** Grove socket
- Adafruit BME280 → second Grove-to-QT cable → **main-half bottom-left** Grove socket
- Observed addresses: SGP41 `0x59`, BME280 **`0x77`** (that board's SDO sits high)

Two of the eight Grove sockets are I²C; both carry the same SDA/SCL pair, so this
is one bus tapped twice, not two buses. The two STEMMA QT ports on each breakout
are wired in parallel — left vs right is a cable-routing choice, nothing more.

## Finding 1 — a scan listing every even address is a driver artifact, not stuck-low SDA

**Verified.** Symptom: 55 devices, every even address `0x0A`–`0x76`, odd addresses
never.

A genuinely stuck-low SDA ACKs *all* 112 addresses, odd included. The even-only
pattern comes from TinyGo 0.41.1: `machine_esp32xx_i2c.go` returns
`errI2CAckExpected` on NACK but never resets the I²C master FSM. The stalled FSM
makes the *next* `Tx` return instantly on stale `TRANS_COMPLETE`, so results
alternate fail / false-hit down the whole address space.

Fix in `cmd/sensor-test/main.go`: call `board.ConfigureI2C()` after every failed
`Tx` to clear the FSM. The same stall poisons the run loop — one NACK followed by
a "successful" read carrying FIFO leftovers — so `recoverBus()` is called on every
sensor read error too.

After the fix the scan reported exactly `0x59` and `0x77`.

## Finding 2 — testing for pull-ups requires releasing the line, then waiting

**Partly verified.** The first attempt sampled `PinInputPullup`, which reads HIGH
whether or not a breakout is attached: the internal ~45 kΩ pull-up holds the line
by itself. Useless test.

The second attempt drove the line low, released it to plain `PinInput`, and read
back immediately — reporting `SDA=NO SCL=NO` on a bus where two devices then
ACKed. A 5–10 kΩ pull-up needs roughly a microsecond to charge the bus
capacitance; the read landed before the line had risen.

`board.PullupsPresent()` now sleeps 1 ms between release and sample. The delay
fix has not yet been confirmed against hardware.

## Finding 3 — the scanner's probe byte disturbs Sensirion parts

**Verified as a real hazard; the mitigation NACKs.** The scan writes one byte
`0x00` to every address. Sensirion commands are two bytes, so the part can hold
half a command and reject what follows.

`SGP41.SoftReset()` sends the general-call reset (write `0x06` to address `0x00`).
On this bench that write NACKs — the general call is not being answered. The
serial-number read succeeded anyway, because the failed soft reset triggers
`recoverBus()`, which reconfigures the peripheral. Worth revisiting: a NACKing
general call means the reset is not doing what it claims.

## Finding 4 — which SGP41 commands worked

| Command | Bytes written | Result |
|---|---:|---|
| `0x3682` get serial | 2 | OK — `serial=000006A9F149` |
| `0x280E` self-test | 2 | OK — passed |
| `0x2612` conditioning, live compensation | 8 | **NACK** |

Sensirion specifies `execute_conditioning` with the *fixed* default parameters
(0x8000 RH ticks, 0x6666 T ticks); the failing call was the only one sending live
BME280-derived values. `ExecuteConditioning()` now takes no arguments and sends
the defaults, and a refused conditioning warns and proceeds to measurement rather
than halting the bench.

**Unverified** — the defaults hypothesis has not yet been run on hardware.

If `0x2612` and `0x2619` both still NACK while `0x3682` and `0x280E` pass, the
part answering at `0x59` is an SGP40, not an SGP41: those two commands are
SGP41-only, and the SGP40 measures with `0x260F`.

## Symptom → cause

| Symptom | Cause | Action |
|---|---|---|
| Every even address found | Stalled FSM after NACK (TinyGo 0.41.1) | Reconfigure the bus after each failed `Tx` |
| `pullups ... NO` but devices ACK | Sampled before the line rose | Wait ≥1 ms after releasing to input |
| Both lines HIGH with internal pull-ups | Test tells nothing | Use `PinInput`, never `PinInputPullup` |
| Serial/self-test pass, conditioning NACKs | Non-default conditioning parameters, or an SGP40 | Send fixed defaults; if still NACK, check the part |
| `failed to find port` on flash | No USB enumeration at all | Data cable, then BOOT+RESET bootloader |

## Bench notes

- The SGP41 heater sits ~30 mm from the BME280 in the printed enclosure. Enclosed
  temperature and humidity will read high; keep a vent between them or calibrate
  the offset after a soak test, not against open-air numbers.
- Raw ticks drift for roughly the first minute without conditioning. They are
  ticks, not ppm.

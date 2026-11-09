// so2mqtt: Pico W -> Grove 5 (GP8/GP9) -> SmartCitizen ASB (ADS1115 0x48) -> Alphasense AFE with SO2-A4 in SN1.
// Publishes WE/AE millivolts every interval, plus SO2 ppb (Alphasense Algorithm 1) once the cert values below are filled in.
package main

import (
	"log/slog"
	"machine"
	"net/netip"
	"strconv"
	"time"

	"github.com/operationspark/aethernet-firmware/internal/sensors"
	"github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439"
	"github.com/operationspark/aethernet-firmware/mqttsensor/mqtt"
	"github.com/operationspark/aethernet-firmware/mqttsensor/ntp"
)

// Linker-injected via Makefile.
var (
	mqttServerAddr string
	mqttDeviceSlug string
	mqttTopic      string
	mqttUsername   string
	mqttPassword   string
)

// Certificate values for the SO2-A4, linker-injected from .env (SO2_WEE_MV, SO2_AEE_MV, SO2_SENS_MV_PPB).
// -X only sets strings, so they're parsed at boot. Unset/invalid sensitivity = raw mV only, no ppb.
var (
	so2WEe  string
	so2AEe  string
	so2Sens string
)

var so2Cal = sensors.AlphasenseCal{
	Serial: "134160063",
	NT:     1, // ponytail: nT fixed at 1 (no temperature source yet).
}

// loadCal parses the injected certificate strings into so2Cal. Any parse failure leaves it uncalibrated.
func loadCal(logger *slog.Logger) {
	if so2Sens == "" {
		return
	}
	we, err1 := strconv.ParseFloat(so2WEe, 64)
	ae, err2 := strconv.ParseFloat(so2AEe, 64)
	sens, err3 := strconv.ParseFloat(so2Sens, 64)
	if err1 != nil || err2 != nil || err3 != nil || sens <= 0 {
		logger.Error("so2: invalid certificate values; staying uncalibrated",
			slog.String("wee", so2WEe), slog.String("aee", so2AEe), slog.String("sens", so2Sens))
		return
	}
	so2Cal.WEe, so2Cal.AEe, so2Cal.SensMVPerP = we, ae, sens
	logger.Info("so2: certificate loaded", slog.String("serial", so2Cal.Serial),
		slog.Float64("wee_mv", we), slog.Float64("aee_mv", ae), slog.Float64("sens_mv_ppb", sens))
}

const (
	adsAddr   = 0x48
	so2WECh   = 0 // 0x48 A0 = WE1 (SN1), per 2026-11-05 probe
	so2AECh   = 1 // 0x48 A1 = AE1 (SN1)
	avgN      = 16
	interval  = 10 * time.Second
	stackIdle = 5 * time.Millisecond
)

var i2c = machine.I2C0

// readADSmV does one single-shot conversion of AINch vs GND at PGA ±4.096 V, 128 SPS.
func readADSmV(ch uint8) (float64, error) {
	if err := i2c.Tx(adsAddr, []byte{0x01, 0xC3 | (ch << 4), 0x83}, nil); err != nil {
		return 0, err
	}
	time.Sleep(10 * time.Millisecond) // 128 SPS = 7.8 ms
	var b [2]byte
	if err := i2c.Tx(adsAddr, []byte{0x00}, b[:]); err != nil {
		return 0, err
	}
	return float64(int16(uint16(b[0])<<8|uint16(b[1]))) * 4096 / 32768, nil
}

// averagedmV reads WE and AE interleaved avgN times and returns the means.
func averagedmV() (we, ae float64, err error) {
	for i := 0; i < avgN; i++ {
		w, err := readADSmV(so2WECh)
		if err != nil {
			return 0, 0, err
		}
		a, err := readADSmV(so2AECh)
		if err != nil {
			return 0, 0, err
		}
		we += w
		ae += a
	}
	return we / avgN, ae / avgN, nil
}

func main() {
	time.Sleep(2 * time.Second)
	start := time.Now()
	logger := slog.New(slog.NewTextHandler(machine.Serial, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := i2c.Configure(machine.I2CConfig{SDA: machine.GP8, SCL: machine.GP9, Frequency: 100_000}); err != nil {
		fatal(logger, "configure I2C", err)
	}
	loadCal(logger)
	if so2Cal.SensMVPerP == 0 {
		logger.Warn("so2: no certificate sensitivity set; publishing raw mV only", slog.String("serial", so2Cal.Serial))
	}

	mqttC := &mqtt.Client{
		ID:                "tinygo-so2",
		DeviceSlug:        mqttDeviceSlug,
		Topic:             mqttTopic,
		Logger:            logger,
		Timeout:           5 * time.Second,
		TCPBufSize:        2030,
		Username:          mqttUsername,
		Password:          mqttPassword,
		HeartbeatInterval: 45 * time.Second,
	}

	stack, err := cyw43439.NewConfiguredPicoWithStack(cyw43439.SSID(), cyw43439.Password(), cyw43439.DefaultWifiConfig(),
		cyw43439.StackConfig{Hostname: mqttC.ID, MaxTCPPorts: 1, Logger: logger})
	if err != nil {
		fatal(logger, "wifi stack setup", err)
	}
	go func() {
		for {
			if send, recv, _ := stack.RecvAndSend(); send == 0 && recv == 0 {
				time.Sleep(stackIdle)
			}
		}
	}()

	dhcp, err := stack.SetupWithDHCP(cyw43439.DHCPConfig{RequestedAddr: netip.AddrFrom4([4]byte{192, 168, 1, 100}), Hostname: mqttC.ID})
	if err != nil {
		fatal(logger, "DHCP setup", err)
	}
	logger.Info("dhcp complete", slog.String("ip", dhcp.AssignedAddr.String()))

	// Canonical telemetry requires a wall-clock timestamp; retry NTP until it lands.
	for ntp.SyncTime(stack.LnetoStack(), logger) != nil {
		logger.Error("ntp sync failed; retrying")
		time.Sleep(5 * time.Second)
	}
	mqttC.TimeSyncedAt = time.Now()

	readings := make(chan mqtt.SensorReading, 4)
	go func() {
		if err := mqttC.ConnectAndPublish(stack, mqttServerAddr, readings, nil); err != nil {
			fatal(logger, "connect to MQTT broker", err)
		}
	}()

	for next := time.Now(); ; next = next.Add(interval) {
		time.Sleep(time.Until(next))
		we, ae, err := averagedmV()
		if err != nil {
			logger.Error("so2: ADS1115 read failed; skipping publish", slog.Any("reason", err))
			continue
		}
		m := map[string]float64{"so2_we_mv": we, "so2_ae_mv": ae}
		if ppb, ok := so2Cal.PPB(we, ae); ok {
			m["so2_ppb"] = ppb
			logger.Info("so2 read", slog.Float64("we_mv", we), slog.Float64("ae_mv", ae), slog.Float64("ppb", ppb))
		} else {
			logger.Info("so2 read (uncalibrated)", slog.Float64("we_mv", we), slog.Float64("ae_mv", ae))
		}

		select {
		case readings <- mqtt.SensorReading{Measurements: m, SinceBootNS: time.Since(start), Timestamp: time.Now()}:
		default:
			logger.Warn("mqtt reading channel full; dropping so2 reading")
		}
	}
}

func fatal(logger *slog.Logger, msg string, err error) {
	for {
		logger.Error(msg, slog.Any("reason", err))
		time.Sleep(10 * time.Second)
	}
}

package mqtt

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439"
	"github.com/operationspark/aethernet-firmware/mqttsensor/lcd"
	"github.com/operationspark/aethernet-firmware/mqttsensor/mqtt/topic"
	"github.com/soypat/lneto/tcp"
	mqtt "github.com/soypat/natiu-mqtt"
)

var (
	pubFlags, _ = mqtt.NewPublishFlags(mqtt.QoS0, false, false)
)

type SensorReading struct {
	Voltage     float32
	PPM         float32
	Ratio       float32       // SEN0568 relative H2S index R0/Rs; ~1 in clean air, rises with H2S. Zero if unused.
	RawUInt16   uint16        // Raw ADC value
	Temperature float32       // Temperature from DHT11
	Humidity    float32       // Relative humidity percentage from DHT11
	ECO2        uint32        // CO2 equivalent (ppm) from SGP30. Zero if sensor not present.
	TVOC        uint32        // Total VOC (ppb) from SGP30. Zero if sensor not present.
	SinceBootNS time.Duration // Nanoseconds since boot.
	Timestamp   time.Time     // Wall-clock time. Zero if NTP sync failed.
}

type Client struct {
	ID                string
	DeviceSlug        string // Stable DB device slug used in the LouisianAir telemetry topic. Required unless Topic is set.
	Topic             string // Optional full MQTT topic override. Defaults to louisianair/dev/<Username>/<DeviceSlug>/telemetry.
	Timeout           time.Duration
	TCPBufSize        int
	Logger            *slog.Logger
	HeartbeatInterval time.Duration
	TimeSyncedAt      time.Time // When NTP sync occurred. Zero if never synced.
	Username          string    // MQTT broker username (optional)
	Password          string    // MQTT broker password (optional, requires Username)
}

// ConnectAndPublish connects to the MQTT broker and publishes sensor readings.
// The stack is provided from main.go where WiFi/DHCP/NTP are set up.
func (c *Client) ConnectAndPublish(
	stack *cyw43439.Stack,
	addr string,
	readings <-chan SensorReading,
	lcdMessages chan<- lcd.Message,
) error {
	const pollTime = 5 * time.Millisecond

	topic, err := c.publishTopic()
	if err != nil {
		return err
	}
	pubVar := mqtt.VariablesPublish{
		TopicName:        []byte(topic),
		PacketIdentifier: 0xc0fe,
	}

	c.Logger.Info("MQTT address: " + addr)
	c.Logger.Info("MQTT topic: " + topic)

	// Parse hostname and port from addr (e.g., "hostname:8883")
	mqttHost, portStr, err := splitHostPort(addr)
	if err != nil {
		return errors.New("parsing host:port from " + addr + ": " + err.Error())
	}

	lnetoStack := stack.LnetoStack()
	rstack := lnetoStack.StackRetrying(pollTime)

	// Try to parse as IP first, otherwise DNS lookup
	var mqttAddr netip.Addr
	if parsedAddr, err := netip.ParseAddr(mqttHost); err == nil {
		mqttAddr = parsedAddr
	} else {
		// DNS lookup for MQTT server
		c.Logger.Info("dns:resolving " + mqttHost)
		addrs, err := rstack.DoLookupIP(mqttHost, 5*time.Second, 3)
		if err != nil {
			return errors.New("dns lookup for " + mqttHost + ": " + err.Error())
		}
		if len(addrs) == 0 {
			return errors.New("dns lookup for " + mqttHost + ": no addresses returned")
		}
		mqttAddr = addrs[0]
	}

	c.Logger.Info("resolved IP: " + mqttAddr.String())
	port := parsePort(portStr)

	cfg := mqtt.ClientConfig{
		Decoder: mqtt.DecoderNoAlloc{UserBuffer: make([]byte, 4096)},
		OnPub: func(pubHead mqtt.Header, varPub mqtt.VariablesPublish, r io.Reader) error {
			c.Logger.Info("received message", slog.String("topic", string(varPub.TopicName)))
			return nil
		},
	}
	var varconn mqtt.VariablesConnect
	varconn.SetDefaultMQTT([]byte(c.ID))

	// Set authentication credentials if provided
	if c.Username != "" {
		varconn.Username = []byte(c.Username)
		if c.Password != "" {
			varconn.Password = []byte(c.Password)
		}
	}

	mqttClient := mqtt.NewClient(cfg)

	// Configure TCP connection
	var conn tcp.Conn
	err = conn.Configure(tcp.ConnConfig{
		RxBuf:             make([]byte, c.TCPBufSize),
		TxBuf:             make([]byte, c.TCPBufSize),
		TxPacketQueueSize: 3,
	})
	if err != nil {
		return errors.New("tcp configure:" + err.Error())
	}

	closeConn := func(reason string) {
		slog.Error("tcpconn:closing", slog.String("reason", reason))
		conn.Close()
		// Wait for connection to close
		for i := 0; i < 50 && !conn.State().IsClosed(); i++ {
			time.Sleep(100 * time.Millisecond)
		}
		conn.Abort()
	}

	serverAddr := netip.AddrPortFrom(mqttAddr, port)

	// Connection loop for TCP+MQTT.
	for {
		// Use stack's PRNG for random port
		localPort := uint16(lnetoStack.Prand32()>>17) + 1024
		c.Logger.Info("socket:dialing", slog.Uint64("localPort", uint64(localPort)))
		lcd.Send(lcdMessages, "addr", addr)

		// Dial TCP using the retrying stack (handles handshake with retries)
		lcd.Send(lcdMessages, "Connecting...", "TCP handshake")
		err = rstack.DoDialTCP(&conn, localPort, serverAddr, 10*time.Second, 3)
		if err != nil {
			c.Logger.Error("socket:dial-failed", slog.String("err", err.Error()))
			closeConn("dial failed: " + err.Error())
			time.Sleep(2 * time.Second)
			continue
		}

		c.Logger.Info("tcp:connected", slog.String("state", conn.State().String()))

		// We start MQTT connect with a deadline on the socket.
		c.Logger.Info("mqtt:start-connecting")
		lcd.Send(lcdMessages, "MQTT Connect", "Authenticating")
		conn.SetDeadline(time.Now().Add(c.Timeout))
		err = mqttClient.StartConnect(&conn, &varconn)
		if err != nil {
			c.Logger.Error("mqtt:start-connect-failed", slog.String("reason", err.Error()))
			lcd.Send(lcdMessages, "Connect Failed", err.Error()[:min(len(err.Error()), 16)])
			closeConn("connect failed")
			continue
		}
		retries := 50
		for retries > 0 && !mqttClient.IsConnected() {
			time.Sleep(100 * time.Millisecond)
			err = mqttClient.HandleNext()
			if err != nil {
				c.Logger.Error("mqtt:handle-next-failed", slog.String("err", err.Error()))
			}
			retries--
		}
		if !mqttClient.IsConnected() {
			c.Logger.Error("mqtt:connect-failed", slog.Any("reason", mqttClient.Err()))
			lcd.Send(lcdMessages, "Connect Failed", "Timed out")
			closeConn("connect timed out")
			continue
		}

		c.Logger.Info("mqtt:connected", slog.String("topic", topic))
		lcd.Send(lcdMessages, "MQTT Connected", "Publishing...")

		heartbeat := time.NewTicker(c.HeartbeatInterval)
		defer heartbeat.Stop()
		for mqttClient.IsConnected() {
			select {
			case reading := <-readings:
				payload, err := canonicalPayload(c.deviceSlug(), reading)
				if err != nil {
					c.Logger.Error("mqtt:canonical-payload-failed", slog.Any("reason", err))
					continue
				}
				conn.SetDeadline(time.Now().Add(c.Timeout))
				pubVar.PacketIdentifier = uint16(lnetoStack.Prand32())
				err = mqttClient.PublishPayload(pubFlags, pubVar, payload)
				if err != nil {
					c.Logger.Error("mqtt:publish-failed", slog.Any("reason", err))
					continue
				}
				c.Logger.Info("published message",
					slog.String("topic", topic),
					slog.Uint64("packetID", uint64(pubVar.PacketIdentifier)),
				)
				err = mqttClient.HandleNext()
				if err != nil {
					c.Logger.Error("mqtt:handle-next-failed", slog.String("err", err.Error()))
					continue
				}
			case <-heartbeat.C:
				// If we haven't read any sensor readings from the channel since the last heartbeat interval,
				// ping the MQTT broken to keep the connnection alive.
				err = mqttClient.HandleNext()
				if err != nil {
					c.Logger.Error("mqtt:handle-next-failed", slog.String("err", err.Error()))
					continue
				}
			default:
				// If we've got nothing to do, release the thread so other go routines can run.
				// We only need to do this because TinyGo runs on a single core
				// https://tinygo.org/docs/guides/tips-n-tricks/
				runtime.Gosched()
			}

		}

		c.Logger.Error("mqtt:disconnected", slog.Any("reason", mqttClient.Err()))
		lcd.Send(lcdMessages, "Disconnected", "Reconnecting...")
		closeConn("disconnected")
		runtime.Gosched()
	}
}

type telemetryPayload struct {
	SourceReadingKey string             `json:"source_reading_key"`
	RecordedAt       string             `json:"recorded_at"`
	Measurements     map[string]float64 `json:"measurements"`
}

func (c *Client) publishTopic() (string, error) {
	return topic.Resolve(c.Topic, c.Username, c.DeviceSlug)
}

func (c *Client) deviceSlug() string {
	if c.DeviceSlug != "" {
		return strings.TrimSpace(c.DeviceSlug)
	}
	return c.ID
}

func canonicalPayload(deviceSlug string, reading SensorReading) ([]byte, error) {
	if reading.Timestamp.IsZero() {
		return nil, errors.New("timestamp is required for canonical telemetry")
	}
	if deviceSlug == "" {
		return nil, errors.New("device slug is required for canonical telemetry")
	}

	recordedAt := reading.Timestamp.UTC()
	measurements := map[string]float64{
		"temperature": fahrenheitToCelsius(float64(reading.Temperature)),
		"humidity":    float64(reading.Humidity),
	}
	// SGP30 reports equivalent CO₂ (≥400 ppm when valid); a zero eCO2 means no gas
	// sensor is attached, so only emit gas metrics for SGP30-equipped devices.
	if reading.ECO2 != 0 {
		measurements["eco2"] = float64(reading.ECO2)
		measurements["tvoc"] = float64(reading.TVOC)
	}
	// Analog H2S sensor (SEN0568 on ADC0) reports a relative index (R0/Rs).
	// We only emit h2s_index; raw voltage/ADC are intentionally dropped because
	// the sensor's 50 ppm ceiling makes absolute readings meaningless for
	// ambient air, and the backend recognizes only h2s_index anyway.
	// Index is zero until the warm-up baseline is captured ("not yet
	// available", not "clean air"), so a zero means skip this reading.
	if reading.Ratio != 0 {
		measurements["h2s_index"] = float64(reading.Ratio)
	}
	payload := telemetryPayload{
		SourceReadingKey: deviceSlug + ":" + strconv.FormatInt(recordedAt.UnixNano(), 10) + ":" + strconv.FormatInt(reading.SinceBootNS.Nanoseconds(), 10),
		RecordedAt:       recordedAt.Format(time.RFC3339),
		Measurements:     measurements,
	}

	return json.Marshal(payload)
}

func fahrenheitToCelsius(tempF float64) float64 {
	return (tempF - 32) / 1.8
}

// splitHostPort splits a host:port string into separate host and port components.
// Returns an error if the format is invalid.
func splitHostPort(addr string) (host, port string, err error) {
	// Find the last colon to support IPv6 addresses
	colonIdx := -1
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			colonIdx = i
			break
		}
	}

	if colonIdx == -1 {
		return "", "", errors.New("missing port in address")
	}

	host = addr[:colonIdx]
	port = addr[colonIdx+1:]

	if host == "" {
		return "", "", errors.New("empty host")
	}
	if port == "" {
		return "", "", errors.New("empty port")
	}

	return host, port, nil
}

// parsePort converts a port string to uint16.
// Returns 0 if parsing fails (caller should validate).
func parsePort(portStr string) uint16 {
	var port uint16
	for i := 0; i < len(portStr); i++ {
		if portStr[i] < '0' || portStr[i] > '9' {
			return 0
		}
		port = port*10 + uint16(portStr[i]-'0')
	}
	return port
}

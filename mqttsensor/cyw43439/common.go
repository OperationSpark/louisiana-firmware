// Package cyw43439 provides common utilities for setting up WiFi connectivity
// on Raspberry Pi Pico W devices using the CYW43439 wireless chip.
//
// This package simplifies the process of:
//   - Initializing the CYW43439 WiFi device
//   - Joining WPA2-secured or open WiFi networks
//   - Performing DHCP configuration with fallback to static IP
//   - DNS hostname resolution via lneto stack
//   - Asynchronous network packet handling
//
// The code is adapted from the examples in the soypat/cyw43439 repository:
// https://github.com/soypat/cyw43439/tree/main/examples/common
//
// Original author: Patricio Whittingslow (soypat)
package cyw43439

import (
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/soypat/cyw43439"
	"github.com/soypat/lneto/x/xnet"
)

const mtu = cyw43439.MTU

var (
	ssid string
	pass string
)

// SSID returns the WiFi SSID set via linker flags.
func SSID() string { return ssid }

// Password returns the WiFi password set via linker flags.
func Password() string { return pass }

// DefaultWifiConfig returns the default WiFi configuration for the CYW43439 device.
func DefaultWifiConfig() cyw43439.Config {
	return cyw43439.DefaultWifiConfig()
}

// StackConfig configures the lneto stack.
type StackConfig struct {
	// Hostname is used for DHCP requests.
	Hostname string
	// MaxTCPPorts is the number of TCP ports to open for the stack.
	MaxTCPPorts int
	// Logger for stack operations.
	Logger *slog.Logger
	// RandSeed is an optional random seed for the stack's PRNG.
	RandSeed int64
}

// DHCPConfig configures the DHCP request.
type DHCPConfig struct {
	// RequestedAddr is the preferred IP address to request via DHCP.
	// If DHCP fails and this is set, it will be used as a static IP.
	RequestedAddr netip.Addr
	// Hostname is used in the DHCP request.
	Hostname string
}

// Stack wraps the lneto StackAsync and CYW43439 device for network operations.
type Stack struct {
	s       xnet.StackAsync
	dev     *cyw43439.Device
	log     *slog.Logger
	sendbuf []byte
}

// NewConfiguredPicoWithStack creates a new WiFi stack with the given configuration.
// It initializes the CYW43439 device, joins the WiFi network, and prepares the stack.
func NewConfiguredPicoWithStack(ssid, pass string, wificfg cyw43439.Config, cfg StackConfig) (*Stack, error) {
	if cfg.Hostname == "" {
		return nil, errors.New("empty hostname")
	}

	logger := cfg.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
			Level: slog.Level(127), // Make temporary logger that does no logging.
		}))
	}

	start := time.Now()
	dev := cyw43439.NewPicoWDevice()
	dev.SetLogger(logger)

	logger.Info("initializing pico W device...")
	err := dev.Init(wificfg)
	if err != nil {
		return nil, errors.New("wifi init failed:" + err.Error())
	}
	logger.Info("cyw43439:Init", slog.Duration("duration", time.Since(start)))

	if len(pass) == 0 {
		logger.Info("joining open network:", slog.String("ssid", ssid))
	} else {
		logger.Info("joining WPA secure network", slog.String("ssid", ssid), slog.Int("passlen", len(pass)))
	}

	for {
		err = dev.JoinWPA2(ssid, pass)
		if err == nil {
			break
		}
		logger.Error("wifi join failed", slog.String("err", err.Error()))
		time.Sleep(5 * time.Second)
	}

	mac, err := dev.HardwareAddr6()
	if err != nil {
		return nil, errors.New("get hardware address:" + err.Error())
	}
	logger.Info("wifi join success!", slog.String("mac", net.HardwareAddr(mac[:]).String()))

	// Configure Stack
	stack := &Stack{
		dev:     dev,
		log:     logger,
		sendbuf: make([]byte, mtu),
	}

	maxTCP := cfg.MaxTCPPorts
	if maxTCP < 1 {
		maxTCP = 1
	}

	elapsed := time.Since(start)
	err = stack.s.Reset(xnet.StackConfig{
		Hostname:        cfg.Hostname,
		MaxTCPConns:     maxTCP,
		RandSeed:        elapsed.Nanoseconds() ^ cfg.RandSeed,
		HardwareAddress: mac,
		MTU:             mtu,
	})
	if err != nil {
		return nil, errors.New("stack reset:" + err.Error())
	}

	dev.RecvEthHandle(func(pkt []byte) error {
		return stack.s.Demux(pkt, 0)
	})

	return stack, nil
}

// SetupWithDHCP performs DHCP configuration and returns the results.
func (s *Stack) SetupWithDHCP(cfg DHCPConfig) (*xnet.DHCPResults, error) {
	if !cfg.RequestedAddr.Is4() {
		// If no address provided, use a zero address
		if !cfg.RequestedAddr.IsValid() {
			cfg.RequestedAddr = netip.AddrFrom4([4]byte{0, 0, 0, 0})
		} else {
			return nil, errors.New("only dhcpv4 supported")
		}
	}

	const pollTime = 50 * time.Millisecond
	rstack := s.s.StackRetrying(pollTime)

	s.log.Info("DHCP:starting")

	dhcpResults, err := rstack.DoDHCPv4(cfg.RequestedAddr.As4(), 3*time.Second, 3)
	if err != nil {
		// If DHCP fails but we have a requested address, use it as static IP
		if cfg.RequestedAddr.IsValid() && !cfg.RequestedAddr.IsUnspecified() {
			s.log.Info("DHCP did not complete, assigning static IP", slog.String("ip", cfg.RequestedAddr.String()))
			s.s.SetIPAddr(cfg.RequestedAddr)
			return &xnet.DHCPResults{
				AssignedAddr: cfg.RequestedAddr,
			}, nil
		}
		return nil, errors.New("dhcp failed:" + err.Error())
	}

	// Apply DHCP results to the stack
	err = s.s.AssimilateDHCPResults(dhcpResults)
	if err != nil {
		return nil, errors.New("assimilate dhcp:" + err.Error())
	}

	// Resolve and set the router hardware address as the gateway
	gatewayHW, err := rstack.DoResolveHardwareAddress6(dhcpResults.Router, 500*time.Millisecond, 4)
	if err != nil {
		return nil, errors.New("resolve gateway:" + err.Error())
	}
	s.s.SetGateway6(gatewayHW)

	s.log.Info("DHCP complete",
		slog.String("ourIP", dhcpResults.AssignedAddr.String()),
		slog.String("gateway", dhcpResults.Gateway.String()),
		slog.String("router", dhcpResults.Router.String()),
		slog.Uint64("lease_sec", uint64(dhcpResults.TLease)),
	)

	return dhcpResults, nil
}

// RecvAndSend processes incoming and outgoing packets.
// Returns the number of bytes sent and received, and any error.
// This should be called in a loop from a goroutine.
func (s *Stack) RecvAndSend() (send, recv int, err error) {
	// Poll for incoming packets
	gotPacket, errRecv := s.dev.PollOne()
	if gotPacket {
		recv = 1
	}
	if errRecv != nil {
		s.log.Error("RecvAndSend:PollOne", slog.String("err", errRecv.Error()))
	}

	// Handle outgoing packets via Encapsulate
	send, err = s.s.Encapsulate(s.sendbuf, -1, 0)
	if err != nil {
		s.log.Error("RecvAndSend:Encapsulate", slog.Int("plen", send), slog.String("err", err.Error()))
	} else {
		err = errRecv // Pass receive error if encapsulate succeeded
	}

	if send == 0 {
		return send, recv, err
	}

	// Send the encapsulated packet
	err = s.dev.SendEth(s.sendbuf[:send])
	if err != nil {
		s.log.Error("RecvAndSend:SendEth", slog.Int("plen", send), slog.String("err", err.Error()))
	}

	return send, recv, err
}

// LnetoStack returns the underlying lneto StackAsync for direct access.
// This is needed for operations like TCP connections and DNS lookups.
func (s *Stack) LnetoStack() *xnet.StackAsync {
	return &s.s
}

// Prand32 returns a pseudo-random 32-bit number from the stack's PRNG.
func (s *Stack) Prand32() uint32 {
	return s.s.Prand32()
}

// Addr returns the current IP address of the stack.
func (s *Stack) Addr() netip.Addr {
	return s.s.Addr()
}

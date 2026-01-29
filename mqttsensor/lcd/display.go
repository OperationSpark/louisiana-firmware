// Package lcd provides a channel-based messaging system for HD44780 LCD displays.
//
// Example usage:
//
//	lcdMessages := make(chan lcd.Message, 10)
//	handler := lcd.NewHandler(device, lcdMessages, logger)
//	go handler.Run()
//
//	// Send messages non-blocking
//	select {
//	case lcdMessages <- lcd.Message{
//	    Line1: []byte("Status"),
//	    Line2: []byte("OK"),
//	}:
//	default:
//	    // Channel full - message dropped
//	}
package lcd

import (
	"log/slog"

	"tinygo.org/x/drivers/hd44780i2c"
)

// Message represents a two-line LCD message.
type Message struct {
	Line1 []byte
	Line2 []byte
}

// Handler processes LCD messages from a channel.
type Handler struct {
	device   hd44780i2c.Device
	messages <-chan Message
	logger   *slog.Logger
	rows     int
	columns  int
}

// NewHandler creates a new 16x2 LCD message handler.
func NewHandler(device hd44780i2c.Device, messages <-chan Message, logger *slog.Logger) *Handler {
	return &Handler{
		device:   device,
		messages: messages,
		logger:   logger,
		rows:     2,
		columns:  16,
	}
}

// Run processes messages from the channel and updates the LCD.
// Run should be called in a separate goroutine.
func (h *Handler) Run() {
	for msg := range h.messages {
		h.display(msg)
	}
}

// display prints msg to the LCD handler.
func (h *Handler) display(msg Message) {
	h.device.ClearDisplay()
	h.device.SetCursor(0, 0)

	// Truncate in-place, no allocation
	if len(msg.Line1) > h.columns {
		h.device.Print(msg.Line1[:h.columns])
	} else {
		h.device.Print(msg.Line1)
	}

	h.device.SetCursor(0, 1)
	if len(msg.Line2) > h.columns {
		h.device.Print(msg.Line2[:h.columns])
	} else {
		h.device.Print(msg.Line2)
	}
}

// Send sends a message to the LCD channel non-blocking.
// If the channel is full, the message is dropped.
func Send(ch chan<- Message, line1, line2 string) {
	select {
	case ch <- Message{
		Line1: []byte(line1),
		Line2: []byte(line2),
	}:
	default:
		// Channel full - drop message
	}
}

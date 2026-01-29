package ntp

import (
	"errors"
	"log/slog"
	"runtime"
	"time"

	"github.com/soypat/lneto/x/xnet"
)

// SyncTime synchronizes the system time using NTP.
// It resolves an NTP server via DNS, performs an NTP time sync,
// and adjusts the system clock accordingly.
//
// Returns nil on success, error if sync fails.
func SyncTime(stack *xnet.StackAsync, logger *slog.Logger) error {
	const pollTime = 5 * time.Millisecond
	rstack := stack.StackRetrying(pollTime)

	// DNS lookup for NTP server (built-in, no custom Resolver needed)
	logger.Info("ntp:resolving pool.ntp.org")
	addrs, err := rstack.DoLookupIP("pool.ntp.org", 5*time.Second, 3)
	if err != nil {
		return errors.New("ntp dns lookup:" + err.Error())
	}
	if len(addrs) == 0 {
		return errors.New("ntp dns lookup: no addresses returned")
	}
	logger.Info("ntp:resolved", slog.String("addr", addrs[0].String()))

	// Perform NTP request (built-in, no manual polling)
	logger.Info("ntp:requesting time")
	offset, err := rstack.DoNTP(addrs[0], 5*time.Second, 3)
	if err != nil {
		return errors.New("ntp request:" + err.Error())
	}

	// Apply time offset
	runtime.AdjustTimeOffset(int64(offset))
	logger.Info("ntp:complete", slog.Duration("offset", offset))
	return nil
}

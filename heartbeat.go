/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"context"
	"log/slog"
	"time"
)

// Heartbeat sends signal for connection keep alive.
type Heartbeat struct {
	clientInterval time.Duration
	serverInterval time.Duration
	deadmanTimeout time.Duration
	adaptive       bool
}

// HeartbeatOption type
type HeartbeatOption func(*Heartbeat)

// WithClientInterval is heartbeat client interval setter
func WithClientInterval(interval time.Duration) HeartbeatOption {
	return func(heartbeat *Heartbeat) {
		heartbeat.clientInterval = interval
	}
}

// WithServerInterval is heartbeat server interval setter
func WithServerInterval(interval time.Duration) HeartbeatOption {
	return func(heartbeat *Heartbeat) {
		// minimum 1 minute
		heartbeat.serverInterval = max(interval, 1*time.Minute)
	}
}

// WithDeadmanTimeout is heartbeat deadman timeout setter
func WithDeadmanTimeout(timeout time.Duration) HeartbeatOption {
	return func(heartbeat *Heartbeat) {
		heartbeat.deadmanTimeout = timeout
	}
}

// WithAdaptive is heartbeat adaptive setter
func WithAdaptive(enabled bool) HeartbeatOption {
	return func(heartbeat *Heartbeat) {
		heartbeat.adaptive = enabled
	}
}

func newHeartbeat(options ...HeartbeatOption) *Heartbeat {
	h := &Heartbeat{}
	for _, option := range options {
		option(h)
	}
	return h
}

func (h *Heartbeat) start(ctx context.Context, logger *slog.Logger, heartbeatAck chan bool, sendHeartbeat func() error, onDisconnect func()) {
	if h.deadmanTimeout <= 0 {
		if h.clientInterval < h.serverInterval {
			h.deadmanTimeout = durationDeadmanTimeout(h.serverInterval)
		} else {
			h.deadmanTimeout = durationDeadmanTimeout(h.clientInterval)
		}
	}

	var (
		pingDeadman  *time.Timer
		pingDeadmanC <-chan time.Time
	)
	if h.deadmanTimeout > 0 {
		pingDeadman = time.NewTimer(h.deadmanTimeout)
		pingDeadmanC = pingDeadman.C
	}
	defer func() {
		logger.Debug("heartbeat stoped")
		if pingDeadman != nil {
			pingDeadman.Stop()
		}
	}()

	var (
		pingTicker  *time.Ticker
		pingTickerC <-chan time.Time
	)
	if h.clientInterval > 0 {
		pingTicker = time.NewTicker(h.clientInterval)
		pingTickerC = pingTicker.C
	}
	defer func() {
		if pingTicker != nil {
			pingTicker.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeatAck:
			if pingDeadman != nil {
				pingDeadman.Reset(h.deadmanTimeout)
			}
		case <-pingDeadmanC:
			// force disconnect
			logger.Info("force disconnect by heartbeat")
			onDisconnect()
			return
		case <-pingTickerC:
			// send heartbeat to FCM
			err := sendHeartbeat()
			if err != nil {
				return
			}
		}
	}
}

func durationDeadmanTimeout(interval time.Duration) time.Duration {
	return interval * 4
}

/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"context"
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

func (h *Heartbeat) start(ctx context.Context, mcs *mcs) {
	if h.deadmanTimeout <= 0 {
		h.deadmanTimeout = durationDeadmanTimeout(max(h.clientInterval, h.serverInterval))
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
		if pingDeadman != nil {
			pingDeadman.Stop()
		}
	}()

	var (
		pingTicker  *time.Ticker
		pingTickerC <-chan time.Time
	)
	if h.clientInterval > 0 {
		pingTicker = time.NewTicker(max(h.clientInterval, 1*time.Minute))
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
		case <-mcs.heartbeatAck:
			if pingDeadman != nil {
				pingDeadman.Reset(h.deadmanTimeout)
			}
		case <-pingDeadmanC:
			// force disconnect
			mcs.log.Print("force disconnect by heartbeat")
			mcs.disconnect()
			return
		case <-pingTickerC:
			// send heartbeat to FCM
			err := mcs.SendHeartbeatPingPacket()
			if err != nil {
				return
			}
		}
	}
}

func durationDeadmanTimeout(interval time.Duration) time.Duration {
	return interval * 4
}

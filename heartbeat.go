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
		heartbeat.serverInterval = interval
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
		h.deadmanTimeout = durationDeadmanTimeout(h.clientInterval)
	}

	pingDeadman := time.NewTimer(h.deadmanTimeout)
	defer pingDeadman.Stop()

	t := time.NewTicker(h.clientInterval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mcs.heartbeatAck:
			pingDeadman.Reset(h.deadmanTimeout)
		case <-pingDeadman.C:
			// force disconnect
			mcs.log.Print("force disconnect by heartbeat")
			mcs.disconnect()
			return
		case <-t.C:
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

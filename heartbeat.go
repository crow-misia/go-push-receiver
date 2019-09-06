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
	period time.Duration
}

func newHeartbeat(period time.Duration) *Heartbeat {
	return &Heartbeat{
		period: period,
	}
}

func (h *Heartbeat) start(ctx context.Context, mcs *mcs) {
	pingDeadman := time.NewTimer(durationDeadman(h.period))
	defer pingDeadman.Stop()

	t := time.NewTicker(h.period)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-mcs.heartbeatAck:
			pingDeadman.Reset(durationDeadman(h.period))
		case <-pingDeadman.C:
			// force disconnect
			mcs.disconnect()
			return
		case <-t.C:
			// send heartbeat to FCM
			err := mcs.SendHeartbeatPacketPing()
			if err != nil {
				return
			}
		}
	}
}

func durationDeadman(period time.Duration) time.Duration {
	return period * 4
}

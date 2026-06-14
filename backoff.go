/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"crypto/rand"
	"encoding/binary"
	"math"
	"time"
)

// Backoff with jitter sleep to prevent overloaded conditions during intervals
// https://www.awsarchitectureblog.com/2015/03/backoff.html
type Backoff struct {
	attempts int
	base     int64
	max      int64
}

// NewBackoff creates Backoff instance.
func NewBackoff(base time.Duration, max time.Duration) *Backoff {
	return &Backoff{
		attempts: 0,
		base:     int64(base),
		max:      int64(max),
	}
}

func (b *Backoff) duration() time.Duration {
	b.attempts++
	if b.attempts > 62 {
		b.attempts = 62
	}
	n := (1 << uint(b.attempts)) * b.base
	if n <= 0 {
		n = math.MaxInt64
	}

	var duration int64
	if n > 1 {
		var buf [8]byte
		if _, err := rand.Read(buf[:]); err != nil {
			duration = 0
		} else {
			tmpUint64 := binary.BigEndian.Uint64(buf[:])
			tmpInt64 := int64(tmpUint64 & 0x7FFFFFFFFFFFFFFF)
			duration = tmpInt64 % n
		}
	}
	if duration > b.max {
		duration = b.max
	}

	return time.Duration(duration)
}

func (b *Backoff) reset() {
	b.attempts = 0
}

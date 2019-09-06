/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"math/rand"
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

	n := 1 << uint(b.attempts) * b.base
	if n < 0 {
		n = 0
	}
	duration := rand.Int63n(n)

	if duration > b.max {
		duration = b.max
	}
	return time.Duration(duration)
}

func (b *Backoff) reset() {
	b.attempts = 0
}

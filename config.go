package pushreceiver

import (
	"github.com/crow-misia/go-push-receiver/internal"
	"time"
)

// Config is Config for FCM Client
type Config struct {
	// Dial timeout for establishing new connections.
	// Default is 30 seconds.
	DialTimeout time.Duration
	// Timeout for socket reads / writes. If reached, commands will fail
	// with a timeout instead of blocking. Use value -1 for no timeout and 0 for default.
	// Default is 30 seconds.
	Timeout time.Duration
	// Minimum backoff between each retry.
	// Default is 5 seconds
	MinRetryBackoff time.Duration
	// Maximum backoff between each retry.
	// Default is 15 minute
	MaxRetryBackoff time.Duration
}

func (opt *Config) init() {
	if opt.DialTimeout == 0 {
		opt.DialTimeout = internal.DialTimeout * time.Second
	}
	switch opt.Timeout {
	case -1:
		opt.Timeout = 0
	case 0:
		opt.Timeout = internal.Timeout * time.Second
	}
	switch opt.MinRetryBackoff {
	case -1:
		opt.MinRetryBackoff = 0
	case 0:
		opt.MinRetryBackoff = internal.MinRetryBackoff * time.Second
	}
	switch opt.MaxRetryBackoff {
	case -1:
		opt.MaxRetryBackoff = 0
	case 0:
		opt.MaxRetryBackoff = internal.MaxRetryBackoff * time.Second
	}
}

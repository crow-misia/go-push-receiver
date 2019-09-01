package pushreceiver

import (
	"github.com/crow-misia/go-push-receiver/internal"
	"net"
	"net/http"
	"time"
)

// Config is Config for FCM Client
type Config struct {
	// Dial timeout for establishing new connections.
	// Default is 5 seconds.
	DialTimeout time.Duration
	// Timeout for socket reads. If reached, commands will fail
	// with a timeout instead of blocking. Use value -1 for no timeout and 0 for default.
	// Default is 3 seconds.
	ReadTimeout time.Duration
	// Timeout for socket writes. If reached, commands will fail
	// with a timeout instead of blocking.
	// Default is ReadTimeout.
	WriteTimeout time.Duration
	// KeepAlive duration.
	// Default is 5 minute.
	KeepAlive time.Duration
	// Minimum backoff between each retry.
	// Default is 5 seconds
	MinRetryBackoff time.Duration
	// Maximum backoff between each retry.
	// Default is 15 minute
	MaxRetryBackoff time.Duration
	// HTTPClient is Client for HTTP Request.
	HTTPClient *http.Client
	// Dialer contains options for connecting to an address.
	Dialer *net.Dialer
}

func (opt *Config) init() {
	if opt.DialTimeout == 0 {
		opt.DialTimeout = internal.ConnectionTimeout * time.Second
	}
	switch opt.ReadTimeout {
	case -1:
		opt.ReadTimeout = 0
	case 0:
		opt.ReadTimeout = internal.NetworkTimeout * time.Second
	}
	switch opt.WriteTimeout {
	case -1:
		opt.WriteTimeout = 0
	case 0:
		opt.WriteTimeout = opt.ReadTimeout
	}
	if opt.KeepAlive == 0 {
		opt.KeepAlive = internal.ConnectionKeepAlive * time.Minute
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

	if opt.Dialer == nil {
		opt.Dialer = &net.Dialer{}
	}
	opt.Dialer.KeepAlive = opt.KeepAlive
	opt.Dialer.Timeout = opt.DialTimeout

	if opt.HTTPClient == nil {
		opt.HTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: opt.Dialer.DialContext,
			},
		}
	}
}

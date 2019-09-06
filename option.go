/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"crypto/tls"
	"net"
	"time"
)

// ClientOption type
type ClientOption func(*Client)

// WithLogger is logger setter
func WithLogger(logger logger) ClientOption {
	return func(client *Client) {
		client.log = internalLog{logger}
	}
}

// WithCreds is credentials setter
func WithCreds(creds *FCMCredentials) ClientOption {
	return func(client *Client) {
		client.creds = creds
	}
}

// WithReceivedPersistentID is received persistentId list setter
func WithReceivedPersistentID(ids []string) ClientOption {
	return func(client *Client) {
		client.receivedPersistentID = ids
	}
}

// WithHTTPClient is http.Client setter
func WithHTTPClient(c httpClient) ClientOption {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithTLSConfig is tls.Config setter
func WithTLSConfig(c *tls.Config) ClientOption {
	return func(client *Client) {
		client.tlsConfig = c
	}
}

// WithBackoff is Backoff setter
func WithBackoff(b *Backoff) ClientOption {
	return func(client *Client) {
		client.backoff = b
	}
}

// WithHeartbeatPeriod is Heartbeat period setter
func WithHeartbeatPeriod(period time.Duration) ClientOption {
	return func(client *Client) {
		client.heartbeat = newHeartbeat(period)
	}
}

// WithDialer is net.Dialer setter
func WithDialer(dialer *net.Dialer) ClientOption {
	return func(client *Client) {
		client.dialer = dialer
	}
}

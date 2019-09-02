package pushreceiver

import (
	"net"
	"net/http"
)

// Option type
type Option func(*Client)

// WithCreds is credentials setter
func WithCreds(creds *FcmCredentials) Option {
	return func(client *Client) {
		client.creds = creds
	}
}

// WithReceivedPersistentIds is received persistentId list setter
func WithReceivedPersistentIds(ids []string) Option {
	return func(client *Client) {
		client.receivedPersistentIds = ids
	}
}

// WithHttpClient is http.Client setter
func WithHttpClient(c *http.Client) Option {
	return func(client *Client) {
		client.httpClient = c
	}
}

// WithDialer is net.Dialer setter
func WithDialer(dialer *net.Dialer) Option {
	return func(client *Client) {
		client.dialer = dialer
	}
}

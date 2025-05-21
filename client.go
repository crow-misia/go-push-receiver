/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

// Package pushreceiver is Push Message Receiver library from FCM.
package pushreceiver

import (
	"context"
	"crypto/tls"
	"github.com/pkg/errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// httpClient defines the minimal interface needed for an http.Client to be implemented.
type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is FCM Push receive client.
type Client struct {
	apiKey               string
	projectId            string
	appId                string
	vapidKey             string
	logger               *slog.Logger
	httpClient           httpClient
	tlsConfig            *tls.Config
	creds                *FCMCredentials
	dialer               *net.Dialer
	backoff              *Backoff
	heartbeat            *Heartbeat
	receivedPersistentId []string
	retryDisabled        bool
	Events               chan Event
}

// New returns a new FCM push receive client instance.
func New(config *Config, options ...ClientOption) *Client {
	c := &Client{
		apiKey:    config.ApiKey,
		projectId: config.ProjectId,
		appId:     config.AppId,
	}

	for _, option := range options {
		option(c)
	}

	// set defaults
	c.setDefaultOptions()

	c.logger.Debug("Config", "apiKey", c.apiKey, "projectId", c.projectId, "appId", c.appId)

	return c
}

func (c *Client) post(ctx context.Context, url string, body io.Reader, headerSetter func(*http.Header)) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, errors.Wrap(err, "create post request error")
	}
	headerSetter(&req.Header)

	return c.httpClient.Do(req)
}

// setDefaultOptions set default options.
func (c *Client) setDefaultOptions() {
	// set defaults
	if c.backoff == nil {
		c.backoff = NewBackoff(defaultBackoffBase*time.Second, defaultBackoffMax*time.Second)
	}
	if c.heartbeat == nil {
		c.heartbeat = newHeartbeat(
			WithClientInterval(defaultHeartbeatPeriod * time.Minute),
		)
	}
	if c.tlsConfig == nil {
		c.tlsConfig = &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS13,
		}
	}
	if c.dialer == nil {
		c.dialer = &net.Dialer{
			Timeout:       defaultDialTimeout * time.Second,
			KeepAlive:     defaultKeepAlive * time.Minute,
			FallbackDelay: 30 * time.Millisecond,
		}
	}
	if c.httpClient == nil {
		c.httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: c.tlsConfig,
			},
		}
	}
	if c.logger == nil {
		c.logger = slog.New(slog.DiscardHandler)
	}
	if c.Events == nil {
		c.Events = make(chan Event, 50)
	}
}

func closeResponse(res *http.Response) error {
	defer res.Body.Close()
	_, err := io.Copy(io.Discard, res.Body)
	return err
}

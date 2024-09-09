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
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/pkg/errors"
)

// httpClient defines the minimal interface needed for an http.Client to be implemented.
type httpClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Client is FCM Push receive client.
type Client struct {
	apiKey               string
	appId                string
	projectId            string
	log                  ilogger
	httpClient           httpClient
	tlsConfig            *tls.Config
	creds                *FCMCredentials
	dialer               *net.Dialer
	backoff              *Backoff
	heartbeat            *Heartbeat
	receivedPersistentID []string
	retryDisabled        bool
	Events               chan Event
}

// New returns a new FCM push receive client instance.
func New(apiKey string, appId string, projectId string, options ...ClientOption) *Client {
	c := &Client{
		apiKey:    apiKey,
		appId:     appId,
		projectId: projectId,
		Events:    make(chan Event, 50),
	}

	for _, option := range options {
		option(c)
	}

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
	if c.log == nil {
		c.log = &discard{}
	}

	c.log.Print("API Key: ", c.apiKey) // TODO: Not sure if this is good practice
	c.log.Print("App ID: ", c.appId)
	c.log.Print("Project ID: ", c.projectId)

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

func closeResponse(res *http.Response) error {
	defer res.Body.Close()
	_, err := io.Copy(io.Discard, res.Body)
	return err
}

func generateFid() (string, error) {
	byteArr := make([]byte, 17)
	_, err := rand.Read(byteArr)
	if err != nil {
		return "", err
	}

	// Replace the first four bits with 0111 for the constant FID header
	byteArr[0] = 0b01110000 + (byteArr[0] % 0b00010000)

	return base64.RawURLEncoding.EncodeToString(byteArr)[:22], nil
}

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.
//
// see https://go.googlesource.com/go/+/refs/changes/55/93255/2/src/crypto/tls/tls.go

package pushreceiver

import (
	"context"
	"crypto/tls"
	"net"
	"strings"
	"time"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "tls: DialWithDialer timed out" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// DialContextWithDialer connects to the given network address using
// dialer.DialContext with the provided Context and then initiates a TLS
// handshake, returning the resulting TLS connection.
//
// Any timeout or deadline given in the dialer applies to the connection and
// TLS handshake as a whole.
//
// The provided Context must be non-nil. If the context expires before
// the connection and TLS handshake are complete, an error is returned.
// Once the TLS handshake completes successfully, any expiration of the
// context will not affect the connection.
//
// DialContextWithDialer interprets a nil configuration as equivalent to
// the zero configuration; see the documentation of Config for the defaults.
func dialContextWithDialer(ctx context.Context, dialer *net.Dialer, network, addr string, config *tls.Config) (*tls.Conn, error) {
	// We want the Timeout and Deadline values from dialer to cover the
	// whole process: TCP connection and TLS handshake. This means that we
	// also need to start our own timers now.
	timeout := dialer.Timeout
	if !dialer.Deadline.IsZero() {
		deadlineTimeout := time.Until(dialer.Deadline)
		if timeout == 0 || deadlineTimeout < timeout {
			timeout = deadlineTimeout
		}
	}
	if timeout != 0 {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		ctx = subCtx
	}
	var errChannel chan error
	if ctx != context.Background() {
		errChannel = make(chan error, 2)
		subCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			<-subCtx.Done()
			switch subCtx.Err() {
			case context.DeadlineExceeded:
				errChannel <- timeoutError{}
			default:
				errChannel <- subCtx.Err()
			}
		}()
		ctx = subCtx
	}
	rawConn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}
	colonPos := strings.LastIndex(addr, ":")
	if colonPos == -1 {
		colonPos = len(addr)
	}
	hostname := addr[:colonPos]
	// If no ServerName is set, infer the ServerName
	// from the hostname we're connecting to.
	if config.ServerName == "" {
		// Make a copy to avoid polluting argument or default.
		c := config.Clone()
		c.ServerName = hostname
		config = c
	}
	conn := tls.Client(rawConn, config)
	if ctx == context.Background() {
		err = conn.Handshake()
	} else {
		go func() {
			errChannel <- conn.Handshake()
		}()
		err = <-errChannel
	}
	if err != nil {
		rawConn.Close()
		return nil, err
	}
	return conn, nil
}

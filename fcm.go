/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"github.com/pkg/errors"
)

type fcmRegisterResponse struct {
	Token   string `json:"token"`
	PushSet string `json:"pushSet"`
}

// FCMCredentials is Credentials for FCM
type FCMCredentials struct {
	AppID         string `json:"appId"`
	AndroidID     uint64 `json:"androidId"`
	SecurityToken uint64 `json:"securityToken"`
	Token         string `json:"token"`
	PrivateKey    []byte `json:"privateKey"`
	PublicKey     []byte `json:"publicKey"`
	AuthSecret    []byte `json:"authSecret"`
}

// Subscribe subscribe to FCM.
func (c *Client) Subscribe(ctx context.Context) {
	defer close(c.Events)

	for ctx.Err() == nil {
		var err error
		if c.creds == nil {
			err = c.register(ctx)
		} else {
			_, err = c.checkIn(ctx, &checkInOption{c.creds.AndroidID, c.creds.SecurityToken})
		}
		if err == nil {
			// reset retry count when connection success
			c.backoff.reset()

			err = c.tryToConnect(ctx)
		}
		if err != nil {
			if errors.Is(err, ErrGcmAuthorization) {
				c.Events <- &UnauthorizedError{err}
				c.creds = nil
			}
			if c.retryDisabled {
				return
			}
			// retry
			sleepDuration := c.backoff.duration()
			c.Events <- &RetryEvent{err, sleepDuration}
			tick := time.After(sleepDuration)
			select {
			case <-tick:
			case <-ctx.Done():
				return
			}
		}
	}
}

func (c *Client) register(ctx context.Context) error {
	response, err := c.registerGCM(ctx)
	if err != nil {
		return err
	}
	creds, err := c.subscribeFCM(ctx, response)
	if err != nil {
		return err
	}
	c.creds = creds
	c.Events <- &UpdateCredentialsEvent{creds}
	return nil
}

func (c *Client) tryToConnect(ctx context.Context) error {
	conn, err := tls.DialWithDialer(c.dialer, "tcp", mtalkServer, c.tlsConfig)
	if err != nil {
		return errors.Wrap(err, "dial failed to FCM")
	}
	defer conn.Close()

	mcs := newMCS(conn, c.log, c.creds, c.heartbeat, c.Events)
	defer mcs.disconnect()

	err = mcs.SendLoginPacket(c.receivedPersistentID)
	if err != nil {
		return errors.Wrap(err, "send login packet failed")
	}

	// start heartbeat
	go c.heartbeat.start(ctx, mcs)

	select {
	case err := <-c.asyncPerformRead(mcs):
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) asyncPerformRead(mcs *mcs) <-chan error {
	ch := make(chan error)
	go func() {
		defer close(ch)
		ch <- c.performRead(mcs)
	}()
	return ch
}

func (c *Client) performRead(mcs *mcs) error {
	// receive version
	err := mcs.ReceiveVersion()
	if err != nil {
		return errors.Wrap(err, "receive version failed")
	}

	for {
		// receive tag
		data, err := mcs.PerformReadTag()
		if err != nil {
			return errors.Wrap(err, "receive tag failed")
		}
		if data == nil {
			return ErrFcmNotEnoughData
		}

		err = c.onDataMessage(data)
		if err != nil {
			return errors.Wrap(err, "process data message failed")
		}
	}
}

func (c *Client) onDataMessage(tagData interface{}) error {
	switch data := tagData.(type) {
	case *pb.LoginResponse:
		c.receivedPersistentID = nil
		c.Events <- &ConnectedEvent{data.GetServerTimestamp()}
	case *pb.DataMessageStanza:
		// To avoid error loops, last streamID is notified even when an error occurs.
		c.receivedPersistentID = append(c.receivedPersistentID, data.GetPersistentId())
		event, err := decryptData(data, c.creds.PrivateKey, c.creds.AuthSecret)
		if err != nil {
			return err
		}
		c.Events <- event
	}
	return nil
}

func (c *Client) subscribeFCM(ctx context.Context, registerResponse *gcmRegisterResponse) (*FCMCredentials, error) {
	credentials := &FCMCredentials{}

	err := credentials.appendCryptoInfo()
	if err != nil {
		return nil, err
	}

	values := url.Values{}
	values.Set("authorized_entity", c.senderID)
	values.Set("endpoint", fcmEndpoint+registerResponse.token)
	values.Set("encryption_key", base64.RawURLEncoding.EncodeToString(credentials.PublicKey))
	values.Set("encryption_auth", base64.RawURLEncoding.EncodeToString(credentials.AuthSecret))

	res, err := c.post(ctx, fcmSubscribe, strings.NewReader(values.Encode()), func(header *http.Header) {
		header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if err != nil {
		return nil, errors.Wrap(err, "request FCM subscribe")
	}
	defer closeResponse(res)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.Errorf("server error: %s", res.Status)
	}
	var fcmRegisterResponse fcmRegisterResponse
	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(&fcmRegisterResponse)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal FCM subscribe response")
	}

	// set responses.
	credentials.AppID = registerResponse.appID
	credentials.AndroidID = registerResponse.androidID
	credentials.SecurityToken = registerResponse.securityToken
	credentials.Token = fcmRegisterResponse.Token

	return credentials, nil
}

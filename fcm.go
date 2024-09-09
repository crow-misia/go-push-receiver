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

type fcmInstallResponse struct {
	Name         string `json:"name"`
	Fid          string `json:"fid"`
	RefreshToken string `json:"refreshToken"`
	AuthToken    struct {
		Token     string `json:"token"`
		ExpiresIn string `json:"expiresIn"`
	} `json:"authToken"`
}

type fcmRegisterResponse struct {
	Token   string `json:"token"`
	PushSet string `json:"pushSet"`
}

// FCMCredentials is Credentials for FCM
type FCMCredentials struct { // TODO: Determine if FCMCredentials needs vapidKey
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
	install, err := c.installFCM(ctx)
	if err != nil {
		return err
	}
	creds, err := c.registerFCM(ctx, response, install)
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

func (c *Client) installFCM(ctx context.Context) (*fcmInstallResponse, error) {
	fid, err := generateFid()
	if err != nil {
		return nil, errors.Wrap(err, "error generating FID")
	}

	body := map[string]string{
		"appId":       c.appId,
		"authVersion": "FIS_v2",
		"fid":         fid,
		"sdkVersion":  "w:0.6.6",
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal FCM install request")
	}

	// TODO: Implement proper heartbeat
	clientHeartbeat := map[string]interface{}{
		"heartbeats": []interface{}{},
		"version":    2,
	}

	clientHeartbeatBytes, err := json.Marshal(clientHeartbeat)
	if err != nil {
		return nil, errors.Wrap(err, "marshal FCM client heartbeat request")
	}

	u, err := url.Parse(fbInstallation)
	if err != nil {
		return nil, errors.Wrap(err, "parse FCM install URL error")
	}
	u.Path, err = url.JoinPath(u.Path, "projects", c.projectId, "installations")
	if err != nil {
		return nil, errors.Wrap(err, "join FCM install URL error")
	}

	res, err := c.post(ctx, u.String(), strings.NewReader(string(bodyBytes)), func(header *http.Header) {
		header.Set("Content-Type", "application/json")
		header.Set("x-firebase-client", base64.StdEncoding.EncodeToString(clientHeartbeatBytes))
		header.Set("x-goog-api-key", c.apiKey)
	})
	if err != nil {
		return nil, errors.Wrap(err, "request FCM install")
	}
	defer closeResponse(res)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.Errorf("server error: %s", res.Status)
	}

	var fcmInstallResponse fcmInstallResponse
	decoder := json.NewDecoder(res.Body)
	err = decoder.Decode(&fcmInstallResponse)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal FCM install response")
	}

	return &fcmInstallResponse, nil
}

func (c *Client) registerFCM(ctx context.Context, registerResponse *gcmRegisterResponse, installResponse *fcmInstallResponse) (*FCMCredentials, error) {
	credentials := &FCMCredentials{}

	err := credentials.appendCryptoInfo()
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"web": map[string]string{
			"applicationPubKey": vapidKey,
			"endpoint":          fcmEndpoint + registerResponse.token,
			"p256dh":            base64.RawURLEncoding.EncodeToString(credentials.PublicKey),
			"auth":              base64.RawURLEncoding.EncodeToString(credentials.AuthSecret),
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal FCM register request")
	}

	u, err := url.Parse(fbRegistration)
	if err != nil {
		return nil, errors.Wrap(err, "parse FCM register URL error")
	}
	u.Path, err = url.JoinPath(u.Path, "projects", c.projectId, "registrations")

	res, err := c.post(ctx, u.String(), strings.NewReader(string(bodyBytes)), func(header *http.Header) {
		header.Set("Content-Type", "application/json")
		header.Set("x-goog-api-key", c.apiKey)
		header.Set("x-goog-firebase-installations-auth", installResponse.AuthToken.Token)
	})
	if err != nil {
		return nil, errors.Wrap(err, "request FCM register")
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

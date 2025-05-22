/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	"github.com/pkg/errors"
)

type authToken struct {
	Token string `json:"token"`
}

type fcmRegisterResponse struct {
	Token   string `json:"token"`
	PushSet string `json:"pushSet"`
}

type fcmInstallResponse struct {
	Name         string    `json:"name"`
	Fid          string    `json:"fid"`
	RefreshToken string    `json:"refreshToken"`
	AuthToken    authToken `json:"authToken"`
}

// FCMCredentials is Credentials for FCM
type FCMCredentials struct {
	AppId         string `json:"appId"`
	AndroidId     uint64 `json:"androidId"`
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
			_, err = c.checkIn(ctx, &checkInOption{c.creds.AndroidId, c.creds.SecurityToken})
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
	register, err := c.registerGCM(ctx)
	if err != nil {
		return err
	}
	install, err := c.installFCM(ctx)
	if err != nil {
		return err
	}
	creds, err := c.registerFCM(ctx, register, install)
	if err != nil {
		return err
	}
	c.creds = creds
	c.Events <- &UpdateCredentialsEvent{creds}
	return nil
}

func (c *Client) tryToConnect(ctx context.Context) error {
	childCtx, cancelChild := context.WithCancel(ctx)
	defer cancelChild()

	conn, err := tls.DialWithDialer(c.dialer, "tcp", mtalkServer, c.tlsConfig)
	if err != nil {
		return errors.Wrap(err, "dial failed to FCM")
	}
	defer conn.Close()

	mcs := c.newMCS(conn)
	defer mcs.disconnect("disconnect")

	err = mcs.SendLoginPacket(c.receivedPersistentId)
	if err != nil {
		return errors.Wrap(err, "send login packet failed")
	}

	// start heartbeat
	go c.heartbeat.start(
		childCtx,
		c.logger,
		mcs.heartbeatAck,
		func() error {
			return mcs.SendHeartbeatPingPacket()
		},
		func() {
			mcs.disconnect("heartbeat")
			cancelChild()
		})

	select {
	case err := <-c.asyncPerformRead(mcs):
		return err
	case <-childCtx.Done():
		return childCtx.Err()
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
		c.receivedPersistentId = nil
		c.Events <- &ConnectedEvent{data.GetServerTimestamp()}
	case *pb.DataMessageStanza:
		// To avoid error loops, last streamId is notified even when an error occurs.
		c.receivedPersistentId = append(c.receivedPersistentId, data.GetPersistentId())
		event, err := decryptData(data, c.creds.PrivateKey, c.creds.AuthSecret)
		if err != nil {
			return err
		}
		c.Events <- event
	}
	return nil
}

func (c *Client) installFCM(ctx context.Context) (*fcmInstallResponse, error) {
	fid, err := generateFID()
	if err != nil {
		return nil, err
	}

	// refs. https://github.com/firebase/firebase-js-sdk/blob/main/packages/installations/src/util/constants.ts#L22
	body := map[string]string{
		"appId":       c.appId,
		"authVersion": "FIS_v2",
		"fid":         fid,
		"sdkVersion":  "w:0.6.17",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal FCM install request")
	}

	url := fmt.Sprintf("%sprojects/%s/installations", firebaseInstallationURL, c.projectId)

	res, err := c.post(ctx, url, bytes.NewReader(bodyBytes), func(header *http.Header) {
		header.Set("Accept", "application/json")
		header.Set("Content-Type", "application/json")
		header.Set("x-goog-api-key", c.apiKey)
		// TODO heartbeat header.
		// https://github.com/firebase/firebase-js-sdk/blob/main/packages/app/src/heartbeatService.ts
		// https://github.com/firebase/firebase-js-sdk/blob/main/packages/installations/src/functions/create-installation-request.ts#L47
		// header.Set("x-firebase-client", ...)
	})
	if err != nil {
		return nil, errors.Wrap(err, "request FCM install")
	}
	defer closeResponse(res)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.Errorf("server error: %s", res.Status)
	}

	var fcmInstallResponse fcmInstallResponse
	err = json.NewDecoder(res.Body).Decode(&fcmInstallResponse)
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
			"applicationPubKey": c.vapidKey,
			"endpoint":          fmt.Sprintf("%s/%s", fcmEndpoint, registerResponse.token),
			"p256dh":            base64.RawURLEncoding.EncodeToString(credentials.PublicKey),
			"auth":              base64.RawURLEncoding.EncodeToString(credentials.AuthSecret),
		},
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, errors.Wrap(err, "marshal FCM register request")
	}

	url := fmt.Sprintf("%sprojects/%s/registrations", firebaseRegistrationURL, c.projectId)

	res, err := c.post(ctx, url, bytes.NewReader(bodyBytes), func(header *http.Header) {
		header.Set("Content-Type", "application/json")
		header.Set("x-goog-api-key", c.apiKey)
		header.Set("x-goog-firebase-installations-auth", fmt.Sprintf("FIS %s", installResponse.AuthToken.Token))
	})
	if err != nil {
		return nil, errors.Wrap(err, "request FCM register")
	}
	defer closeResponse(res)

	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.Errorf("server error: %s", res.Status)
	}
	var fcmRegisterResponse fcmRegisterResponse
	err = json.NewDecoder(res.Body).Decode(&fcmRegisterResponse)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal FCM register response")
	}

	// set responses.
	credentials.AppId = c.appId
	credentials.AndroidId = registerResponse.androidId
	credentials.SecurityToken = registerResponse.securityToken
	credentials.Token = fcmRegisterResponse.Token

	return credentials, nil
}

func generateFID() (string, error) {
	// refs. https://github.com/firebase/firebase-js-sdk/blob/main/packages/installations/src/helpers/generate-fid.ts

	// A valid FID has exactly 22 base64 characters, which is 132 bits, or 16.5
	// bytes. our implementation generates a 17 byte array instead.
	fid := make([]byte, 17)
	_, err := rand.Read(fid)
	if err != nil {
		return "", err
	}

	// Replace the first 4 random bits with the constant FID header of 0b0111.
	fid[0] = 0b01110000 + (fid[0] % 0b00010000)

	return base64.StdEncoding.EncodeToString(fid), nil
}

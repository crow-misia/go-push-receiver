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
	"fmt"
	pb "github.com/crow-misia/go-push-receiver/pb/checkin"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type checkInOption struct {
	androidID     uint64
	securityToken uint64
}

type gcmRegisterResponse struct {
	token         string
	androidID     uint64
	securityToken uint64
	appID         string
}

func (c *Client) registerGCM(ctx context.Context) (*gcmRegisterResponse, error) {
	checkInResp, err := c.checkIn(ctx, &checkInOption{})
	if err != nil {
		return nil, err
	}
	return c.doRegister(ctx, *checkInResp.AndroidId, *checkInResp.SecurityToken)
}

func (c *Client) checkIn(ctx context.Context, opt *checkInOption) (*pb.AndroidCheckinResponse, error) {
	id := int64(opt.androidID)
	r := &pb.AndroidCheckinRequest{
		Checkin: &pb.AndroidCheckinProto{
			ChromeBuild: &pb.ChromeBuildProto{
				Platform:      pb.ChromeBuildProto_PLATFORM_LINUX.Enum(),
				ChromeVersion: proto.String(chromeVersion),
				Channel:       pb.ChromeBuildProto_CHANNEL_STABLE.Enum(),
			},
			Type:       pb.DeviceType_DEVICE_CHROME_BROWSER.Enum(),
			UserNumber: proto.Int32(0),
		},
		Fragment:         proto.Int32(0),
		Version:          proto.Int32(3),
		UserSerialNumber: proto.Int32(0),
		Id:               &id,
		SecurityToken:    &opt.securityToken,
	}

	message, err := proto.Marshal(r)
	if err != nil {
		return nil, errors.Wrap(err, "marshal GCM checkin request")
	}

	res, err := c.post(ctx, checkinURL, bytes.NewReader(message), func(header *http.Header) {
		header.Set("Content-Type", "application/x-protobuf")
	})
	if err != nil {
		return nil, errors.Wrap(err, "request GCM checkin")
	}
	defer closeResponse(res)

	// unauthorized error
	if res.StatusCode == http.StatusUnauthorized {
		return nil, ErrGcmAuthorization
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		return nil, errors.Errorf("server error: %s", res.Status)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read GCM checkin response")
	}

	var responseProto pb.AndroidCheckinResponse
	err = proto.Unmarshal(data, &responseProto)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal GCM checkin response")
	}
	return &responseProto, nil
}

func (c *Client) doRegister(ctx context.Context, androidID uint64, securityToken uint64) (*gcmRegisterResponse, error) {
	values := url.Values{}
	values.Set("app", "org.chromium.linux")
	values.Set("X-subtype", c.appId)
	values.Set("device", fmt.Sprint(androidID))
	values.Set("sender", fcmServerKey)

	res, err := c.post(ctx, registerURL, strings.NewReader(values.Encode()), func(header *http.Header) {
		header.Set("Content-Type", "application/x-www-form-urlencoded")
		header.Set("Authorization", fmt.Sprintf("AidLogin %d:%d", androidID, securityToken))
	})
	if err != nil {
		return nil, errors.Wrap(err, "request GCM register")
	}
	defer closeResponse(res)

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read GCM register response")
	}

	subscription, err := url.ParseQuery(string(data))
	if err != nil {
		return nil, errors.Wrap(err, "parse GCM register URL")
	}
	token := subscription.Get("token")

	return &gcmRegisterResponse{
		token:         token,
		androidID:     androidID,
		securityToken: securityToken,
		appID:         c.appId,
	}, nil
}

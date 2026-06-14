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
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	pb "github.com/crow-misia/go-push-receiver/pb/checkin"
	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"
)

type checkInOption struct {
	androidID     int64
	securityToken uint64
}

type gcmRegisterResponse struct {
	token         string
	androidID     int64
	securityToken uint64
}

func (c *Client) registerGCM(ctx context.Context) (*gcmRegisterResponse, error) {
	checkInResp, err := c.checkIn(ctx, &checkInOption{})
	if err != nil {
		return nil, err
	}

	androidID := *checkInResp.AndroidId
	if androidID > math.MaxInt64 {
		return nil, fmt.Errorf("invalid Android ID %d", androidID)
	}
	return c.doRegister(ctx, int64(androidID), *checkInResp.SecurityToken)
}

func (c *Client) checkIn(ctx context.Context, opt *checkInOption) (resp *pb.AndroidCheckinResponse, err error) {
	id := opt.androidID
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
	defer closeResponse(c.logger, res)

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

func (c *Client) doRegister(ctx context.Context, androidID int64, securityToken uint64) (resp *gcmRegisterResponse, err error) {
	device := strconv.FormatInt(androidID, 10)

	values := url.Values{}
	values.Set("app", "org.chromium.linux")
	values.Set("X-subtype", c.appID)
	values.Set("device", device)
	values.Set("sender", c.vapidKey)

	res, err := c.post(ctx, registerURL, strings.NewReader(values.Encode()), func(header *http.Header) {
		header.Set("Content-Type", "application/x-www-form-urlencoded")
		header.Set("Authorization", fmt.Sprintf("AidLogin %s:%s", device, strconv.FormatUint(securityToken, 10)))
		header.Set("User-Agent", "")
	})
	if err != nil {
		return nil, errors.Wrap(err, "request GCM register")
	}
	defer closeResponse(c.logger, res)

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
	}, nil
}

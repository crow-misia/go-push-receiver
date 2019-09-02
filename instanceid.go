package pushreceiver

import (
	"bytes"
	"context"
	"fmt"
	"github.com/crow-misia/go-push-receiver/internal"
	pb "github.com/crow-misia/go-push-receiver/pb/checkin"
	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"
	"github.com/pkg/errors"
	"io/ioutil"
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

func newCheckinOption(androidID uint64, securityToken uint64) *checkInOption {
	return &checkInOption{
		androidID:     androidID,
		securityToken: securityToken,
	}
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
				ChromeVersion: proto.String(internal.ChromeVersion),
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

	req, _ := http.NewRequest("POST", internal.CheckinUrl, bytes.NewReader(message))
	req.Header.Set("Content-Type", "application/x-protobuf")

	response, err := c.httpClient.Do(req.WithContext(ctx))
	if response == nil || err != nil {
		return nil, errors.Wrap(err, "request GCM checkin")
	}

	// unauthorized error
	if response.StatusCode == http.StatusUnauthorized {
		return nil, ErrGcmAuthorization
	}

	data, err := ioutil.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read GCM checkin response")
	}

	responseProto := pb.AndroidCheckinResponse{}
	err = proto.Unmarshal(data, &responseProto)
	if err != nil {
		return nil, errors.Wrapf(err, "unmarshal GCM checkin response")
	}
	return &responseProto, nil
}

func (c *Client) doRegister(ctx context.Context, androidID uint64, securityToken uint64) (*gcmRegisterResponse, error) {
	appID := fmt.Sprintf("wp:receiver.push.com#%s", uuid.New())

	values := url.Values{}
	values.Set("app", "org.chromium.linux")
	values.Set("X-subtype", appID)
	values.Set("device", fmt.Sprint(androidID))
	values.Set("sender", internal.FcmServerKey)

	httpClient := c.httpClient

	req, _ := http.NewRequest("POST", internal.RegisterUrl, strings.NewReader(values.Encode()))
	req.Header.Set("Authorization", fmt.Sprintf("AidLogin %d:%d", androidID, securityToken))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := httpClient.Do(req.WithContext(ctx))
	if err != nil {
		return nil, errors.Wrap(err, "request GCM register")
	}

	data, err := ioutil.ReadAll(response.Body)
	_ = response.Body.Close()
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
		appID:         appID,
	}, nil
}

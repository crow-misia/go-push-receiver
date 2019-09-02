package pushreceiver

import (
	"context"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/crow-misia/go-push-receiver/internal"
	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	ece "github.com/crow-misia/http-ece"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"io/ioutil"
	"math"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var cryptoCurve = elliptic.P256()

// FcmCredentials is Credentials for FCM
type FcmCredentials struct {
	AppID         string `json:"appId"`
	AndroidID     uint64 `json:"androidId"`
	SecurityToken uint64 `json:"securityToken"`
	Token         string `json:"token"`
	PrivateKey    []byte `json:"privateKey"`
	PublicKey     []byte `json:"publicKey"`
	AuthSecret    []byte `json:"authSecret"`
}

type fcmRegisterResponse struct {
	Token   string `json:"token"`
	PushSet string `json:"pushSet"`
}

type fcmKeys struct {
	privateKey []byte
	publicKey  []byte
	authSecret []byte
}

// Client is FCM Push receive client.
type Client struct {
	httpClient            *http.Client
	senderID              string
	creds                 *FcmCredentials
	onUpdateCreds         func(*FcmCredentials)
	onMessage             func(string, []byte)
	onError               func(error, time.Duration)
	dialer                *net.Dialer
	minRetryBackoff       time.Duration
	maxRetryBackoff       time.Duration
	retry                 int
	receivedPersistentIds []string
}

// NewClient creates FCM push receive client.
func NewClient(senderID string, config *Config, opts ...Option) *Client {
	config.init()

	c := &Client{
		senderID:              senderID,
		minRetryBackoff:       config.MinRetryBackoff,
		maxRetryBackoff:       config.MaxRetryBackoff,
		retry:                 0,
		receivedPersistentIds: make([]string, 0),
		httpClient:            http.DefaultClient,
		dialer: &net.Dialer{
			Timeout:   config.Timeout,
			KeepAlive: config.Timeout,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// SetOnUpdateCreds is FCM Credentials update callback setter.
func (c *Client) SetOnUpdateCreds(callback func(*FcmCredentials)) {
	c.onUpdateCreds = callback
}

// SetOnError is FCM error callback setter.
func (c *Client) SetOnError(callback func(error, time.Duration)) {
	c.onError = callback
}

// SetOnMessage is FCM data receive callback setter.
func (c *Client) SetOnMessage(callback func(string, []byte)) {
	c.onMessage = callback
}

// Connect connect to FCM.
func (c *Client) Connect(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		var err error
		if c.creds == nil {
			err = c.register(ctx)
		} else {
			options := newCheckinOption(c.creds.AndroidID, c.creds.SecurityToken)
			_, err = c.checkIn(ctx, options)
		}
		if err == nil {
			// reset retry count when connection success
			c.retry = 0

			err = c.tryToConnect(ctx)
		}
		if err != nil {
			if err == ErrGcmAuthorization {
				c.creds = nil
			}

			// retry
			if c.retry >= math.MaxInt16 {
				c.retry = math.MaxInt16
			} else {
				c.retry++
			}
			sleepDuration := internal.RetryBackoff(c.retry, c.minRetryBackoff, c.maxRetryBackoff)

			// notify error
			c.onError(err, sleepDuration)

			time.Sleep(sleepDuration)
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
	c.onUpdateCreds(creds)
	return nil
}

func (c *Client) tryToConnect(ctx context.Context) error {
	conn, err := internal.DialContextWithDialer(
		ctx,
		c.dialer,
		"tcp",
		internal.MtalkServer,
		&tls.Config{
			InsecureSkipVerify:       false,
			PreferServerCipherSuites: true,
			MinVersion:               tls.VersionTLS12,
		})
	if err != nil {
		return err
	}
	defer conn.Close()

	data, err := c.createLoginBuffer()
	if err != nil {
		return err
	}
	_, err = conn.Write(data)
	if err != nil {
		return err
	}
	return c.performRead(conn)
}

func (c *Client) performRead(conn *tls.Conn) error {
	parser := internal.NewParser(conn)

	// receive version
	err := parser.ReceiveVersion()
	if err != nil {
		return err
	}

	for {
		// receive tag
		tagType, tag, err := parser.PerformReadTag()
		if err == nil {
			err = c.onDataMessage(tagType, tag)
		}
		if err != nil {
			return err
		}
	}
}

func (c *Client) onDataMessage(tagType internal.TagType, tag interface{}) error {
	if tagType == internal.TagDataMessageStanza {
		persistentID, plaintext, err := decryptData(tag.(*pb.DataMessageStanza), c.creds.PrivateKey, c.creds.AuthSecret)
		if err != nil {
			return err
		}
		if c.onMessage != nil {
			c.onMessage(*persistentID, plaintext)
		}
		c.receivedPersistentIds = append(c.receivedPersistentIds, *persistentID)
	}
	return nil
}

func (c *Client) createLoginBuffer() ([]byte, error) {
	androidID := proto.String(strconv.FormatUint(c.creds.AndroidID, 10))
	setting := []*pb.Setting{
		{
			Name:  proto.String("new_vc"),
			Value: proto.String("1"),
		},
	}

	authToken := strconv.FormatUint(c.creds.SecurityToken, 10)

	request := &pb.LoginRequest{
		AccountId:            proto.Int64(1000000),
		AuthService:          pb.LoginRequest_ANDROID_ID.Enum(),
		AuthToken:            proto.String(authToken),
		Id:                   proto.String(fmt.Sprintf("chrome-%s", internal.ChromeVersion)),
		Domain:               proto.String(internal.McsDomain),
		DeviceId:             proto.String(fmt.Sprintf("android-%s", strconv.FormatUint(c.creds.AndroidID, 16))),
		NetworkType:          proto.Int32(1), // Wi-Fi
		Resource:             androidID,
		User:                 androidID,
		UseRmq2:              proto.Bool(true),
		LastRmqId:            proto.Int64(1), // Sending not enabled yet so this stays as 1.
		Setting:              setting,
		ReceivedPersistentId: c.receivedPersistentIds,
	}

	buffer := proto.NewBuffer([]byte{internal.FcmVersion, byte(internal.TagLoginRequest)})
	err := buffer.EncodeMessage(request)
	if err != nil {
		return nil, errors.Wrap(err, "encode protocol buffer data")
	}
	return buffer.Bytes(), nil
}

func (c *Client) subscribeFCM(ctx context.Context, registerResponse *gcmRegisterResponse) (*FcmCredentials, error) {
	keys, err := createKeys()
	if err != nil {
		return nil, err
	}

	base64Encoding := base64.RawURLEncoding

	values := url.Values{}
	values.Set("authorized_entity", c.senderID)
	values.Set("endpoint", internal.FcmEndpoint+registerResponse.token)
	values.Set("encryption_key", base64Encoding.EncodeToString(keys.publicKey))
	values.Set("encryption_auth", base64Encoding.EncodeToString(keys.authSecret))

	req, _ := http.NewRequest("POST", internal.FcmSubscribe, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.httpClient.Do(req.WithContext(ctx))
	if response == nil || err != nil {
		return nil, errors.Wrap(err, "request FCM subscribe")
	}

	data, err := ioutil.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read FCM subscribe response")
	}

	fcmRegisterResponse := fcmRegisterResponse{}
	err = json.Unmarshal(data, &fcmRegisterResponse)
	if err != nil {
		return nil, errors.Wrap(err, "unmarshal FCM subscribe response")
	}

	return &FcmCredentials{
		AppID:         registerResponse.appID,
		AndroidID:     registerResponse.androidID,
		SecurityToken: registerResponse.securityToken,
		Token:         fcmRegisterResponse.Token,
		PrivateKey:    keys.privateKey,
		PublicKey:     keys.publicKey,
		AuthSecret:    keys.authSecret,
	}, nil
}

func createKeys() (*fcmKeys, error) {
	privateKey, publicKey, err := randomKey(cryptoCurve)
	if err != nil {
		return nil, errors.Wrap(err, "generate random key for FCM")
	}

	authSecret, err := randomSalt()
	if err != nil {
		return nil, errors.Wrap(err, "generate random salt for FCM")
	}

	return &fcmKeys{
		privateKey,
		publicKey,
		authSecret,
	}, nil
}

func decryptData(tag *pb.DataMessageStanza, privateKey []byte, authSecret []byte) (*string, []byte, error) {
	cryptoKeyData := findByKey(tag.AppData, "crypto-key")
	if cryptoKeyData == nil {
		return nil, nil, errors.New("dh is not provided")
	}

	cryptoKey, err := base64.URLEncoding.DecodeString(cryptoKeyData.GetValue()[3:])
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode decrypt data")
	}

	saltData := findByKey(tag.AppData, "encryption")
	if saltData == nil {
		return nil, nil, errors.New("salt is not provided")
	}
	salt, err := base64.URLEncoding.DecodeString(saltData.GetValue()[5:])
	if err != nil {
		return nil, nil, errors.Wrap(err, "decode salt")
	}

	plaintext, err := ece.Decrypt(tag.RawData,
		ece.WithEncoding(ece.AESGCM),
		ece.WithPrivate(privateKey),
		ece.WithDh(cryptoKey),
		ece.WithSalt(salt),
		ece.WithAuthSecret(authSecret),
	)
	return tag.PersistentId, plaintext, errors.Wrap(err, "decrypt HTTP-ECE data")
}

func findByKey(data []*pb.AppData, key string) *pb.AppData {
	for _, data := range data {
		if *data.Key == key {
			return data
		}
	}
	return nil
}

func randomKey(curve elliptic.Curve) (private []byte, public []byte, err error) {
	var x, y *big.Int
	private, x, y, err = elliptic.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	public = elliptic.Marshal(curve, x, y)
	return
}

func randomSalt() ([]byte, error) {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}

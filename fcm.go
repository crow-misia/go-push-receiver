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

// FcmClient is FCM Push receiver
type FcmClient struct {
	httpClient            *internal.HTTPClient
	senderID              string
	creds                 *FcmCredentials
	onUpdateCreds         func(*FcmCredentials)
	onMessage             func(string, []byte)
	onError               func(error)
	dialer                *net.Dialer
	tlsConfig             *tls.Config
	minRetryBackoff       time.Duration
	maxRetryBackoff       time.Duration
	retry                 int
	receivedPersistentIds []string
}

// NewFcmClient creates FCMClient.
func NewFcmClient(senderID string, config *Config, opts ...Option) *FcmClient {
	config.init()

	tlsConfig := tls.Config{
		InsecureSkipVerify:       false,
		PreferServerCipherSuites: true,
		MinVersion:               tls.VersionTLS12,
	}

	client := &FcmClient{
		senderID:              senderID,
		dialer:                config.Dialer,
		httpClient:            &internal.HTTPClient{Client: config.HTTPClient},
		tlsConfig:             &tlsConfig,
		minRetryBackoff:       config.MinRetryBackoff,
		maxRetryBackoff:       config.MaxRetryBackoff,
		retry:                 0,
		receivedPersistentIds: make([]string, 0),
	}

	for _, opt := range opts {
		opt.Apply(client)
	}

	return client
}

// SetOnUpdateCreds is FCM Credentials update callback setter.
func (c *FcmClient) SetOnUpdateCreds(callback func(*FcmCredentials)) {
	c.onUpdateCreds = callback
}

// SetOnError is FCM error callback setter.
func (c *FcmClient) SetOnError(callback func(error)) {
	c.onError = callback
}

// SetOnMessage is FCM data receive callback setter.
func (c *FcmClient) SetOnMessage(callback func(string, []byte)) {
	c.onMessage = callback
}

// Connect connect to FCM.
func (c *FcmClient) Connect(ctx context.Context) {
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
			if err == nil {
				err = c.tryToConnect(ctx)
			}
		}
		if err == nil {
			// reset retry count when connection success
			c.retry = 0
		} else {
			if err == ErrGcmAuthorization {
				c.creds = nil
			}

			// notify error
			c.onError(err)

			// retry
			if c.retry >= math.MaxInt16 {
				c.retry = math.MaxInt16
			} else {
				c.retry++
			}
			sleepDuration := internal.RetryBackoff(c.retry, c.minRetryBackoff, c.maxRetryBackoff)

			time.Sleep(sleepDuration)
		}
	}
}

func (c *FcmClient) register(ctx context.Context) error {
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

func (c *FcmClient) tryToConnect(ctx context.Context) error {
	conn, err := internal.DialContextWithDialer(
		ctx,
		c.dialer,
		"tcp",
		internal.MtalkServer,
		c.tlsConfig)
	if err != nil {
		return err
	}
	defer conn.Close()

	c.retry = 0

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

func (c *FcmClient) performRead(conn *tls.Conn) error {
	parser := internal.NewParser(conn)

	// receive version
	err := parser.ReceiveVersion()
	if err != nil {
		return err
	}

	for {
		// receive tag
		tagType, tag, err := parser.PerformReadTag()
		if err != nil {
			return err
		}

		c.onDataMessage(tagType, tag)
	}
}

func (c *FcmClient) onDataMessage(tagType internal.TagType, tag interface{}) {
	if tagType == internal.TagDataMessageStanza {
		persistentID, plaintext, err := decryptData(tag.(*pb.DataMessageStanza), c.creds.PrivateKey, c.creds.AuthSecret)
		if err != nil {
			c.onError(err)
		} else if c.onMessage != nil {
			c.onMessage(*persistentID, plaintext)
		}
	}
}

func (c *FcmClient) createLoginBuffer() ([]byte, error) {
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

func (c *FcmClient) subscribeFCM(ctx context.Context, registerResponse *gcmRegisterResponse) (*FcmCredentials, error) {
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

	response, err := c.httpClient.Do(ctx, req)
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

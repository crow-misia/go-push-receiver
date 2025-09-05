package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/SherClockHolmes/webpush-go"
)

var log = slog.Default()

type Config struct {
	Subscriber      string `json:"subscriber"`
	VAPIDPublicKey  string `json:"publicKey"`
	VAPIDPrivateKey string `json:"privateKey"`
}

type SubscriberCredentials struct {
	Endpoint   string `json:"endpoint"`
	PublicKey  string `json:"publicKey"`
	AuthSecret string `json:"authSecret"`
}

func main() {
	var (
		ttl            int
		configFilename string
		credsFilename  string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.IntVar(&ttl, "ttl", 86400, "Message TTL. zero or negative is disable")
	flag.StringVar(&credsFilename, "credentials", "", "subscriber's credentials filename")
	flag.StringVar(&configFilename, "config", "config.json", "vapid config filename")
	flag.Parse()
	if len(configFilename) == 0 && len(credsFilename) == 0 {
		flag.PrintDefaults()
		return
	}

	realMain(context.Background(), credsFilename, configFilename, ttl)
}

func realMain(ctx context.Context, credsFilename, configFilename string, ttl int) {
	config, err := loadConfig(configFilename)
	if err != nil {
		log.Error("failed load config", "message", err)
		os.Exit(-1)
	}

	creds, err := loadCredentials(credsFilename)
	if err != nil {
		log.Error("failed load credentials", "message", err)
		os.Exit(-1)
	}

	// Decode subscription
	s := &webpush.Subscription{
		Endpoint: creds.Endpoint,
		Keys: webpush.Keys{
			Auth:   creds.AuthSecret,
			P256dh: creds.PublicKey,
		},
	}

	message := &map[string]interface{}{
		"notification": &map[string]string{
			"title": "Hello world",
			"body":  fmt.Sprintf("Test: %s", time.Now()),
		},
	}
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Error("marshal message error:", "err", err)
		os.Exit(-1)
	}

	// Send Notification
	resp, err := webpush.SendNotificationWithContext(ctx, messageBytes, s, &webpush.Options{
		Subscriber:      config.Subscriber,
		VAPIDPublicKey:  config.VAPIDPublicKey,
		VAPIDPrivateKey: config.VAPIDPrivateKey,
		TTL:             ttl,
	})
	if err != nil {
		log.Error("error: %v", err)
		os.Exit(-1)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("error:", "err", err)
		os.Exit(-1)
	}
	log.Info("response", "body", string(body))
	defer resp.Body.Close()
}

func isExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func loadConfig(filename string) (*Config, error) {
	if !isExist(filename) {
		return nil, errors.New("config file not found")
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	config := &Config{}
	err = json.NewDecoder(f).Decode(config)
	return config, err
}

func loadCredentials(filename string) (*SubscriberCredentials, error) {
	if !isExist(filename) {
		return nil, errors.New("credentials file not found")
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	creds := &SubscriberCredentials{}
	err = json.NewDecoder(f).Decode(creds)
	return creds, err
}

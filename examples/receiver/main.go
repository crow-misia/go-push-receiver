package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"log/slog"
	"os"
	"reflect"
	"time"

	pr "github.com/crow-misia/go-push-receiver"
)

func main() {
	var (
		configFilename       string
		credsFilename        string
		persistentIDFilename string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.StringVar(&configFilename, "config", "config.json", "FCM's Config filename (needed)")
	flag.StringVar(&credsFilename, "credentials", "credentials.json", "Credentials filename")
	flag.StringVar(&persistentIDFilename, "persistent-id", "persistent_id.txt", "PersistentID filename")
	flag.Parse()

	if len(configFilename) == 0 || len(credsFilename) == 0 {
		flag.PrintDefaults()
		return
	}

	ctx := context.Background()
	realMain(ctx, configFilename, credsFilename, persistentIDFilename)
}

func realMain(ctx context.Context, configFilename, credsFilename, persistentIDFilename string) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

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

	// load received persistent id list
	persistentIDList, err := loadPersistentIDList(persistentIDFilename)
	if err != nil {
		log.Error("failed load persistentID list", "message", err)
		os.Exit(-1)
	}

	fcmClient := pr.New(config,
		pr.WithCreds(creds),
		pr.WithHeartbeat(
			pr.WithServerInterval(1*time.Minute),
			pr.WithClientInterval(2*time.Minute),
			pr.WithAdaptive(true),
		),
		pr.WithLogger(log),
		pr.WithReceivedPersistentID(persistentIDList),
	)

	go fcmClient.Subscribe(ctx)

	for event := range fcmClient.Events {
		switch ev := event.(type) {
		case *pr.UpdateCredentialsEvent:
			log.Info("Registration Token:", "token", ev.Credentials.Token)
			if err := saveCredentials(credsFilename, ev.Credentials); err != nil {
				log.Error("failed save credentials", "message", err)
				os.Exit(-1)
			}
		case *pr.ConnectedEvent:
			if err := clearPersistentID(persistentIDFilename); err != nil {
				log.Error("failed clear credentials", "message", err)
				os.Exit(-1)
			}
		case *pr.UnauthorizedError:
			log.Warn("UnauthorizedError", "message", err)
		case *pr.HeartbeatError:
			log.Warn("HeartbeatError", "message", err)
		case *pr.MessageEvent:
			log.Info("Received message:", "data", string(ev.Data), "persistentID", ev.PersistentID)

			// save persistentID
			if err := savePersistentID(persistentIDFilename, ev.PersistentID); err != nil {
				log.Error("failed save persistentID", "message", err)
				os.Exit(-1)
			}
		case *pr.RetryEvent:
			log.Warn("retry:", "error", ev.ErrorObj, "retryAfter", ev.RetryAfter)
		default:
			data, _ := json.Marshal(ev)
			log.Info("Event:", "type", reflect.TypeOf(ev), "data", data)
		}
	}
}

func isExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func loadConfig(filename string) (*pr.Config, error) {
	if !isExist(filename) {
		return nil, errors.New("config file not found")
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	config := &pr.Config{}
	err = json.NewDecoder(f).Decode(config)
	return config, err
}

func loadCredentials(filename string) (*pr.FCMCredentials, error) {
	if !isExist(filename) {
		return nil, nil
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	creds := &pr.FCMCredentials{}
	err = json.NewDecoder(f).Decode(creds)
	return creds, err
}

func saveCredentials(filename string, credentials *pr.FCMCredentials) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	return encoder.Encode(credentials)
}

func loadPersistentIDList(filename string) ([]string, error) {
	persistentIDList := make([]string, 0, 100)

	if !isExist(filename) {
		return persistentIDList, nil
	}

	f, err := os.Open(filename)
	if err != nil {
		return persistentIDList, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		persistentIDList = append(persistentIDList, scanner.Text())
	}
	return persistentIDList, nil
}

func savePersistentID(filename, persistentID string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(fmt.Sprintln(persistentID))
	return err
}

func clearPersistentID(filename string) error {
	if isExist(filename) {
		return os.Remove(filename)
	}
	return nil
}

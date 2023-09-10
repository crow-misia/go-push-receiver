package main

import (
	"context"
	"encoding/json"
	"flag"
	pr "github.com/crow-misia/go-push-receiver"
	"log"
	"os"
	"reflect"
	"time"
)

func main() {
	var (
		senderId      string
		credsFilename string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.StringVar(&senderId, "sender-id", "", "FCM's sender ID (needed)")
	flag.StringVar(&credsFilename, "credentials", "credentials.json", "Credentials filename (needed)")
	flag.Parse()

	if len(senderId) == 0 || len(credsFilename) == 0 {
		flag.PrintDefaults()
		return
	}

	ctx := context.Background()
	realMain(ctx, senderId, credsFilename)
}

func realMain(ctx context.Context, senderId string, credsFilename string) {
	var creds *pr.FCMCredentials

	logger := log.New(os.Stderr, "app : ", log.Lshortfile|log.Ldate|log.Ltime)

	if isExist(credsFilename) {
		f, err := os.Open(credsFilename)
		if err != nil {
			logger.Fatal(err)
		}
		creds = &pr.FCMCredentials{}
		decoder := json.NewDecoder(f)
		err = decoder.Decode(creds)
		_ = f.Close()
		if err != nil {
			logger.Fatal(err)
		}
	}

	// set received persistent ids
	var persistentIDs []string
	// persistentIDs = []string{"0:xxxxxxxxxxxxxxxxxxxxxxxxxx"}

	fcmClient := pr.New(senderId,
		pr.WithCreds(creds),
		pr.WithHeartbeat(
			pr.WithServerInterval(30*time.Second),
			pr.WithClientInterval(1*time.Minute),
			pr.WithAdaptive(true),
		),
		pr.WithLogger(log.New(os.Stderr, "push: ", log.Lshortfile|log.Ldate|log.Ltime)),
		pr.WithReceivedPersistentID(persistentIDs),
	)

	go fcmClient.Subscribe(ctx)

	for event := range fcmClient.Events {
		switch ev := event.(type) {
		case *pr.UpdateCredentialsEvent:
			logger.Print(ev)
			f, err := os.OpenFile(credsFilename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				logger.Fatal(err)
			}
			encoder := json.NewEncoder(f)
			err = encoder.Encode(ev.Credentials)
			_ = f.Close()
			if err != nil {
				logger.Fatal(err)
			}
			logger.Printf("Registration Token: %s", ev.Credentials.Token)
		case *pr.UnauthorizedError:
			logger.Printf("error: %v", ev.ErrorObj)
		case *pr.HeartbeatError:
			logger.Printf("error: %v", ev.ErrorObj)
		case *pr.MessageEvent:
			logger.Printf("Received message: %s, %s", string(ev.Data), ev.PersistentID)
		case *pr.RetryEvent:
			logger.Printf("retry : %v, %s", ev.ErrorObj, ev.RetryAfter)
		default:
			data, _ := json.Marshal(ev)
			logger.Printf("Event: %s (%s)", reflect.TypeOf(ev), data)
		}
	}
}

func isExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	pr "github.com/crow-misia/go-push-receiver"
	"log"
	"os"
	"reflect"
	"time"
)

func main() {
	var (
		apiKey               string
		appId                string
		projectId            string
		credsFilename        string
		persistentIdFilename string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.StringVar(&apiKey, "api-key", "", "FCM's API key (required)")
	flag.StringVar(&appId, "app-id", "", "FCM's App ID (required)")
	flag.StringVar(&projectId, "project-id", "", "FCM's Project ID (required)")
	flag.StringVar(&credsFilename, "credentials", "credentials.json", "Credentials filename")
	flag.StringVar(&persistentIdFilename, "persistent-id", "persistent_id.txt", "PersistentID filename")
	flag.Parse()

	if len(apiKey) == 0 || len(appId) == 0 || len(projectId) == 0 || len(credsFilename) == 0 {
		flag.PrintDefaults()
		return
	}

	ctx := context.Background()
	realMain(ctx, apiKey, appId, projectId, credsFilename, persistentIdFilename)
}

func realMain(ctx context.Context, apiKey, appId, projectId, credsFilename, persistentIdFilename string) {
	var creds *pr.FCMCredentials

	logger := log.New(os.Stderr, "app : ", log.Lshortfile|log.Ldate|log.Ltime)

	creds, err := loadCredentials(credsFilename)
	if err != nil {
		logger.Fatal(err)
	}

	// load received persistent ids
	persistentIDs, err := loadPersistentIDs(persistentIdFilename)
	if err != nil {
		logger.Fatal(err)
	}

	fcmClient := pr.New(apiKey, appId, projectId,
		pr.WithCreds(creds),
		pr.WithHeartbeat(
			pr.WithServerInterval(1*time.Minute),
			pr.WithClientInterval(2*time.Minute),
			pr.WithAdaptive(true),
		),
		pr.WithLogger(log.New(os.Stderr, "push: ", log.Lshortfile|log.Ldate|log.Ltime)),
		pr.WithReceivedPersistentID(persistentIDs),
	)

	go fcmClient.Subscribe(ctx)

	for event := range fcmClient.Events {
		switch ev := event.(type) {
		case *pr.UpdateCredentialsEvent:
			logger.Printf("Registration Token: %s", ev.Credentials.Token)
			if err := saveCredentials(credsFilename, ev.Credentials); err != nil {
				logger.Fatal(err)
			}
		case *pr.ConnectedEvent:
			if err := clearPersistentID(persistentIdFilename); err != nil {
				logger.Fatal(err)
			}
		case *pr.UnauthorizedError:
			logger.Printf("error: %v", ev.ErrorObj)
		case *pr.HeartbeatError:
			logger.Printf("error: %v", ev.ErrorObj)
		case *pr.MessageEvent:
			logger.Printf("Received message: %s, %s", string(ev.Data), ev.PersistentID)

			// save persistentID
			if err := savePersistentID(persistentIdFilename, ev.PersistentID); err != nil {
				logger.Fatal(err)
			}
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

func loadCredentials(filename string) (*pr.FCMCredentials, error) {
	if !isExist(filename) {
		return nil, nil
	}

	f, err := os.Open(filename)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return nil, err
	}
	creds := &pr.FCMCredentials{}
	decoder := json.NewDecoder(f)
	err = decoder.Decode(creds)
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
	return encoder.Encode(credentials)
}

func loadPersistentIDs(filename string) ([]string, error) {
	var persistentIDs []string

	if !isExist(filename) {
		return persistentIDs, nil
	}

	f, err := os.Open(filename)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return persistentIDs, err
	}
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		persistentIDs = append(persistentIDs, scanner.Text())
	}
	return persistentIDs, nil
}

func savePersistentID(filename, persistentID string) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if f != nil {
		defer f.Close()
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(fmt.Sprintln(persistentID))
	return err
}

func clearPersistentID(filename string) error {
	if isExist(filename) {
		return os.Remove(filename)
	}
	return nil
}

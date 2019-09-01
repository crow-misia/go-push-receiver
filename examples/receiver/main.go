package main

import (
	"context"
	"encoding/json"
	"flag"
	pr "github.com/crow-misia/go-push-receiver"
	"io/ioutil"
	"log"
	"os"
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
	opts := make([]pr.Option, 0)
	if isExist(credsFilename) {
		creds := &pr.FcmCredentials{}
		data, err := ioutil.ReadFile(credsFilename)
		if err == nil {
			err = json.Unmarshal(data, creds)
		}
		if err != nil {
			log.Fatal(err)
		}

		opts = append(opts, pr.WithCreds(creds))
	}
	// set received persistent ids
	// opts = append(opts, pr.WithReceivedPersistentIds([]string{"0:xxxxxxxxxxxxxxxxxxxxxxxxxx"}))

	config := pr.Config{}
	fcmClient := pr.NewFcmClient(senderId, &config, opts...)

	fcmClient.SetOnUpdateCreds(func(creds *pr.FcmCredentials) {
		data, err := json.Marshal(creds)
		if err == nil {
			err = ioutil.WriteFile(credsFilename, data, 0600)
		}
		if err != nil {
			log.Fatal(err)
		}
	})

	fcmClient.SetOnError(func(err error) {
		log.Println(err)
	})
	fcmClient.SetOnMessage(func(persistentId string, data []byte) {
		log.Printf("%s : %s", persistentId, string(data))
	})

	go fcmClient.Connect(ctx)

	for {
		time.Sleep(10 * time.Second)
	}
}

func isExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

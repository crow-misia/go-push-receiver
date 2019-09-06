package main

import (
	"context"
	"firebase.google.com/go"
	"firebase.google.com/go/messaging"
	"flag"
	"fmt"
	"google.golang.org/api/option"
	"log"
	"time"
)

func main() {
	var (
		ttl                 int
		credentialsFilename string
		registrationToken   string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.IntVar(&ttl, "ttl", 86400, "Message TTL. zero or negative is disable")
	flag.StringVar(&credentialsFilename, "credentials", "serviceAccountKey.json", "FCM's credentials filename")
	flag.StringVar(&registrationToken, "token", "", "registration token (needed)")
	flag.Parse()

	if len(registrationToken) == 0 {
		flag.PrintDefaults()
		return
	}

	realMain(context.Background(), credentialsFilename, ttl, registrationToken)
}

func realMain(ctx context.Context, credentialsFilename string, ttl int, registrationToken string) {
	opt := option.WithCredentialsFile(credentialsFilename)
	config := firebase.Config{}
	app, err := firebase.NewApp(ctx, &config, opt)
	if err != nil {
		log.Fatalf("error new application: %v", err)
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		log.Fatalf("error getting Messaging client: %v", err)
	}

	headers := map[string]string{}
	if ttl > 0 {
		headers["ttl"] = fmt.Sprint(ttl)
	}

	// See documentation on defining a message payload.
	message := &messaging.Message{
		Webpush: &messaging.WebpushConfig{
			Headers: headers,
		},
		Notification: &messaging.Notification{
			Title: "Hello world",
			Body:  fmt.Sprintf("Test: %s", time.Now()),
		},
		Token: registrationToken,
	}

	// Send a message to the device corresponding to the provided
	// registration token.
	response, err := client.Send(ctx, message)
	if err != nil {
		log.Fatalf("fcm send error: %v", err)
	}

	// Response is a message ID string.
	log.Printf("Successfully sent message: %s", response)
}

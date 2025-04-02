package main

import (
	"context"
	"flag"
	"log"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

func main() {
	var (
		unsubscribe         bool
		credentialsFilename string
		registrationToken   string
		topic               string
	)
	flag.NewFlagSet("help", flag.ExitOnError)
	flag.BoolVar(&unsubscribe, "unsubscribe", false, "topic unsubscribe")
	flag.StringVar(&credentialsFilename, "credentials", "serviceAccountKey.json_push", "FCM's credentials filename")
	flag.StringVar(&registrationToken, "token", "", "registration token")
	flag.StringVar(&topic, "topic", "", "topic name")
	flag.Parse()

	if len(registrationToken) == 0 || len(topic) == 0 {
		flag.PrintDefaults()
		return
	}

	realMain(context.Background(), credentialsFilename, unsubscribe, registrationToken, topic)
}

func realMain(ctx context.Context, credentialsFilename string, unsubscribe bool, registrationToken, topic string) {
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

	var response *messaging.TopicManagementResponse
	if unsubscribe {
		response, err = client.UnsubscribeFromTopic(ctx, []string{registrationToken}, topic)
	} else {
		response, err = client.SubscribeToTopic(ctx, []string{registrationToken}, topic)
	}
	if err != nil {
		log.Fatalf("fcm error: %v", err)
	}

	log.Printf("Successfully: %d, %d", response.SuccessCount, response.FailureCount)
}

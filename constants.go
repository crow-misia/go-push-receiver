/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

// GCM / FCM constants.
const (
	registerURL             = "https://android.clients.google.com/c2dm/register3"
	checkinURL              = "https://android.clients.google.com/checkin"
	fcmLegacyEndpoint       = "https://fcm.googleapis.com/fcm/send/%s"
	fcmV1Endpoint           = "https://fcm.googleapis.com/v1/projects/%s/messages:send"
	firebaseInstallationURL = "https://firebaseinstallations.googleapis.com/v1/"
	firebaseRegistrationURL = "https://fcmregistrations.googleapis.com/v1/"

	fcmServerKey = "BDOU99-h67HcA6JeFXHbSNMu7e2yNNu3RzoMj8TM4W88jITfq7ZmPvIM1Iv-4_l2LxQcYwhqby2xGpWwzjfAnG4"

	mtalkServer   = "mtalk.google.com:5228"
	mcsDomain     = "mcs.android.com"
	chromeVersion = "63.0.3234.0"
	fcmVersion    = 41
	sdkVersion    = "w:0.6.17"
	authVersion   = "FIS_v2"

	// Packet defines

	// of bytes a MCS version packet consumes.
	versionPacketLen = 1
	// of bytes a tag packet consumes.
	tagPacketLen     = 1
	sizePacketLenMin = 1
	sizePacketLenMax = 5
)

// Default values
const (
	// default Dial timeout second
	defaultDialTimeout = 30

	// default keep-alive duration (minutes)
	defaultKeepAlive = 1

	// Default Base backoff second
	defaultBackoffBase = 5

	// Default Max backoff second
	defaultBackoffMax = 15 * 60

	// Default Heartbeat period (minutes)
	defaultHeartbeatPeriod = 10
)

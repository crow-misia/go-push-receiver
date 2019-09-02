package internal

//go:generate stringer -output=constants_string.go -type=TagType --trimprefix=Tag

// TagType is FCM Request/Response Tag type
type TagType byte

// GCM / FCM constants.
const (
	RegisterUrl   = "https://android.clients.google.com/c2dm/register3"
	CheckinUrl    = "https://android.clients.google.com/checkin"
	ChromeVersion = "63.0.3234.0"
	FcmServerKey  = "BDOU99-h67HcA6JeFXHbSNMu7e2yNNu3RzoMj8TM4W88jITfq7ZmPvIM1Iv-4_l2LxQcYwhqby2xGpWwzjfAnG4"

	FcmSubscribe = "https://fcm.googleapis.com/fcm/connect/subscribe"
	FcmEndpoint  = "https://fcm.googleapis.com/fcm/send/"
	MtalkServer  = "mtalk.google.com:5228"
	McsDomain    = "mcs.android.com"
	FcmVersion   = 41

	// Packet defines

	// of bytes a MCS version packet consumes.
	VersionPacketLen = 1
	// of bytes a tag packet consumes.
	TagPacketLen     = 1
	SizePacketLenMin = 1
	SizePacketLenMax = 5

	// Default values

	// Dial timeout second
	DialTimeout = 30

	// Network reads/writes timeout second
	Timeout = 30

	// Min backoff second
	MinRetryBackoff = 5

	// Max backoff second
	MaxRetryBackoff = 15 * 60
)

// Tag enumeration.
const (
	TagHeartbeatPing       TagType = 0
	TagHeartbeatAck        TagType = 1
	TagLoginRequest        TagType = 2
	TagLoginResponse       TagType = 3
	TagClose               TagType = 4
	TagMessageStanza       TagType = 5
	TagPresenceStanza      TagType = 6
	TagIqStanza            TagType = 7
	TagDataMessageStanza   TagType = 8
	TagBatchPresenceStanza TagType = 9
	TagStreamErrorStanza   TagType = 10
	TagHTTPRequest         TagType = 11
	TagHTTPResponse        TagType = 12
	TagBindAccountRequest  TagType = 13
	TagBindAccountResponse TagType = 14
	TagTalkMetadata        TagType = 15
	TagNumProtoTypes       TagType = 16
)

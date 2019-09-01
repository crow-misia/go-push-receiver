package pushreceiver

// Option is an option for FCM client.
type Option interface {
	Apply(*FcmClient)
}

type withCreds struct{ creds *FcmCredentials }

func (c withCreds) Apply(client *FcmClient) {
	client.creds = c.creds
}

// WithCreds is credentials setter
func WithCreds(creds *FcmCredentials) Option {
	return withCreds{creds}
}

type withReceivedPersistentIds []string

func (c withReceivedPersistentIds) Apply(client *FcmClient) {
	client.receivedPersistentIds = c
}

// WithReceivedPersistentId is received persistentId list setter
func WithReceivedPersistentIds(ids []string) Option {
	return withReceivedPersistentIds(ids)
}

package pushreceiver

import "github.com/pkg/errors"

// ErrGcmAuthorization is Authorization error of GCM.
var ErrGcmAuthorization = errors.New("GCM authorization error")

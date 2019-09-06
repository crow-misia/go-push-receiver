/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import "github.com/pkg/errors"

// ErrGcmAuthorization is authorization error of GCM.
var ErrGcmAuthorization = errors.New("GCM authorization error")

// ErrFcmNotEnoughData is error that data is not enough data from FCM.
var ErrFcmNotEnoughData = errors.New("data Enough from FCM")

// ErrNotFoundInAppData is error that key not found in app data.
var ErrNotFoundInAppData = errors.New("key not found")

/*
 * Copyright (c) 2025 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

// Config type
type Config struct {
	ApiKey    string `json:"apiKey"`
	ProjectID string `json:"projectId"`
	AppID     string `json:"appId"`
	VapidKey  string `json:"VapidKey,omitempty"`
}

/*
 * Copyright (c) 2019 Zenichi Amano
 *
 * This file is part of go-push-receiver, which is MIT licensed.
 * See http://opensource.org/licenses/MIT
 */

package pushreceiver

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	pb "github.com/crow-misia/go-push-receiver/pb/mcs"
	ece "github.com/crow-misia/http-ece"
	"github.com/pkg/errors"
)

var cryptoCurve = ecdh.P256()

// appendCryptoInfo appends key for crypto to Credentials.
func (c *FCMCredentials) appendCryptoInfo() error {
	privateKey, publicKey, err := generateKey(cryptoCurve)
	if err != nil {
		return errors.Wrap(err, "generate random key for FCM")
	}

	authSecret, err := generateSalt()
	if err != nil {
		return errors.Wrap(err, "generate random salt for FCM")
	}

	c.PrivateKey = privateKey
	c.PublicKey = publicKey
	c.AuthSecret = authSecret

	return nil
}

func decryptData(data *pb.DataMessageStanza, privateKey []byte, authSecret []byte) (*MessageEvent, error) {
	cryptoKeyData, err := findByKey(data.GetAppData(), "crypto-key")
	if err != nil {
		return nil, errors.Wrap(err, "dh is not provided")
	}

	cryptoKey, err := base64.URLEncoding.DecodeString(cryptoKeyData.GetValue()[3:])
	if err != nil {
		return nil, errors.Wrap(err, "decode decrypt data")
	}

	saltData, err := findByKey(data.GetAppData(), "encryption")
	if err != nil {
		return nil, errors.Wrap(err, "salt is not provided")
	}
	salt, err := base64.URLEncoding.DecodeString(saltData.GetValue()[5:])
	if err != nil {
		return nil, errors.Wrap(err, "decode salt")
	}

	bytes, err := ece.Decrypt(data.GetRawData(),
		ece.WithEncoding(ece.AESGCM),
		ece.WithPrivate(privateKey),
		ece.WithDh(cryptoKey),
		ece.WithSalt(salt),
		ece.WithAuthSecret(authSecret),
	)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt HTTP-ECE data")
	}
	return newMessageEvent(data, bytes), nil
}

// generateKey generates for public key crypto.
func generateKey(curve ecdh.Curve) (private []byte, public []byte, err error) {
	var privateKey *ecdh.PrivateKey
	var publicKey *ecdh.PublicKey
	if privateKey, err = curve.GenerateKey(rand.Reader); err != nil {
		return nil, nil, err
	}
	publicKey = privateKey.PublicKey()
	return privateKey.Bytes(), publicKey.Bytes(), nil
}

// generateSalt generates salt.
func generateSalt() ([]byte, error) {
	salt := make([]byte, 16)
	_, err := rand.Read(salt)
	return salt, err
}

func findByKey(data []*pb.AppData, key string) (*pb.AppData, error) {
	for _, data := range data {
		if *data.Key == key {
			return data, nil
		}
	}
	return nil, ErrNotFoundInAppData
}

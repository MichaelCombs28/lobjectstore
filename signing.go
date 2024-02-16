package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type signedURL struct {
	Path   string
	Expiry time.Time
}

func toURL(secret []byte, s *signedURL) []byte {
	payload, _ := json.Marshal(s)
	return append([]byte("/pre-signed/"), sign(secret, payload)...)
}

func sign(secret, payload []byte) []byte {
	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := mac.Sum(nil)
	combined := append(signature[:], payload[:]...)
	dst := make([]byte, base64.URLEncoding.EncodedLen(len(combined)))
	base64.URLEncoding.Encode(dst, combined)
	return dst
}

func verify(secret, body []byte) (*signedURL, error) {
	dst := make([]byte, base64.URLEncoding.DecodedLen(len(body)))
	if _, err := base64.URLEncoding.Decode(dst, body); err != nil {
		return nil, err
	}
	signature := dst[:32]
	payload := dst[32:]
	payload = bytes.TrimRight(payload, "\x00")
	if len(payload) <= 1 {
		return nil, errors.New("Invalid signature")
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	expectedHMAC := mac.Sum(nil)
	if !hmac.Equal(signature, expectedHMAC) {
		return nil, errors.New("Invalid signature")
	}

	var url signedURL
	err := json.Unmarshal(payload, &url)
	return &url, err
}

func generateRandomUUID() string {
	b := make([]byte, 16)
	// Assume you'll never fail to read here, this is a toy
	io.ReadFull(rand.Reader, b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

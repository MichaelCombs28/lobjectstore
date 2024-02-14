package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestSign(t *testing.T) {
	url := signedURL{
		Path:   "/affffa",
		Expiry: time.Now().Add(time.Second * 10),
	}

	secret := []byte("DEADBEEFCAFE")
	payload, _ := json.Marshal(url)
	b := sign(secret, payload)
	if _, err := verify(secret, b); err != nil {
		t.Error(err)
	}
	t.Error(generateRandomUUID())
}

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSign(t *testing.T) {
	url := signedURL{
		Path:   "/affffa",
		Expiry: time.Now().Add(time.Second * 10),
	}

	secret := []byte("DEADBEEFCAFE")
	payload, _ := json.Marshal(url)
	b := sign(secret, payload)
	_, err := verify(secret, b)
	require.NoError(t, err)
}

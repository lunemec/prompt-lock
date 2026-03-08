package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const secureTokenBytes = 32

func newSecureToken(prefix string) (string, error) {
	b := make([]byte, secureTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%s", prefix, hex.EncodeToString(b)), nil
}

func mustSecureToken(prefix string) string {
	tok, err := newSecureToken(prefix)
	if err != nil {
		panic(err)
	}
	return tok
}

package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const (
	KeyPrefix = "chime_"
	keyBytes  = 16
)

func GenerateKey() (string, error) {
	b := make([]byte, keyBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}
	return KeyPrefix + hex.EncodeToString(b), nil
}

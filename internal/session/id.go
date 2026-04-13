package session

import (
	"crypto/rand"
	"encoding/hex"
)

func newID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sess_" + hex.EncodeToString(buf), nil
}

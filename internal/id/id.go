package id

import (
	"crypto/rand"
	"encoding/hex"
)

func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "job-fallback-id"
	}
	return hex.EncodeToString(b[:])
}

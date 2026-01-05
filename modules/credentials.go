package modules

import (
	"crypto/rand"
	"encoding/hex"

	"github.com/google/uuid"
)

func GenerateCredentials(protocol string) (string, string) {
	uuidValue := uuid.New().String()
	password := randomHex(16)

	switch protocol {
	case "trojan", "hysteria":
		return uuidValue, password
	default:
		return uuidValue, password
	}
}

func randomHex(length int) string {
	buf := make([]byte, length)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

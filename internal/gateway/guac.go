// Package gateway implements the KubeTRE access gateway broker. It signs users in with
// OIDC, works out which remote sessions each user may open, and hands Guacamole a
// short-lived, single-use, signed and encrypted description of exactly those sessions
// (Guacamole's guacamole-auth-json extension).
package gateway

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

// GuacConnection is one connection in a guacamole-auth-json payload.
type GuacConnection struct {
	Protocol   string            `json:"protocol"`
	Parameters map[string]string `json:"parameters"`
}

// GuacPayload is the user data guacamole-auth-json accepts.
type GuacPayload struct {
	Username    string                    `json:"username"`
	Expires     int64                     `json:"expires"` // milliseconds since the Unix epoch
	SingleUse   bool                      `json:"singleUse"`
	Connections map[string]GuacConnection `json:"connections"`
}

// GuacKey is the 128-bit key shared with Guacamole's json-secret-key property.
type GuacKey [16]byte

// ParseGuacKey parses the 32 hexadecimal characters Guacamole expects.
func ParseGuacKey(s string) (GuacKey, error) {
	var k GuacKey
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != len(k) {
		return k, errors.New("guacamole JSON secret key must be 32 hexadecimal characters")
	}
	copy(k[:], b)
	return k, nil
}

// NewGuacPayload builds a single-use payload that expires after ttl.
func NewGuacPayload(username string, conns map[string]GuacConnection, now time.Time, ttl time.Duration) GuacPayload {
	return GuacPayload{Username: username, Expires: now.Add(ttl).UnixMilli(), SingleUse: true, Connections: conns}
}

// Seal produces the base64 value of Guacamole's "data" parameter. It mirrors
// guacamole-auth-json's CryptoService: HMAC-SHA256 over the JSON, prepended to it, then
// AES-128-CBC with a zero IV and PKCS#5 padding, all keyed with the shared secret.
func (k GuacKey) Seal(p GuacPayload) (string, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, k[:])
	mac.Write(body)
	plain := append(mac.Sum(nil), body...)

	pad := aes.BlockSize - len(plain)%aes.BlockSize
	plain = append(plain, bytes.Repeat([]byte{byte(pad)}, pad)...)

	block, err := aes.NewCipher(k[:])
	if err != nil {
		return "", err
	}
	out := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, aes.BlockSize)).CryptBlocks(out, plain)
	return base64.StdEncoding.EncodeToString(out), nil
}

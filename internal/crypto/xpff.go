// Package crypto implements XPFF AES-256-GCM header generation.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// XPFFGenerator builds the XPFF (x-xp-forwarded-for) header.
type XPFFGenerator struct {
	BaseKey string // hex string, used as UTF-8 bytes (NOT decoded from hex)
}

func New(baseKey string) *XPFFGenerator { return &XPFFGenerator{BaseKey: baseKey} }

// deriveKey: SHA256(base_key_str + guest_id) → 32 bytes.
// IMPORTANT: base_key is concatenated as a UTF-8 string, not decoded from hex.
func (g *XPFFGenerator) deriveKey(guestID string) []byte {
	sum := sha256.Sum256([]byte(g.BaseKey + guestID))
	return sum[:]
}

// Generate encrypts plaintext under a freshly random 12-byte nonce.
// Returns hex(nonce[12] || ciphertext || tag[16]).
func (g *XPFFGenerator) Generate(plaintext, guestID string) (string, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("nonce: %w", err)
	}
	return g.GenerateWithNonce(plaintext, guestID, nonce[:])
}

func (g *XPFFGenerator) GenerateWithNonce(plaintext, guestID string, nonce []byte) (string, error) {
	if len(nonce) != 12 {
		return "", fmt.Errorf("nonce must be 12 bytes, got %d", len(nonce))
	}
	key := g.deriveKey(guestID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	// gcm.Seal appends ciphertext||tag to dst. We allocate dst = nonce so
	// the layout is nonce || ct || tag without an extra copy.
	out := make([]byte, 12, 12+len(plaintext)+16)
	copy(out, nonce)
	out = gcm.Seal(out, nonce, []byte(plaintext), nil)
	return hex.EncodeToString(out), nil
}

func (g *XPFFGenerator) Decode(hexString, guestID string) (string, error) {
	raw, err := hex.DecodeString(hexString)
	if err != nil {
		return "", err
	}
	if len(raw) < 12+16 {
		return "", errors.New("xpff payload too short")
	}
	nonce, ctTag := raw[:12], raw[12:]
	key := g.deriveKey(guestID)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	pt, err := gcm.Open(nil, nonce, ctTag, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}

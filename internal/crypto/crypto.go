// Package crypto encrypts settings secrets at rest with AES-256-GCM.
//
// Stored values use the envelope "enc:" + base64(nonce || ciphertext), so a
// value's encryption status is visible without decrypting it.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Prefix marks a stored value as encrypted.
const Prefix = "enc:"

// KeySize is the required key length in bytes (AES-256).
const KeySize = 32

// ErrNotEncrypted is returned by Decrypt for values without Prefix.
var ErrNotEncrypted = errors.New("value is not encrypted")

// Cipher encrypts and decrypts secret settings values.
type Cipher struct {
	aead cipher.AEAD
}

// New builds a Cipher from a base64-encoded 32-byte key.
func New(keyB64 string) (*Cipher, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(keyB64))
	if err != nil {
		return nil, fmt.Errorf("decoding encryption key: %w", err)
	}
	if len(key) != KeySize {
		return nil, fmt.Errorf("encryption key is %d bytes, want %d", len(key), KeySize)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating aes cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating gcm: %w", err)
	}
	return &Cipher{aead: aead}, nil
}

// GenerateKey returns a fresh base64-encoded 32-byte key.
func GenerateKey() (string, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// IsEncrypted reports whether value carries the encryption envelope.
func IsEncrypted(value string) bool {
	return strings.HasPrefix(value, Prefix)
}

// Encrypt seals plaintext and returns the "enc:"-prefixed envelope.
func (c *Cipher) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("reading nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return Prefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Decrypt opens an "enc:"-prefixed envelope.
func (c *Cipher) Decrypt(value string) (string, error) {
	if !IsEncrypted(value) {
		return "", ErrNotEncrypted
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, Prefix))
	if err != nil {
		return "", fmt.Errorf("decoding ciphertext: %w", err)
	}
	nonceSize := c.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext shorter than nonce")
	}
	nonce, ct := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := c.aead.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("opening ciphertext: %w", err)
	}
	return string(plaintext), nil
}

// DecryptIfEncrypted decrypts value when it carries the envelope and returns it
// unchanged otherwise. Used when reading settings written before encryption was
// enabled, or plaintext defaults.
func (c *Cipher) DecryptIfEncrypted(value string) (string, error) {
	if !IsEncrypted(value) {
		return value, nil
	}
	return c.Decrypt(value)
}

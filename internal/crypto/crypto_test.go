package crypto_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
)

func testKey(t *testing.T) string {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestRoundTrip(t *testing.T) {
	c, err := crypto.New(testKey(t))
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	const secret = "s3cr3t-token-שלום"
	enc, err := c.Encrypt(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, crypto.Prefix) {
		t.Fatalf("ciphertext %q lacks %q prefix", enc, crypto.Prefix)
	}
	if strings.Contains(enc, secret) {
		t.Fatal("plaintext leaked into ciphertext")
	}
	got, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if got != secret {
		t.Errorf("decrypt = %q, want %q", got, secret)
	}
}

func TestNonceIsRandomPerCall(t *testing.T) {
	c, _ := crypto.New(testKey(t))
	a, _ := c.Encrypt("same")
	b, _ := c.Encrypt("same")
	if a == b {
		t.Fatal("two encryptions of the same plaintext produced identical output")
	}
}

func TestDecryptRejectsTamper(t *testing.T) {
	c, _ := crypto.New(testKey(t))
	enc, _ := c.Encrypt("hello")
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, crypto.Prefix))
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 0xff
	tampered := crypto.Prefix + base64.StdEncoding.EncodeToString(raw)
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext decrypted successfully")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	a, _ := crypto.New(testKey(t))
	b, _ := crypto.New(testKey(t))
	enc, _ := a.Encrypt("hello")
	if _, err := b.Decrypt(enc); err == nil {
		t.Fatal("decrypted with the wrong key")
	}
}

func TestNewRejectsBadKeys(t *testing.T) {
	if _, err := crypto.New(""); err == nil {
		t.Error("empty key accepted")
	}
	if _, err := crypto.New("not-base64!!"); err == nil {
		t.Error("non-base64 key accepted")
	}
	short := base64.StdEncoding.EncodeToString(make([]byte, 16))
	if _, err := crypto.New(short); err == nil {
		t.Error("16-byte key accepted; want 32-byte requirement")
	}
}

func TestIsEncryptedAndPassthrough(t *testing.T) {
	c, _ := crypto.New(testKey(t))
	if crypto.IsEncrypted("plain") {
		t.Error("plain value reported as encrypted")
	}
	got, err := c.DecryptIfEncrypted("plain")
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if got != "plain" {
		t.Errorf("passthrough = %q, want plain", got)
	}
}

func TestDecryptRequiresPrefix(t *testing.T) {
	c, _ := crypto.New(testKey(t))
	enc, _ := c.Encrypt("hello")
	if _, err := c.Decrypt(strings.TrimPrefix(enc, crypto.Prefix)); err == nil {
		t.Fatal("decrypted a value without the enc: prefix")
	}
}

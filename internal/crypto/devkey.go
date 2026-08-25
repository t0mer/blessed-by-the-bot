package crypto

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DevKeyFile is the filename used by LoadOrCreateDevKey inside the data dir.
const DevKeyFile = "dev-encryption.key"

// LoadOrCreateDevKey returns a persistent key for --dev runs, generating one on
// first use and storing it mode 0600 in the data directory.
//
// It exists so development settings survive a restart without the process
// shipping a hardcoded key. Never call this outside development: production
// requires BBTB_ENCRYPTION_KEY to be supplied by the operator.
func LoadOrCreateDevKey(dataDir string) (string, error) {
	path := filepath.Join(dataDir, DevKeyFile)

	body, err := os.ReadFile(path) //nolint:gosec // path derives from operator-supplied config
	if err == nil {
		key := strings.TrimSpace(string(body))
		if _, vErr := New(key); vErr != nil {
			return "", fmt.Errorf("dev key at %s is invalid: %w", path, vErr)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("reading dev key %s: %w", path, err)
	}

	key, err := GenerateKey()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		return "", fmt.Errorf("creating data dir %s: %w", dataDir, err)
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("writing dev key %s: %w", path, err)
	}
	return key, nil
}

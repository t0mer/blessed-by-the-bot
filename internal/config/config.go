// Package config resolves runtime configuration from flags, environment
// variables and an optional YAML file, in that order of precedence.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// EnvPrefix is prepended to every environment variable this app reads.
const EnvPrefix = "BBTB"

// EncryptionKeyEnv is the only setting that is env-only: it is never a flag and
// never read from the YAML file.
const EncryptionKeyEnv = EnvPrefix + "_ENCRYPTION_KEY"

// ErrMissingKey is returned by Load when the encryption key is absent and the
// process was not started with --dev.
var ErrMissingKey = errors.New(EncryptionKeyEnv + " is required (or pass --dev)")

var validLogLevels = map[string]bool{"debug": true, "info": true, "warn": true, "error": true}

// Config holds infrastructure settings. Behavioural settings (provider, send
// times, thresholds) live in the database, not here.
type Config struct {
	ConfigFile    string
	Port          int
	DataDir       string
	LogLevel      string
	Dev           bool
	EncryptionKey string
}

// Bind registers this package's flags on fs. Call it before fs.Parse.
func Bind(fs *pflag.FlagSet) {
	fs.String("config", "", "path to YAML config file (default ./config.yaml if present)")
	fs.Int("port", 8080, "HTTP port")
	fs.String("data-dir", "./data", "directory for the SQLite database and state")
	fs.String("log-level", "info", "log level: debug|info|warn|error")
	fs.Bool("dev", false, "development mode: verbose logs, encryption key not required")
}

// Load resolves configuration. Precedence is flags > environment > YAML file >
// built-in defaults.
func Load(fs *pflag.FlagSet) (*Config, error) {
	v := viper.New()

	v.SetDefault("port", 8080)
	v.SetDefault("data-dir", "./data")
	v.SetDefault("log-level", "info")
	v.SetDefault("dev", false)

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if err := v.BindPFlags(fs); err != nil {
		return nil, fmt.Errorf("binding flags: %w", err)
	}

	// The config file path itself only comes from --config or BBTB_CONFIG.
	configFile := v.GetString("config")
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", configFile, err)
		}
	} else {
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			var notFound viper.ConfigFileNotFoundError
			if !errors.As(err, &notFound) {
				return nil, fmt.Errorf("reading config file: %w", err)
			}
		} else {
			configFile = v.ConfigFileUsed()
		}
	}

	cfg := &Config{
		ConfigFile:    configFile,
		Port:          v.GetInt("port"),
		DataDir:       v.GetString("data-dir"),
		LogLevel:      strings.ToLower(v.GetString("log-level")),
		Dev:           v.GetBool("dev"),
		EncryptionKey: strings.TrimSpace(os.Getenv(EncryptionKeyEnv)),
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port %d out of range 1-65535", c.Port)
	}
	if !validLogLevels[c.LogLevel] {
		return fmt.Errorf("invalid log level %q: want debug|info|warn|error", c.LogLevel)
	}
	if c.DataDir == "" {
		return errors.New("data-dir must not be empty")
	}
	if c.EncryptionKey == "" && !c.Dev {
		return ErrMissingKey
	}
	return nil
}

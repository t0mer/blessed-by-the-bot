// Command blessedbot runs the blessed-by-the-bot server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	// Embed the zoneinfo database: the scratch image ships no OS tzdata and the
	// scheduler needs named zones such as Asia/Jerusalem.
	_ "time/tzdata"

	"github.com/spf13/cobra"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/server"
)

// Injected at build time via -ldflags "-X main.version=...".
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "blessedbot",
		Short:         "Self-hosted WhatsApp blessing bot",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	}
	config.Bind(root.PersistentFlags())

	root.AddCommand(newVersionCmd(), newHealthcheckCmd(), newGenkeyCmd())
	return root
}

func run(cmd *cobra.Command) error {
	cfg, err := config.Load(cmd.Flags())
	if err != nil {
		return err
	}

	log := logging.New(cfg.LogLevel, cfg.Dev)
	log.Info("starting blessedbot",
		"version", version, "commit", commit, "built", date,
		"data_dir", cfg.DataDir, "port", cfg.Port, "dev", cfg.Dev,
	)
	if cfg.ConfigFile != "" {
		log.Info("loaded config file", "path", cfg.ConfigFile)
	}

	if cfg.EncryptionKey != "" {
		if _, err := crypto.New(cfg.EncryptionKey); err != nil {
			return fmt.Errorf("validating %s: %w", config.EncryptionKeyEnv, err)
		}
	} else {
		log.Warn("running without an encryption key; provider secrets will not be stored")
	}

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("creating data dir %s: %w", cfg.DataDir, err)
	}

	srv, err := server.New(server.Options{Config: cfg, Logger: log, Version: version})
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	return srv.Run(ctx)
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version and exit",
		Run: func(cmd *cobra.Command, _ []string) {
			// cobra's cmd.Print* family writes to stderr; these are outputs a
			// caller pipes or captures, so write to stdout explicitly.
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "blessedbot %s (commit %s, built %s)\n", version, commit, date)
		},
	}
}

func newHealthcheckCmd() *cobra.Command {
	var url string
	cmd := &cobra.Command{
		Use:   "healthcheck",
		Short: "Probe the /healthz endpoint and exit non-zero on failure",
		RunE: func(_ *cobra.Command, _ []string) error {
			return probe(url)
		},
	}
	cmd.Flags().StringVar(&url, "url", "http://127.0.0.1:8080/healthz", "health endpoint to probe")
	return cmd
}

func newGenkeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "genkey",
		Short: "Generate a base64 AES-256 key for " + config.EncryptionKeyEnv,
		RunE: func(cmd *cobra.Command, _ []string) error {
			key, err := crypto.GenerateKey()
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), key)
			return nil
		},
	}
}

// probe performs the healthcheck request used by the container healthcheck.
func probe(url string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("probing %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return errors.New("unhealthy: " + resp.Status)
	}
	return nil
}

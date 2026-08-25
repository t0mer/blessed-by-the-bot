// Command blessedbot runs the blessed-by-the-bot server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	// Embed the zoneinfo database: the scratch image ships no OS tzdata and the
	// scheduler needs named zones such as Asia/Jerusalem.
	_ "time/tzdata"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/t0mer/blessed-by-the-bot/internal/config"
	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/logging"
	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/factory"
	"github.com/t0mer/blessed-by-the-bot/internal/server"
	"github.com/t0mer/blessed-by-the-bot/internal/service/blessing"
	"github.com/t0mer/blessed-by-the-bot/internal/service/echo"
	"github.com/t0mer/blessed-by-the-bot/internal/service/scheduler"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
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

	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return fmt.Errorf("creating data dir %s: %w", cfg.DataDir, err)
	}

	// --dev waives the BBTB_ENCRYPTION_KEY requirement, but settings still have
	// to survive a restart, so fall back to a key persisted in the data dir.
	keyB64 := cfg.EncryptionKey
	if keyB64 == "" && cfg.Dev {
		keyB64, err = crypto.LoadOrCreateDevKey(cfg.DataDir)
		if err != nil {
			return err
		}
		log.Warn("dev mode: using a generated encryption key from the data dir",
			"file", filepath.Join(cfg.DataDir, crypto.DevKeyFile))
	}
	cipher, err := crypto.New(keyB64)
	if err != nil {
		return fmt.Errorf("validating %s: %w", config.EncryptionKeyEnv, err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbPath := filepath.Join(cfg.DataDir, "blessedbot.db")
	st, err := store.Open(ctx, dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := st.Close(); closeErr != nil {
			log.Error("closing database", "error", closeErr)
		}
	}()
	log.Info("database ready", "path", dbPath)

	settingsSvc, err := settings.New(st, cipher)
	if err != nil {
		return err
	}
	current, err := settingsSvc.Load(ctx)
	if err != nil {
		return fmt.Errorf("loading settings: %w", err)
	}
	log.Info("settings loaded",
		"provider", current.Provider,
		"timezone", current.Scheduler.Timezone,
		"send_time", current.Scheduler.SendTime,
	)

	// A fresh install has no credentials yet, and the user needs the UI up in
	// order to enter them — so a provider that will not build is a warning, not
	// a startup failure.
	transport := provider.NewTransport(provider.TransportOptions{Logger: log})
	providers := provider.NewManager()
	if err := factory.Rebuild(providers, current, transport, log); err != nil {
		log.Warn("whatsapp provider not ready; configure it in the settings UI", "error", err)
	} else {
		log.Info("whatsapp provider ready", "provider", providers.Name())
	}

	selector, err := blessing.NewSelector(st, log)
	if err != nil {
		return err
	}
	sched, err := scheduler.New(scheduler.Deps{
		Store:     st,
		Settings:  settingsSvc,
		Providers: providers,
		Blessings: selector,
		Logger:    log,
	})
	if err != nil {
		return err
	}

	echoEngine, err := echo.New(echo.Deps{
		Store:     st,
		Settings:  settingsSvc,
		Providers: providers,
		Blessings: selector,
		Logger:    log,
	})
	if err != nil {
		return err
	}

	// Rebuilding on a settings change is what lets the user switch providers or
	// fix a token from the UI without restarting the process (spec §4).
	rebuild := func(_ context.Context, s *settings.Settings) error {
		if err := factory.Rebuild(providers, s, transport, log); err != nil {
			return err
		}
		log.Info("whatsapp provider reconfigured", "provider", providers.Name())
		return nil
	}

	srv, err := server.New(server.Options{
		Config:    cfg,
		Logger:    log,
		Version:   version,
		Store:     st,
		Settings:  settingsSvc,
		Providers: providers,
		Rebuild:   rebuild,
		Sender:    sched,
		Incoming:  echoEngine,
	})
	if err != nil {
		return err
	}

	// The scheduler and the HTTP server share one context, so SIGTERM stops both
	// and Wait blocks until in-flight sends finish (spec §6).
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return srv.Run(groupCtx) })
	group.Go(func() error { return sched.Run(groupCtx) })
	group.Go(func() error { return echoEngine.RunJanitor(groupCtx) })
	return group.Wait()
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

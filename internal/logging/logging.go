// Package logging builds the application's slog logger: JSON in production,
// human-readable text in development.
package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel maps a level name to a slog.Level, defaulting to info.
func ParseLevel(name string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// New returns a logger writing to stdout.
func New(level string, dev bool) *slog.Logger {
	return NewTo(os.Stdout, level, dev)
}

// NewTo returns a logger writing to w. Exported for tests.
func NewTo(w io.Writer, level string, dev bool) *slog.Logger {
	opts := &slog.HandlerOptions{Level: ParseLevel(level)}
	if dev {
		opts.Level = slog.LevelDebug
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

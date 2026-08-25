package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/logging"
)

func TestNewJSONHandlerInProduction(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "info", false)
	log.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("production output is not JSON: %v (%q)", err, buf.String())
	}
	if rec["msg"] != "hello" || rec["k"] != "v" {
		t.Errorf("unexpected record: %v", rec)
	}
}

func TestNewTextHandlerInDev(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "info", true)
	log.Info("hello")
	if !strings.Contains(buf.String(), "msg=hello") {
		t.Errorf("dev output is not text: %q", buf.String())
	}
}

func TestLevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	log := logging.NewTo(&buf, "warn", false)
	log.Info("suppressed")
	if buf.Len() != 0 {
		t.Errorf("info record emitted at warn level: %q", buf.String())
	}
	log.Warn("kept")
	if !strings.Contains(buf.String(), "kept") {
		t.Errorf("warn record dropped: %q", buf.String())
	}
}

func TestUnknownLevelFallsBackToInfo(t *testing.T) {
	if got := logging.ParseLevel("loud"); got != slog.LevelInfo {
		t.Errorf("ParseLevel(loud) = %v, want info", got)
	}
	if got := logging.ParseLevel("DEBUG"); got != slog.LevelDebug {
		t.Errorf("ParseLevel(DEBUG) = %v, want debug", got)
	}
}

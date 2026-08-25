package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
)

func TestHealthcheckSucceedsOn200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer srv.Close()

	if err := probe(srv.URL + "/healthz"); err != nil {
		t.Fatalf("probe returned %v, want nil", err)
	}
}

func TestHealthcheckFailsOn500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := probe(srv.URL + "/healthz"); err == nil {
		t.Fatal("probe returned nil for a 500 response")
	}
}

func TestHealthcheckFailsWhenUnreachable(t *testing.T) {
	if err := probe("http://127.0.0.1:1/healthz"); err == nil {
		t.Fatal("probe returned nil for an unreachable server")
	}
}

// genkey and version are consumed via shell command substitution
// (BBTB_ENCRYPTION_KEY="$(blessedbot genkey)"), so their output must go to
// stdout. cobra's cmd.Print* helpers default to stderr.
func TestGenkeyWritesUsableKeyToStdout(t *testing.T) {
	var out, errOut bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"genkey"})
	if err := root.Execute(); err != nil {
		t.Fatalf("genkey: %v", err)
	}
	key := strings.TrimSpace(out.String())
	if key == "" {
		t.Fatalf("genkey wrote nothing to stdout (stderr had %q)", errOut.String())
	}
	if _, err := crypto.New(key); err != nil {
		t.Fatalf("genkey output is not a usable key: %v", err)
	}
}

func TestVersionWritesToStdout(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("version: %v", err)
	}
	if !strings.Contains(out.String(), "blessedbot") {
		t.Errorf("version stdout = %q", out.String())
	}
}

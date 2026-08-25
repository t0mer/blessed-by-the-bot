package gowa_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/gowa"
)

func newClient(t *testing.T, cfg gowa.Config, handler http.HandlerFunc) *gowa.Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	cfg.BaseURL = srv.URL
	c, err := gowa.New(cfg, provider.NewTransport(provider.TransportOptions{
		BaseBackoff: time.Millisecond,
	}), nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c
}

func TestNameIsGOWA(t *testing.T) {
	c := newClient(t, gowa.Config{}, func(http.ResponseWriter, *http.Request) {})
	if c.Name() != "gowa" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestSendTextPostsToSendMessage(t *testing.T) {
	var path, method string
	body := make(chan map[string]string, 1)

	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		path, method = r.URL.Path, r.Method
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body <- payload
		_, _ = w.Write([]byte(`{"code":"SUCCESS","results":{"message_id":"3EB0","status":"sent"}}`))
	})

	id, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "3EB0" {
		t.Errorf("message id = %q", id)
	}
	if path != "/send/message" || method != http.MethodPost {
		t.Errorf("%s %s, want POST /send/message", method, path)
	}
	got := <-body
	if got["phone"] != "972501234567@c.us" || got["message"] != "hi" {
		t.Errorf("body = %v", got)
	}
}

func TestSendTextSendsDeviceHeader(t *testing.T) {
	var device string
	c := newClient(t, gowa.Config{DeviceID: "device-7"}, func(w http.ResponseWriter, r *http.Request) {
		device = r.Header.Get("X-Device-Id")
		_, _ = w.Write([]byte(`{"results":{"message_id":"x"}}`))
	})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	// v8 scopes device calls by this header.
	if device != "device-7" {
		t.Errorf("X-Device-Id = %q, want device-7", device)
	}
}

func TestSendTextOmitsDeviceHeaderWhenUnset(t *testing.T) {
	present := true
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["X-Device-Id"]
		_, _ = w.Write([]byte(`{"results":{"message_id":"x"}}`))
	})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	// Absent, not empty: GOWA falls back to the single registered device only
	// when the header is missing entirely.
	if present {
		t.Error("X-Device-Id sent despite no device being configured")
	}
}

func TestSendTextUsesBasicAuth(t *testing.T) {
	var user, pass string
	var ok bool
	c := newClient(t, gowa.Config{Username: "admin", Password: "hunter2"},
		func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok = r.BasicAuth()
			_, _ = w.Write([]byte(`{"results":{"message_id":"x"}}`))
		})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !ok || user != "admin" || pass != "hunter2" {
		t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
	}
}

func TestSendTextOmitsBasicAuthWhenUnset(t *testing.T) {
	present := true
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		_, present = r.Header["Authorization"]
		_, _ = w.Write([]byte(`{"results":{"message_id":"x"}}`))
	})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if present {
		t.Error("Authorization header sent with no credentials configured")
	}
}

func TestSendTextNormalisesPhoneNumbers(t *testing.T) {
	body := make(chan map[string]string, 1)
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body <- payload
		_, _ = w.Write([]byte(`{"results":{"message_id":"x"}}`))
	})

	if _, err := c.SendText(context.Background(), "+972 50-123 4567", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := <-body; got["phone"] != "972501234567@c.us" {
		t.Errorf("phone = %q", got["phone"])
	}
}

func TestSendTextSucceedsWhenMessageIDFieldIsAbsent(t *testing.T) {
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":"SUCCESS","message":"sent"}`))
	})

	// A send that worked must not be reported as failed just because the id
	// field moved between GOWA versions.
	id, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "" {
		t.Errorf("message id = %q, want empty", id)
	}
}

func TestSendTextSurfacesErrorsWithoutRetrying(t *testing.T) {
	var calls int32
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err == nil {
		t.Fatal("expected an error for a 401")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d requests, want 1", n)
	}
}

func TestListGroupsParsesJIDAndName(t *testing.T) {
	var path string
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"code":"SUCCESS","results":{"data":[
			{"JID":"120363012345678901@g.us","Name":"Family"},
			{"JID":"120363099999999999@g.us","Name":"Work"}
		]}}`))
	})

	groups, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if path != "/user/my/groups" {
		t.Errorf("path = %q, want /user/my/groups", path)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if groups[0].ChatID != "120363012345678901@g.us" || groups[0].Name != "Family" {
		t.Errorf("first group = %+v", groups[0])
	}
}

func TestListGroupsEmptyIsNonNil(t *testing.T) {
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"data":[]}}`))
	})
	groups, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if groups == nil {
		t.Error("empty result is nil; JSON encodes nil as null, want []")
	}
}

func TestStatusConnected(t *testing.T) {
	var path string
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = w.Write([]byte(`{"code":"SUCCESS","results":{"is_connected":true,"is_logged_in":true,"device_id":"d1"}}`))
	})

	got, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if path != "/app/status" {
		t.Errorf("path = %q, want /app/status", path)
	}
	if !got.Connected || got.NeedsQR {
		t.Errorf("status = %+v, want connected and no QR", got)
	}
	if got.Provider != "gowa" {
		t.Errorf("provider = %q", got.Provider)
	}
}

func TestStatusNeedsQR(t *testing.T) {
	c := newClient(t, gowa.Config{}, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":{"is_connected":true,"is_logged_in":false}}`))
	})

	got, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if got.Connected {
		t.Error("connected = true while logged out")
	}
	if !got.NeedsQR {
		t.Error("needsQR = false while logged out")
	}
	// v1 does not proxy the QR flow, so the user has to be pointed at GOWA's UI.
	if got.Detail == "" {
		t.Error("no detail explaining where to scan the QR code")
	}
}

func TestNewRejectsMissingBaseURL(t *testing.T) {
	tr := provider.NewTransport(provider.TransportOptions{})
	if _, err := gowa.New(gowa.Config{}, tr, nil); err == nil {
		t.Fatal("missing base url accepted")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	tr := provider.NewTransport(provider.TransportOptions{})
	c, err := gowa.New(gowa.Config{BaseURL: "http://gowa:3000/"}, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.BaseURL(); got != "http://gowa:3000" {
		t.Errorf("base url = %q, want the trailing slash trimmed", got)
	}
}

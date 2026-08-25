package greenapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/provider/greenapi"
)

const (
	testInstance = "7103123456"
	testToken    = "test-token"
)

func newClient(t *testing.T, handler http.HandlerFunc) (*greenapi.Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := greenapi.New(greenapi.Config{
		APIURL:     srv.URL,
		IDInstance: testInstance,
		APIToken:   testToken,
	}, provider.NewTransport(provider.TransportOptions{BaseBackoff: time.Millisecond}), nil)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return c, srv
}

func TestNameIsGreenAPI(t *testing.T) {
	c, _ := newClient(t, func(http.ResponseWriter, *http.Request) {})
	if c.Name() != "greenapi" {
		t.Errorf("Name() = %q", c.Name())
	}
}

func TestSendTextBuildsTheDocumentedURL(t *testing.T) {
	var gotPath, gotMethod string
	body := make(chan map[string]string, 1)

	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body <- payload
		_, _ = w.Write([]byte(`{"idMessage":"BAE5F4886F6F2D05"}`))
	})

	id, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if id != "BAE5F4886F6F2D05" {
		t.Errorf("message id = %q", id)
	}
	if want := "/waInstance" + testInstance + "/sendMessage/" + testToken; gotPath != want {
		t.Errorf("path = %q, want %q", gotPath, want)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	got := <-body
	if got["chatId"] != "972501234567@c.us" || got["message"] != "hi" {
		t.Errorf("body = %v", got)
	}
}

func TestSendTextNormalisesBarePhoneNumbers(t *testing.T) {
	body := make(chan map[string]string, 1)
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body <- payload
		_, _ = w.Write([]byte(`{"idMessage":"x"}`))
	})

	if _, err := c.SendText(context.Background(), "+972 50-123 4567", "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := <-body; got["chatId"] != "972501234567@c.us" {
		t.Errorf("chatId = %q, want 972501234567@c.us", got["chatId"])
	}
}

func TestSendTextPassesGroupJIDsThrough(t *testing.T) {
	body := make(chan map[string]string, 1)
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		body <- payload
		_, _ = w.Write([]byte(`{"idMessage":"x"}`))
	})

	const jid = "120363012345678901@g.us"
	if _, err := c.SendText(context.Background(), jid, "hi"); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := <-body; got["chatId"] != jid {
		t.Errorf("chatId = %q, want it unchanged", got["chatId"])
	}
}

func TestSendTextSurfacesAPIErrorsWithoutRetrying(t *testing.T) {
	var calls int32
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	if _, err := c.SendText(context.Background(), "972501234567@c.us", "hi"); err == nil {
		t.Fatal("expected an error for a 401")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Errorf("%d requests, want 1 — a rejected credential must not be retried", n)
	}
}

func TestSendTextErrorDoesNotLeakTheToken(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})

	_, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
	if err == nil {
		t.Fatal("expected an error")
	}
	// GreenAPI puts the token in the URL path, so an error carrying the full URL
	// would leak the credential into logs.
	if strings.Contains(err.Error(), testToken) {
		t.Errorf("error message leaks the API token: %v", err)
	}
}

func TestListGroupsReturnsOnlyGroups(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "getContacts") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`[
			{"id":"972501234567@c.us","name":"Dana","type":"user"},
			{"id":"120363012345678901@g.us","name":"Family","type":"group"},
			{"id":"120363099999999999@g.us","name":"Work","type":"group"}
		]`))
	})

	groups, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2: %+v", len(groups), groups)
	}
	if groups[0].ChatID != "120363012345678901@g.us" || groups[0].Name != "Family" {
		t.Errorf("first group = %+v", groups[0])
	}
}

func TestListGroupsEmptyIsNonNil(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	groups, err := c.ListGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if groups == nil {
		t.Error("empty result is nil; JSON encodes nil as null, want []")
	}
}

func TestStatusStates(t *testing.T) {
	cases := []struct {
		state     string
		connected bool
		needsQR   bool
	}{
		{"authorized", true, false},
		{"notAuthorized", false, true},
		{"blocked", false, false},
		{"sleepMode", false, false},
		{"starting", false, false},
	}
	for _, tc := range cases {
		state := tc.state
		c, _ := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"stateInstance":"` + state + `"}`))
		})
		got, err := c.Status(context.Background())
		if err != nil {
			t.Fatalf("%s: status: %v", state, err)
		}
		if got.Connected != tc.connected {
			t.Errorf("%s: connected = %v, want %v", state, got.Connected, tc.connected)
		}
		if got.NeedsQR != tc.needsQR {
			t.Errorf("%s: needsQR = %v, want %v", state, got.NeedsQR, tc.needsQR)
		}
		if got.State != state {
			t.Errorf("%s: raw state = %q, want it preserved", state, got.State)
		}
		if got.Provider != "greenapi" {
			t.Errorf("%s: provider = %q", state, got.Provider)
		}
	}
}

func TestNewRejectsMissingCredentials(t *testing.T) {
	tr := provider.NewTransport(provider.TransportOptions{})
	if _, err := greenapi.New(greenapi.Config{APIToken: "t"}, tr, nil); err == nil {
		t.Error("missing instance id accepted")
	}
	if _, err := greenapi.New(greenapi.Config{IDInstance: "1"}, tr, nil); err == nil {
		t.Error("missing token accepted")
	}
}

func TestNewDefaultsAPIURLAndTrimsSlash(t *testing.T) {
	tr := provider.NewTransport(provider.TransportOptions{})

	c, err := greenapi.New(greenapi.Config{IDInstance: "1", APIToken: "t"}, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.APIURL(); got != "https://api.green-api.com" {
		t.Errorf("default api url = %q", got)
	}

	c, err = greenapi.New(greenapi.Config{
		APIURL: "https://7103.api.greenapi.com/", IDInstance: "1", APIToken: "t",
	}, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := c.APIURL(); got != "https://7103.api.greenapi.com" {
		t.Errorf("api url = %q, want the trailing slash trimmed", got)
	}
}

func TestErrorDoesNotLeakTokenEchoedInResponseBody(t *testing.T) {
	// A provider is free to quote the request URL back in an error body, and the
	// GreenAPI token is a path segment of that URL.
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized for ` + r.URL.Path + `"}`))
	})

	for name, call := range map[string]func() error{
		"SendText": func() error {
			_, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
			return err
		},
		"ListGroups": func() error {
			_, err := c.ListGroups(context.Background())
			return err
		},
		"Status": func() error {
			_, err := c.Status(context.Background())
			return err
		},
	} {
		err := call()
		if err == nil {
			t.Errorf("%s: expected an error", name)
			continue
		}
		if strings.Contains(err.Error(), testToken) {
			t.Errorf("%s: error leaks the API token: %v", name, err)
		}
		if !strings.Contains(err.Error(), "***") {
			t.Errorf("%s: token was not redacted: %v", name, err)
		}
	}
}

func TestRedactedErrorStaysInspectable(t *testing.T) {
	c, _ := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(r.URL.Path))
	})

	_, err := c.SendText(context.Background(), "972501234567@c.us", "hi")
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("redaction broke the error chain: %v", err)
	}
	if apiErr.Status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", apiErr.Status)
	}
}

func TestCheckAuthHeaderPackageFunction(t *testing.T) {
	if err := greenapi.CheckAuthHeader("", "anything"); err != nil {
		t.Fatalf("an unset token must accept any header, got %v", err)
	}
	if err := greenapi.CheckAuthHeader("Bearer s3cret", "  Bearer s3cret  "); err != nil {
		t.Fatalf("surrounding whitespace must be tolerated, got %v", err)
	}
	if err := greenapi.CheckAuthHeader("Bearer s3cret", "Bearer wrong"); err == nil {
		t.Fatal("want a mismatch to be rejected")
	}
}

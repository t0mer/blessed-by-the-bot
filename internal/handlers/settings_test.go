package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

// configuredSettings is a full valid payload with both providers filled in.
func configuredSettings() map[string]any {
	return map[string]any{
		"provider": "greenapi",
		"greenapi": map[string]any{
			"api_url":             "https://7103.api.greenapi.com",
			"id_instance":         "7103123456",
			"api_token":           "super-secret-token",
			"mode":                "polling",
			"webhook_auth_header": "hook-secret",
		},
		"gowa": map[string]any{
			"base_url":       "http://gowa:3000",
			"username":       "admin",
			"password":       "gowa-password",
			"device_id":      "device-1",
			"webhook_secret": "gowa-hmac",
		},
		"scheduler": map[string]any{
			"timezone":  "Asia/Jerusalem",
			"send_time": "09:00",
		},
		"group_echo": map[string]any{
			"threshold":      3,
			"window_hours":   6,
			"cooldown_hours": 20,
		},
	}
}

func TestGetSettingsReturnsDefaultsOnFreshInstall(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/settings", nil)
	requireStatus(t, rec, http.StatusOK)

	var got settings.Settings
	decodeInto(t, rec, &got)
	if got.Provider != settings.ProviderGreenAPI {
		t.Errorf("provider = %q, want the default %q", got.Provider, settings.ProviderGreenAPI)
	}
	if got.Scheduler.Timezone != "Asia/Jerusalem" || got.Scheduler.SendTime != "09:00" {
		t.Errorf("unexpected scheduler defaults: %#v", got.Scheduler)
	}
	if got.GroupEcho.Threshold != 3 {
		t.Errorf("threshold = %d, want the default 3", got.GroupEcho.Threshold)
	}
	// Nothing is configured yet, so secrets must be empty rather than masked —
	// the UI uses that to tell "not set" from "set but hidden".
	if got.GreenAPI.APIToken != "" || got.GOWA.Password != "" {
		t.Errorf("want empty secrets on a fresh install, got %#v", got)
	}
}

func TestSettingsNeverReturnSecretsInPlaintext(t *testing.T) {
	ta := newTestAPI(t)

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	rec := ta.do(t, http.MethodGet, "/api/v1/settings", nil)
	requireStatus(t, rec, http.StatusOK)

	body := rec.Body.String()
	for _, secret := range []string{"super-secret-token", "hook-secret", "gowa-password", "gowa-hmac"} {
		if strings.Contains(body, secret) {
			t.Fatalf("secret %q leaked in the settings response: %s", secret, body)
		}
	}

	var got settings.Settings
	decodeInto(t, rec, &got)
	for name, value := range map[string]string{
		"greenapi.api_token":           got.GreenAPI.APIToken,
		"greenapi.webhook_auth_header": got.GreenAPI.WebhookAuthHeader,
		"gowa.password":                got.GOWA.Password,
		"gowa.webhook_secret":          got.GOWA.WebhookSecret,
	} {
		if value != settings.Mask {
			t.Errorf("%s = %q, want the mask %q", name, value, settings.Mask)
		}
	}
	// Non-secret fields stay readable so the UI can display them.
	if got.GreenAPI.IDInstance != "7103123456" {
		t.Errorf("id_instance = %q, want it returned in the clear", got.GreenAPI.IDInstance)
	}
}

func TestPutSettingsWithMaskKeepsStoredSecret(t *testing.T) {
	ta := newTestAPI(t)
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	// Round-trip exactly what GET returned — masks included — which is what the
	// SPA does when the user edits an unrelated field.
	masked := ta.do(t, http.MethodGet, "/api/v1/settings", nil)
	var payload map[string]any
	decodeInto(t, masked, &payload)
	payload["scheduler"].(map[string]any)["send_time"] = "08:15"

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", payload), http.StatusOK)

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.GreenAPI.APIToken != "super-secret-token" {
		t.Fatalf("api_token = %q, want the stored value preserved", stored.GreenAPI.APIToken)
	}
	if stored.GOWA.Password != "gowa-password" {
		t.Fatalf("gowa password = %q, want the stored value preserved", stored.GOWA.Password)
	}
	if stored.Scheduler.SendTime != "08:15" {
		t.Fatalf("send_time = %q, want the edit applied", stored.Scheduler.SendTime)
	}
}

func TestPutSettingsWithEmptySecretClearsIt(t *testing.T) {
	ta := newTestAPI(t)
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	payload := configuredSettings()
	payload["greenapi"].(map[string]any)["api_token"] = ""
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", payload), http.StatusOK)

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.GreenAPI.APIToken != "" {
		t.Fatalf("api_token = %q, want it cleared", stored.GreenAPI.APIToken)
	}
}

func TestPutSettingsRejectsInvalidValues(t *testing.T) {
	cases := map[string]func(map[string]any){
		"unknown provider": func(p map[string]any) { p["provider"] = "carrier-pigeon" },
		"bad timezone":     func(p map[string]any) { p["scheduler"].(map[string]any)["timezone"] = "Mars/Olympus" },
		"bad send time":    func(p map[string]any) { p["scheduler"].(map[string]any)["send_time"] = "9am" },
		"zero threshold":   func(p map[string]any) { p["group_echo"].(map[string]any)["threshold"] = 0 },
		"bad mode":         func(p map[string]any) { p["greenapi"].(map[string]any)["mode"] = "carrier" },
	}
	for name, spoil := range cases {
		t.Run(name, func(t *testing.T) {
			ta := newTestAPI(t)
			payload := configuredSettings()
			spoil(payload)

			rec := ta.do(t, http.MethodPut, "/api/v1/settings", payload)
			requireStatus(t, rec, http.StatusUnprocessableEntity)
			if got := errorCode(t, rec); got != codeValidationFailed {
				t.Fatalf("code = %q, want %q", got, codeValidationFailed)
			}
		})
	}
}

func TestPutSettingsRebuildsTheProvider(t *testing.T) {
	var (
		mu   sync.Mutex
		seen []string
	)
	ta := newTestAPI(t, func(d *Deps) {
		d.Rebuild = func(_ context.Context, s *settings.Settings) error {
			mu.Lock()
			defer mu.Unlock()
			seen = append(seen, s.Provider)
			return nil
		}
	})

	payload := configuredSettings()
	payload["provider"] = "gowa"
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", payload), http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 || seen[0] != "gowa" {
		t.Fatalf("rebuild calls = %#v, want one call with gowa", seen)
	}
}

// The rebuild needs the real credentials, not the masks the client sent.
func TestPutSettingsRebuildReceivesDecryptedSecrets(t *testing.T) {
	var (
		mu    sync.Mutex
		token string
	)
	ta := newTestAPI(t, func(d *Deps) {
		d.Rebuild = func(_ context.Context, s *settings.Settings) error {
			mu.Lock()
			defer mu.Unlock()
			token = s.GreenAPI.APIToken
			return nil
		}
	})

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	mu.Lock()
	defer mu.Unlock()
	if token != "super-secret-token" {
		t.Fatalf("rebuild saw api_token %q, want the plaintext value", token)
	}
}

func TestPutSettingsRebuildFailureStillSavesAndReportsIt(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) {
		d.Rebuild = func(context.Context, *settings.Settings) error {
			return errors.New("instance id is not a number")
		}
	})

	rec := ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings())
	// The config is valid and persisted; only the live connection failed. A 4xx
	// here would strand the user with settings they cannot save.
	requireStatus(t, rec, http.StatusOK)

	var payload map[string]json.RawMessage
	decodeInto(t, rec, &payload)
	raw, ok := payload["provider_error"]
	if !ok {
		t.Fatalf("want provider_error in the response, got %s", rec.Body.String())
	}
	var msg string
	if err := json.Unmarshal(raw, &msg); err != nil || !strings.Contains(msg, "instance id") {
		t.Fatalf("provider_error = %s, want the rebuild failure", raw)
	}

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.GreenAPI.IDInstance != "7103123456" {
		t.Fatal("want the settings persisted despite the rebuild failure")
	}
}

func TestPutSettingsResponseIsAlsoMasked(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings())
	requireStatus(t, rec, http.StatusOK)

	if strings.Contains(rec.Body.String(), "super-secret-token") {
		t.Fatalf("PUT echoed a secret back in plaintext: %s", rec.Body.String())
	}
}

// A store failure is not the client's fault. It must not be dressed up as a
// validation error, and its text must never reach the response body.
func TestPutSettingsStoreFailureIs500AndLeaksNothing(t *testing.T) {
	ta := newTestAPI(t)
	if err := ta.store.Close(); err != nil {
		t.Fatalf("closing store: %v", err)
	}

	rec := ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings())
	requireStatus(t, rec, http.StatusInternalServerError)
	if got := errorCode(t, rec); got != codeInternal {
		t.Fatalf("code = %q, want %q", got, codeInternal)
	}
	if body := rec.Body.String(); strings.Contains(body, "sql") || strings.Contains(body, "database") {
		t.Fatalf("internal detail leaked: %s", body)
	}
}

// A GET whose rebuild failed carries provider_error. The SPA re-submits the
// document it was handed, so PUT must accept its own response shape.
func TestSettingsGetResponseIsAcceptedByPut(t *testing.T) {
	ta := newTestAPI(t, func(d *Deps) {
		d.Rebuild = func(context.Context, *settings.Settings) error {
			return errors.New("instance id is not a number")
		}
	})

	first := ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings())
	requireStatus(t, first, http.StatusOK)

	var echoed map[string]any
	decodeInto(t, first, &echoed)
	if _, ok := echoed["provider_error"]; !ok {
		t.Fatal("precondition: want provider_error in the response")
	}

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", echoed), http.StatusOK)
}

// Omitting a section must not destroy it. A client switching providers sends
// only the fields it edited; wiping the other provider's credentials — which an
// empty secret does — would be silent data loss.
func TestPutSettingsOmittedSectionKeepsStoredValues(t *testing.T) {
	ta := newTestAPI(t)
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", `{"provider":"gowa"}`), http.StatusOK)

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.Provider != settings.ProviderGOWA {
		t.Errorf("provider = %q, want the edit applied", stored.Provider)
	}
	if stored.GreenAPI.APIToken != "super-secret-token" {
		t.Errorf("greenapi api_token = %q, want it preserved", stored.GreenAPI.APIToken)
	}
	if stored.GreenAPI.IDInstance != "7103123456" {
		t.Errorf("greenapi id_instance = %q, want it preserved", stored.GreenAPI.IDInstance)
	}
	if stored.GOWA.Password != "gowa-password" {
		t.Errorf("gowa password = %q, want it preserved", stored.GOWA.Password)
	}
	if stored.Scheduler.SendTime != "09:00" {
		t.Errorf("send_time = %q, want it preserved", stored.Scheduler.SendTime)
	}
}

// A section sent as null is "not mentioned", not "reset to zero".
func TestPutSettingsNullSectionKeepsStoredValues(t *testing.T) {
	ta := newTestAPI(t)
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings",
		`{"greenapi":null,"scheduler":{"timezone":"UTC","send_time":"10:00"}}`), http.StatusOK)

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.GreenAPI.APIToken != "super-secret-token" {
		t.Errorf("api_token = %q, want it preserved", stored.GreenAPI.APIToken)
	}
	if stored.Scheduler.Timezone != "UTC" || stored.Scheduler.SendTime != "10:00" {
		t.Errorf("scheduler = %#v, want the edit applied", stored.Scheduler)
	}
}

// Clearing is still possible — it just has to be explicit.
func TestPutSettingsExplicitEmptySecretStillClears(t *testing.T) {
	ta := newTestAPI(t)
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", configuredSettings()), http.StatusOK)

	payload := configuredSettings()
	payload["greenapi"].(map[string]any)["api_token"] = ""
	requireStatus(t, ta.do(t, http.MethodPut, "/api/v1/settings", payload), http.StatusOK)

	stored, err := ta.settings.Load(context.Background())
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if stored.GreenAPI.APIToken != "" {
		t.Fatalf("api_token = %q, want it cleared", stored.GreenAPI.APIToken)
	}
	if stored.GOWA.Password != "gowa-password" {
		t.Fatalf("gowa password = %q, want the untouched secret preserved", stored.GOWA.Password)
	}
}

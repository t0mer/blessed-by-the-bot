package settings_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	// The scratch image has no OS zoneinfo; cmd/blessedbot embeds tzdata. Tests
	// need it too or Asia/Jerusalem fails to resolve.
	_ "time/tzdata"

	"github.com/t0mer/blessed-by-the-bot/internal/crypto"
	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func newService(t *testing.T) (*settings.Service, *store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := crypto.New(key)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := settings.New(st, cipher)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return svc, st
}

func TestLoadReturnsDefaultsOnEmptyStore(t *testing.T) {
	svc, _ := newService(t)
	got, err := svc.Load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Provider != settings.ProviderGreenAPI {
		t.Errorf("provider = %q, want greenapi", got.Provider)
	}
	if got.Scheduler.Timezone != "Asia/Jerusalem" {
		t.Errorf("timezone = %q, want Asia/Jerusalem", got.Scheduler.Timezone)
	}
	if got.Scheduler.SendTime != "09:00" {
		t.Errorf("send time = %q, want 09:00", got.Scheduler.SendTime)
	}
	if got.GroupEcho.Threshold != 3 {
		t.Errorf("threshold = %d, want 3", got.GroupEcho.Threshold)
	}
	if got.GroupEcho.WindowHours != 6 {
		t.Errorf("window = %d, want 6", got.GroupEcho.WindowHours)
	}
	if got.GroupEcho.CooldownHours != 20 {
		t.Errorf("cooldown = %d, want 20", got.GroupEcho.CooldownHours)
	}
}

func TestSaveThenLoadRoundTripsSecrets(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	in := settings.Defaults()
	in.GreenAPI.IDInstance = "7103123456"
	in.GreenAPI.APIToken = "super-secret-token"
	in.GOWA.BaseURL = "http://gowa:3000"
	in.GOWA.Password = "hunter2"
	if err := svc.Save(ctx, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := svc.Load(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.GreenAPI.APIToken != "super-secret-token" {
		t.Errorf("greenapi token = %q, want the original", got.GreenAPI.APIToken)
	}
	if got.GOWA.Password != "hunter2" {
		t.Errorf("gowa password = %q, want the original", got.GOWA.Password)
	}
	if got.GreenAPI.IDInstance != "7103123456" {
		t.Errorf("instance id = %q", got.GreenAPI.IDInstance)
	}
}

func TestSecretsAreEncryptedAtRest(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	in := settings.Defaults()
	in.GreenAPI.APIToken = "super-secret-token"
	if err := svc.Save(ctx, in); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := st.GetSetting(ctx, "provider.greenapi")
	if err != nil {
		t.Fatalf("read raw setting: %v", err)
	}
	if strings.Contains(raw, "super-secret-token") {
		t.Errorf("plaintext token found in the stored row: %s", raw)
	}
	if !strings.Contains(raw, crypto.Prefix) {
		t.Errorf("stored row carries no %q envelope: %s", crypto.Prefix, raw)
	}
}

func TestLoadMaskedHidesSecrets(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	in := settings.Defaults()
	in.GreenAPI.IDInstance = "7103123456"
	in.GreenAPI.APIToken = "super-secret-token"
	if err := svc.Save(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := svc.LoadMasked(ctx)
	if err != nil {
		t.Fatalf("load masked: %v", err)
	}
	if got.GreenAPI.APIToken != settings.Mask {
		t.Errorf("token = %q, want the mask", got.GreenAPI.APIToken)
	}
	if got.GreenAPI.IDInstance != "7103123456" {
		t.Errorf("non-secret field was masked: %q", got.GreenAPI.IDInstance)
	}
	if got.GOWA.Password != "" {
		t.Errorf("unset secret = %q, want empty (not the mask)", got.GOWA.Password)
	}
}

func TestSaveWithMaskKeepsExistingSecret(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	in := settings.Defaults()
	in.GreenAPI.APIToken = "super-secret-token"
	if err := svc.Save(ctx, in); err != nil {
		t.Fatal(err)
	}

	// Simulate the UI: GET returns masked values, the user edits an unrelated
	// field, PUT sends the mask back untouched.
	masked, err := svc.LoadMasked(ctx)
	if err != nil {
		t.Fatal(err)
	}
	masked.GreenAPI.IDInstance = "changed"
	if err := svc.Save(ctx, masked); err != nil {
		t.Fatalf("save masked: %v", err)
	}

	got, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.GreenAPI.APIToken != "super-secret-token" {
		t.Errorf("token = %q; saving the mask destroyed the stored secret", got.GreenAPI.APIToken)
	}
	if got.GreenAPI.IDInstance != "changed" {
		t.Errorf("instance id = %q, want changed", got.GreenAPI.IDInstance)
	}
}

func TestSaveWithEmptySecretClearsIt(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	in := settings.Defaults()
	in.GreenAPI.APIToken = "super-secret-token"
	if err := svc.Save(ctx, in); err != nil {
		t.Fatal(err)
	}

	in.GreenAPI.APIToken = ""
	if err := svc.Save(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.GreenAPI.APIToken != "" {
		t.Errorf("token = %q, want cleared; empty must be distinct from the mask", got.GreenAPI.APIToken)
	}
}

func TestSaveRejectsUnknownProvider(t *testing.T) {
	svc, _ := newService(t)
	in := settings.Defaults()
	in.Provider = "carrier-pigeon"
	if err := svc.Save(context.Background(), in); err == nil {
		t.Fatal("unknown provider accepted")
	}
}

func TestSaveRejectsInvalidTimezone(t *testing.T) {
	svc, _ := newService(t)
	in := settings.Defaults()
	in.Scheduler.Timezone = "Mars/Olympus"
	if err := svc.Save(context.Background(), in); err == nil {
		t.Fatal("unknown timezone accepted")
	}
}

func TestSendTimeValidation(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	for _, bad := range []string{"9am", "25:00", "9:00", "24:00", "07:60", ""} {
		in := settings.Defaults()
		in.Scheduler.SendTime = bad
		if err := svc.Save(ctx, in); err == nil {
			t.Errorf("send time %q accepted", bad)
		}
	}
	for _, good := range []string{"00:00", "09:00", "23:59"} {
		in := settings.Defaults()
		in.Scheduler.SendTime = good
		if err := svc.Save(ctx, in); err != nil {
			t.Errorf("send time %q rejected: %v", good, err)
		}
	}
}

func TestSaveRejectsNonPositiveGroupEchoValues(t *testing.T) {
	svc, _ := newService(t)
	ctx := context.Background()

	cases := map[string]func(*settings.Settings){
		"threshold": func(s *settings.Settings) { s.GroupEcho.Threshold = 0 },
		"window":    func(s *settings.Settings) { s.GroupEcho.WindowHours = 0 },
		"cooldown":  func(s *settings.Settings) { s.GroupEcho.CooldownHours = -1 },
	}
	for name, mutate := range cases {
		in := settings.Defaults()
		mutate(in)
		if err := svc.Save(ctx, in); err == nil {
			t.Errorf("%s: non-positive value accepted", name)
		}
	}
}

func TestSaveRejectsUnknownGreenAPIMode(t *testing.T) {
	svc, _ := newService(t)
	in := settings.Defaults()
	in.GreenAPI.Mode = "telepathy"
	if err := svc.Save(context.Background(), in); err == nil {
		t.Fatal("unknown greenapi mode accepted")
	}
}

func TestLoadSurvivesUnknownKeysInStoredJSON(t *testing.T) {
	svc, st := newService(t)
	ctx := context.Background()

	if err := st.SetSetting(ctx, "scheduler",
		`{"timezone":"UTC","send_time":"10:30","future_field":"ignored"}`); err != nil {
		t.Fatal(err)
	}
	got, err := svc.Load(ctx)
	if err != nil {
		t.Fatalf("load with an unknown key: %v", err)
	}
	if got.Scheduler.SendTime != "10:30" || got.Scheduler.Timezone != "UTC" {
		t.Errorf("known fields lost: %+v", got.Scheduler)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	if _, err := settings.New(nil, nil); err == nil {
		t.Error("nil store accepted")
	}
}

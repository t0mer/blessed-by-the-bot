package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// hasField reports whether the validation error mentions the named field.
func hasField(fields []FieldError, name string) bool {
	for _, f := range fields {
		if f.Field == name {
			return true
		}
	}
	return false
}

func TestValidatorCollectsEveryBadField(t *testing.T) {
	v := newValidator()
	v.require("name", "  ")
	v.oneOf("event_type", "graduation", "birthday", "wedding")
	v.date("event_date", "31/12/2026")
	v.intRange("importance", 9, 1, 5)

	err := v.err()
	if err == nil {
		t.Fatal("want a validation error, got nil")
	}

	var apiErr *apiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("want an apiError, got %#v", err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", apiErr.Status, http.StatusUnprocessableEntity)
	}
	if apiErr.Code != codeValidationFailed {
		t.Fatalf("code = %q, want %q", apiErr.Code, codeValidationFailed)
	}

	// All four problems must be reported together: a user fixing one field at a
	// time because the API only ever reports the first is a bad experience.
	for _, field := range []string{"name", "event_type", "event_date", "importance"} {
		if !hasField(apiErr.Fields, field) {
			t.Errorf("missing field error for %q; got %#v", field, apiErr.Fields)
		}
	}
}

func TestValidatorPassesGoodInput(t *testing.T) {
	v := newValidator()
	v.require("name", "Dana")
	v.oneOf("event_type", "birthday", "birthday", "wedding")
	v.date("event_date", "1992-02-29")
	v.intRange("importance", 3, 1, 5)
	v.language("language", "he")
	v.clock("send_time", nil)

	if err := v.err(); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
}

func TestDateRejectsImpossibleCalendarDates(t *testing.T) {
	cases := map[string]bool{
		"1992-02-29": true,  // leap year
		"1991-02-29": false, // not a leap year
		"2026-13-01": false,
		"2026-00-10": false,
		"2026-1-1":   false, // must be zero-padded
		"2026-12-31": true,
	}
	for value, want := range cases {
		v := newValidator()
		v.date("event_date", value)
		got := v.err() == nil
		if got != want {
			t.Errorf("date(%q) valid = %v, want %v", value, got, want)
		}
	}
}

func TestClockAcceptsOnly24HourTimes(t *testing.T) {
	cases := map[string]bool{
		"00:00": true,
		"09:05": true,
		"23:59": true,
		"24:00": false,
		"9:05":  false,
		"09:60": false,
		"":      false,
	}
	for value, want := range cases {
		v := newValidator()
		v.clock("send_time", &value)
		if got := v.err() == nil; got != want {
			t.Errorf("clock(%q) valid = %v, want %v", value, got, want)
		}
	}
}

func TestNormalizePhoneStripsFormatting(t *testing.T) {
	cases := []struct {
		in    string
		out   string
		valid bool
	}{
		{"+972 50-123 4567", "972501234567", true},
		{"972501234567", "972501234567", true},
		{"(972) 50.123.4567", "972501234567", true},
		{"12345", "", false},               // too short
		{"1234567890123456789", "", false}, // too long
		{"not a phone", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := normalizePhone(tc.in)
		if ok != tc.valid {
			t.Errorf("normalizePhone(%q) ok = %v, want %v", tc.in, ok, tc.valid)
			continue
		}
		if ok && got != tc.out {
			t.Errorf("normalizePhone(%q) = %q, want %q", tc.in, got, tc.out)
		}
	}
}

func TestGroupChatIDRequiresGroupSuffix(t *testing.T) {
	cases := map[string]bool{
		"120363001234567890@g.us": true,
		"972501234567@c.us":       false,
		"120363001234567890":      false,
		"":                        false,
	}
	for value, want := range cases {
		v := newValidator()
		v.groupChatID("chat_id", value)
		if got := v.err() == nil; got != want {
			t.Errorf("groupChatID(%q) valid = %v, want %v", value, got, want)
		}
	}
}

func TestPatternRejectsUncompilableRegex(t *testing.T) {
	cases := map[string]bool{
		"mazal tov":      true,
		`/happy\s+bday/`: true,
		"/[unclosed/":    false,
		"/":              false, // a lone slash opens a regex it never closes
		"/happy":         false, // unterminated regex
		"and/or":         true,  // an internal slash is just a substring
		"":               false,
	}
	for value, want := range cases {
		v := newValidator()
		v.pattern("pattern", value)
		if got := v.err() == nil; got != want {
			t.Errorf("pattern(%q) valid = %v, want %v", value, got, want)
		}
	}
}

func TestLanguageAcceptsBCP47ish(t *testing.T) {
	cases := map[string]bool{
		"he":                      true,
		"en":                      true,
		"pt-BR":                   true,
		"any":                     true,
		"":                        false,
		"h":                       false,
		"ENGLISH-TOO-LONG-BY-FAR": false,
		"he_IL":                   false,
	}
	for value, want := range cases {
		v := newValidator()
		v.language("language", value)
		if got := v.err() == nil; got != want {
			t.Errorf("language(%q) valid = %v, want %v", value, got, want)
		}
	}
}

func TestEmptyToNilCollapsesBlankOptionals(t *testing.T) {
	blank := "   "
	if got := emptyToNil(&blank); got != nil {
		t.Fatalf("want nil for a blank value, got %q", *got)
	}
	value := " male "
	got := emptyToNil(&value)
	if got == nil || *got != "male" {
		t.Fatalf("want trimmed %q, got %v", "male", got)
	}
	if emptyToNil(nil) != nil {
		t.Fatal("want nil for a nil input")
	}
}

func TestOptionalOneOfAllowsNilButChecksPresentValues(t *testing.T) {
	v := newValidator()
	v.optionalOneOf("gender", nil, "male", "female")
	if err := v.err(); err != nil {
		t.Fatalf("nil means any and must pass, got %v", err)
	}

	ok := "female"
	v = newValidator()
	v.optionalOneOf("gender", &ok, "male", "female")
	if err := v.err(); err != nil {
		t.Fatalf("want %q accepted, got %v", ok, err)
	}

	bad := "robot"
	v = newValidator()
	v.optionalOneOf("gender", &bad, "male", "female")
	if err := v.err(); err == nil {
		t.Fatalf("want %q rejected", bad)
	}
}

func TestPhoneReturnsDigitsAndReportsBadInput(t *testing.T) {
	v := newValidator()
	if got := v.phone("phone", "+972 50-123 4567"); got != "972501234567" {
		t.Errorf("phone = %q, want the digits only", got)
	}
	if err := v.err(); err != nil {
		t.Fatalf("want a valid number accepted, got %v", err)
	}

	v = newValidator()
	// On failure the original value comes back; callers must not persist it,
	// which err() being non-nil tells them.
	if got := v.phone("phone", "123"); got != "123" {
		t.Errorf("phone = %q, want the input echoed on failure", got)
	}
	if !hasField(v.fields, "phone") {
		t.Fatal("want a phone field error")
	}
}

func TestPathIDParsesAndRejects(t *testing.T) {
	cases := map[string]int64{
		"7":   7,
		"0":   0, // rejected: ids start at 1
		"-3":  0,
		"abc": 0,
		"":    0,
	}
	for raw, want := range cases {
		r := chi.NewRouter()
		var got int64
		var gotErr error
		r.Get("/x/{id}", func(_ http.ResponseWriter, req *http.Request) {
			got, gotErr = pathID(req)
		})

		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x/"+raw, nil))

		if want == 0 {
			if gotErr == nil && rec.Code == http.StatusOK {
				t.Errorf("pathID(%q) = %d, want an error", raw, got)
			}
			continue
		}
		if gotErr != nil || got != want {
			t.Errorf("pathID(%q) = %d, %v; want %d, nil", raw, got, gotErr, want)
		}
	}
}

// time.Parse accepts year 0000; a scheduler computing an age from it would be
// nonsense, and it is far more likely a slipped digit than a real date.
func TestDateRejectsImplausibleYears(t *testing.T) {
	cases := map[string]bool{
		"1900-01-01": true,
		"1990-05-17": true,
		"2200-12-31": true,
		"0000-01-01": false,
		"0202-05-17": false, // a slipped digit in 2020
		"9999-12-31": false,
	}
	for value, want := range cases {
		v := newValidator()
		v.date("event_date", value)
		if got := v.err() == nil; got != want {
			t.Errorf("date(%q) valid = %v, want %v", value, got, want)
		}
	}
}

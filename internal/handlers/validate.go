package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// FieldError names one invalid request field and says what is wrong with it.
// The SPA renders these next to the corresponding form input.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

var (
	clockPattern    = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
	languagePattern = regexp.MustCompile(`^[a-zA-Z]{2,8}(-[a-zA-Z0-9]{2,8})?$`)
	nonDigits       = regexp.MustCompile(`\D`)
)

// Phone length bounds. E.164 allows at most 15 digits including the country
// code; 8 is below any real international number but above obvious typos.
const (
	minPhoneDigits = 8
	maxPhoneDigits = 15
)

// Event-year bounds. time.Parse happily accepts "0000-01-01", which is a typo,
// not a birthday; these bounds catch a slipped digit without second-guessing
// anyone's actual dates.
const (
	minEventYear = 1900
	maxEventYear = 2200
)

// validator accumulates field errors so one request reports every problem at
// once rather than making the user fix them one round-trip at a time.
type validator struct {
	fields []FieldError
}

func newValidator() *validator { return &validator{} }

// add records a problem with the named field.
func (v *validator) add(field, message string) {
	v.fields = append(v.fields, FieldError{Field: field, Message: message})
}

// err returns a 422 apiError listing every collected problem, or nil.
func (v *validator) err() error {
	if len(v.fields) == 0 {
		return nil
	}
	return &apiError{
		Status:  http.StatusUnprocessableEntity,
		Code:    codeValidationFailed,
		Message: "one or more fields are invalid",
		Fields:  v.fields,
	}
}

func (v *validator) require(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(field, "is required")
	}
}

func (v *validator) oneOf(field, value string, allowed ...string) {
	for _, a := range allowed {
		if value == a {
			return
		}
	}
	v.add(field, "must be one of: "+strings.Join(allowed, ", "))
}

// optionalOneOf allows nil (meaning "any") but validates a present value.
func (v *validator) optionalOneOf(field string, value *string, allowed ...string) {
	if value == nil {
		return
	}
	v.oneOf(field, *value, allowed...)
}

// date requires a zero-padded YYYY-MM-DD that is a real calendar date.
// time.Parse alone accepts "2026-02-30" and normalizes it, so the round-trip
// comparison is what actually rejects impossible dates.
func (v *validator) date(field, value string) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil || parsed.Format(time.DateOnly) != value {
		v.add(field, "must be a real date in YYYY-MM-DD form")
		return
	}
	if year := parsed.Year(); year < minEventYear || year > maxEventYear {
		v.add(field, fmt.Sprintf("year must be between %d and %d", minEventYear, maxEventYear))
	}
}

// clock validates an optional HH:MM override. Nil means "use the general
// setting"; a present value must be a 24-hour time.
func (v *validator) clock(field string, value *string) {
	if value == nil {
		return
	}
	if !clockPattern.MatchString(*value) {
		v.add(field, "must be HH:MM in 24-hour form")
	}
}

func (v *validator) intRange(field string, value, low, high int) {
	if value < low || value > high {
		v.add(field, fmt.Sprintf("must be between %d and %d", low, high))
	}
}

func (v *validator) language(field, value string) {
	if !languagePattern.MatchString(value) {
		v.add(field, "must be a language tag such as he, en or pt-BR")
	}
}

// phone normalizes and validates a contact phone, returning the digits to store.
// On failure it records the problem and returns the original value; callers
// should not persist when err() is non-nil.
func (v *validator) phone(field, value string) string {
	digits, ok := normalizePhone(value)
	if !ok {
		v.add(field, fmt.Sprintf(
			"must be an international number with %d-%d digits, no + or spaces",
			minPhoneDigits, maxPhoneDigits))
		return value
	}
	return digits
}

// groupChatID requires a WhatsApp group JID. Private chat IDs are rejected on
// purpose: a group configured with a @c.us id would silently never match.
func (v *validator) groupChatID(field, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		v.add(field, "is required")
		return
	}
	if !strings.HasSuffix(trimmed, "@g.us") {
		v.add(field, "must be a WhatsApp group id ending in @g.us")
		return
	}
	if strings.TrimSuffix(trimmed, "@g.us") == "" {
		v.add(field, "must be a WhatsApp group id ending in @g.us")
	}
}

// pattern validates a wish-detection rule. A value wrapped in slashes is a
// regex and must compile now, so a broken rule fails at edit time rather than
// silently never matching in the echo engine.
func (v *validator) pattern(field, value string) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		v.add(field, "is required")
		return
	}
	// A leading slash signals regex intent. Anything that opens with one must
	// also close with one and carry a body, so "/" or "/happy" is a malformed
	// regex rather than a substring that happens to contain a slash.
	if !strings.HasPrefix(trimmed, "/") {
		return
	}
	if !isDelimitedRegex(trimmed) {
		v.add(field, "looks like a regular expression but is not wrapped in /.../")
		return
	}
	body := trimmed[1 : len(trimmed)-1]
	if _, err := regexp.Compile(body); err != nil {
		v.add(field, "is not a valid regular expression: "+err.Error())
	}
}

// isDelimitedRegex reports whether s is of the form /.../ with a non-empty body.
func isDelimitedRegex(s string) bool {
	return len(s) >= 3 && strings.HasPrefix(s, "/") && strings.HasSuffix(s, "/")
}

// normalizePhone strips punctuation and returns the bare digits, reporting
// whether the result is a plausible international number.
func normalizePhone(raw string) (string, bool) {
	digits := nonDigits.ReplaceAllString(raw, "")
	if len(digits) < minPhoneDigits || len(digits) > maxPhoneDigits {
		return "", false
	}
	return digits, true
}

// emptyToNil trims an optional string and collapses a blank one to nil, so the
// SPA can clear an optional targeting field by sending "".
func emptyToNil(v *string) *string {
	if v == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*v)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// pathID reads and validates the {id} URL parameter.
func pathID(r *http.Request) (int64, error) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errorf(http.StatusBadRequest, codeInvalidID, "%q is not a valid id", raw)
	}
	return id, nil
}

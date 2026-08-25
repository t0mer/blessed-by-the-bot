// Package handlers implements the JSON REST API and the provider webhook
// endpoints.
//
// Layering rule from the spec: handlers never touch SQL and never build
// providers. They call store methods and services, and translate the result
// into an HTTP response. Everything a client sees goes through WriteJSON or
// writeError, so the error envelope and the "don't leak internals" rule are
// enforced in exactly one place.
package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/provider"
	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// maxBodyBytes caps request bodies. Nothing this API accepts is large; a
// blessing template is the biggest realistic payload.
const maxBodyBytes = 1 << 20 // 1 MiB

// Error codes returned in the envelope's "code" field. They are part of the
// API contract: the SPA switches on them, so treat renames as breaking.
const (
	codeNotFound            = "not_found"
	codeMethodNotAllowed    = "method_not_allowed"
	codeInvalidJSON         = "invalid_json"
	codeValidationFailed    = "validation_failed"
	codeInvalidID           = "invalid_id"
	codeInvalidQuery        = "invalid_query"
	codeInternal            = "internal_error"
	codeProviderUnavailable = "provider_unavailable"
	codeProviderFailed      = "provider_failed"
	codeUnauthorized        = "unauthorized"
	codeNotImplemented      = "not_implemented"
	codeConflict            = "conflict"
)

type errorEnvelope struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Fields  []FieldError `json:"fields,omitempty"`
}

// apiError is an error that already knows which status code and machine-readable
// code the client should see. Anything that is not an apiError is treated as an
// internal fault and reported generically.
type apiError struct {
	Status  int
	Code    string
	Message string
	Fields  []FieldError
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

// errorf builds an apiError with a formatted message.
func errorf(status int, code, format string, args ...any) *apiError {
	return &apiError{Status: status, Code: code, Message: fmt.Sprintf(format, args...)}
}

// WriteJSON writes payload as the response body. A nil payload writes the status
// only, which is what 204 responses want.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError renders err as the standard envelope. Known sentinel errors get
// meaningful status codes; everything else becomes a 500 whose detail is logged
// but never sent, so a SQL or connection-string error cannot reach a browser.
func writeError(w http.ResponseWriter, log *slog.Logger, err error) {
	var apiErr *apiError
	switch {
	case errors.As(err, &apiErr):
		if apiErr.Status >= http.StatusInternalServerError && log != nil {
			log.Error("request failed", "code", apiErr.Code, "error", err)
		}
	case errors.Is(err, store.ErrNotFound):
		apiErr = errorf(http.StatusNotFound, codeNotFound, "resource not found")
	case errors.Is(err, provider.ErrNoProvider):
		apiErr = errorf(http.StatusServiceUnavailable, codeProviderUnavailable,
			"no whatsapp provider is configured; set one up in Settings")
	default:
		if log != nil {
			log.Error("request failed", "error", err)
		}
		apiErr = errorf(http.StatusInternalServerError, codeInternal, "internal server error")
	}

	WriteJSON(w, apiErr.Status, errorEnvelope{errorBody{
		Code:    apiErr.Code,
		Message: apiErr.Message,
		Fields:  apiErr.Fields,
	}})
}

// NotFoundJSON is the router's 404 for API routes, so an unknown path returns
// the same envelope as everything else instead of chi's plain text.
func NotFoundJSON(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusNotFound, errorEnvelope{errorBody{
		Code: codeNotFound, Message: "resource not found",
	}})
}

// MethodNotAllowedJSON is the router's 405 for API routes.
func MethodNotAllowedJSON(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusMethodNotAllowed, errorEnvelope{errorBody{
		Code: codeMethodNotAllowed, Message: "method not allowed",
	}})
}

// decodeJSON reads exactly one JSON object from the request into dst. Unknown
// fields are rejected so a typo in the SPA surfaces as a 400 instead of being
// silently dropped, and a second JSON value in the body is rejected too.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errorf(http.StatusBadRequest, codeInvalidJSON, "body must contain a single JSON object")
	}
	return nil
}

func decodeError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return errorf(http.StatusRequestEntityTooLarge, codeInvalidJSON,
			"request body must not exceed %d bytes", maxBodyBytes)
	}

	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return errorf(http.StatusBadRequest, codeInvalidJSON,
			"malformed JSON at byte %d", syntaxErr.Offset)
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		return &apiError{
			Status:  http.StatusBadRequest,
			Code:    codeInvalidJSON,
			Message: "one or more fields have the wrong type",
			Fields:  []FieldError{{Field: typeErr.Field, Message: "expected " + typeErr.Type.String()}},
		}
	}

	if errors.Is(err, io.EOF) {
		return errorf(http.StatusBadRequest, codeInvalidJSON, "request body is empty")
	}

	// DisallowUnknownFields reports a plain error; its text is the only handle.
	if field, ok := unknownField(err); ok {
		return &apiError{
			Status:  http.StatusBadRequest,
			Code:    codeInvalidJSON,
			Message: "unknown field " + field,
			Fields:  []FieldError{{Field: field, Message: "unknown field"}},
		}
	}
	return errorf(http.StatusBadRequest, codeInvalidJSON, "request body is not valid JSON")
}

const unknownFieldPrefix = "json: unknown field "

func unknownField(err error) (string, bool) {
	msg := err.Error()
	if !strings.HasPrefix(msg, unknownFieldPrefix) {
		return "", false
	}
	return strings.Trim(strings.TrimPrefix(msg, unknownFieldPrefix), `"`), true
}

// FieldError names one invalid request field. Moved to validate.go in Task 2.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

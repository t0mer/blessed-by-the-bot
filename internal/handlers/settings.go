package handlers

import (
	"errors"
	"net/http"

	"github.com/t0mer/blessed-by-the-bot/internal/service/settings"
)

// settingsResponse is the masked settings plus, on a PUT, whatever went wrong
// when the new configuration was handed to the provider. The settings pointer is
// embedded so the JSON stays flat and GET and PUT share one shape.
type settingsResponse struct {
	*settings.Settings
	ProviderError string `json:"provider_error,omitempty"`
}

func (a *API) getSettings(w http.ResponseWriter, r *http.Request) {
	masked, err := a.settings.LoadMasked(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, settingsResponse{Settings: masked})
}

// putSettings validates, persists and then re-applies the configuration.
//
// The settings service owns mask resolution and validation, so this handler must
// not pre-process the payload: rewriting a masked secret here would defeat the
// "PUT with the mask means unchanged" contract.
func (a *API) putSettings(w http.ResponseWriter, r *http.Request) {
	var incoming settings.Settings
	if err := decodeJSON(w, r, &incoming); err != nil {
		writeError(w, a.log, err)
		return
	}

	if err := a.settings.Save(r.Context(), &incoming); err != nil {
		// Only a validation failure is the client's to fix, and only its text is
		// safe to echo — it names the offending setting and nothing else. A store
		// or cipher failure falls through to writeError, which logs the detail
		// and returns a generic 500.
		if errors.Is(err, settings.ErrInvalid) {
			writeError(w, a.log, &apiError{
				Status:  http.StatusUnprocessableEntity,
				Code:    codeValidationFailed,
				Message: err.Error(),
			})
			return
		}
		writeError(w, a.log, err)
		return
	}

	masked, err := a.settings.LoadMasked(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	resp := settingsResponse{Settings: masked}

	// Rebuild after the save so the running process picks up the change without
	// a restart (spec §4). A failure here means the credentials are wrong, not
	// that the write should be rolled back — report it alongside the settings.
	if a.rebuild != nil {
		// Load again rather than reusing the request body: Save copies its
		// input, so `incoming` still holds whatever masks the client sent.
		plain, loadErr := a.settings.Load(r.Context())
		if loadErr != nil {
			writeError(w, a.log, loadErr)
			return
		}
		if err := a.rebuild(r.Context(), plain); err != nil {
			a.log.Warn("provider rebuild after settings change failed", "error", err)
			resp.ProviderError = err.Error()
		} else {
			a.log.Info("provider rebuilt after settings change", "provider", a.providers.Name())
		}
	}

	WriteJSON(w, http.StatusOK, resp)
}

package handlers

import (
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// wishPatternRequest is the client-supplied half of a wish-detection rule.
type wishPatternRequest struct {
	Language string `json:"language"`
	Pattern  string `json:"pattern"`
	Enabled  *bool  `json:"enabled"`
}

func (req *wishPatternRequest) toModel() (*store.WishPattern, error) {
	v := newValidator()
	v.language("language", req.Language)
	v.pattern("pattern", req.Pattern)

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := v.err(); err != nil {
		return nil, err
	}

	return &store.WishPattern{
		Language: req.Language,
		// Only the outer whitespace goes: a pattern's internal spacing is
		// significant, and "mazal tov " with a trailing space would never match.
		Pattern: strings.TrimSpace(req.Pattern),
		Enabled: enabled,
	}, nil
}

func (a *API) listWishPatterns(w http.ResponseWriter, r *http.Request) {
	patterns, err := a.store.ListWishPatterns(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, patterns)
}

func (a *API) createWishPattern(w http.ResponseWriter, r *http.Request) {
	var req wishPatternRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}
	model, err := req.toModel()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	created, err := a.store.CreateWishPattern(r.Context(), model)
	if err != nil {
		writeError(w, a.log, conflictOnUnique(err, "pattern",
			"this pattern already exists for that language"))
		return
	}
	WriteJSON(w, http.StatusCreated, created)
}

func (a *API) updateWishPattern(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	var req wishPatternRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}
	model, err := req.toModel()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	model.ID = id

	updated, err := a.store.UpdateWishPattern(r.Context(), model)
	if err != nil {
		writeError(w, a.log, conflictOnUnique(err, "pattern",
			"another pattern for that language already uses this text"))
		return
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (a *API) deleteWishPattern(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	if err := a.store.DeleteWishPattern(r.Context(), id); err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusNoContent, nil)
}

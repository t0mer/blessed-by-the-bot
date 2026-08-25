package handlers

import (
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// groupRequest is the client-supplied half of a watched group. Threshold is a
// pointer because NULL means "use the global group-echo default" — a distinct
// state from any concrete number.
type groupRequest struct {
	Name      string `json:"name"`
	ChatID    string `json:"chat_id"`
	Language  string `json:"language"`
	Threshold *int   `json:"threshold"`
	Enabled   *bool  `json:"enabled"`
}

func (req *groupRequest) toModel() (*store.Group, error) {
	v := newValidator()

	v.require("name", req.Name)
	v.groupChatID("chat_id", req.ChatID)
	v.language("language", req.Language)
	if req.Threshold != nil && *req.Threshold < 1 {
		v.add("threshold", "must be at least 1, or omitted to use the global default")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := v.err(); err != nil {
		return nil, err
	}

	return &store.Group{
		Name:      strings.TrimSpace(req.Name),
		ChatID:    strings.TrimSpace(req.ChatID),
		Language:  req.Language,
		Threshold: req.Threshold,
		Enabled:   enabled,
	}, nil
}

func (a *API) listGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := a.store.ListGroups(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, groups)
}

func (a *API) createGroup(w http.ResponseWriter, r *http.Request) {
	var req groupRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}
	model, err := req.toModel()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	created, err := a.store.CreateGroup(r.Context(), model)
	if err != nil {
		writeError(w, a.log, conflictOnUnique(err, "chat_id",
			"a group with this chat id already exists"))
		return
	}
	WriteJSON(w, http.StatusCreated, created)
}

func (a *API) getGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	group, err := a.store.GetGroup(r.Context(), id)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, group)
}

func (a *API) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	var req groupRequest
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

	updated, err := a.store.UpdateGroup(r.Context(), model)
	if err != nil {
		writeError(w, a.log, conflictOnUnique(err, "chat_id",
			"another group already uses this chat id"))
		return
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (a *API) deleteGroup(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	if err := a.store.DeleteGroup(r.Context(), id); err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusNoContent, nil)
}

// availableGroups asks the live provider which groups the account belongs to, so
// the UI can offer a picker. A 503 here is expected on a fresh install and the
// SPA falls back to manual chat-id entry.
func (a *API) availableGroups(w http.ResponseWriter, r *http.Request) {
	active, err := a.providers.Active()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	groups, err := active.ListGroups(r.Context())
	if err != nil {
		a.log.Warn("listing provider groups failed", "provider", active.Name(), "error", err)
		writeError(w, a.log, errorf(http.StatusBadGateway, codeProviderFailed,
			"the whatsapp provider could not list groups: %s", err))
		return
	}
	WriteJSON(w, http.StatusOK, groups)
}

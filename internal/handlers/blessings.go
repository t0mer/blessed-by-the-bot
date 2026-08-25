package handlers

import (
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// blessingRequest is the client-supplied half of a blessing template.
//
// Gender and Relation are *string because NULL is meaningful here: it means the
// template suits any gender or any relation. A blank string from a "clear this
// filter" UI control collapses to NULL rather than being stored literally.
type blessingRequest struct {
	EventType string  `json:"event_type"`
	Language  string  `json:"language"`
	Gender    *string `json:"gender"`
	Relation  *string `json:"relation"`
	Text      string  `json:"text"`
	Enabled   *bool   `json:"enabled"`
}

func (req *blessingRequest) toModel() (*store.Blessing, error) {
	v := newValidator()

	v.oneOf("event_type", req.EventType,
		store.EventBirthday, store.EventWedding, store.EventAnniversary, store.EventCustom)
	v.language("language", req.Language)
	v.require("text", req.Text)

	gender := emptyToNil(req.Gender)
	v.optionalOneOf("gender", gender, store.GenderMale, store.GenderFemale, store.GenderOther)

	relation := emptyToNil(req.Relation)
	v.optionalOneOf("relation", relation,
		store.RelationFriend, store.RelationCloseFriend, store.RelationFamily, store.RelationCoworker)

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := v.err(); err != nil {
		return nil, err
	}

	return &store.Blessing{
		EventType: req.EventType,
		Language:  req.Language,
		Gender:    gender,
		Relation:  relation,
		Text:      strings.TrimSpace(req.Text),
		Enabled:   enabled,
	}, nil
}

func (a *API) listBlessings(w http.ResponseWriter, r *http.Request) {
	blessings, err := a.store.ListBlessings(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, blessings)
}

func (a *API) createBlessing(w http.ResponseWriter, r *http.Request) {
	var req blessingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}
	model, err := req.toModel()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	created, err := a.store.CreateBlessing(r.Context(), model)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusCreated, created)
}

func (a *API) getBlessing(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	blessing, err := a.store.GetBlessing(r.Context(), id)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, blessing)
}

func (a *API) updateBlessing(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	var req blessingRequest
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

	updated, err := a.store.UpdateBlessing(r.Context(), model)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (a *API) deleteBlessing(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	if err := a.store.DeleteBlessing(r.Context(), id); err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusNoContent, nil)
}

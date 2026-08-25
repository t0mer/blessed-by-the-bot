package handlers

import (
	"net/http"
	"strings"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// Defaults for optional create fields, matching the DDL in 0001_init.sql.
const (
	defaultImportance = 3
	minImportance     = 1
	maxImportance     = 5
)

// contactRequest is the client-supplied half of a contact. It deliberately does
// not reuse store.Contact: ids and timestamps are owned by the server, and a
// client must not be able to set them.
//
// Importance and Enabled are pointers so an omitted field takes the documented
// default rather than Go's zero value (importance 0 would fail the CHECK, and
// enabled false would silently mute every new contact).
type contactRequest struct {
	Name       string  `json:"name"`
	Phone      string  `json:"phone"`
	EventDate  string  `json:"event_date"`
	EventType  string  `json:"event_type"`
	Language   string  `json:"language"`
	Relation   string  `json:"relation"`
	Importance *int    `json:"importance"`
	Gender     string  `json:"gender"`
	SendTime   *string `json:"send_time"`
	Enabled    *bool   `json:"enabled"`
}

// toModel validates the request and returns the contact to persist.
func (req *contactRequest) toModel() (*store.Contact, error) {
	v := newValidator()

	v.require("name", req.Name)
	phone := v.phone("phone", req.Phone)
	v.date("event_date", req.EventDate)
	v.oneOf("event_type", req.EventType,
		store.EventBirthday, store.EventWedding, store.EventAnniversary, store.EventCustom)
	v.language("language", req.Language)
	v.oneOf("relation", req.Relation,
		store.RelationFriend, store.RelationCloseFriend, store.RelationFamily, store.RelationCoworker)
	v.oneOf("gender", req.Gender, store.GenderMale, store.GenderFemale, store.GenderOther)

	importance := defaultImportance
	if req.Importance != nil {
		importance = *req.Importance
	}
	v.intRange("importance", importance, minImportance, maxImportance)

	sendTime := emptyToNil(req.SendTime)
	v.clock("send_time", sendTime)

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if err := v.err(); err != nil {
		return nil, err
	}

	return &store.Contact{
		Name:       strings.TrimSpace(req.Name),
		Phone:      phone,
		EventDate:  req.EventDate,
		EventType:  req.EventType,
		Language:   req.Language,
		Relation:   req.Relation,
		Importance: importance,
		Gender:     req.Gender,
		SendTime:   sendTime,
		Enabled:    enabled,
	}, nil
}

func (a *API) listContacts(w http.ResponseWriter, r *http.Request) {
	contacts, err := a.store.ListContacts(r.Context())
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, contacts)
}

func (a *API) createContact(w http.ResponseWriter, r *http.Request) {
	var req contactRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, a.log, err)
		return
	}
	model, err := req.toModel()
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	created, err := a.store.CreateContact(r.Context(), model)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusCreated, created)
}

func (a *API) getContact(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	contact, err := a.store.GetContact(r.Context(), id)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, contact)
}

func (a *API) updateContact(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	var req contactRequest
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

	updated, err := a.store.UpdateContact(r.Context(), model)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, updated)
}

func (a *API) deleteContact(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	if err := a.store.DeleteContact(r.Context(), id); err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusNoContent, nil)
}

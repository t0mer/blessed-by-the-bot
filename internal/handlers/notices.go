package handlers

import (
	"net/http"
)

// listNotices returns conditions the operator should act on.
//
// Dismissed notices are excluded by default; ?all=true includes them, which is
// what a "show history" toggle would use. They are not deleted on dismissal
// because a recurrence reopens the same row.
func (a *API) listNotices(w http.ResponseWriter, r *http.Request) {
	activeOnly := r.URL.Query().Get("all") != "true"

	notices, err := a.store.ListNotices(r.Context(), activeOnly)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, notices)
}

// dismissNotice marks one notice as read.
func (a *API) dismissNotice(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	if err := a.store.DismissNotice(r.Context(), id); err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusNoContent, nil)
}

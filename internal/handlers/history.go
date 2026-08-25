package handlers

import (
	"net/http"
	"strconv"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// listHistory serves the send-log feed that backs the dashboard and the history
// page. Paging is by limit only: the log is small, newest-first, and the UI
// shows a recent window rather than an archive.
func (a *API) listHistory(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	kind := query.Get("kind")
	switch kind {
	case "", store.KindScheduled, store.KindGroupEcho:
	default:
		writeError(w, a.log, errorf(http.StatusBadRequest, codeInvalidQuery,
			"kind must be %s or %s", store.KindScheduled, store.KindGroupEcho))
		return
	}

	limit := 0
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeError(w, a.log, errorf(http.StatusBadRequest, codeInvalidQuery,
				"limit must be a positive integer"))
			return
		}
		limit = parsed
	}

	// The store clamps the limit to its own maximum, so an absurd value is safe.
	entries, err := a.store.ListSendLog(r.Context(), kind, limit)
	if err != nil {
		writeError(w, a.log, err)
		return
	}
	WriteJSON(w, http.StatusOK, entries)
}

package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func seedNotice(t *testing.T, st *store.Store, key string) {
	t.Helper()
	detail := "Add a birthday template in \"ru\" under Blessings."
	if err := st.RaiseNotice(context.Background(), store.Notice{
		Key:     key,
		Level:   store.NoticeWarning,
		Code:    store.NoticeLanguageFallback,
		Message: "No birthday blessing in \"ru\"; using the en template instead.",
		Detail:  &detail,
	}); err != nil {
		t.Fatalf("raising notice: %v", err)
	}
}

func TestListNoticesReturnsEmptyArrayNotNull(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodGet, "/api/v1/notices", nil)
	requireStatus(t, rec, http.StatusOK)
	if body := rec.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want an empty JSON array", body)
	}
}

func TestListNoticesReturnsActiveOnes(t *testing.T) {
	ta := newTestAPI(t)
	seedNotice(t, ta.store, "language_fallback:birthday:ru")

	rec := ta.do(t, http.MethodGet, "/api/v1/notices", nil)
	requireStatus(t, rec, http.StatusOK)

	var got []store.Notice
	decodeInto(t, rec, &got)
	if len(got) != 1 {
		t.Fatalf("got %d notices, want 1", len(got))
	}
	if got[0].Code != store.NoticeLanguageFallback || got[0].Level != store.NoticeWarning {
		t.Fatalf("unexpected notice: %#v", got[0])
	}
	if got[0].Detail == nil {
		t.Error("want the detail — it is what tells the user what to do")
	}
}

func TestDismissRemovesFromTheActiveList(t *testing.T) {
	ta := newTestAPI(t)
	seedNotice(t, ta.store, "language_fallback:birthday:ru")

	listed := ta.do(t, http.MethodGet, "/api/v1/notices", nil)
	var got []store.Notice
	decodeInto(t, listed, &got)

	requireStatus(t, ta.do(t, http.MethodDelete, "/api/v1/notices/"+itoa(got[0].ID), nil), http.StatusNoContent)

	after := ta.do(t, http.MethodGet, "/api/v1/notices", nil)
	requireStatus(t, after, http.StatusOK)
	if body := after.Body.String(); body != "[]\n" {
		t.Fatalf("body = %q, want the dismissed notice gone", body)
	}
}

// ?all=true is what a "show dismissed" toggle uses; the row is hidden, not gone.
func TestAllIncludesDismissedNotices(t *testing.T) {
	ta := newTestAPI(t)
	seedNotice(t, ta.store, "language_fallback:birthday:ru")

	var got []store.Notice
	decodeInto(t, ta.do(t, http.MethodGet, "/api/v1/notices", nil), &got)
	requireStatus(t, ta.do(t, http.MethodDelete, "/api/v1/notices/"+itoa(got[0].ID), nil), http.StatusNoContent)

	rec := ta.do(t, http.MethodGet, "/api/v1/notices?all=true", nil)
	requireStatus(t, rec, http.StatusOK)

	var all []store.Notice
	decodeInto(t, rec, &all)
	if len(all) != 1 || all[0].DismissedAt == nil {
		t.Fatalf("unexpected list: %#v", all)
	}
}

func TestDismissingAMissingNoticeIs404(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodDelete, "/api/v1/notices/9999", nil)
	requireStatus(t, rec, http.StatusNotFound)
}

func TestDismissWithABadIDIs400(t *testing.T) {
	ta := newTestAPI(t)

	rec := ta.do(t, http.MethodDelete, "/api/v1/notices/abc", nil)
	requireStatus(t, rec, http.StatusBadRequest)
	if got := errorCode(t, rec); got != codeInvalidID {
		t.Fatalf("code = %q, want %q", got, codeInvalidID)
	}
}

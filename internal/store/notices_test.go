package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func sampleNotice() store.Notice {
	detail := "contact 1 speaks ru"
	return store.Notice{
		Key:     "language_fallback:ru",
		Level:   store.NoticeWarning,
		Code:    store.NoticeLanguageFallback,
		Message: "No Russian template; sent the English one instead.",
		Detail:  &detail,
	}
}

func TestRaiseNoticeRoundTrip(t *testing.T) {
	s := openTestStore(t)
	if err := s.RaiseNotice(context.Background(), sampleNotice()); err != nil {
		t.Fatalf("raise: %v", err)
	}

	got, err := s.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d notices, want 1", len(got))
	}
	if got[0].Code != store.NoticeLanguageFallback || got[0].Occurrences != 1 {
		t.Fatalf("unexpected notice: %#v", got[0])
	}
	if got[0].Detail == nil || got[0].FirstSeenAt.IsZero() || got[0].LastSeenAt.IsZero() {
		t.Fatalf("incomplete notice: %#v", got[0])
	}
}

// The same condition recurring must not pile up rows — it fires every year for
// every affected contact until a template is added.
func TestRaisingTheSameKeyCountsInsteadOfDuplicating(t *testing.T) {
	s := openTestStore(t)
	for range 5 {
		if err := s.RaiseNotice(context.Background(), sampleNotice()); err != nil {
			t.Fatalf("raise: %v", err)
		}
	}

	got, err := s.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d notices, want them collapsed into 1", len(got))
	}
	if got[0].Occurrences != 5 {
		t.Fatalf("occurrences = %d, want 5", got[0].Occurrences)
	}
}

func TestDismissHidesFromTheActiveList(t *testing.T) {
	s := openTestStore(t)
	if err := s.RaiseNotice(context.Background(), sampleNotice()); err != nil {
		t.Fatalf("raise: %v", err)
	}
	active, err := s.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if err := s.DismissNotice(context.Background(), active[0].ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	if remaining, listErr := s.ListNotices(context.Background(), true); listErr != nil {
		t.Fatalf("list: %v", listErr)
	} else if len(remaining) != 0 {
		t.Fatalf("%d notices still active after dismissal", len(remaining))
	}
	// It is hidden, not deleted: the history is what lets a recurrence reopen it.
	if all, listErr := s.ListNotices(context.Background(), false); listErr != nil {
		t.Fatalf("list: %v", listErr)
	} else if len(all) != 1 || all[0].DismissedAt == nil {
		t.Fatalf("unexpected full list: %#v", all)
	}
}

// Dismissing acknowledges the last occurrence, not the underlying problem. If it
// happens again the operator needs to see it again.
func TestRecurrenceReopensADismissedNotice(t *testing.T) {
	s := openTestStore(t)
	if err := s.RaiseNotice(context.Background(), sampleNotice()); err != nil {
		t.Fatalf("raise: %v", err)
	}
	active, _ := s.ListNotices(context.Background(), true)
	if err := s.DismissNotice(context.Background(), active[0].ID); err != nil {
		t.Fatalf("dismiss: %v", err)
	}

	if err := s.RaiseNotice(context.Background(), sampleNotice()); err != nil {
		t.Fatalf("re-raise: %v", err)
	}

	reopened, err := s.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(reopened) != 1 {
		t.Fatalf("got %d active notices, want the recurrence to reopen it", len(reopened))
	}
	if reopened[0].Occurrences != 2 {
		t.Fatalf("occurrences = %d, want 2", reopened[0].Occurrences)
	}
}

func TestDismissingAMissingNoticeIsNotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.DismissNotice(context.Background(), 9999); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListNoticesReturnsEmptySliceNotNil(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ListNotices(context.Background(), true)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("want an empty slice, not nil — it is marshalled straight to JSON")
	}
}

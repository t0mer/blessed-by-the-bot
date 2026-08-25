package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

// clearBlessings removes the starter templates seeded by migration 0003 so a
// test can control exactly which ones exist. The seed is real product
// behaviour — a fresh install must be able to send — but it makes "the only
// template is the one I just added" false by default.
func clearBlessings(t *testing.T, st *store.Store) {
	t.Helper()
	all, err := st.ListBlessings(context.Background())
	if err != nil {
		t.Fatalf("listing seeded blessings: %v", err)
	}
	for _, b := range all {
		if err := st.DeleteBlessing(context.Background(), b.ID); err != nil {
			t.Fatalf("deleting seeded blessing %d: %v", b.ID, err)
		}
	}
}

func sampleBlessing() *store.Blessing {
	return &store.Blessing{
		EventType: store.EventBirthday,
		Language:  "he",
		Text:      "יום הולדת שמח {{name}}!",
		Enabled:   true,
	}
}

func TestCreateBlessingRoundTrip(t *testing.T) {
	s := openTestStore(t)
	got, err := s.CreateBlessing(context.Background(), sampleBlessing())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == 0 {
		t.Error("ID not assigned")
	}
	if got.Gender != nil || got.Relation != nil {
		t.Errorf("untargeted blessing got gender=%v relation=%v, want both nil", got.Gender, got.Relation)
	}
	if got.Text != "יום הולדת שמח {{name}}!" {
		t.Errorf("text = %q", got.Text)
	}
}

func TestCreateBlessingWithTargeting(t *testing.T) {
	s := openTestStore(t)
	b := sampleBlessing()
	b.Gender = strPtr(store.GenderFemale)
	b.Relation = strPtr(store.RelationFamily)

	got, err := s.CreateBlessing(context.Background(), b)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Gender == nil || *got.Gender != store.GenderFemale {
		t.Errorf("gender = %v, want female", got.Gender)
	}
	if got.Relation == nil || *got.Relation != store.RelationFamily {
		t.Errorf("relation = %v, want family", got.Relation)
	}
}

func TestGetMissingBlessingReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	if _, err := s.GetBlessing(context.Background(), 777); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListBlessingsEmptyIsNonNil(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ListBlessings(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("empty list is nil; JSON encodes nil as null, want []")
	}
}

func TestUpdateBlessingBumpsUpdatedAt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateBlessing(ctx, sampleBlessing())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	created.Text = "מזל טוב {{name}}"
	created.Gender = strPtr(store.GenderMale)
	created.Enabled = false

	got, err := s.UpdateBlessing(ctx, created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Text != "מזל טוב {{name}}" || got.Enabled {
		t.Errorf("update did not persist: %+v", got)
	}
	if got.Gender == nil || *got.Gender != store.GenderMale {
		t.Errorf("gender = %v, want male", got.Gender)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("updated_at (%v) not after created_at (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestUpdateMissingBlessingReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	b := sampleBlessing()
	b.ID = 888
	if _, err := s.UpdateBlessing(context.Background(), b); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteBlessing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateBlessing(ctx, sampleBlessing())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteBlessing(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetBlessing(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteBlessing(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}

func TestFindBlessingsFiltersByEventTypeAndLanguage(t *testing.T) {
	s := openTestStore(t)
	clearBlessings(t, s)
	ctx := context.Background()

	match := sampleBlessing()
	otherLang := sampleBlessing()
	otherLang.Language = "en"
	otherLang.Text = "Happy birthday {{name}}"
	otherType := sampleBlessing()
	otherType.EventType = store.EventWedding

	for _, b := range []*store.Blessing{match, otherLang, otherType} {
		if _, err := s.CreateBlessing(ctx, b); err != nil {
			t.Fatalf("create: %v", err)
		}
	}

	got, err := s.FindBlessings(ctx, store.EventBirthday, "he")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d blessings, want 1: %+v", len(got), got)
	}
	if got[0].Language != "he" || got[0].EventType != store.EventBirthday {
		t.Errorf("wrong row returned: %+v", got[0])
	}
}

func TestFindBlessingsExcludesDisabled(t *testing.T) {
	s := openTestStore(t)
	clearBlessings(t, s)
	ctx := context.Background()

	disabled := sampleBlessing()
	disabled.Enabled = false
	if _, err := s.CreateBlessing(ctx, disabled); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.FindBlessings(ctx, store.EventBirthday, "he")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("disabled blessing returned: %+v", got)
	}
}

func TestCreateBlessingRejectsInvalidGender(t *testing.T) {
	s := openTestStore(t)
	b := sampleBlessing()
	b.Gender = strPtr("unknown")
	if _, err := s.CreateBlessing(context.Background(), b); err == nil {
		t.Fatal("invalid gender accepted; CHECK constraint is not enforced")
	}
}

package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func strPtr(s string) *string { return &s }

func sampleContact() *store.Contact {
	return &store.Contact{
		Name:       "Dana",
		Phone:      "972501234567",
		EventDate:  "1990-03-14",
		EventType:  store.EventBirthday,
		Language:   "he",
		Relation:   store.RelationFriend,
		Importance: 4,
		Gender:     store.GenderFemale,
		Enabled:    true,
	}
}

func TestCreateContactAssignsIDAndTimestamps(t *testing.T) {
	s := openTestStore(t)
	got, err := s.CreateContact(context.Background(), sampleContact())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == 0 {
		t.Error("ID not assigned")
	}
	if got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Errorf("timestamps not set: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
	if !got.CreatedAt.Equal(got.UpdatedAt) {
		t.Errorf("created (%v) != updated (%v) on a fresh row", got.CreatedAt, got.UpdatedAt)
	}
}

func TestGetContactRoundTrip(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	in := sampleContact()
	in.SendTime = strPtr("07:30")
	created, err := s.CreateContact(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetContact(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != in.Name || got.Phone != in.Phone || got.EventDate != in.EventDate {
		t.Errorf("identity fields changed: %+v", got)
	}
	if got.EventType != in.EventType || got.Language != in.Language || got.Relation != in.Relation {
		t.Errorf("classification fields changed: %+v", got)
	}
	if got.Importance != in.Importance || got.Gender != in.Gender || !got.Enabled {
		t.Errorf("scalar fields changed: %+v", got)
	}
	if got.SendTime == nil || *got.SendTime != "07:30" {
		t.Errorf("send time = %v, want 07:30", got.SendTime)
	}
}

func TestGetContactNilSendTime(t *testing.T) {
	s := openTestStore(t)
	created, err := s.CreateContact(context.Background(), sampleContact())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.SendTime != nil {
		t.Errorf("send time = %v, want nil (not empty string)", *created.SendTime)
	}
}

func TestGetMissingContactReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	_, err := s.GetContact(context.Background(), 4242)
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestListContactsOrderedByName(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	for _, name := range []string{"Yuval", "Avi", "Maya"} {
		c := sampleContact()
		c.Name = name
		if _, err := s.CreateContact(ctx, c); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	got, err := s.ListContacts(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := []string{"Avi", "Maya", "Yuval"}
	if len(got) != len(want) {
		t.Fatalf("got %d contacts, want %d", len(got), len(want))
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("position %d = %q, want %q", i, got[i].Name, name)
		}
	}
}

func TestListContactsEmpty(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ListContacts(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("empty list is nil; JSON encodes nil as null, want []")
	}
	if len(got) != 0 {
		t.Errorf("got %d contacts, want 0", len(got))
	}
}

func TestUpdateContactChangesFieldsAndBumpsUpdatedAt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	created.Name = "Dana Cohen"
	created.Importance = 5
	created.Enabled = false
	created.SendTime = strPtr("21:15")

	got, err := s.UpdateContact(ctx, created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Dana Cohen" || got.Importance != 5 || got.Enabled {
		t.Errorf("update did not persist: %+v", got)
	}
	if got.SendTime == nil || *got.SendTime != "21:15" {
		t.Errorf("send time = %v, want 21:15", got.SendTime)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("updated_at (%v) not after created_at (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestUpdateMissingContactReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	c := sampleContact()
	c.ID = 999
	if _, err := s.UpdateContact(context.Background(), c); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteContact(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateContact(ctx, sampleContact())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteContact(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetContact(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteMissingContactReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	if err := s.DeleteContact(context.Background(), 12345); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateContactRejectsInvalidEventType(t *testing.T) {
	s := openTestStore(t)
	c := sampleContact()
	c.EventType = "nope"
	if _, err := s.CreateContact(context.Background(), c); err == nil {
		t.Fatal("invalid event_type accepted; CHECK constraint is not enforced")
	}
}

func TestCreateContactRejectsImportanceOutOfRange(t *testing.T) {
	s := openTestStore(t)
	c := sampleContact()
	c.Importance = 9
	if _, err := s.CreateContact(context.Background(), c); err == nil {
		t.Fatal("importance 9 accepted; CHECK constraint is not enforced")
	}
}

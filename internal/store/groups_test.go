package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/t0mer/blessed-by-the-bot/internal/store"
)

func intPtr(i int) *int { return &i }

func sampleGroup() *store.Group {
	return &store.Group{
		Name:     "Family",
		ChatID:   "120363012345678901@g.us",
		Language: "he",
		Enabled:  true,
	}
}

func TestCreateGroupRoundTrip(t *testing.T) {
	s := openTestStore(t)
	got, err := s.CreateGroup(context.Background(), sampleGroup())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.ID == 0 {
		t.Error("ID not assigned")
	}
	if got.Threshold != nil {
		t.Errorf("threshold = %v, want nil (use the global default)", *got.Threshold)
	}
	if got.ChatID != "120363012345678901@g.us" || got.Language != "he" || !got.Enabled {
		t.Errorf("round trip changed fields: %+v", got)
	}
}

func TestCreateGroupWithThreshold(t *testing.T) {
	s := openTestStore(t)
	g := sampleGroup()
	g.Threshold = intPtr(5)

	got, err := s.CreateGroup(context.Background(), g)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Threshold == nil || *got.Threshold != 5 {
		t.Errorf("threshold = %v, want 5", got.Threshold)
	}
}

func TestCreateGroupRejectsDuplicateChatID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if _, err := s.CreateGroup(ctx, sampleGroup()); err != nil {
		t.Fatalf("first create: %v", err)
	}
	dup := sampleGroup()
	dup.Name = "Different name, same chat"
	if _, err := s.CreateGroup(ctx, dup); err == nil {
		t.Fatal("duplicate chat_id accepted; UNIQUE constraint is not enforced")
	}
}

func TestGetGroupByChatID(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateGroup(ctx, sampleGroup())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := s.GetGroupByChatID(ctx, created.ChatID)
	if err != nil {
		t.Fatalf("get by chat id: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id = %d, want %d", got.ID, created.ID)
	}
	if _, err := s.GetGroupByChatID(ctx, "nope@g.us"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("unknown chat id: err = %v, want ErrNotFound", err)
	}
}

func TestListGroupsEmptyIsNonNil(t *testing.T) {
	s := openTestStore(t)
	got, err := s.ListGroups(context.Background())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got == nil {
		t.Fatal("empty list is nil; JSON encodes nil as null, want []")
	}
}

func TestUpdateGroupBumpsUpdatedAt(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateGroup(ctx, sampleGroup())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	time.Sleep(2 * time.Millisecond)

	created.Name = "Family & friends"
	created.Threshold = intPtr(2)
	created.Enabled = false

	got, err := s.UpdateGroup(ctx, created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Family & friends" || got.Enabled {
		t.Errorf("update did not persist: %+v", got)
	}
	if got.Threshold == nil || *got.Threshold != 2 {
		t.Errorf("threshold = %v, want 2", got.Threshold)
	}
	if !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("updated_at (%v) not after created_at (%v)", got.UpdatedAt, got.CreatedAt)
	}
}

func TestUpdateMissingGroupReturnsErrNotFound(t *testing.T) {
	s := openTestStore(t)
	g := sampleGroup()
	g.ID = 555
	if _, err := s.UpdateGroup(context.Background(), g); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteGroup(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	created, err := s.CreateGroup(ctx, sampleGroup())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.DeleteGroup(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.GetGroup(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get after delete: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteGroup(ctx, created.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteGroupCascadesWishEvents(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	group, err := s.CreateGroup(ctx, sampleGroup())
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	// Raw SQL: the typed wish-event API arrives in the next commit, and this
	// test is about the schema's ON DELETE CASCADE, not about that API.
	if _, err := s.DB().ExecContext(ctx,
		`INSERT INTO wish_events (group_id, sender_id, message_id, matched, created_at)
		 VALUES (?, 'sender-1', 'msg-1', 'מזל טוב', '2026-08-25T00:00:00Z')`,
		group.ID); err != nil {
		t.Fatalf("insert wish event: %v", err)
	}

	if err := s.DeleteGroup(ctx, group.ID); err != nil {
		t.Fatalf("delete group: %v", err)
	}

	var remaining int
	if err := s.DB().QueryRow(
		`SELECT count(*) FROM wish_events WHERE group_id = ?`, group.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("%d wish events survived the group delete; ON DELETE CASCADE is not working", remaining)
	}
}

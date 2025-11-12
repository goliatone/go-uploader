package uploader

import (
	"reflect"
	"testing"
	"time"
)

func TestChunkSessionStoreCreateAndGet(t *testing.T) {
	now := time.Unix(1700000000, 0)
	store := NewChunkSessionStore(45 * time.Minute)
	store.timeNowFn = func() time.Time {
		return now
	}

	session, err := store.Create(&ChunkSession{
		ID:        "session-1",
		Key:       "path/image.jpg",
		TotalSize: 128,
		PartSize:  64,
	})
	if err != nil {
		t.Fatalf("expected no error creating session, got %v", err)
	}

	if session.CreatedAt != now {
		t.Fatalf("expected CreatedAt to be %v, got %v", now, session.CreatedAt)
	}

	expectedExpiry := now.Add(45 * time.Minute)
	if session.ExpiresAt != expectedExpiry {
		t.Fatalf("expected ExpiresAt to be %v, got %v", expectedExpiry, session.ExpiresAt)
	}

	if session.State != ChunkSessionStateActive {
		t.Fatalf("expected active state, got %s", session.State)
	}

	got, ok := store.Get("session-1")
	if !ok {
		t.Fatalf("expected session to be retrievable")
	}

	if got.ID != "session-1" || got.Key != "path/image.jpg" {
		t.Fatalf("unexpected session data: %#v", got)
	}

	_, err = store.Create(&ChunkSession{
		ID:  "session-1",
		Key: "dup",
	})
	if err == nil {
		t.Fatalf("expected duplicate session error")
	}
}

func TestChunkSessionStoreAddPart(t *testing.T) {
	now := time.Unix(1700000000, 0)
	store := NewChunkSessionStore(time.Hour)
	store.timeNowFn = func() time.Time { return now }

	_, err := store.AddPart("none", ChunkPart{Index: 0})
	if err == nil {
		t.Fatalf("expected error for missing session")
	}

	if _, err := store.Create(&ChunkSession{ID: "session-2", Key: "file"}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	part := ChunkPart{Index: 0, Size: 10}
	updated, err := store.AddPart("session-2", part)
	if err != nil {
		t.Fatalf("expected add part to succeed, got %v", err)
	}

	gotPart, ok := updated.UploadedParts[0]
	if !ok {
		t.Fatalf("expected part index 0 to exist")
	}

	if gotPart.Size != part.Size {
		t.Fatalf("expected part size %d, got %d", part.Size, gotPart.Size)
	}

	if gotPart.UploadedAt.IsZero() {
		t.Fatalf("expected UploadedAt to be set")
	}

	if _, err := store.AddPart("session-2", part); err != ErrChunkPartDuplicate {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestChunkSessionStoreCleanupExpired(t *testing.T) {
	now := time.Unix(1700000000, 0)
	store := NewChunkSessionStore(time.Hour)
	store.timeNowFn = func() time.Time { return now }

	expired := &ChunkSession{
		ID:        "expired",
		Key:       "file",
		CreatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-time.Hour),
		State:     ChunkSessionStateActive,
	}

	active := &ChunkSession{
		ID:        "active",
		Key:       "file",
		CreatedAt: now,
		ExpiresAt: now.Add(time.Hour),
		State:     ChunkSessionStateActive,
	}

	if _, err := store.Create(expired); err != nil {
		t.Fatalf("create expired session: %v", err)
	}

	if _, err := store.Create(active); err != nil {
		t.Fatalf("create active session: %v", err)
	}

	removed := store.CleanupExpired(now)
	expectedRemoved := []string{"expired"}
	if !reflect.DeepEqual(expectedRemoved, removed) {
		t.Fatalf("expected %v removed, got %v", expectedRemoved, removed)
	}

	if _, ok := store.Get("active"); !ok {
		t.Fatalf("expected active session to remain")
	}
}

package shareserver

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFSStoreRoundTrip(t *testing.T) {
	s, err := NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	id, _ := NewID()
	expires := time.Now().Add(1 * time.Hour)
	payload := []byte("fake gzipped bytes")

	if err := s.Put(ctx, id, expires, payload); err != nil {
		t.Fatal(err)
	}
	got, exp, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Errorf("payload mismatch")
	}
	if exp.Unix() != expires.Unix() {
		t.Errorf("expiry mismatch: got %v want %v", exp, expires)
	}
}

func TestFSStoreExpired(t *testing.T) {
	s, _ := NewFSStore(t.TempDir())
	ctx := context.Background()
	id, _ := NewID()
	if err := s.Put(ctx, id, time.Now().Add(-time.Minute), []byte("x")); err != nil {
		t.Fatal(err)
	}
	_, _, err := s.Get(ctx, id)
	if !errors.Is(err, ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestFSStoreNotFound(t *testing.T) {
	s, _ := NewFSStore(t.TempDir())
	_, _, err := s.Get(context.Background(), "abcdefghij")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestFSStoreOverwriteReplaces(t *testing.T) {
	s, _ := NewFSStore(t.TempDir())
	ctx := context.Background()
	id, _ := NewID()
	if err := s.Put(ctx, id, time.Now().Add(time.Hour), []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, id, time.Now().Add(2*time.Hour), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "v2" {
		t.Errorf("want v2, got %q", got)
	}
}

func TestFSStoreListExpired(t *testing.T) {
	s, _ := NewFSStore(t.TempDir())
	ctx := context.Background()
	past, _ := NewID()
	future, _ := NewID()
	if err := s.Put(ctx, past, time.Now().Add(-time.Hour), []byte("p")); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, future, time.Now().Add(time.Hour), []byte("f")); err != nil {
		t.Fatal(err)
	}
	expired, err := s.ListExpired(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(expired) != 1 || expired[0] != past {
		t.Fatalf("want [%s], got %v", past, expired)
	}
}

func TestValidateIDRejectsPathTraversal(t *testing.T) {
	bad := []string{"../evil", "a/b", "", "abc", "foo.bar", "x*y"}
	for _, id := range bad {
		if err := validateID(id); err == nil {
			t.Errorf("expected error for %q", id)
		}
	}
}

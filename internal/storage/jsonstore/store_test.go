package jsonstore

import (
	"context"
	"testing"
	"time"

	"nuntius/internal/core/model"
)

func TestStoreRoundTrip(t *testing.T) {
	store := New(t.TempDir())
	want := model.Snapshot{
		Name:      "home",
		CreatedAt: time.Date(2026, 8, 26, 1, 2, 3, 0, time.UTC),
		Version:   1,
		State: model.NetworkState{
			Hostname: "test-host",
			OS:       "linux",
			Arch:     "amd64",
		},
	}
	if err := store.Save(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	got, err := store.Load(context.Background(), "home")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != want.Name || got.State.Hostname != want.State.Hostname {
		t.Fatalf("round trip mismatch: %#v", got)
	}
}

func TestStoreRejectsUnsafeName(t *testing.T) {
	store := New(t.TempDir())
	err := store.Save(context.Background(), model.Snapshot{Name: "../escape"})
	if err == nil {
		t.Fatal("expected unsafe name to be rejected")
	}
}

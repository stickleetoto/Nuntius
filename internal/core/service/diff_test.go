package service

import (
	"testing"

	"nuntius/internal/core/model"
)

func TestCompareSnapshotsIgnoresDNSOrder(t *testing.T) {
	from := model.Snapshot{Name: "a", State: model.NetworkState{DNS: model.DNSConfig{Servers: []string{"1.1.1.1", "8.8.8.8"}}}}
	to := model.Snapshot{Name: "b", State: model.NetworkState{DNS: model.DNSConfig{Servers: []string{"8.8.8.8", "1.1.1.1"}}}}

	got := CompareSnapshots(from, to)
	if len(got.Changes) != 0 {
		t.Fatalf("expected no changes, got %#v", got.Changes)
	}
}

func TestCompareSnapshotsDetectsAddedDNS(t *testing.T) {
	from := model.Snapshot{Name: "a", State: model.NetworkState{DNS: model.DNSConfig{Servers: []string{"1.1.1.1"}}}}
	to := model.Snapshot{Name: "b", State: model.NetworkState{DNS: model.DNSConfig{Servers: []string{"1.1.1.1", "8.8.8.8"}}}}

	got := CompareSnapshots(from, to)
	if len(got.Changes) != 1 {
		t.Fatalf("expected 1 change, got %#v", got.Changes)
	}
	if got.Changes[0].Kind != "added" || got.Changes[0].After != "8.8.8.8" {
		t.Fatalf("unexpected change: %#v", got.Changes[0])
	}
}

func TestCompareSnapshotsDetectsInterfaceFlagChange(t *testing.T) {
	from := model.Snapshot{Name: "a", State: model.NetworkState{Interfaces: []model.NetworkInterface{{Name: "eth0", Flags: []string{"up"}}}}}
	to := model.Snapshot{Name: "b", State: model.NetworkState{Interfaces: []model.NetworkInterface{{Name: "eth0", Flags: []string{"down"}}}}}

	got := CompareSnapshots(from, to)
	if len(got.Changes) != 2 {
		t.Fatalf("expected removed+added interface state, got %#v", got.Changes)
	}
}

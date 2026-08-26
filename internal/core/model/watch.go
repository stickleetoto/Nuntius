package model

import "time"

type WatchEvent struct {
	ObservedAt time.Time `json:"observed_at"`
	Category   string    `json:"category"`
	Kind       string    `json:"kind"`
	Key        string    `json:"key"`
	Before     string    `json:"before,omitempty"`
	After      string    `json:"after,omitempty"`
}

type WatchBatch struct {
	ObservedAt   time.Time    `json:"observed_at"`
	Events       []WatchEvent `json:"events"`
	SnapshotName string       `json:"snapshot_name,omitempty"`
}

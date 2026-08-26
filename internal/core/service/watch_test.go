package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"nuntius/internal/core/model"
)

type sequenceCollector struct {
	mu     sync.Mutex
	states []model.NetworkState
	index  int
}

func (s *sequenceCollector) Collect(context.Context) (model.NetworkState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.states) == 0 {
		return model.NetworkState{}, nil
	}
	idx := s.index
	if idx >= len(s.states) {
		idx = len(s.states) - 1
	}
	state := s.states[idx]
	s.index++
	return state, nil
}

func TestBuildWatchBatchFiltersConnectionNoise(t *testing.T) {
	before := model.NetworkState{
		DNS:         model.DNSConfig{Servers: []string{"1.1.1.1"}},
		Connections: []model.Connection{{Protocol: "tcp", Local: "a", Remote: "b"}},
	}
	after := model.NetworkState{
		CapturedAt:  time.Unix(100, 0),
		DNS:         model.DNSConfig{Servers: []string{"8.8.8.8"}},
		Connections: []model.Connection{{Protocol: "tcp", Local: "a", Remote: "c"}},
	}
	batch := BuildWatchBatch(before, after, []string{"dns"})
	if len(batch.Events) != 2 {
		t.Fatalf("expected two DNS events, got %#v", batch.Events)
	}
	for _, event := range batch.Events {
		if event.Category != "dns" {
			t.Fatalf("unexpected category: %#v", event)
		}
	}
}

func TestObserveOnceDetectsListener(t *testing.T) {
	collector := &sequenceCollector{states: []model.NetworkState{
		{},
		{CapturedAt: time.Now(), Listeners: []model.Listener{{Protocol: "tcp", Local: "0.0.0.0:8080", PID: 42, Process: "test"}}},
	}}
	watch := WatchService{Collector: collector}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	batch, err := watch.ObserveOnce(ctx, MinWatchInterval, []string{"listener"})
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) != 1 || batch.Events[0].Category != "listener" || batch.Events[0].Kind != "added" {
		t.Fatalf("unexpected batch: %#v", batch)
	}
}

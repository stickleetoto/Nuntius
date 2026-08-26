package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type SnapshotService struct {
	Collector port.Collector
	Storage   port.SnapshotStorage
}

func (s SnapshotService) Create(ctx context.Context, name string) (model.Snapshot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.Snapshot{}, errors.New("snapshot name is required")
	}
	state, err := s.Collector.Collect(ctx)
	if err != nil {
		return model.Snapshot{}, err
	}
	snapshot := model.Snapshot{Name: name, CreatedAt: time.Now().UTC(), Version: 1, State: state}
	if err := s.Storage.Save(ctx, snapshot); err != nil {
		return model.Snapshot{}, err
	}
	return snapshot, nil
}

func (s SnapshotService) Load(ctx context.Context, name string) (model.Snapshot, error) {
	return s.Storage.Load(ctx, name)
}
func (s SnapshotService) List(ctx context.Context) ([]model.Snapshot, error) {
	return s.Storage.List(ctx)
}

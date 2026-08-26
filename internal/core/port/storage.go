package port

import (
	"context"
	"nuntius/internal/core/model"
)

type SnapshotStorage interface {
	Save(ctx context.Context, snapshot model.Snapshot) error
	Load(ctx context.Context, name string) (model.Snapshot, error)
	List(ctx context.Context) ([]model.Snapshot, error)
}

package port

import (
	"context"
	"nuntius/internal/core/model"
)

type Collector interface {
	Collect(ctx context.Context) (model.NetworkState, error)
}

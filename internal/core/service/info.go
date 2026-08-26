package service

import (
	"context"
	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

type InfoService struct{ Collector port.Collector }

func (s InfoService) Get(ctx context.Context) (model.NetworkState, error) {
	return s.Collector.Collect(ctx)
}

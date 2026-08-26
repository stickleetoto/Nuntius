package port

import (
	"context"
	"time"

	"nuntius/internal/core/model"
)

type PathProbe interface {
	Ping(ctx context.Context, target string, count int, timeout time.Duration) (model.PingResult, error)
	Trace(ctx context.Context, target string, maxHops int, timeout time.Duration) (model.TraceResult, error)
}

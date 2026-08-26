package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nuntius/internal/core/model"
)

type fakePathProbe struct {
	ping     model.PingResult
	trace    model.TraceResult
	pingErr  error
	traceErr error
}

func (f fakePathProbe) Ping(context.Context, string, int, time.Duration) (model.PingResult, error) {
	return f.ping, f.pingErr
}
func (f fakePathProbe) Trace(context.Context, string, int, time.Duration) (model.TraceResult, error) {
	return f.trace, f.traceErr
}

func TestPathInspectDegradedOnLoss(t *testing.T) {
	s := PathService{Probe: fakePathProbe{
		ping:  model.PingResult{Sent: 4, Received: 3, LossPercent: 25, AvgMS: 10},
		trace: model.TraceResult{Reached: true},
	}}
	got, err := s.Inspect(context.Background(), "example.com", 4, 20, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Overall != "degraded" {
		t.Fatalf("overall=%q", got.Overall)
	}
}

func TestPathInspectKeepsPartialResult(t *testing.T) {
	s := PathService{Probe: fakePathProbe{
		ping:     model.PingResult{Sent: 4, Received: 4},
		traceErr: errors.New("traceroute missing"),
	}}
	got, err := s.Inspect(context.Background(), "example.com", 4, 20, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ping == nil || len(got.Warnings) != 1 || got.Overall != "degraded" {
		t.Fatalf("unexpected report: %#v", got)
	}
}

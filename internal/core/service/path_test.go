package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"nuntius/internal/core/model"
)

type fakePathProbe struct {
	ping       model.PingResult
	trace      model.TraceResult
	pingErr    error
	traceErr   error
	pingCalls  int
	traceCalls int
	onPing     func()
	onTrace    func()
}

func (f *fakePathProbe) Ping(context.Context, string, int, time.Duration) (model.PingResult, error) {
	f.pingCalls++
	if f.onPing != nil {
		f.onPing()
	}
	return f.ping, f.pingErr
}

func (f *fakePathProbe) Trace(context.Context, string, int, time.Duration) (model.TraceResult, error) {
	f.traceCalls++
	if f.onTrace != nil {
		f.onTrace()
	}
	return f.trace, f.traceErr
}

func TestPathInspectDegradedOnLoss(t *testing.T) {
	s := PathService{Probe: &fakePathProbe{
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
	s := PathService{Probe: &fakePathProbe{
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

func TestPathInspectCanceledBeforeProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	probe := &fakePathProbe{}
	s := PathService{Probe: probe}
	got, err := s.Inspect(ctx, "example.com", 4, 20, time.Second)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if probe.pingCalls != 0 || probe.traceCalls != 0 {
		t.Fatalf("probe calls: ping=%d trace=%d", probe.pingCalls, probe.traceCalls)
	}
	if got.Target != "example.com" || got.StartedAt.IsZero() {
		t.Fatalf("metadata not preserved: %#v", got)
	}
	if got.DurationMS < 0 || got.Overall == "ok" {
		t.Fatalf("unexpected interrupted report: %#v", got)
	}
}

func TestPathInspectExpiredBeforeProbe(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	probe := &fakePathProbe{}
	s := PathService{Probe: probe}
	got, err := s.Inspect(ctx, "example.com", 4, 20, time.Second)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
	if probe.pingCalls != 0 || probe.traceCalls != 0 {
		t.Fatalf("probe calls: ping=%d trace=%d", probe.pingCalls, probe.traceCalls)
	}
	if got.Target != "example.com" || got.StartedAt.IsZero() || got.Overall == "ok" {
		t.Fatalf("unexpected interrupted report: %#v", got)
	}
}

func TestPathInspectCancellationDuringPingSkipsTraceAndKeepsPing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := &fakePathProbe{
		ping: model.PingResult{Sent: 4, Received: 4, AvgMS: 10},
		onPing: func() {
			cancel()
		},
	}
	s := PathService{Probe: probe}
	got, err := s.Inspect(ctx, "example.com", 4, 20, time.Second)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if probe.pingCalls != 1 || probe.traceCalls != 0 {
		t.Fatalf("probe calls: ping=%d trace=%d", probe.pingCalls, probe.traceCalls)
	}
	if got.Ping == nil || got.Trace != nil {
		t.Fatalf("unexpected partial results: %#v", got)
	}
	if got.Target != "example.com" || got.StartedAt.IsZero() || got.DurationMS < 0 || got.Overall == "ok" {
		t.Fatalf("unexpected interrupted report: %#v", got)
	}
}

func TestPathInspectCancellationDuringTraceReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	probe := &fakePathProbe{
		ping:  model.PingResult{Sent: 4, Received: 4, AvgMS: 10},
		trace: model.TraceResult{Reached: true},
		onTrace: func() {
			cancel()
		},
	}
	s := PathService{Probe: probe}
	got, err := s.Inspect(ctx, "example.com", 4, 20, time.Second)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if probe.pingCalls != 1 || probe.traceCalls != 1 {
		t.Fatalf("probe calls: ping=%d trace=%d", probe.pingCalls, probe.traceCalls)
	}
	if got.Ping == nil || got.Trace == nil {
		t.Fatalf("successful probe results not retained: %#v", got)
	}
	if got.Overall == "ok" || got.DurationMS < 0 {
		t.Fatalf("unexpected interrupted report: %#v", got)
	}
}

func TestPathInspectNormalBehavior(t *testing.T) {
	probe := &fakePathProbe{
		ping:  model.PingResult{Sent: 4, Received: 4, AvgMS: 10},
		trace: model.TraceResult{Reached: true},
	}
	s := PathService{Probe: probe}
	got, err := s.Inspect(context.Background(), "example.com", 4, 20, time.Second)

	if err != nil {
		t.Fatal(err)
	}
	if probe.pingCalls != 1 || probe.traceCalls != 1 {
		t.Fatalf("probe calls: ping=%d trace=%d", probe.pingCalls, probe.traceCalls)
	}
	if got.Ping == nil || got.Trace == nil || got.Overall != "ok" || len(got.Warnings) != 0 {
		t.Fatalf("unexpected report: %#v", got)
	}
}

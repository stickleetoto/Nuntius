package service

import (
	"context"
	"fmt"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

const (
	DefaultPingCount    = 4
	DefaultProbeTimeout = 2 * time.Second
	DefaultMaxHops      = 20
)

type PathService struct {
	Probe port.PathProbe
}

func (s PathService) Ping(ctx context.Context, target string, count int, timeout time.Duration) (model.PingResult, error) {
	if s.Probe == nil {
		return model.PingResult{}, fmt.Errorf("path probe is not configured")
	}
	if count <= 0 {
		count = DefaultPingCount
	}
	if count > 100 {
		return model.PingResult{}, fmt.Errorf("ping count must be 100 or less")
	}
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	return s.Probe.Ping(ctx, target, count, timeout)
}

func (s PathService) Trace(ctx context.Context, target string, maxHops int, timeout time.Duration) (model.TraceResult, error) {
	if s.Probe == nil {
		return model.TraceResult{}, fmt.Errorf("path probe is not configured")
	}
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	if maxHops > 64 {
		return model.TraceResult{}, fmt.Errorf("max hops must be 64 or less")
	}
	if timeout <= 0 {
		timeout = DefaultProbeTimeout
	}
	return s.Probe.Trace(ctx, target, maxHops, timeout)
}

func (s PathService) Inspect(ctx context.Context, target string, count, maxHops int, timeout time.Duration) (model.PathReport, error) {
	started := time.Now()
	report := model.PathReport{Target: target, StartedAt: started.UTC(), Overall: "ok"}

	ping, pingErr := s.Ping(ctx, target, count, timeout)
	if pingErr != nil {
		report.Warnings = append(report.Warnings, "ping: "+pingErr.Error())
		report.Overall = "degraded"
	} else {
		report.Ping = &ping
		if ping.Received == 0 {
			report.Overall = "unreachable"
		} else if ping.LossPercent > 0 || ping.JitterMS > 50 {
			report.Overall = "degraded"
		}
	}

	trace, traceErr := s.Trace(ctx, target, maxHops, timeout)
	if traceErr != nil {
		report.Warnings = append(report.Warnings, "trace: "+traceErr.Error())
		if report.Overall == "ok" {
			report.Overall = "degraded"
		}
	} else {
		report.Trace = &trace
		if trace.Reached && report.Overall == "unreachable" {
			// ICMP echo can be filtered independently of general path reachability.
			report.Overall = "degraded"
			report.Warnings = append(report.Warnings, "ICMP echo received no replies, but traceroute reached the target")
		}
		if !trace.Reached && report.Overall == "ok" {
			report.Overall = "degraded"
		}
	}

	report.DurationMS = time.Since(started).Milliseconds()
	return report, nil
}

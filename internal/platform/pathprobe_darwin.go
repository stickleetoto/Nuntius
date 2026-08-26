//go:build darwin

package platform

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"nuntius/internal/core/model"
)

func (p *commandPathProbe) Ping(ctx context.Context, target string, count int, timeout time.Duration) (model.PingResult, error) {
	started := time.Now()
	result := model.PingResult{Target: target, ResolvedIP: resolveTarget(ctx, target), StartedAt: started.UTC(), Sent: count}
	if _, err := lookPath("ping"); err != nil {
		return result, err
	}
	millis := int(timeout.Milliseconds())
	if millis < 100 {
		millis = 100
	}
	for i := 1; i <= count; i++ {
		probeCtx, cancel := context.WithTimeout(ctx, timeout+time.Second)
		out, elapsed, err := runCommand(probeCtx, "ping", "-n", "-c", "1", "-W", strconv.Itoa(millis), target)
		cancel()
		sample := model.PingSample{Sequence: i}
		if err == nil {
			sample.Success = true
			sample.RTTMS = parseRTT(string(out), elapsed)
		} else {
			sample.Error = "timeout or unreachable"
		}
		result.Samples = append(result.Samples, sample)
	}
	summarizePing(&result)
	result.DurationMS = time.Since(started).Milliseconds()
	return result, nil
}

func (p *commandPathProbe) Trace(ctx context.Context, target string, maxHops int, timeout time.Duration) (model.TraceResult, error) {
	started := time.Now()
	targetIP := resolveTarget(ctx, target)
	if _, err := lookPath("traceroute"); err != nil {
		return model.TraceResult{}, err
	}
	seconds := timeout.Seconds()
	if seconds < 1 {
		seconds = 1
	}
	out, _, cmdErr := runCommand(ctx, "traceroute", "-n", "-m", strconv.Itoa(maxHops), "-w", fmt.Sprintf("%.1f", seconds), "-q", "1", target)
	result := parseTraceOutput(string(out), targetIP, "traceroute", maxHops)
	result.Target, result.StartedAt, result.DurationMS = target, started.UTC(), time.Since(started).Milliseconds()
	if cmdErr != nil && len(result.Hops) == 0 {
		return result, fmt.Errorf("traceroute failed: %w", cmdErr)
	}
	return result, nil
}

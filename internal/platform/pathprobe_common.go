package platform

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"nuntius/internal/core/model"
)

var rttPattern = regexp.MustCompile(`(?i)(?:time|시간)?\s*[=<]?\s*([0-9]+(?:[\.,][0-9]+)?)\s*ms`)

type commandPathProbe struct{}

func NewPathProbe() *commandPathProbe { return &commandPathProbe{} }

func resolveTarget(ctx context.Context, target string) string {
	host := target
	if h, _, err := net.SplitHostPort(target); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err == nil && len(ips) > 0 {
		return ips[0].IP.String()
	}
	return ""
}

func runCommand(ctx context.Context, name string, args ...string) ([]byte, time.Duration, error) {
	started := time.Now()
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return out, time.Since(started), err
}

func parseRTT(output string, elapsed time.Duration) float64 {
	if regexp.MustCompile(`(?i)<\s*1\s*ms`).MatchString(output) {
		return 0.5
	}
	matches := rttPattern.FindAllStringSubmatch(output, -1)
	if len(matches) > 0 {
		value := strings.ReplaceAll(matches[0][1], ",", ".")
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return float64(elapsed.Microseconds()) / 1000.0
}

func summarizePing(result *model.PingResult) {
	var values []float64
	for _, sample := range result.Samples {
		if sample.Success {
			values = append(values, sample.RTTMS)
		}
	}
	result.Received = len(values)
	if result.Sent > 0 {
		result.LossPercent = float64(result.Sent-result.Received) * 100 / float64(result.Sent)
	}
	if len(values) == 0 {
		return
	}
	result.MinMS, result.MaxMS = values[0], values[0]
	var sum float64
	for _, v := range values {
		sum += v
		result.MinMS = math.Min(result.MinMS, v)
		result.MaxMS = math.Max(result.MaxMS, v)
	}
	result.AvgMS = sum / float64(len(values))
	if len(values) > 1 {
		var jitter float64
		for i := 1; i < len(values); i++ {
			jitter += math.Abs(values[i] - values[i-1])
		}
		result.JitterMS = jitter / float64(len(values)-1)
	}
}

func parseTraceOutput(output, targetIP, tool string, maxHops int) model.TraceResult {
	result := model.TraceResult{ResolvedIP: targetIP, MaxHops: maxHops, Tool: tool}
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		hopText := strings.TrimSuffix(fields[0], ":")
		hop, err := strconv.Atoi(hopText)
		if err != nil || hop < 1 || hop > maxHops {
			continue
		}
		entry := model.TraceHop{Hop: hop}
		for _, field := range fields[1:] {
			candidate := strings.Trim(field, "[](),")
			if ip := net.ParseIP(candidate); ip != nil {
				entry.Address = ip.String()
			}
		}
		matches := rttPattern.FindAllStringSubmatch(line, -1)
		if len(matches) > 0 {
			var total float64
			var count int
			for _, m := range matches {
				v := strings.ReplaceAll(m[1], ",", ".")
				if f, err := strconv.ParseFloat(v, 64); err == nil {
					total += f
					count++
				}
			}
			if count > 0 {
				entry.RTTMS = total / float64(count)
			}
		}
		if strings.Contains(line, "*") && entry.Address == "" {
			entry.TimedOut = true
		}
		result.Hops = append(result.Hops, entry)
		if targetIP != "" && entry.Address == targetIP {
			result.Reached = true
		}
	}
	return result
}

func commandNotFound(name string) error {
	return fmt.Errorf("required system command %q was not found", name)
}

func lookPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", commandNotFound(name)
	}
	return path, nil
}

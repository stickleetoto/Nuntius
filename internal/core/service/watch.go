package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"nuntius/internal/core/model"
	"nuntius/internal/core/port"
)

const DefaultWatchInterval = 2 * time.Second
const MinWatchInterval = 500 * time.Millisecond

type WatchOptions struct {
	Interval       time.Duration
	Categories     []string
	Count          int
	AutoSnapshot   bool
	SnapshotPrefix string
}

type WatchService struct {
	Collector port.Collector
	Storage   port.SnapshotStorage
}

func (s WatchService) Stream(ctx context.Context, opts WatchOptions, emit func(model.WatchBatch) error) error {
	if emit == nil {
		return errors.New("watch emit callback is required")
	}
	opts = normalizeWatchOptions(opts)
	if opts.Interval < MinWatchInterval {
		return fmt.Errorf("watch interval must be at least %s", MinWatchInterval)
	}
	if opts.Count < 0 {
		return errors.New("watch count cannot be negative")
	}
	if opts.AutoSnapshot && s.Storage == nil {
		return errors.New("watch auto-snapshot requires snapshot storage")
	}

	previous, err := s.Collector.Collect(ctx)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()
	cycles := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			current, err := s.Collector.Collect(ctx)
			if err != nil {
				return err
			}
			batch := BuildWatchBatch(previous, current, opts.Categories)
			if len(batch.Events) > 0 && opts.AutoSnapshot {
				name := watchSnapshotName(opts.SnapshotPrefix, batch.ObservedAt)
				snap := model.Snapshot{Name: name, CreatedAt: batch.ObservedAt.UTC(), Version: 1, State: current}
				if err := s.Storage.Save(ctx, snap); err != nil {
					return err
				}
				batch.SnapshotName = name
			}
			if len(batch.Events) > 0 {
				if err := emit(batch); err != nil {
					return err
				}
			}
			previous = current
			cycles++
			if opts.Count > 0 && cycles >= opts.Count {
				return nil
			}
		}
	}
}

// ObserveOnce captures a baseline, waits for interval, captures again, and returns the resulting event batch.
func (s WatchService) ObserveOnce(ctx context.Context, interval time.Duration, categories []string) (model.WatchBatch, error) {
	if interval <= 0 {
		interval = DefaultWatchInterval
	}
	if interval < MinWatchInterval {
		return model.WatchBatch{}, fmt.Errorf("watch interval must be at least %s", MinWatchInterval)
	}
	before, err := s.Collector.Collect(ctx)
	if err != nil {
		return model.WatchBatch{}, err
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return model.WatchBatch{}, ctx.Err()
	case <-timer.C:
	}
	after, err := s.Collector.Collect(ctx)
	if err != nil {
		return model.WatchBatch{}, err
	}
	return BuildWatchBatch(before, after, categories), nil
}

func BuildWatchBatch(before, after model.NetworkState, categories []string) model.WatchBatch {
	if len(categories) == 0 {
		categories = []string{"interface", "dns", "route", "listener"}
	}
	from := model.Snapshot{Name: "before", State: before}
	to := model.Snapshot{Name: "after", State: after}
	diff := CompareSnapshots(from, to)
	allowed := categorySet(categories)
	observedAt := after.CapturedAt
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	batch := model.WatchBatch{ObservedAt: observedAt.UTC(), Events: []model.WatchEvent{}}
	for _, change := range diff.Changes {
		category := watchCategory(change.Key)
		if len(allowed) > 0 {
			if _, ok := allowed[category]; !ok {
				continue
			}
		}
		batch.Events = append(batch.Events, model.WatchEvent{
			ObservedAt: batch.ObservedAt,
			Category:   category,
			Kind:       change.Kind,
			Key:        change.Key,
			Before:     change.Before,
			After:      change.After,
		})
	}
	sort.SliceStable(batch.Events, func(i, j int) bool {
		if batch.Events[i].Category != batch.Events[j].Category {
			return batch.Events[i].Category < batch.Events[j].Category
		}
		if batch.Events[i].Key != batch.Events[j].Key {
			return batch.Events[i].Key < batch.Events[j].Key
		}
		return batch.Events[i].Kind < batch.Events[j].Kind
	})
	return batch
}

func normalizeWatchOptions(opts WatchOptions) WatchOptions {
	if opts.Interval <= 0 {
		opts.Interval = DefaultWatchInterval
	}
	if len(opts.Categories) == 0 {
		// Active connections are intentionally excluded by default because they are extremely noisy.
		opts.Categories = []string{"interface", "dns", "route", "listener"}
	}
	if strings.TrimSpace(opts.SnapshotPrefix) == "" {
		opts.SnapshotPrefix = "watch"
	}
	return opts
}

func categorySet(categories []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, category := range categories {
		category = strings.ToLower(strings.TrimSpace(category))
		if category == "ports" {
			category = "listener"
		}
		if category != "" {
			out[category] = struct{}{}
		}
	}
	return out
}

func watchCategory(key string) string {
	prefix, _, _ := strings.Cut(key, ".")
	switch prefix {
	case "dns", "route", "interface", "listener", "connection", "host":
		return prefix
	default:
		return "other"
	}
}

func watchSnapshotName(prefix string, t time.Time) string {
	prefix = sanitizeSnapshotPrefix(prefix)
	if prefix == "" {
		prefix = "watch"
	}
	// Layout intentionally avoids ':' so it is valid on Windows and in snapshot names.
	return fmt.Sprintf("%s-%s", prefix, t.UTC().Format("20060102T150405.000000000Z"))
}

func sanitizeSnapshotPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	var b strings.Builder
	lastDash := false
	for _, r := range prefix {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-._")
}

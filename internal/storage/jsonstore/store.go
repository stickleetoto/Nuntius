package jsonstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nuntius/internal/core/model"
)

var safeName = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

type Store struct{ root string }

func New(root string) *Store { return &Store{root: root} }

func DefaultRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv("NUNTIUS_HOME")); v != "" {
		return v, nil
	}
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "Nuntius"), nil
}

func (s *Store) Save(_ context.Context, snapshot model.Snapshot) error {
	if !safeName.MatchString(snapshot.Name) {
		return errors.New("snapshot name may contain only letters, numbers, dot, underscore, and hyphen")
	}
	if err := os.MkdirAll(filepath.Join(s.root, "snapshots"), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}
	path := s.path(snapshot.Name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) Load(_ context.Context, name string) (model.Snapshot, error) {
	if !safeName.MatchString(name) {
		return model.Snapshot{}, errors.New("invalid snapshot name")
	}
	data, err := os.ReadFile(s.path(name))
	if err != nil {
		return model.Snapshot{}, err
	}
	var snap model.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return model.Snapshot{}, fmt.Errorf("decode snapshot %q: %w", name, err)
	}
	return snap, nil
}

func (s *Store) List(ctx context.Context) ([]model.Snapshot, error) {
	dir := filepath.Join(s.root, "snapshots")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []model.Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]model.Snapshot, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".json")
		snap, err := s.Load(ctx, name)
		if err != nil {
			continue
		}
		out = append(out, snap)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) path(name string) string { return filepath.Join(s.root, "snapshots", name+".json") }

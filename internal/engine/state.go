package engine

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// State is the persisted emotion vector.
type State struct {
	Version   int                `json:"version"`
	UpdatedAt time.Time          `json:"updated_at"`
	Axes      map[string]float64 `json:"axes"`
}

// NewState returns a fresh state with every axis at its configured baseline.
func NewState(cfg Config, now time.Time) State {
	axes := make(map[string]float64, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		axes[ax.Name] = ax.Baseline
	}
	return State{Version: 1, UpdatedAt: now, Axes: axes}
}

// LoadState reads the state file. A missing file yields a baseline state.
// A corrupt file is a hard error. Axes absent from the file are filled with
// their configured baseline.
func LoadState(path string, cfg Config, now time.Time) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewState(cfg, now), nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("state file %s is corrupt: %w", path, err)
	}
	if s.Axes == nil {
		s.Axes = map[string]float64{}
	}
	for _, ax := range cfg.Axes {
		if _, ok := s.Axes[ax.Name]; !ok {
			s.Axes[ax.Name] = ax.Baseline
		}
	}
	return s, nil
}

// SaveState writes the state atomically (temp file + rename).
func SaveState(path string, s State) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// WithLock runs fn while holding an exclusive advisory lock tied to the state
// file, so concurrent `feel`/`tick` invocations cannot interleave their
// read-modify-write cycles.
func WithLock(statePath string, fn func() error) error {
	lockPath := statePath + ".lock"
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock %s: %w", lockPath, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

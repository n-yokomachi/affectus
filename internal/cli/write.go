package cli

import (
	"encoding/json"
	"fmt"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// ApplyFeel decays, applies the given deltas, persists, refreshes the optional
// snapshot file with the current axes JSON, and returns the rendered axes JSON
// and axis map. It holds the state lock for the whole read-modify-write cycle.
func ApplyFeel(env Env, deltas map[string]float64) (string, map[string]float64, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", nil, err
	}
	var rendered string
	var axes map[string]float64
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.ApplyDeltas(s, deltas, cfg)
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = engine.Render(s, cfg)
		axes = s.Axes
		return writeFragment(cfg, s)
	})
	if err != nil {
		return "", nil, err
	}
	return rendered, axes, nil
}

// Feel parses a JSON delta object and applies it via ApplyFeel.
func Feel(env Env, deltasJSON string) error {
	var deltas map[string]float64
	if err := json.Unmarshal([]byte(deltasJSON), &deltas); err != nil {
		return fmt.Errorf("invalid deltas JSON: %w", err)
	}
	rendered, _, err := ApplyFeel(env, deltas)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}

// Tick applies time decay only and persists. This is the cron target.
func Tick(env Env) error {
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	return engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		return writeFragment(cfg, s)
	})
}

// Reset returns every axis to its baseline and persists.
func Reset(env Env) error {
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	return engine.WithLock(env.StatePath, func() error {
		s := engine.NewState(cfg, env.Now())
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		return writeFragment(cfg, s)
	})
}

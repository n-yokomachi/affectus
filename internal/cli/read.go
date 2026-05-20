package cli

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// ComputeShow loads state, applies decay for output (without persisting), and
// returns the rendered JSON line and the decayed axis values.
func ComputeShow(env Env) (string, map[string]float64, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", nil, err
	}
	s, err := engine.LoadState(env.StatePath, cfg, env.Now())
	if err != nil {
		return "", nil, err
	}
	s = engine.Decay(s, cfg, env.Now())
	return engine.Render(s, cfg), s.Axes, nil
}

// Show prints the current emotion as text or JSON. Read-only: does not persist.
// text format: one-line JSON of all axes (the Render output).
// json format: {"axes": {...}} pretty-printed.
func Show(env Env, format string) error {
	axesJSON, axes, err := ComputeShow(env)
	if err != nil {
		return err
	}
	switch format {
	case "", "text":
		fmt.Fprintln(env.Stdout, axesJSON)
		return nil
	case "json":
		rounded := make(map[string]float64, len(axes))
		for k, v := range axes {
			rounded[k] = math.Round(v*100) / 100
		}
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"axes": rounded})
	default:
		return fmt.Errorf("unknown format %q (want text|json)", format)
	}
}

// Get prints the decayed raw axis values as JSON. Read-only: does not persist.
func Get(env Env) error {
	_, axes, err := ComputeShow(env)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(env.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(axes)
}

package cli

import (
	"encoding/json"
	"fmt"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// ComputeShow loads state, applies decay for output (without persisting), and
// returns the rendered fragment and the decayed axis values.
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
func Show(env Env, format string) error {
	fragment, axes, err := ComputeShow(env)
	if err != nil {
		return err
	}
	switch format {
	case "", "text":
		fmt.Fprintln(env.Stdout, fragment)
		return nil
	case "json":
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"fragment": fragment, "axes": axes})
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

package cli

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// ComputeShowFull loads state, applies decay for output (without persisting),
// and returns the rendered line, the decayed state, and the config. Callers
// that need the prospect ledger (occ) use this; others use ComputeShow.
func ComputeShowFull(env Env) (string, engine.State, engine.Config, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", engine.State{}, engine.Config{}, err
	}
	s, err := engine.LoadState(env.StatePath, cfg, env.Now())
	if err != nil {
		return "", engine.State{}, engine.Config{}, err
	}
	s = engine.Decay(s, cfg, env.Now())
	return renderLine(cfg, s), s, cfg, nil
}

// ComputeShow loads state, applies decay for output (without persisting), and
// returns the rendered JSON line and the decayed axis values.
func ComputeShow(env Env) (string, map[string]float64, error) {
	line, s, _, err := ComputeShowFull(env)
	if err != nil {
		return "", nil, err
	}
	return line, s.Axes, nil
}

// Show prints the current emotion as text or JSON. Read-only: does not persist.
// text format: one-line JSON of all axes (the Render output).
// json format: {"axes": {...}} pretty-printed; occ also includes "prospects".
func Show(env Env, format string) error {
	line, s, cfg, err := ComputeShowFull(env)
	if err != nil {
		return err
	}
	switch format {
	case "", "text":
		fmt.Fprintln(env.Stdout, line)
		return nil
	case "json":
		rounded := make(map[string]float64, len(s.Axes))
		for k, v := range s.Axes {
			rounded[k] = math.Round(v*100) / 100
		}
		out := map[string]any{"axes": rounded}
		if cfg.Model == "occ" {
			out["prospects"] = s.Prospects
		}
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
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

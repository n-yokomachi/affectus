package engine

import "fmt"

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ApplyDeltas adds self-reported deltas to the state. Each delta is clamped to
// the configured delta range, and each resulting axis value to the axis range.
// An unknown axis name in deltas is an error.
func ApplyDeltas(s State, deltas map[string]float64, cfg Config) (State, error) {
	known := make(map[string]bool, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		known[ax.Name] = true
	}
	for name := range deltas {
		if !known[name] {
			return State{}, fmt.Errorf("unknown axis %q", name)
		}
	}
	axes := make(map[string]float64, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		d := clamp(deltas[ax.Name], cfg.DeltaClamp.Min, cfg.DeltaClamp.Max)
		axes[ax.Name] = clamp(s.Axes[ax.Name]+d, cfg.Clamp.Min, cfg.Clamp.Max)
	}
	return State{Version: s.Version, UpdatedAt: s.UpdatedAt, Axes: axes}, nil
}

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

// axisClamp returns the effective value range for one axis: its own Range
// override when set, otherwise the global Clamp.
func axisClamp(ax AxisConfig, cfg Config) Range {
	if ax.Range != nil {
		return *ax.Range
	}
	return cfg.Clamp
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
		r := axisClamp(ax, cfg)
		axes[ax.Name] = clamp(s.Axes[ax.Name]+d, r.Min, r.Max)
	}
	return State{Version: s.Version, UpdatedAt: s.UpdatedAt, Axes: axes,
		ProspectSeq: s.ProspectSeq, Prospects: s.Prospects,
		ConceptSeq: s.ConceptSeq, Concepts: s.Concepts}, nil
}

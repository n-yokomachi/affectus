package engine

import (
	"math"
	"time"
)

// Decay relaxes every axis toward its baseline by exponential decay based on
// the wall-clock time elapsed since s.UpdatedAt. The returned state's
// UpdatedAt is set to now. Negative elapsed time (clock skew) is treated as 0.
func Decay(s State, cfg Config, now time.Time) State {
	elapsed := now.Sub(s.UpdatedAt).Minutes()
	if elapsed < 0 {
		elapsed = 0
	}
	axes := make(map[string]float64, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		v := s.Axes[ax.Name]
		factor := math.Pow(0.5, elapsed/ax.HalflifeMinutes)
		axes[ax.Name] = ax.Baseline + (v-ax.Baseline)*factor
	}
	return State{Version: s.Version, UpdatedAt: now, Axes: axes}
}

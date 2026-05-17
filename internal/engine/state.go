package engine

import "time"

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

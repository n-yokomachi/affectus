package viz

import (
	"encoding/json"
	"os"
	"time"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// axisValue is one axis in the /state response.
type axisValue struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// clampRange is the value range in the /state response.
type clampRange struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// stateResponse is the JSON body of GET /state.
type stateResponse struct {
	UpdatedAt time.Time   `json:"updated_at"`
	Clamp     clampRange  `json:"clamp"`
	Axes      []axisValue `json:"axes"`
}

// loadConfig loads the config file if present, otherwise the embedded default.
func loadConfig(path string) (engine.Config, error) {
	if _, err := os.Stat(path); err == nil {
		return engine.LoadConfig(path)
	}
	return engine.DefaultConfig()
}

// stateJSON loads the state, decays it to now, and marshals the /state
// response. Axes are emitted in config order.
func stateJSON(cfg engine.Config, statePath string, now time.Time) ([]byte, error) {
	s, err := engine.LoadState(statePath, cfg, now)
	if err != nil {
		return nil, err
	}
	s = engine.Decay(s, cfg, now)
	resp := stateResponse{
		UpdatedAt: s.UpdatedAt,
		Clamp:     clampRange{Min: cfg.Clamp.Min, Max: cfg.Clamp.Max},
		Axes:      make([]axisValue, 0, len(cfg.Axes)),
	}
	for _, ax := range cfg.Axes {
		resp.Axes = append(resp.Axes, axisValue{Name: ax.Name, Value: s.Axes[ax.Name]})
	}
	return json.MarshalIndent(resp, "", "  ")
}

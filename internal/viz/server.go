package viz

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return engine.DefaultConfig()
	}
	return engine.LoadConfig(path)
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

//go:embed assets/index.html
var indexHTML []byte

// newMux builds the HTTP handler for the viz server.
func newMux(cfg engine.Config, statePath string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		b, err := stateJSON(cfg, statePath, time.Now())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	})
	return mux
}

// Serve loads the config and runs the visualization HTTP server on
// localhost:port. It blocks until the process is interrupted.
func Serve(configPath, statePath string, port int) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return err
	}
	addr := fmt.Sprintf("localhost:%d", port)
	fmt.Printf("affectus viz running at http://%s  (Ctrl-C to stop)\n", addr)
	return http.ListenAndServe(addr, newMux(cfg, statePath))
}

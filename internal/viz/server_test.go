package viz

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-yokomachi/affectus/internal/engine"
)

func TestStateJSONDecaysToNow(t *testing.T) {
	cfg, _ := engine.DefaultConfig()
	statePath := filepath.Join(t.TempDir(), "state.json")
	base := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)

	s := engine.NewState(cfg, base)
	s.Axes["joy"] = 0.8
	if err := engine.SaveState(statePath, s); err != nil {
		t.Fatal(err)
	}

	// query one halflife (90 min) later -> joy ~0.4
	b, err := stateJSON(cfg, statePath, base.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var joy float64
	for _, ax := range resp.Axes {
		if ax.Name == "joy" {
			joy = ax.Value
		}
	}
	if joy < 0.39 || joy > 0.41 {
		t.Errorf("joy after one halflife = %v, want ~0.4", joy)
	}
}

func TestStateJSONAxisOrderMatchesConfig(t *testing.T) {
	cfg, _ := engine.DefaultConfig()
	b, err := stateJSON(cfg, filepath.Join(t.TempDir(), "missing.json"), time.Now())
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Axes) != len(cfg.Axes) {
		t.Fatalf("got %d axes, want %d", len(resp.Axes), len(cfg.Axes))
	}
	for i, ax := range cfg.Axes {
		if resp.Axes[i].Name != ax.Name {
			t.Errorf("axis %d = %q, want %q", i, resp.Axes[i].Name, ax.Name)
		}
	}
}

func TestLoadConfigFallsBackToDefault(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if len(cfg.Axes) != 8 {
		t.Errorf("default config should have 8 axes, got %d", len(cfg.Axes))
	}
}

func TestMuxServesIndex(t *testing.T) {
	cfg, _ := engine.DefaultConfig()
	mux := newMux(cfg, filepath.Join(t.TempDir(), "s.json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 200 {
		t.Fatalf("GET / = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("index body is empty")
	}
}

func TestMuxServesState(t *testing.T) {
	cfg, _ := engine.DefaultConfig()
	mux := newMux(cfg, filepath.Join(t.TempDir(), "s.json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/state", nil))
	if rec.Code != 200 {
		t.Fatalf("GET /state = %d", rec.Code)
	}
	var resp stateResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("/state is not valid JSON: %v", err)
	}
	if len(resp.Axes) != 8 {
		t.Errorf("got %d axes, want 8", len(resp.Axes))
	}
}

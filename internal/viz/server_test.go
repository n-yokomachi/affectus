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

func TestStateJSONIncludesProspects(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["occ"])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	s := engine.NewState(cfg, now)
	s.ProspectSeq = 1
	s.Prospects = []engine.Prospect{{ID: "p1", Label: "pr merge", Desirability: 0.6, Likelihood: 0.7, CreatedAt: now}}
	if err := engine.SaveState(statePath, s); err != nil {
		t.Fatal(err)
	}
	b, err := stateJSON(cfg, statePath, now)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `"prospects"`) || !strings.Contains(out, `"p1"`) || !strings.Contains(out, `"model": "occ"`) {
		t.Errorf("state JSON missing occ fields: %s", out)
	}
}

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

func TestMuxServes404(t *testing.T) {
	cfg, _ := engine.DefaultConfig()
	mux := newMux(cfg, filepath.Join(t.TempDir(), "s.json"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/nonexistent", nil))
	if rec.Code != 404 {
		t.Errorf("GET /nonexistent = %d, want 404", rec.Code)
	}
}

func TestStateJSONPlutchikRangeFallsBackToClamp(t *testing.T) {
	cfg, _ := engine.DefaultConfig() // plutchik: no per-axis ranges
	b, err := stateJSON(cfg, filepath.Join(t.TempDir(), "missing.json"), time.Now())
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "plutchik" {
		t.Errorf("model = %q, want plutchik", resp.Model)
	}
	for _, ax := range resp.Axes {
		if ax.Range.Min != 0.0 || ax.Range.Max != 1.0 {
			t.Errorf("axis %q range = %+v, want global clamp {0,1}", ax.Name, ax.Range)
		}
	}
}

func TestStateJSONBarrettIncludesConcepts(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	s := engine.NewState(cfg, now)
	s.ConceptSeq = 1
	s.Concepts = []engine.Concept{{
		ID: "c1", Label: "quiet joy", Vector: make([]float64, 14),
		Valence: 0.5, Arousal: 0.4, Importance: 0.42,
		CreatedAt: now, LastRecalled: now,
	}}
	if err := engine.SaveState(statePath, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	b, err := stateJSON(cfg, statePath, now)
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Model != "barrett" {
		t.Errorf("model = %q, want barrett", resp.Model)
	}
	if len(resp.Concepts) != 1 {
		t.Fatalf("got %d concepts, want 1", len(resp.Concepts))
	}
	c := resp.Concepts[0]
	if c.ID != "c1" {
		t.Errorf("concept ID = %q, want c1", c.ID)
	}
	if c.Label != "quiet joy" {
		t.Errorf("concept Label = %q, want %q", c.Label, "quiet joy")
	}
	if c.Valence != 0.5 {
		t.Errorf("concept Valence = %v, want 0.5", c.Valence)
	}
	if c.Arousal != 0.4 {
		t.Errorf("concept Arousal = %v, want 0.4", c.Arousal)
	}
	if c.Importance != 0.42 {
		t.Errorf("concept Importance = %v, want 0.42", c.Importance)
	}
	if !c.CreatedAt.Equal(now) {
		t.Errorf("concept CreatedAt = %v, want %v", c.CreatedAt, now)
	}
	if !c.LastRecalled.Equal(now) {
		t.Errorf("concept LastRecalled = %v, want %v", c.LastRecalled, now)
	}
	if resp.CultureMap == "" {
		t.Error("CultureMap is empty, want non-empty (from embedded barrett default config)")
	}
}

func TestStateJSONIncludesModelAndPerAxisRange(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["russell"])
	if err != nil {
		t.Fatalf("russell config: %v", err)
	}
	b, err := stateJSON(cfg, filepath.Join(t.TempDir(), "missing.json"), time.Now())
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "russell" {
		t.Errorf("model = %q, want russell", resp.Model)
	}
	byName := map[string]axisValue{}
	for _, ax := range resp.Axes {
		byName[ax.Name] = ax
	}
	if byName["valence"].Range.Min != -1.0 || byName["valence"].Range.Max != 1.0 {
		t.Errorf("valence range = %+v, want {-1,1}", byName["valence"].Range)
	}
	if byName["arousal"].Range.Min != 0.0 || byName["arousal"].Range.Max != 1.0 {
		t.Errorf("arousal range = %+v, want {0,1}", byName["arousal"].Range)
	}
}

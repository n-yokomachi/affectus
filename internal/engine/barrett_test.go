package engine

import (
	"strings"
	"testing"
	"time"
)

func barrettCfg(t *testing.T) Config {
	t.Helper()
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse barrett default config: %v", err)
	}
	return cfg
}

var barrettNow = time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

// namedVec returns a full named vector with every dim at base, overridden by kv.
func namedVec(t *testing.T, cfg Config, base float64, kv map[string]float64) map[string]float64 {
	t.Helper()
	m := make(map[string]float64, len(cfg.Barrett.VectorDims))
	for _, d := range cfg.Barrett.VectorDims {
		m[d] = base
	}
	for k, v := range kv {
		m[k] = v
	}
	return m
}

func TestBarrettVectorMapsNamedToDimOrder(t *testing.T) {
	cfg := barrettCfg(t)
	named := namedVec(t, cfg, 0, map[string]float64{"valence": 0.5, "novelty": 0.9})
	vec, err := BarrettVector(named, cfg)
	if err != nil {
		t.Fatalf("BarrettVector: %v", err)
	}
	if len(vec) != 14 {
		t.Fatalf("len = %d, want 14", len(vec))
	}
	if vec[0] != 0.5 { // vector_dims[0] = valence
		t.Errorf("vec[0] = %v, want 0.5", vec[0])
	}
	if vec[13] != 0.9 { // vector_dims[13] = novelty
		t.Errorf("vec[13] = %v, want 0.9", vec[13])
	}
}

func TestBarrettVectorRejectsMissingAndUnknownDims(t *testing.T) {
	cfg := barrettCfg(t)
	missing := namedVec(t, cfg, 0, nil)
	delete(missing, "fairness")
	if _, err := BarrettVector(missing, cfg); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("want missing-dimension error, got %v", err)
	}
	unknown := namedVec(t, cfg, 0, nil)
	unknown["moxie"] = 1
	if _, err := BarrettVector(unknown, cfg); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("want unknown-dimension error, got %v", err)
	}
}

func TestBarrettVectorRequiresBarrettModel(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	if _, err := BarrettVector(map[string]float64{}, cfg); err == nil || !strings.Contains(err.Error(), "barrett") {
		t.Errorf("want barrett-model error, got %v", err)
	}
}

func TestCosineRelevance(t *testing.T) {
	a := []float64{1, 0, 0}
	tests := []struct {
		name string
		b    []float64
		want float64
	}{
		{"identical", []float64{1, 0, 0}, 1.0},
		{"opposite", []float64{-1, 0, 0}, 0.0},
		{"orthogonal", []float64{0, 1, 0}, 0.5},
		{"zero vector", []float64{0, 0, 0}, 0.0},
	}
	for _, tt := range tests {
		if got := cosineRelevance(a, tt.b); !almostEqual(got, tt.want) {
			t.Errorf("%s: relevance = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConceptImportance(t *testing.T) {
	cfg := barrettCfg(t)
	// Neutral point = axis baselines (valence 0.0, arousal 0.3).
	if got := conceptImportance(0.0, 0.3, cfg); !almostEqual(got, 0) {
		t.Errorf("neutral importance = %v, want 0", got)
	}
	// Farthest corner: valence ±1 (dist 1.0), arousal 1.0 (dist 0.7) -> importance 1.
	if got := conceptImportance(-1.0, 1.0, cfg); !almostEqual(got, 1) {
		t.Errorf("extreme importance = %v, want 1", got)
	}
	// Monotonic: farther from neutral = more important.
	mild := conceptImportance(0.2, 0.4, cfg)
	strong := conceptImportance(0.8, 0.9, cfg)
	if !(mild > 0 && strong > mild && strong < 1) {
		t.Errorf("want 0 < mild(%v) < strong(%v) < 1", mild, strong)
	}
}

func TestMinMaxNorm(t *testing.T) {
	got := minMaxNorm([]float64{2, 4, 3})
	want := []float64{0, 1, 0.5}
	for i := range want {
		if !almostEqual(got[i], want[i]) {
			t.Fatalf("norm[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	// Degenerate: all equal -> all zeros (component drops out of ranking).
	for _, v := range minMaxNorm([]float64{7, 7, 7}) {
		if v != 0 {
			t.Fatalf("degenerate norm = %v, want all zeros", v)
		}
	}
}

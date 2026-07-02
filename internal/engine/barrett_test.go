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

// storeWith returns a barrett state holding the given concepts.
func storeWith(t *testing.T, cfg Config, cs ...Concept) State {
	t.Helper()
	s := NewState(cfg, barrettNow)
	s.ConceptSeq = len(cs)
	s.Concepts = cs
	return s
}

// unitConcept builds a concept whose vector is all zeros except dims[i]=1.
func unitConcept(t *testing.T, cfg Config, id string, dim string, created time.Time) Concept {
	t.Helper()
	vec := make([]float64, len(cfg.Barrett.VectorDims))
	found := false
	for i, d := range cfg.Barrett.VectorDims {
		if d == dim {
			vec[i] = 1
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown dim %q", dim)
	}
	return Concept{ID: id, Label: dim, Vector: vec, Importance: 0.5, CreatedAt: created, LastRecalled: created}
}

func TestRecallConceptsRanksByRelevance(t *testing.T) {
	cfg := barrettCfg(t)
	// Same recency and importance; only relevance differs.
	a := unitConcept(t, cfg, "c1", "novelty", barrettNow)
	b := unitConcept(t, cfg, "c2", "fairness", barrettNow)
	s := storeWith(t, cfg, a, b)
	query, err := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"fairness": 1}), cfg)
	if err != nil {
		t.Fatalf("BarrettVector: %v", err)
	}
	_, recalled, err := RecallConcepts(s, query, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if len(recalled) != 2 || recalled[0].ID != "c2" {
		t.Fatalf("want c2 ranked first, got %+v", recalled)
	}
}

func TestRecallConceptsTopKAndBump(t *testing.T) {
	cfg := barrettCfg(t)
	cfg.Barrett.RecallK = 1
	a := unitConcept(t, cfg, "c1", "novelty", barrettNow)
	b := unitConcept(t, cfg, "c2", "fairness", barrettNow)
	s := storeWith(t, cfg, a, b)
	query, _ := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"fairness": 1}), cfg)
	later := barrettNow.Add(10 * time.Minute)
	s2, recalled, err := RecallConcepts(s, query, cfg, later)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if len(recalled) != 1 || recalled[0].ID != "c2" {
		t.Fatalf("want top-1 = c2, got %+v", recalled)
	}
	// Returned entry's LastRecalled bumped; the other untouched.
	for _, c := range s2.Concepts {
		if c.ID == "c2" && !c.LastRecalled.Equal(later) {
			t.Errorf("c2 LastRecalled = %v, want %v", c.LastRecalled, later)
		}
		if c.ID == "c1" && !c.LastRecalled.Equal(barrettNow) {
			t.Errorf("c1 LastRecalled = %v, want unchanged %v", c.LastRecalled, barrettNow)
		}
	}
	// Caller's original state must not be mutated (no slice aliasing).
	for _, c := range s.Concepts {
		if !c.LastRecalled.Equal(barrettNow) {
			t.Errorf("original state mutated: %s LastRecalled = %v", c.ID, c.LastRecalled)
		}
	}
}

func TestRecallConceptsRecencyBreaksTies(t *testing.T) {
	cfg := barrettCfg(t)
	// Identical vectors and importance; c2 recalled more recently -> ranks first.
	old := unitConcept(t, cfg, "c1", "novelty", barrettNow.Add(-48*time.Hour))
	fresh := unitConcept(t, cfg, "c2", "novelty", barrettNow)
	s := storeWith(t, cfg, old, fresh)
	query, _ := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"novelty": 1}), cfg)
	_, recalled, err := RecallConcepts(s, query, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if recalled[0].ID != "c2" {
		t.Fatalf("want fresher c2 first, got %+v", recalled)
	}
}

func TestRecallConceptsStoreOffAndEdges(t *testing.T) {
	cfg := barrettCfg(t)
	query, _ := BarrettVector(namedVec(t, cfg, 0, nil), cfg)

	// recall_k = 0: store-off, no results, no bumps.
	off := barrettCfg(t)
	off.Barrett.RecallK = 0
	s := storeWith(t, off, unitConcept(t, off, "c1", "novelty", barrettNow))
	s2, recalled, err := RecallConcepts(s, query, off, barrettNow.Add(time.Hour))
	if err != nil || recalled != nil {
		t.Fatalf("store-off: want (nil, nil), got %v %v", recalled, err)
	}
	if !s2.Concepts[0].LastRecalled.Equal(barrettNow) {
		t.Error("store-off must not bump LastRecalled")
	}

	// Empty store: no results.
	if _, recalled, err := RecallConcepts(NewState(cfg, barrettNow), query, cfg, barrettNow); err != nil || len(recalled) != 0 {
		t.Fatalf("empty store: want no results, got %v %v", recalled, err)
	}

	// Wrong model gate.
	pl, _ := DefaultConfig()
	if _, _, err := RecallConcepts(NewState(pl, barrettNow), query, pl, barrettNow); err == nil {
		t.Error("want barrett-model error")
	}

	// Query length mismatch.
	if _, _, err := RecallConcepts(NewState(cfg, barrettNow), []float64{1, 2}, cfg, barrettNow); err == nil {
		t.Error("want dimension-count error")
	}

	// Stored vector with stale length: relevance 0, no panic.
	stale := Concept{ID: "c9", Label: "old", Vector: []float64{1}, CreatedAt: barrettNow, LastRecalled: barrettNow}
	s3 := storeWith(t, cfg, stale)
	if _, _, err := RecallConcepts(s3, query, cfg, barrettNow); err != nil {
		t.Fatalf("stale vector must not error: %v", err)
	}
}

func TestRememberConceptAppendsWithSnapshot(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	s.Axes["valence"] = -0.6
	s.Axes["arousal"] = 0.9
	vec, _ := BarrettVector(namedVec(t, cfg, 0.1, map[string]float64{"anger-face": 0.8}), cfg)
	s2, err := RememberConcept(s, "frustration", vec, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RememberConcept: %v", err)
	}
	if len(s2.Concepts) != 1 || s2.ConceptSeq != 1 {
		t.Fatalf("want 1 concept seq=1, got n=%d seq=%d", len(s2.Concepts), s2.ConceptSeq)
	}
	c := s2.Concepts[0]
	if c.ID != "c1" || c.Label != "frustration" {
		t.Errorf("id/label = %s/%s, want c1/frustration", c.ID, c.Label)
	}
	if c.Valence != -0.6 || c.Arousal != 0.9 {
		t.Errorf("snapshot = (%v, %v), want (-0.6, 0.9)", c.Valence, c.Arousal)
	}
	if want := conceptImportance(-0.6, 0.9, cfg); !almostEqual(c.Importance, want) {
		t.Errorf("importance = %v, want %v", c.Importance, want)
	}
	if !c.CreatedAt.Equal(barrettNow) || !c.LastRecalled.Equal(barrettNow) {
		t.Errorf("timestamps: created=%v lastRecalled=%v, want both %v", c.CreatedAt, c.LastRecalled, barrettNow)
	}
}

func TestRememberConceptEvictsLowestScore(t *testing.T) {
	cfg := barrettCfg(t)
	cfg.Barrett.MaxConcepts = 2
	s := NewState(cfg, barrettNow)
	// c1: old AND unimportant -> lowest recency+importance, must be evicted.
	old := unitConcept(t, cfg, "c1", "novelty", barrettNow.Add(-72*time.Hour))
	old.Importance = 0.05
	// c2: old but very important -> survives.
	keeper := unitConcept(t, cfg, "c2", "fairness", barrettNow.Add(-72*time.Hour))
	keeper.Importance = 0.95
	s.Concepts = []Concept{old, keeper}
	s.ConceptSeq = 2

	vec, _ := BarrettVector(namedVec(t, cfg, 0.2, nil), cfg)
	s2, err := RememberConcept(s, "fresh", vec, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RememberConcept: %v", err)
	}
	if len(s2.Concepts) != 2 {
		t.Fatalf("want 2 concepts after eviction, got %d", len(s2.Concepts))
	}
	ids := map[string]bool{}
	for _, c := range s2.Concepts {
		ids[c.ID] = true
	}
	if ids["c1"] || !ids["c2"] || !ids["c3"] {
		t.Errorf("want c1 evicted, c2+c3 kept; got %v", ids)
	}
}

func TestRememberConceptValidation(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	vec, _ := BarrettVector(namedVec(t, cfg, 0, nil), cfg)
	if _, err := RememberConcept(s, "", vec, cfg, barrettNow); err == nil || !strings.Contains(err.Error(), "label") {
		t.Errorf("want label-required error, got %v", err)
	}
	if _, err := RememberConcept(s, "x", []float64{1}, cfg, barrettNow); err == nil {
		t.Error("want dimension-count error")
	}
	pl, _ := DefaultConfig()
	if _, err := RememberConcept(NewState(pl, barrettNow), "x", vec, pl, barrettNow); err == nil {
		t.Error("want barrett-model error")
	}
}

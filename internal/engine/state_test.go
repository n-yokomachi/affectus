package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadStateMissingFileReturnsBaseline(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "state.json")
	s, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqual(s.Axes["joy"], 0.0) || !s.UpdatedAt.Equal(now) {
		t.Fatalf("missing file should give baseline state, got %+v", s)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "sub", "state.json")
	s := NewState(cfg, now)
	s.Axes["joy"] = 0.42
	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if !almostEqual(got.Axes["joy"], 0.42) {
		t.Fatalf("joy round-trip = %v, want 0.42", got.Axes["joy"])
	}
}

func TestLoadStateCorruptFileErrors(t *testing.T) {
	cfg, _ := DefaultConfig()
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadState(path, cfg, time.Now())
	if err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("want corrupt error, got %v", err)
	}
}

func TestLoadStateFillsMissingAxes(t *testing.T) {
	cfg, _ := DefaultConfig()
	path := filepath.Join(t.TempDir(), "state.json")
	// state file missing the "trust" axis entirely
	if err := os.WriteFile(path, []byte(`{"version":1,"updated_at":"2026-05-17T12:00:00Z","axes":{"joy":0.5}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := LoadState(path, cfg, time.Now())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if _, ok := s.Axes["trust"]; !ok {
		t.Fatal("missing axis should be filled with baseline")
	}
}

func TestStateProspectsRoundTrip(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := NewState(cfg, now)
	s.ProspectSeq = 2
	s.Prospects = []Prospect{
		{ID: "p2", Label: "pr merge", Desirability: 0.6, Likelihood: 0.7, CreatedAt: now},
	}
	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.ProspectSeq != 2 || len(got.Prospects) != 1 || got.Prospects[0].ID != "p2" {
		t.Errorf("prospects not round-tripped: %+v", got)
	}
}

func TestStateWithoutProspectsOmitsFields(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := SaveState(path, NewState(cfg, now)); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "prospect") {
		t.Errorf("plutchik state file must not contain prospect fields: %s", data)
	}
}

func TestDecayAndApplyPreserveProspects(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ProspectSeq = 1
	s.Prospects = []Prospect{{ID: "p1", Label: "x", Desirability: 0.5, Likelihood: 0.5, CreatedAt: now}}

	s = Decay(s, cfg, now.Add(10*time.Minute))
	if s.ProspectSeq != 1 || len(s.Prospects) != 1 {
		t.Fatalf("Decay dropped prospects: %+v", s)
	}
	s, err := ApplyDeltas(s, map[string]float64{"joy": 0.1}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.ProspectSeq != 1 || len(s.Prospects) != 1 {
		t.Fatalf("ApplyDeltas dropped prospects: %+v", s)
	}
}

func TestStateConceptsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ConceptSeq = 2
	s.Concepts = []Concept{{
		ID: "c2", Label: "frustration",
		Vector:  []float64{0.1, 0.9, 0, 0.5, 0, 0, 0, 0, 0.2, 0.1, 0.8, 0.3, 0.4, 0.6},
		Valence: -0.4, Arousal: 0.7, Importance: 0.55,
		CreatedAt: now, LastRecalled: now,
	}}
	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.ConceptSeq != 2 || len(got.Concepts) != 1 {
		t.Fatalf("round trip lost concepts: seq=%d n=%d", got.ConceptSeq, len(got.Concepts))
	}
	c := got.Concepts[0]
	if c.ID != "c2" || c.Label != "frustration" || len(c.Vector) != 14 {
		t.Errorf("concept fields lost: %+v", c)
	}
}

func TestStateWithoutConceptsOmitsKeys(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	b, err := json.Marshal(NewState(cfg, now))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "concept") {
		t.Errorf("plutchik state must not contain concept keys, got %s", b)
	}
}

func TestDecayAndApplyPreserveConcepts(t *testing.T) {
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ConceptSeq = 1
	s.Concepts = []Concept{{ID: "c1", Label: "calm", Vector: make([]float64, 14), CreatedAt: now, LastRecalled: now}}

	d := Decay(s, cfg, now.Add(30*time.Minute))
	if d.ConceptSeq != 1 || len(d.Concepts) != 1 {
		t.Fatalf("Decay dropped concepts: seq=%d n=%d", d.ConceptSeq, len(d.Concepts))
	}
	a, err := ApplyDeltas(d, map[string]float64{"valence": 0.2}, cfg)
	if err != nil {
		t.Fatalf("ApplyDeltas: %v", err)
	}
	if a.ConceptSeq != 1 || len(a.Concepts) != 1 {
		t.Fatalf("ApplyDeltas dropped concepts: seq=%d n=%d", a.ConceptSeq, len(a.Concepts))
	}
}

func TestWithLockSerializesWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	var mu sync.Mutex
	counter := 0
	overlap := 0
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = WithLock(path, func() error {
				mu.Lock()
				counter++
				if counter > 1 {
					overlap++
				}
				mu.Unlock()
				time.Sleep(time.Millisecond)
				mu.Lock()
				counter--
				mu.Unlock()
				return nil
			})
		}()
	}
	wg.Wait()
	if overlap != 0 {
		t.Fatalf("WithLock allowed %d overlapping critical sections", overlap)
	}
}

package engine

import (
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

package engine

import (
	"testing"
	"time"
)

func decayTestConfig(t *testing.T) Config {
	t.Helper()
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	return cfg
}

func TestNewStateUsesBaseline(t *testing.T) {
	cfg := decayTestConfig(t)
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	if !s.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %v, want %v", s.UpdatedAt, now)
	}
	for _, ax := range cfg.Axes {
		if !almostEqual(s.Axes[ax.Name], ax.Baseline) {
			t.Errorf("axis %q = %v, want baseline %v", ax.Name, s.Axes[ax.Name], ax.Baseline)
		}
	}
}

func TestDecayHalflife(t *testing.T) {
	cfg := decayTestConfig(t) // all halflife 90 min, baseline 0
	start := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	s := State{Version: 1, UpdatedAt: start, Axes: map[string]float64{"joy": 0.8}}

	cases := []struct {
		name      string
		elapseMin int
		want      float64
	}{
		{"zero elapsed", 0, 0.8},
		{"one halflife", 90, 0.4},
		{"two halflives", 180, 0.2},
		{"clock skew negative", -30, 0.8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := start.Add(time.Duration(tc.elapseMin) * time.Minute)
			out := Decay(s, cfg, now)
			if !almostEqual(out.Axes["joy"], tc.want) {
				t.Fatalf("joy after %d min = %v, want %v", tc.elapseMin, out.Axes["joy"], tc.want)
			}
			if !out.UpdatedAt.Equal(now) && tc.elapseMin >= 0 {
				t.Fatalf("UpdatedAt = %v, want %v", out.UpdatedAt, now)
			}
		})
	}
}

func TestDecayTowardNonZeroBaseline(t *testing.T) {
	cfg := decayTestConfig(t)
	cfg.Axes[0].Baseline = 0.2 // joy baseline 0.2
	start := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	s := State{Version: 1, UpdatedAt: start, Axes: map[string]float64{"joy": 0.8}}
	out := Decay(s, cfg, start.Add(90*time.Minute))
	// 0.2 + (0.8-0.2)*0.5 = 0.5
	if !almostEqual(out.Axes["joy"], 0.5) {
		t.Fatalf("joy = %v, want 0.5", out.Axes["joy"])
	}
}

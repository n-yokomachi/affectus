package engine

import (
	"strings"
	"testing"
	"time"
)

func TestApplyDeltasBasic(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.3
	out, err := ApplyDeltas(s, map[string]float64{"joy": 0.2, "surprise": 0.1}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqual(out.Axes["joy"], 0.5) {
		t.Errorf("joy = %v, want 0.5", out.Axes["joy"])
	}
	if !almostEqual(out.Axes["surprise"], 0.1) {
		t.Errorf("surprise = %v, want 0.1", out.Axes["surprise"])
	}
}

func TestApplyDeltasClampsToAxisRange(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.9
	s.Axes["anger"] = 0.1
	out, err := ApplyDeltas(s, map[string]float64{"joy": 0.5, "anger": -0.5}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqual(out.Axes["joy"], 1.0) {
		t.Errorf("joy = %v, want clamped to 1.0", out.Axes["joy"])
	}
	if !almostEqual(out.Axes["anger"], 0.0) {
		t.Errorf("anger = %v, want clamped to 0.0", out.Axes["anger"])
	}
}

func TestApplyDeltasClampsDelta(t *testing.T) {
	cfg, _ := DefaultConfig() // delta_clamp -1..1
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.0
	// delta 5.0 is clamped to 1.0, then 0.0+1.0 clamped to 1.0
	out, err := ApplyDeltas(s, map[string]float64{"joy": 5.0}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqual(out.Axes["joy"], 1.0) {
		t.Errorf("joy = %v, want 1.0", out.Axes["joy"])
	}
}

func TestApplyDeltasUnknownAxis(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	_, err := ApplyDeltas(s, map[string]float64{"glee": 0.2}, cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown axis") {
		t.Fatalf("want unknown axis error, got %v", err)
	}
}

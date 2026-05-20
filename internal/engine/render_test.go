package engine

import (
	"strings"
	"testing"
	"time"
)

func TestRenderAllAxesInConfigOrder(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.50
	s.Axes["trust"] = 0.40
	s.Axes["surprise"] = 0.20
	got := Render(s, cfg)
	// Must be a single-line JSON with all 8 axes in config order
	want := `{"joy":0.50,"trust":0.40,"fear":0.00,"surprise":0.20,"sadness":0.00,"disgust":0.00,"anger":0.00,"anticipation":0.00}`
	if got != want {
		t.Fatalf("Render =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderTwoDecimalPlaces(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.333
	got := Render(s, cfg)
	if !strings.Contains(got, `"joy":0.33`) {
		t.Fatalf("expected 2 decimal places for joy in %q", got)
	}
}

func TestRenderZeroValuesIncluded(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.8
	got := Render(s, cfg)
	if !strings.Contains(got, `"sadness":0.00`) {
		t.Fatalf("zero sadness axis missing in %q", got)
	}
}

func TestRenderEmptyStateAllZeros(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now()) // all axes at baseline (0.0)
	got := Render(s, cfg)
	want := `{"joy":0.00,"trust":0.00,"fear":0.00,"surprise":0.00,"sadness":0.00,"disgust":0.00,"anger":0.00,"anticipation":0.00}`
	if got != want {
		t.Fatalf("Render empty =\n%q\nwant\n%q", got, want)
	}
}

func TestRenderNoSpecialEmptyString(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	got := Render(s, cfg)
	// Must be JSON, not a prose "calm and even" string
	if !strings.HasPrefix(got, "{") || !strings.HasSuffix(got, "}") {
		t.Fatalf("expected JSON object, got %q", got)
	}
}

func TestRenderSingleAxisConfig(t *testing.T) {
	cfg := Config{
		Clamp:      Range{Min: 0.0, Max: 1.0},
		DeltaClamp: Range{Min: -1.0, Max: 1.0},
		Axes: []AxisConfig{
			{Name: "joy", Baseline: 0.0, HalflifeMinutes: 90},
		},
	}
	s := State{Axes: map[string]float64{"joy": 0.75}}
	got := Render(s, cfg)
	if got != `{"joy":0.75}` {
		t.Fatalf("Render single axis = %q", got)
	}
}

package engine

import (
	"testing"
	"time"
)

func TestRenderMultipleAxes(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.8
	s.Axes["anticipation"] = 0.5
	s.Axes["trust"] = 0.2
	got := Render(s, cfg)
	want := "Right now you feel a strong sense of joy, a moderate sense of anticipation and a faint trace of trust."
	if got != want {
		t.Fatalf("Render = %q\nwant %q", got, want)
	}
}

func TestRenderEmpty(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now()) // all 0, below threshold 0.15
	got := Render(s, cfg)
	if got != "Right now you feel calm and even." {
		t.Fatalf("Render empty = %q", got)
	}
}

func TestRenderRespectsMaxAxes(t *testing.T) {
	cfg, _ := DefaultConfig() // max_axes 3
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.9
	s.Axes["trust"] = 0.8
	s.Axes["fear"] = 0.7
	s.Axes["anger"] = 0.6 // 4th highest, must be dropped
	got := Render(s, cfg)
	want := "Right now you feel a strong sense of joy, a strong sense of trust and a strong sense of unease."
	if got != want {
		t.Fatalf("Render = %q\nwant %q", got, want)
	}
}

func TestRenderSingleAxis(t *testing.T) {
	cfg, _ := DefaultConfig()
	s := NewState(cfg, time.Now())
	s.Axes["surprise"] = 0.30 // <= 0.35 -> "a faint trace of "
	got := Render(s, cfg)
	if got != "Right now you feel a faint trace of surprise." {
		t.Fatalf("Render = %q", got)
	}
}

func TestRenderUsesConfiguredConjunction(t *testing.T) {
	// Japanese-style joining: separator "、" and conjunction "と", with band
	// labels carrying no trailing space.
	cfg, _ := DefaultConfig()
	cfg.Render.Conjunction = "と"
	cfg.Render.Separator = "、"
	cfg.Render.Template = "いまは{clauses}。"
	cfg.Render.Bands = []Band{{Max: 1.0, Label: "強い"}}
	cfg.Render.AxisPhrases = map[string]string{"joy": "喜び", "trust": "信頼感"}
	s := NewState(cfg, time.Now())
	s.Axes["joy"] = 0.9
	s.Axes["trust"] = 0.8
	got := Render(s, cfg)
	want := "いまは強い喜びと強い信頼感。"
	if got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
}

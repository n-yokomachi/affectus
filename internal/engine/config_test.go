package engine

import (
	"strings"
	"testing"
)

const minimalConfig = `
version: 1
clamp:       { min: 0.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }
axes:
  - { name: joy,     baseline: 0.0, halflife_minutes: 90, opposite: sadness }
  - { name: sadness, baseline: 0.0, halflife_minutes: 90, opposite: joy }
render:
  threshold: 0.15
  max_axes: 3
  bands:
    - { max: 0.5, label: "a bit of " }
    - { max: 1.0, label: "a lot of " }
  template: "You feel {clauses}."
  empty: "You feel calm."
  conjunction: " and "
  separator: ", "
  axis_phrases: { joy: "joy", sadness: "sadness" }
fragment_file: ""
`

func TestParseConfigValid(t *testing.T) {
	cfg, err := ParseConfig([]byte(minimalConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Axes) != 2 {
		t.Fatalf("expected 2 axes, got %d", len(cfg.Axes))
	}
	if cfg.Axes[0].Name != "joy" || cfg.Axes[0].HalflifeMinutes != 90 {
		t.Fatalf("axis 0 parsed wrong: %+v", cfg.Axes[0])
	}
	if cfg.Render.MaxAxes != 3 {
		t.Fatalf("render.max_axes parsed wrong: %d", cfg.Render.MaxAxes)
	}
}

func TestValidateErrors(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"no axes", func(c *Config) { c.Axes = nil }, "no axes"},
		{"dup axis", func(c *Config) { c.Axes[1].Name = "joy" }, "duplicate axis"},
		{"bad halflife", func(c *Config) { c.Axes[0].HalflifeMinutes = 0 }, "halflife"},
		{"bad opposite", func(c *Config) { c.Axes[0].Opposite = "nope" }, "opposite"},
		{"clamp", func(c *Config) { c.Clamp.Min = 1.0; c.Clamp.Max = 1.0 }, "clamp min"},
		{"max_axes", func(c *Config) { c.Render.MaxAxes = 0 }, "max_axes"},
		{"no bands", func(c *Config) { c.Render.Bands = nil }, "bands must not"},
		{"bands order", func(c *Config) {
			c.Render.Bands = []Band{{Max: 1.0, Label: "x"}, {Max: 0.5, Label: "y"}}
		}, "ascending"},
		{"no clauses placeholder", func(c *Config) { c.Render.Template = "static text" }, "{clauses}"},
		{"missing axis phrase", func(c *Config) { delete(c.Render.AxisPhrases, "joy") }, "axis_phrases"},
		{"bad threshold", func(c *Config) { c.Render.Threshold = -0.1 }, "threshold"},
		{"duplicate band max", func(c *Config) {
			c.Render.Bands = []Band{{Max: 0.5, Label: "x"}, {Max: 0.5, Label: "y"}}
		}, "ascending"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(minimalConfig))
			if err != nil {
				t.Fatalf("base config invalid: %v", err)
			}
			tc.mutate(&cfg)
			err = cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("want error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

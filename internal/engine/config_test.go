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
}

func TestParseConfigWithObsoleteRenderBlockSucceeds(t *testing.T) {
	// Configs with a leftover render: block from v0.2 should parse without error.
	cfgWithRender := minimalConfig + `
render:
  threshold: 0.15
  max_axes: 3
`
	_, err := ParseConfig([]byte(cfgWithRender))
	if err != nil {
		t.Fatalf("config with obsolete render block should succeed, got: %v", err)
	}
}

func TestValidateRangeAndBaseline(t *testing.T) {
	good := Config{
		Version:    1,
		Clamp:      Range{Min: -1.0, Max: 1.0},
		DeltaClamp: Range{Min: -1.0, Max: 1.0},
		Axes: []AxisConfig{
			{Name: "valence", Baseline: 0.0, HalflifeMinutes: 90, Range: &Range{Min: -1.0, Max: 1.0}},
			{Name: "arousal", Baseline: 0.3, HalflifeMinutes: 90, Range: &Range{Min: 0.0, Max: 1.0}},
		},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid range config rejected: %v", err)
	}

	badRange := good
	badRange.Axes = []AxisConfig{{Name: "x", Baseline: 0.0, HalflifeMinutes: 90, Range: &Range{Min: 1.0, Max: 1.0}}}
	if err := badRange.Validate(); err == nil || !strings.Contains(err.Error(), "range min") {
		t.Fatalf("want range min error, got %v", err)
	}

	badBaseline := good
	badBaseline.Axes = []AxisConfig{{Name: "arousal", Baseline: -0.2, HalflifeMinutes: 90, Range: &Range{Min: 0.0, Max: 1.0}}}
	if err := badBaseline.Validate(); err == nil || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("want baseline-out-of-range error, got %v", err)
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
		{"delta_clamp", func(c *Config) { c.DeltaClamp.Min = 0.0; c.DeltaClamp.Max = 0.0 }, "delta_clamp min"},
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

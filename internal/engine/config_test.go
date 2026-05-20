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

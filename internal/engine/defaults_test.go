package engine

import "testing"

func TestDefaultConfig(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
	if len(cfg.Axes) != 8 {
		t.Fatalf("expected 8 default axes, got %d", len(cfg.Axes))
	}
	for _, ax := range cfg.Axes {
		if ax.HalflifeMinutes != 90 {
			t.Errorf("axis %q halflife = %v, want 90 (symmetric decay)", ax.Name, ax.HalflifeMinutes)
		}
	}
}

func TestDefaultConfigYAMLNotEmpty(t *testing.T) {
	if len(DefaultConfigYAML) == 0 {
		t.Fatal("DefaultConfigYAML is empty; embed failed")
	}
}

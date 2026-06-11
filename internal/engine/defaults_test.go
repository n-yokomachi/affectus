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

func TestDefaultConfigAxisOrder(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	want := []string{"joy", "trust", "fear", "surprise", "sadness", "disgust", "anger", "anticipation"}
	if len(cfg.Axes) != len(want) {
		t.Fatalf("got %d axes, want %d", len(cfg.Axes), len(want))
	}
	for i, name := range want {
		if cfg.Axes[i].Name != name {
			t.Errorf("axis %d = %q, want %q", i, cfg.Axes[i].Name, name)
		}
	}
}

func TestModelsRegistry(t *testing.T) {
	for _, name := range []string{"plutchik", "russell", "occ"} {
		yaml, ok := Models[name]
		if !ok {
			t.Fatalf("Models[%q] missing", name)
		}
		if len(yaml) == 0 {
			t.Fatalf("Models[%q] is empty; embed failed", name)
		}
		cfg, err := ParseConfig(yaml)
		if err != nil {
			t.Fatalf("Models[%q] invalid config: %v", name, err)
		}
		if cfg.Model != name {
			t.Errorf("Models[%q] declares model %q", name, cfg.Model)
		}
	}
}

func TestRussellConfigShape(t *testing.T) {
	cfg, err := ParseConfig(Models["russell"])
	if err != nil {
		t.Fatalf("russell config: %v", err)
	}
	if len(cfg.Axes) != 2 {
		t.Fatalf("russell axes = %d, want 2", len(cfg.Axes))
	}
	want := []string{"valence", "arousal"}
	for i, name := range want {
		if cfg.Axes[i].Name != name {
			t.Errorf("axis %d = %q, want %q", i, cfg.Axes[i].Name, name)
		}
		if cfg.Axes[i].Range == nil {
			t.Errorf("axis %q should have an explicit range", name)
		}
	}
	if !almostEqual(cfg.Axes[1].Baseline, 0.3) {
		t.Errorf("arousal baseline = %v, want 0.3", cfg.Axes[1].Baseline)
	}
}

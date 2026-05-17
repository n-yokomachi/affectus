package engine

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Range is an inclusive numeric range used for clamping.
type Range struct {
	Min float64 `yaml:"min"`
	Max float64 `yaml:"max"`
}

// AxisConfig defines one emotion axis.
type AxisConfig struct {
	Name            string  `yaml:"name"`
	Baseline        float64 `yaml:"baseline"`
	HalflifeMinutes float64 `yaml:"halflife_minutes"`
	Opposite        string  `yaml:"opposite"`
}

// Band maps an upper-bound intensity to an adverb label.
type Band struct {
	Max float64 `yaml:"max"`
	// Label must include any spacing needed between it and the axis phrase
	// (English uses a trailing space, e.g. "a strong sense of "; Japanese none).
	Label string `yaml:"label"`
}

// RenderConfig controls vector-to-natural-language rendering.
type RenderConfig struct {
	Threshold   float64           `yaml:"threshold"`
	MaxAxes     int               `yaml:"max_axes"`
	Bands       []Band            `yaml:"bands"`
	Template    string            `yaml:"template"`
	Empty       string            `yaml:"empty"`
	Conjunction string            `yaml:"conjunction"`
	Separator   string            `yaml:"separator"`
	AxisPhrases map[string]string `yaml:"axis_phrases"`
}

// Config is the full library configuration.
type Config struct {
	Version      int          `yaml:"version"`
	Clamp        Range        `yaml:"clamp"`
	DeltaClamp   Range        `yaml:"delta_clamp"`
	Axes         []AxisConfig `yaml:"axes"`
	Render       RenderConfig `yaml:"render"`
	FragmentFile string       `yaml:"fragment_file"`
}

// ParseConfig unmarshals YAML and validates it.
func ParseConfig(data []byte) (Config, error) {
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// LoadConfig reads and parses a config file from disk.
func LoadConfig(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("config: reading %q: %w", path, err)
	}
	return ParseConfig(data)
}

// Validate checks the config for internal consistency.
func (c Config) Validate() error {
	if len(c.Axes) == 0 {
		return fmt.Errorf("config: no axes defined")
	}
	seen := map[string]bool{}
	for _, ax := range c.Axes {
		if seen[ax.Name] {
			return fmt.Errorf("config: duplicate axis %q", ax.Name)
		}
		seen[ax.Name] = true
		if ax.HalflifeMinutes <= 0 {
			return fmt.Errorf("config: axis %q has non-positive halflife_minutes", ax.Name)
		}
	}
	for _, ax := range c.Axes {
		if ax.Opposite != "" && !seen[ax.Opposite] {
			return fmt.Errorf("config: axis %q opposite %q is not a defined axis", ax.Name, ax.Opposite)
		}
	}
	if c.Clamp.Min >= c.Clamp.Max {
		return fmt.Errorf("config: clamp min must be less than max")
	}
	if c.DeltaClamp.Min >= c.DeltaClamp.Max {
		return fmt.Errorf("config: delta_clamp min must be less than max")
	}
	if c.Render.MaxAxes <= 0 {
		return fmt.Errorf("config: render.max_axes must be positive")
	}
	if len(c.Render.Bands) == 0 {
		return fmt.Errorf("config: render.bands must not be empty")
	}
	for i := 1; i < len(c.Render.Bands); i++ {
		if c.Render.Bands[i].Max < c.Render.Bands[i-1].Max {
			return fmt.Errorf("config: render.bands must be ascending by max")
		}
	}
	return nil
}

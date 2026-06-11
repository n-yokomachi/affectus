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
	// Opposite names the polar-opposite axis (e.g. joy <-> sadness). Reserved
	// for future use: validated for referential integrity but not yet consumed
	// by decay, apply, or render logic.
	Opposite string `yaml:"opposite"`
	// Range optionally overrides the global Clamp for this axis. nil means the
	// axis uses Config.Clamp. Used by models with asymmetric axis domains
	// (e.g. Russell: valence -1..1, arousal 0..1).
	Range *Range `yaml:"range"`
}

// OCCGains holds per-branch conversion strengths from appraisal values to
// emotion deltas (occ model).
type OCCGains struct {
	Wellbeing   float64 `yaml:"wellbeing"`
	Prospect    float64 `yaml:"prospect"`
	Fortunes    float64 `yaml:"fortunes"`
	Attribution float64 `yaml:"attribution"`
	Attraction  float64 `yaml:"attraction"`
	Compound    float64 `yaml:"compound"`
}

// OCCConfig is the occ-model section of the config. nil for other models.
type OCCConfig struct {
	Gains        OCCGains `yaml:"gains"`
	MaxProspects int      `yaml:"max_prospects"`
}

// Config is the full library configuration.
type Config struct {
	Version int `yaml:"version"`
	// Model identifies the emotion model ("plutchik" | "russell"). Optional;
	// empty means the legacy default (Plutchik). Used by viz to choose a
	// rendering and as profile self-description.
	Model        string       `yaml:"model"`
	Clamp        Range        `yaml:"clamp"`
	DeltaClamp   Range        `yaml:"delta_clamp"`
	Axes         []AxisConfig `yaml:"axes"`
	OCC          *OCCConfig   `yaml:"occ"`
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
	if c.Clamp.Min >= c.Clamp.Max {
		return fmt.Errorf("config: clamp min must be less than max")
	}
	if c.DeltaClamp.Min >= c.DeltaClamp.Max {
		return fmt.Errorf("config: delta_clamp min must be less than max")
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
		lo, hi := c.Clamp.Min, c.Clamp.Max
		if ax.Range != nil {
			if ax.Range.Min >= ax.Range.Max {
				return fmt.Errorf("config: axis %q range min must be less than max", ax.Name)
			}
			lo, hi = ax.Range.Min, ax.Range.Max
		}
		if ax.Baseline < lo || ax.Baseline > hi {
			return fmt.Errorf("config: axis %q baseline %v outside range [%v, %v]", ax.Name, ax.Baseline, lo, hi)
		}
	}
	for _, ax := range c.Axes {
		if ax.Opposite != "" && !seen[ax.Opposite] {
			return fmt.Errorf("config: axis %q opposite %q is not a defined axis", ax.Name, ax.Opposite)
		}
	}
	if c.Model == "occ" {
		if c.OCC == nil {
			return fmt.Errorf("config: model occ requires an occ section")
		}
		g := c.OCC.Gains
		gains := map[string]float64{
			"wellbeing": g.Wellbeing, "prospect": g.Prospect, "fortunes": g.Fortunes,
			"attribution": g.Attribution, "attraction": g.Attraction, "compound": g.Compound,
		}
		for name, v := range gains {
			if v < 0 {
				return fmt.Errorf("config: occ gain %s must be non-negative", name)
			}
		}
		if c.OCC.MaxProspects <= 0 {
			return fmt.Errorf("config: occ max_prospects must be positive")
		}
		for _, name := range OCCAxisNames {
			if !seen[name] {
				return fmt.Errorf("config: model occ requires axis %q", name)
			}
		}
	}
	return nil
}

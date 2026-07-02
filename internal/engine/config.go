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

// BarrettWeights holds the ranking weights of the concept-store retrieval
// score (barrett model).
type BarrettWeights struct {
	Relevance  float64 `yaml:"relevance"`
	Recency    float64 `yaml:"recency"`
	Importance float64 `yaml:"importance"`
}

// BarrettConfig is the barrett-model section of the config. nil for other
// models.
type BarrettConfig struct {
	// VectorDims declares the named dimensions of the retrieval vector.
	// v1 convention: the 14 emotion-concept attributes of Li et al. 2024.
	VectorDims []string `yaml:"vector_dims"`
	// RecallK is the number of experiences recall returns. 0 = store-off
	// (recall always returns an empty list; the eval control-group switch).
	RecallK     int `yaml:"recall_k"`
	MaxConcepts int `yaml:"max_concepts"`
	// HalflifeMinutes is the retrieval-recency half-life. It decays an
	// entry's search weight, never its stored values.
	HalflifeMinutes float64        `yaml:"concept_halflife_minutes"`
	Weights         BarrettWeights `yaml:"weights"`
	Distance        string         `yaml:"distance"` // "" | "cosine"
	CultureMap      string         `yaml:"culture_map"`
}

// Config is the full library configuration.
type Config struct {
	Version int `yaml:"version"`
	// Model identifies the emotion model ("plutchik" | "russell" | "occ"). Optional;
	// empty means the legacy default (Plutchik). Used by viz to choose a
	// rendering and as profile self-description.
	Model        string         `yaml:"model"`
	Clamp        Range          `yaml:"clamp"`
	DeltaClamp   Range          `yaml:"delta_clamp"`
	Axes         []AxisConfig   `yaml:"axes"`
	OCC          *OCCConfig     `yaml:"occ"`
	Barrett      *BarrettConfig `yaml:"barrett"`
	FragmentFile string         `yaml:"fragment_file"`
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
	if c.Model == "barrett" {
		if c.Barrett == nil {
			return fmt.Errorf("config: model barrett requires a barrett section")
		}
		b := c.Barrett
		for _, name := range []string{"valence", "arousal"} {
			if !seen[name] {
				return fmt.Errorf("config: model barrett requires axis %q", name)
			}
		}
		if len(b.VectorDims) == 0 {
			return fmt.Errorf("config: barrett vector_dims must not be empty")
		}
		seenDim := map[string]bool{}
		for _, d := range b.VectorDims {
			if seenDim[d] {
				return fmt.Errorf("config: duplicate vector dimension %q", d)
			}
			seenDim[d] = true
		}
		if b.RecallK < 0 {
			return fmt.Errorf("config: barrett recall_k must be non-negative")
		}
		if b.MaxConcepts <= 0 {
			return fmt.Errorf("config: barrett max_concepts must be positive")
		}
		if b.HalflifeMinutes <= 0 {
			return fmt.Errorf("config: barrett concept_halflife_minutes must be positive")
		}
		w := b.Weights
		if w.Relevance < 0 || w.Recency < 0 || w.Importance < 0 {
			return fmt.Errorf("config: barrett weights must be non-negative")
		}
		switch b.Distance {
		case "", "cosine":
		default:
			return fmt.Errorf("config: barrett distance %q is not supported (want cosine)", b.Distance)
		}
	}
	return nil
}

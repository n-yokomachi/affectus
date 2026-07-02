package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// ApplyFeel decays, applies the given deltas, persists, refreshes the optional
// snapshot file with the current axes JSON, and returns the rendered axes JSON
// and axis map. It holds the state lock for the whole read-modify-write cycle.
func ApplyFeel(env Env, deltas map[string]float64) (string, map[string]float64, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", nil, err
	}
	var rendered string
	var axes map[string]float64
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.ApplyDeltas(s, deltas, cfg)
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = renderLine(cfg, s)
		axes = s.Axes
		return writeFragment(cfg, s)
	})
	if err != nil {
		return "", nil, err
	}
	return rendered, axes, nil
}

// Feel parses a JSON delta object and applies it via ApplyFeel.
func Feel(env Env, deltasJSON string) error {
	var deltas map[string]float64
	if err := json.Unmarshal([]byte(deltasJSON), &deltas); err != nil {
		return fmt.Errorf("invalid deltas JSON: %w", err)
	}
	rendered, _, err := ApplyFeel(env, deltas)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}

// Tick applies time decay only and persists. This is the cron target.
func Tick(env Env) error {
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	return engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		return writeFragment(cfg, s)
	})
}

// Reset returns every axis to its baseline and persists.
func Reset(env Env) error {
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	return engine.WithLock(env.StatePath, func() error {
		s := engine.NewState(cfg, env.Now())
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		return writeFragment(cfg, s)
	})
}

// ApplyAppraise decays, applies the appraisal through the OCC rules, persists,
// refreshes the optional snapshot file, and returns the rendered line and the
// new state. It holds the state lock for the whole read-modify-write cycle.
func ApplyAppraise(env Env, a engine.Appraisal) (string, engine.State, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", engine.State{}, err
	}
	var rendered string
	var out engine.State
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.ApplyAppraisal(s, a, cfg, env.Now())
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = renderLine(cfg, s)
		out = s
		return writeFragment(cfg, s)
	})
	if err != nil {
		return "", engine.State{}, err
	}
	return rendered, out, nil
}

// Appraise parses an appraisal JSON object and applies it via ApplyAppraise.
// Unknown fields are rejected so LLM typos fail loudly instead of silently
// dropping part of the appraisal.
func Appraise(env Env, appraisalJSON string) error {
	dec := json.NewDecoder(strings.NewReader(appraisalJSON))
	dec.DisallowUnknownFields()
	var a engine.Appraisal
	if err := dec.Decode(&a); err != nil {
		return fmt.Errorf("invalid appraisal JSON: %w", err)
	}
	rendered, _, err := ApplyAppraise(env, a)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}

// rememberPayload is the remember command input: the category the LLM
// constructed this turn plus the named-key retrieval vector.
type rememberPayload struct {
	Label  string             `json:"label"`
	Vector map[string]float64 `json:"vector"`
}

// Recall retrieves the top-K experiences for a named-key query vector, bumps
// their LastRecalled (retrieval reinforcement), persists when anything was
// recalled, and prints {"axes":...,"recalled":[...],"culture_map":"..."}.
// With recall_k: 0 (store-off) or an empty store it is effectively
// read-only: nothing is bumped and the state file is left untouched.
func Recall(env Env, queryJSON string) error {
	var named map[string]float64
	if err := json.Unmarshal([]byte(queryJSON), &named); err != nil {
		return fmt.Errorf("invalid query JSON: %w", err)
	}
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	query, err := engine.BarrettVector(named, cfg)
	if err != nil {
		return err
	}
	var out string
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, recalled, err := engine.RecallConcepts(s, query, cfg, env.Now())
		if err != nil {
			return err
		}
		if len(recalled) > 0 {
			if err := engine.SaveState(env.StatePath, s); err != nil {
				return err
			}
		}
		out = engine.RenderRecall(s, recalled, cfg)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, out)
	return nil
}

// Remember stores one experience in the barrett concept store: it decays to
// now, snapshots the current core affect, appends the entry (evicting over
// max_concepts), persists, and prints the rendered line. Unknown fields are
// rejected so LLM typos fail loudly.
func Remember(env Env, payloadJSON string) error {
	dec := json.NewDecoder(strings.NewReader(payloadJSON))
	dec.DisallowUnknownFields()
	var p rememberPayload
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("invalid remember JSON: %w", err)
	}
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	vec, err := engine.BarrettVector(p.Vector, cfg)
	if err != nil {
		return err
	}
	var rendered string
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.RememberConcept(s, p.Label, vec, cfg, env.Now())
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = renderLine(cfg, s)
		return writeFragment(cfg, s)
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}

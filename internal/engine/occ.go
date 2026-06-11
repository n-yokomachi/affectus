package engine

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// OCCAxisNames is the full 22-emotion axis set of the OCC model
// (Ortony, Clore & Collins 1988), grouped by appraisal branch.
var OCCAxisNames = []string{
	// well-being (consequences of events, for self, actual)
	"joy", "distress",
	// prospect-based (for self, uncertain, then resolved)
	"hope", "fear", "satisfaction", "disappointment", "relief", "fears-confirmed",
	// fortunes-of-others (consequences for others x liking)
	"happy-for", "pity", "resentment", "gloating",
	// attribution (actions of agents x praiseworthiness)
	"pride", "shame", "admiration", "reproach",
	// attraction (aspects of objects)
	"love", "hate",
	// compounds (well-being x attribution)
	"gratification", "gratitude", "remorse", "anger",
}

// Resolution outcomes.
const (
	OutcomeConfirmed    = "confirmed"
	OutcomeDisconfirmed = "disconfirmed"
	OutcomeDropped      = "dropped"
)

// Appraisal is one self-reported cognitive appraisal (occ model). The LLM
// reports how it construes an event/action/object; emotions are derived
// deterministically by ApplyAppraisal. All sections are optional, but at
// least one must be present.
type Appraisal struct {
	Consequence *ConsequenceAppraisal `json:"consequence,omitempty"`
	Action      *ActionAppraisal      `json:"action,omitempty"`
	Object      *ObjectAppraisal      `json:"object,omitempty"`
	Resolve     []Resolution          `json:"resolve,omitempty"`
}

// ConsequenceAppraisal appraises the consequences of an event.
type ConsequenceAppraisal struct {
	// Desirability in [-1, 1]. When For is "other" it is the desirability
	// for the other party.
	Desirability float64 `json:"desirability"`
	// For is "self" (default) or "other".
	For string `json:"for,omitempty"`
	// Likelihood in (0, 1) marks an uncertain prospect; nil or 1.0 is an
	// actual (certain) event.
	Likelihood *float64 `json:"likelihood,omitempty"`
	// Label is a short description stored in the prospect ledger. Required
	// when Likelihood < 1.
	Label string `json:"label,omitempty"`
	// Liking in [-1, 1] is the momentary liking for the other party.
	// Required when For is "other".
	Liking *float64 `json:"liking,omitempty"`
}

// ActionAppraisal appraises an agent's action against standards.
type ActionAppraisal struct {
	Praiseworthiness float64 `json:"praiseworthiness"` // [-1, 1]; negative = blameworthy
	Agent            string  `json:"agent"`            // "self" | "other"
}

// ObjectAppraisal appraises an object against attitudes/tastes.
type ObjectAppraisal struct {
	Appealingness float64 `json:"appealingness"` // [-1, 1]
}

// Resolution resolves a prospect ledger entry.
type Resolution struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // confirmed | disconfirmed | dropped
}

// Validate checks ranges and structural requirements of the appraisal.
func (a Appraisal) Validate() error {
	if a.Consequence == nil && a.Action == nil && a.Object == nil && len(a.Resolve) == 0 {
		return fmt.Errorf("appraisal: at least one of consequence, action, object, resolve is required")
	}
	if c := a.Consequence; c != nil {
		if c.Desirability < -1 || c.Desirability > 1 {
			return fmt.Errorf("appraisal: desirability %v outside [-1, 1]", c.Desirability)
		}
		if c.Likelihood != nil {
			if l := *c.Likelihood; l <= 0 || l > 1 {
				return fmt.Errorf("appraisal: likelihood %v outside (0, 1]", l)
			}
		}
		switch c.For {
		case "", "self":
			if c.Likelihood != nil && *c.Likelihood < 1 && c.Label == "" {
				return fmt.Errorf("appraisal: label is required for a prospect (likelihood < 1)")
			}
		case "other":
			if c.Liking == nil {
				return fmt.Errorf(`appraisal: liking is required when for is "other"`)
			}
			if *c.Liking < -1 || *c.Liking > 1 {
				return fmt.Errorf("appraisal: liking %v outside [-1, 1]", *c.Liking)
			}
			if c.Likelihood != nil && *c.Likelihood < 1 {
				return fmt.Errorf("appraisal: prospects about others are not supported")
			}
		default:
			return fmt.Errorf(`appraisal: for must be "self" or "other", got %q`, c.For)
		}
	}
	if act := a.Action; act != nil {
		if act.Praiseworthiness < -1 || act.Praiseworthiness > 1 {
			return fmt.Errorf("appraisal: praiseworthiness %v outside [-1, 1]", act.Praiseworthiness)
		}
		if act.Agent != "self" && act.Agent != "other" {
			return fmt.Errorf(`appraisal: agent must be "self" or "other", got %q`, act.Agent)
		}
	}
	if o := a.Object; o != nil {
		if o.Appealingness < -1 || o.Appealingness > 1 {
			return fmt.Errorf("appraisal: appealingness %v outside [-1, 1]", o.Appealingness)
		}
	}
	for i, r := range a.Resolve {
		if r.ID == "" {
			return fmt.Errorf("appraisal: resolve[%d] missing id", i)
		}
		switch r.Outcome {
		case OutcomeConfirmed, OutcomeDisconfirmed, OutcomeDropped:
		default:
			return fmt.Errorf("appraisal: resolve[%d] outcome must be confirmed|disconfirmed|dropped, got %q", i, r.Outcome)
		}
	}
	return nil
}

// ApplyAppraisal validates the appraisal, derives emotion deltas per the OCC
// rules, updates the prospect ledger, and applies the deltas through
// ApplyDeltas. now stamps newly created ledger entries.
func ApplyAppraisal(s State, a Appraisal, cfg Config, now time.Time) (State, error) {
	if cfg.Model != "occ" || cfg.OCC == nil {
		return State{}, fmt.Errorf("appraise requires an occ-model config (got model %q)", cfg.Model)
	}
	if err := a.Validate(); err != nil {
		return State{}, err
	}
	g := cfg.OCC.Gains
	deltas := map[string]float64{}

	// Prospect resolutions consume ledger entries first.
	for _, r := range a.Resolve {
		p, rest, ok := takeProspect(s.Prospects, r.ID)
		if !ok {
			return State{}, fmt.Errorf("unknown prospect %q", r.ID)
		}
		s.Prospects = rest
		if r.Outcome == OutcomeDropped {
			continue
		}
		mag := g.Prospect * math.Abs(p.Desirability)
		switch {
		case r.Outcome == OutcomeConfirmed && p.Desirability > 0:
			deltas["satisfaction"] += mag
		case r.Outcome == OutcomeConfirmed && p.Desirability < 0:
			deltas["fears-confirmed"] += mag
		case r.Outcome == OutcomeDisconfirmed && p.Desirability > 0:
			deltas["disappointment"] += mag
		case r.Outcome == OutcomeDisconfirmed && p.Desirability < 0:
			deltas["relief"] += mag
		}
	}

	// Consequences of events (well-being branch; prospect and
	// fortunes-of-others branches are added in later rules).
	if c := a.Consequence; c != nil && c.Desirability != 0 {
		des := c.Desirability
		switch {
		case c.For == "other":
			// fortunes-of-others: 4 quadrants of desirability x liking.
			if lik := *c.Liking; lik != 0 {
				mag := g.Fortunes * math.Abs(des) * math.Abs(lik)
				switch {
				case des > 0 && lik > 0:
					deltas["happy-for"] += mag
				case des < 0 && lik > 0:
					deltas["pity"] += mag
				case des > 0 && lik < 0:
					deltas["resentment"] += mag
				default:
					deltas["gloating"] += mag
				}
			}
		case c.Likelihood != nil && *c.Likelihood < 1:
			// prospect: uncertain consequence for self (Validate guarantees
			// likelihood > 0) — hope/fear now, ledger entry so a later
			// session can resolve it.
			l := *c.Likelihood
			mag := g.Prospect * math.Abs(des) * l
			if des > 0 {
				deltas["hope"] += mag
			} else {
				deltas["fear"] += mag
			}
			s.ProspectSeq++
			s.Prospects = append(s.Prospects, Prospect{
				ID:           fmt.Sprintf("p%d", s.ProspectSeq),
				Label:        c.Label,
				Desirability: des,
				Likelihood:   l,
				CreatedAt:    now,
			})
			if max := cfg.OCC.MaxProspects; len(s.Prospects) > max {
				s.Prospects = append([]Prospect(nil), s.Prospects[len(s.Prospects)-max:]...)
			}
		default:
			// well-being: actual consequence for self.
			mag := g.Wellbeing * math.Abs(des)
			if des > 0 {
				deltas["joy"] += mag
			} else {
				deltas["distress"] += mag
			}
		}
	}

	// Attribution: actions of agents against standards.
	if act := a.Action; act != nil && act.Praiseworthiness != 0 {
		mag := g.Attribution * math.Abs(act.Praiseworthiness)
		switch {
		case act.Agent == "self" && act.Praiseworthiness > 0:
			deltas["pride"] += mag
		case act.Agent == "self":
			deltas["shame"] += mag
		case act.Praiseworthiness > 0:
			deltas["admiration"] += mag
		default:
			deltas["reproach"] += mag
		}
	}

	// Attraction: aspects of objects against attitudes.
	if o := a.Object; o != nil && o.Appealingness != 0 {
		mag := g.Attraction * math.Abs(o.Appealingness)
		if o.Appealingness > 0 {
			deltas["love"] += mag
		} else {
			deltas["hate"] += mag
		}
	}

	// Compounds fire in addition to their components when an actual
	// consequence for self and an action share the appraisal with aligned
	// signs (OCC: compound emotions are co-occurrences).
	if c, act := a.Consequence, a.Action; c != nil && act != nil &&
		c.For != "other" && (c.Likelihood == nil || *c.Likelihood >= 1) &&
		c.Desirability != 0 && act.Praiseworthiness != 0 &&
		(c.Desirability > 0) == (act.Praiseworthiness > 0) {
		mag := g.Compound * math.Min(math.Abs(c.Desirability), math.Abs(act.Praiseworthiness))
		switch {
		case c.Desirability > 0 && act.Agent == "self":
			deltas["gratification"] += mag
		case c.Desirability > 0:
			deltas["gratitude"] += mag
		case act.Agent == "self":
			deltas["remorse"] += mag
		default:
			deltas["anger"] += mag
		}
	}

	return ApplyDeltas(s, deltas, cfg)
}

// RenderOCC returns the occ state as a one-line JSON object holding the axes
// and the unresolved prospect ledger. Showing the ledger every turn is what
// lets a later session (with no conversational memory of the prospect)
// recognize and resolve it.
func RenderOCC(s State, cfg Config) string {
	type slimProspect struct {
		ID           string  `json:"id"`
		Label        string  `json:"label"`
		Desirability float64 `json:"desirability"`
		Likelihood   float64 `json:"likelihood"`
	}
	slim := make([]slimProspect, 0, len(s.Prospects))
	for _, p := range s.Prospects {
		slim = append(slim, slimProspect{p.ID, p.Label, p.Desirability, p.Likelihood})
	}
	b, _ := json.Marshal(slim)
	return `{"axes":` + Render(s, cfg) + `,"prospects":` + string(b) + `}`
}

// takeProspect removes the ledger entry with the given id, returning it and
// the remaining entries.
func takeProspect(ps []Prospect, id string) (Prospect, []Prospect, bool) {
	for i, p := range ps {
		if p.ID == id {
			rest := append(append([]Prospect(nil), ps[:i]...), ps[i+1:]...)
			return p, rest, true
		}
	}
	return Prospect{}, ps, false
}

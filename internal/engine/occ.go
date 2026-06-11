package engine

import "fmt"

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
	for _, r := range a.Resolve {
		if r.ID == "" {
			return fmt.Errorf("appraisal: resolve entry missing id")
		}
		switch r.Outcome {
		case OutcomeConfirmed, OutcomeDisconfirmed, OutcomeDropped:
		default:
			return fmt.Errorf("appraisal: outcome must be confirmed|disconfirmed|dropped, got %q", r.Outcome)
		}
	}
	return nil
}

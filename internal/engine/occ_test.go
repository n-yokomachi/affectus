package engine

import (
	"strings"
	"testing"
	"time"
)

func f64(v float64) *float64 { return &v }

func occCfg(t *testing.T) Config {
	t.Helper()
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatalf("parse occ default config: %v", err)
	}
	return cfg
}

var occNow = time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

// applyOCC is a test shorthand: fresh state -> ApplyAppraisal.
func applyOCC(t *testing.T, a Appraisal) State {
	t.Helper()
	cfg := occCfg(t)
	s, err := ApplyAppraisal(NewState(cfg, occNow), a, cfg, occNow)
	if err != nil {
		t.Fatalf("ApplyAppraisal: %v", err)
	}
	return s
}

func TestApplyAppraisalRequiresOCCModel(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	_, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{Object: &ObjectAppraisal{Appealingness: 0.5}}, cfg, occNow)
	if err == nil || !strings.Contains(err.Error(), "occ") {
		t.Fatalf("want occ-model error, got %v", err)
	}
}

func TestApplyAppraisalWellbeing(t *testing.T) {
	// gains.wellbeing = 0.8
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5}})
	if !almostEqual(s.Axes["joy"], 0.40) {
		t.Errorf("joy = %v, want 0.40", s.Axes["joy"])
	}
	if s.Axes["distress"] != 0 {
		t.Errorf("distress = %v, want 0", s.Axes["distress"])
	}
	s = applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{Desirability: -0.5}})
	if !almostEqual(s.Axes["distress"], 0.40) {
		t.Errorf("distress = %v, want 0.40", s.Axes["distress"])
	}
	if s.Axes["joy"] != 0 {
		t.Errorf("joy = %v, want 0", s.Axes["joy"])
	}
}

func TestApplyAppraisalAttribution(t *testing.T) {
	// gains.attribution = 0.8
	tests := []struct {
		praise float64
		agent  string
		axis   string
	}{
		{0.5, "self", "pride"},
		{-0.5, "self", "shame"},
		{0.5, "other", "admiration"},
		{-0.5, "other", "reproach"},
	}
	for _, tt := range tests {
		s := applyOCC(t, Appraisal{Action: &ActionAppraisal{Praiseworthiness: tt.praise, Agent: tt.agent}})
		if !almostEqual(s.Axes[tt.axis], 0.40) {
			t.Errorf("%s = %v, want 0.40", tt.axis, s.Axes[tt.axis])
		}
		for _, other := range []string{"pride", "shame", "admiration", "reproach"} {
			if other != tt.axis && s.Axes[other] != 0 {
				t.Errorf("%s = %v, want 0 when only %s should fire", other, s.Axes[other], tt.axis)
			}
		}
	}
}

func TestApplyAppraisalForOtherDoesNotRaiseWellbeing(t *testing.T) {
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4)}})
	if s.Axes["joy"] != 0 || s.Axes["distress"] != 0 {
		t.Errorf("for-other consequence must not raise joy/distress: joy=%v distress=%v", s.Axes["joy"], s.Axes["distress"])
	}
}

func TestApplyAppraisalAttraction(t *testing.T) {
	// gains.attraction = 0.6
	s := applyOCC(t, Appraisal{Object: &ObjectAppraisal{Appealingness: 0.5}})
	if !almostEqual(s.Axes["love"], 0.30) {
		t.Errorf("love = %v, want 0.30", s.Axes["love"])
	}
	s = applyOCC(t, Appraisal{Object: &ObjectAppraisal{Appealingness: -0.5}})
	if !almostEqual(s.Axes["hate"], 0.30) {
		t.Errorf("hate = %v, want 0.30", s.Axes["hate"])
	}
}

func TestApplyAppraisalZeroValuesAreNoop(t *testing.T) {
	s := applyOCC(t, Appraisal{
		Consequence: &ConsequenceAppraisal{Desirability: 0},
		Action:      &ActionAppraisal{Praiseworthiness: 0, Agent: "self"},
		Object:      &ObjectAppraisal{Appealingness: 0},
	})
	for name, v := range s.Axes {
		if v != 0 {
			t.Errorf("axis %s = %v, want 0 (zero appraisal is a no-op)", name, v)
		}
	}
}

func TestApplyAppraisalRejectsInvalid(t *testing.T) {
	cfg := occCfg(t)
	_, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{}, cfg, occNow)
	if err == nil {
		t.Fatal("empty appraisal should error")
	}
}

func TestAppraisalValidate(t *testing.T) {
	tests := []struct {
		name    string
		a       Appraisal
		wantErr string // "" = valid
	}{
		{"empty appraisal", Appraisal{}, "at least one"},
		{"wellbeing ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5}}, ""},
		{"desirability out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 1.5}}, "desirability"},
		{"likelihood out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(1.5)}}, "likelihood"},
		{"likelihood zero", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0)}}, "likelihood"},
		{"prospect needs label", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0.5)}}, "label"},
		{"prospect ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0.5), Label: "x"}}, ""},
		{"likelihood one no label ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(1.0)}}, ""},
		{"for other needs liking", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other"}}, "liking"},
		{"for other ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4)}}, ""},
		{"for other with certain event ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4), Likelihood: f64(1.0)}}, ""},
		{"liking out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(2)}}, "liking"},
		{"other prospect unsupported", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4), Likelihood: f64(0.5), Label: "x"}}, "not supported"},
		{"bad for", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "them"}}, "for must be"},
		{"action ok", Appraisal{Action: &ActionAppraisal{Praiseworthiness: -0.5, Agent: "other"}}, ""},
		{"action bad agent", Appraisal{Action: &ActionAppraisal{Praiseworthiness: 0.5, Agent: "me"}}, "agent"},
		{"praise out of range", Appraisal{Action: &ActionAppraisal{Praiseworthiness: -2, Agent: "self"}}, "praiseworthiness"},
		{"object ok", Appraisal{Object: &ObjectAppraisal{Appealingness: 0.3}}, ""},
		{"appealingness out of range", Appraisal{Object: &ObjectAppraisal{Appealingness: -2}}, "appealingness"},
		{"resolve ok", Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: "confirmed"}}}, ""},
		{"resolve missing id", Appraisal{Resolve: []Resolution{{Outcome: "confirmed"}}}, "id"},
		{"resolve bad outcome", Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: "done"}}}, "outcome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.a.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

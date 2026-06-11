package engine

import (
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

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
		{"for other needs liking", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other"}}, "liking"},
		{"for other ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4)}}, ""},
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

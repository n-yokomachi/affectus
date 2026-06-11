package mcp

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-yokomachi/affectus/internal/cli"
	"github.com/n-yokomachi/affectus/internal/engine"
)

func mcpTestEnv(t *testing.T) cli.Env {
	t.Helper()
	dir := t.TempDir()
	return cli.Env{
		ConfigPath: filepath.Join(dir, "config.yaml"),
		StatePath:  filepath.Join(dir, "state.json"),
		Now:        func() time.Time { return time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC) },
		Stdout:     nil,
		Stderr:     nil,
	}
}

func TestHandleShowReturnsAxes(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	res, err := handleShow(env)
	if err != nil {
		t.Fatalf("handleShow: %v", err)
	}
	if len(res.Axes) != 8 {
		t.Fatalf("expected 8 axes, got %d: %+v", len(res.Axes), res)
	}
}

func TestHandleShowAllZerosAtInit(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	res, err := handleShow(env)
	if err != nil {
		t.Fatalf("handleShow: %v", err)
	}
	for name, val := range res.Axes {
		if val != 0.0 {
			t.Errorf("axis %q = %v, want 0.0 at init", name, val)
		}
	}
}

func TestHandleFeelAppliesDeltas(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	res, err := handleFeel(env, feelInput{Deltas: map[string]float64{"joy": 0.5}})
	if err != nil {
		t.Fatalf("handleFeel: %v", err)
	}
	if res.Axes["joy"] < 0.49 || res.Axes["joy"] > 0.51 {
		t.Fatalf("joy = %v, want ~0.5", res.Axes["joy"])
	}
}

func TestHandleFeelUnknownAxisErrors(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := handleFeel(env, feelInput{Deltas: map[string]float64{"glee": 0.5}}); err == nil {
		t.Fatal("unknown axis should error")
	}
}

func TestToolDescriptionsAreModelNeutral(t *testing.T) {
	for _, d := range []string{showDesc, feelDesc} {
		if strings.Contains(d, "Plutchik") {
			t.Errorf("tool description should not hardcode a model name: %q", d)
		}
	}
}

func TestHandleAppraiseDerivesEmotions(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	lik := 0.5
	res, err := handleAppraise(env, engine.Appraisal{
		Consequence: &engine.ConsequenceAppraisal{Desirability: 0.6, Likelihood: &lik, Label: "pr merge"},
	})
	if err != nil {
		t.Fatalf("handleAppraise: %v", err)
	}
	if res.Axes["hope"] < 0.23 || res.Axes["hope"] > 0.25 {
		t.Errorf("hope = %v, want ~0.24", res.Axes["hope"])
	}
	if len(res.Prospects) != 1 || res.Prospects[0].ID != "p1" {
		t.Errorf("prospects = %+v, want one entry p1", res.Prospects)
	}
}

func TestHandleAppraiseRequiresOCC(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := handleAppraise(env, engine.Appraisal{
		Object: &engine.ObjectAppraisal{Appealingness: 0.5},
	}); err == nil {
		t.Fatal("appraise on plutchik config should error")
	}
}

func TestHandleShowIncludesProspects(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	lik := 0.5
	if _, err := handleAppraise(env, engine.Appraisal{
		Consequence: &engine.ConsequenceAppraisal{Desirability: 0.6, Likelihood: &lik, Label: "x"},
	}); err != nil {
		t.Fatalf("handleAppraise: %v", err)
	}
	res, err := handleShow(env)
	if err != nil {
		t.Fatalf("handleShow: %v", err)
	}
	if len(res.Prospects) != 1 {
		t.Errorf("show should include the ledger, got %+v", res)
	}
}

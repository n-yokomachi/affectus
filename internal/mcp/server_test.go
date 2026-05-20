package mcp

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/n-yokomachi/affectus/internal/cli"
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
	if err := cli.Init(env, false); err != nil {
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
	if err := cli.Init(env, false); err != nil {
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
	if err := cli.Init(env, false); err != nil {
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
	if err := cli.Init(env, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := handleFeel(env, feelInput{Deltas: map[string]float64{"glee": 0.5}}); err == nil {
		t.Fatal("unknown axis should error")
	}
}

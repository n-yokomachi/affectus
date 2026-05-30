package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/n-yokomachi/affectus/internal/engine"
)

func testEnv(t *testing.T) (Env, *bytes.Buffer) {
	t.Helper()
	dir := t.TempDir()
	out := &bytes.Buffer{}
	env := Env{
		ConfigPath: filepath.Join(dir, "config.yaml"),
		StatePath:  filepath.Join(dir, "state.json"),
		Now:        func() time.Time { return time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC) },
		Stdout:     out,
		Stderr:     &bytes.Buffer{},
	}
	return env, out
}

func TestResolvePathPrefersFlag(t *testing.T) {
	if got := ResolvePath("/explicit/path", "AFFECTUS_X", "config.yaml"); got != "/explicit/path" {
		t.Fatalf("ResolvePath = %q, want /explicit/path", got)
	}
}

func TestResolvePathUsesEnv(t *testing.T) {
	t.Setenv("AFFECTUS_TEST_VAR", "/from/env")
	if got := ResolvePath("", "AFFECTUS_TEST_VAR", "config.yaml"); got != "/from/env" {
		t.Fatalf("ResolvePath = %q, want /from/env", got)
	}
}

func TestInitCreatesFiles(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := os.Stat(env.ConfigPath); err != nil {
		t.Errorf("config file not created: %v", err)
	}
	if _, err := os.Stat(env.StatePath); err != nil {
		t.Errorf("state file not created: %v", err)
	}
}

func TestInitRefusesOverwriteWithoutForce(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := Init(env, "plutchik", false); err == nil {
		t.Fatal("second Init without --force should fail")
	}
	if err := Init(env, "plutchik", true); err != nil {
		t.Fatalf("Init with --force should succeed: %v", err)
	}
}

func TestInitRussellWritesRussellConfig(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "russell", false); err != nil {
		t.Fatalf("Init russell: %v", err)
	}
	cfg, err := engine.LoadConfig(env.ConfigPath)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Model != "russell" {
		t.Errorf("written model = %q, want russell", cfg.Model)
	}
	if len(cfg.Axes) != 2 {
		t.Errorf("written axes = %d, want 2", len(cfg.Axes))
	}
}

func TestInitUnknownModel(t *testing.T) {
	env, _ := testEnv(t)
	err := Init(env, "freud", false)
	if err == nil {
		t.Fatal("Init with unknown model should error")
	}
	if !strings.Contains(err.Error(), "plutchik") {
		t.Errorf("error should list valid models, got: %v", err)
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
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
	if err := Init(env, false); err != nil {
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
	if err := Init(env, false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	if err := Init(env, false); err == nil {
		t.Fatal("second Init without --force should fail")
	}
	if err := Init(env, true); err != nil {
		t.Fatalf("Init with --force should succeed: %v", err)
	}
}

package main

import (
	"path/filepath"
	"testing"
)

func TestRunInitThenTick(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	state := filepath.Join(dir, "state.json")
	if err := run([]string{"--config", cfg, "--state", state, "init"}); err != nil {
		t.Fatalf("run init: %v", err)
	}
	if err := run([]string{"--config", cfg, "--state", state, "tick"}); err != nil {
		t.Fatalf("run tick: %v", err)
	}
}

func TestRunNoArgs(t *testing.T) {
	if err := run(nil); err == nil {
		t.Fatal("run with no args should return a usage error")
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if err := run([]string{"frobnicate"}); err == nil {
		t.Fatal("unknown command should error")
	}
}

func TestRunFeelRequiresArg(t *testing.T) {
	if err := run([]string{"feel"}); err == nil {
		t.Fatal("feel without payload should error")
	}
}

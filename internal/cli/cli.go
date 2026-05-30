package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/n-yokomachi/affectus/internal/engine"
)

// Env carries the resolved paths, clock, and output streams for one command
// invocation. Tests construct it with a fixed clock and buffer writers.
type Env struct {
	ConfigPath string
	StatePath  string
	Now        func() time.Time
	Stdout     io.Writer
	Stderr     io.Writer
}

// ResolvePath picks the first of: explicit flag value, environment variable,
// or ~/.config/affectus/<defaultName>.
func ResolvePath(flagVal, envVar, defaultName string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv(envVar); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".config", "affectus", defaultName)
}

// loadConfig loads the config file if present, otherwise the embedded default.
func loadConfig(env Env) (engine.Config, error) {
	if _, err := os.Stat(env.ConfigPath); err == nil {
		return engine.LoadConfig(env.ConfigPath)
	}
	return engine.DefaultConfig()
}

// writeFragment writes the current axes JSON snapshot to cfg.FragmentFile when set.
func writeFragment(cfg engine.Config, s engine.State) error {
	if cfg.FragmentFile == "" {
		return nil
	}
	return os.WriteFile(cfg.FragmentFile, []byte(engine.Render(s, cfg)+"\n"), 0o644)
}

// Init writes the config for the given model and a baseline state file.
// Existing files are only overwritten when force is true.
func Init(env Env, model string, force bool) error {
	yamlBytes, ok := engine.Models[model]
	if !ok {
		return fmt.Errorf("unknown model %q (valid: plutchik, russell)", model)
	}
	for _, p := range []string{env.ConfigPath, env.StatePath} {
		if _, err := os.Stat(p); err == nil && !force {
			return fmt.Errorf("%s already exists (use --force to overwrite)", p)
		}
	}
	if err := os.MkdirAll(filepath.Dir(env.ConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(env.ConfigPath, yamlBytes, 0o644); err != nil {
		return err
	}
	cfg, err := engine.ParseConfig(yamlBytes)
	if err != nil {
		return err
	}
	if err := engine.SaveState(env.StatePath, engine.NewState(cfg, env.Now())); err != nil {
		return err
	}
	out := env.Stdout
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "initialized model=%s config=%s state=%s\n", model, env.ConfigPath, env.StatePath)
	return nil
}

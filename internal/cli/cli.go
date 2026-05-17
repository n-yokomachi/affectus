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

// writeFragment writes the rendered fragment to cfg.FragmentFile when set.
func writeFragment(cfg engine.Config, s engine.State) error {
	if cfg.FragmentFile == "" {
		return nil
	}
	return os.WriteFile(cfg.FragmentFile, []byte(engine.Render(s, cfg)+"\n"), 0o644)
}

// Init writes the default config and a baseline state file. Existing files are
// only overwritten when force is true.
func Init(env Env, force bool) error {
	for _, p := range []string{env.ConfigPath, env.StatePath} {
		if _, err := os.Stat(p); err == nil && !force {
			return fmt.Errorf("%s already exists (use --force to overwrite)", p)
		}
	}
	if err := os.MkdirAll(filepath.Dir(env.ConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(env.ConfigPath, engine.DefaultConfigYAML, 0o644); err != nil {
		return err
	}
	cfg, err := engine.DefaultConfig()
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
	fmt.Fprintf(out, "initialized config=%s state=%s\n", env.ConfigPath, env.StatePath)
	return nil
}

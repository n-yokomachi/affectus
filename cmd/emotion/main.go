package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/n-yokomachi/affectus/internal/cli"
)

const usage = "usage: emotion [--config P] [--state P] <init|show|get|feel|tick|reset|mcp> [args]"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// run parses args and dispatches to a CLI command. It is separated from main
// so tests can drive it directly.
func run(args []string) error {
	fs := flag.NewFlagSet("emotion", flag.ContinueOnError)
	configFlag := fs.String("config", "", "path to config file")
	stateFlag := fs.String("state", "", "path to state file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return errors.New(usage)
	}

	env := cli.Env{
		ConfigPath: cli.ResolvePath(*configFlag, "AFFECTUS_CONFIG", "config.yaml"),
		StatePath:  cli.ResolvePath(*stateFlag, "AFFECTUS_STATE", "state.json"),
		Now:        time.Now,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	}

	cmd, cmdArgs := rest[0], rest[1:]
	switch cmd {
	case "init":
		ifs := flag.NewFlagSet("init", flag.ContinueOnError)
		force := ifs.Bool("force", false, "overwrite existing files")
		if err := ifs.Parse(cmdArgs); err != nil {
			return err
		}
		return cli.Init(env, *force)
	case "show":
		sfs := flag.NewFlagSet("show", flag.ContinueOnError)
		format := sfs.String("format", "text", "output format: text|json")
		if err := sfs.Parse(cmdArgs); err != nil {
			return err
		}
		return cli.Show(env, *format)
	case "get":
		return cli.Get(env)
	case "feel":
		if len(cmdArgs) != 1 {
			return fmt.Errorf("usage: emotion feel '<deltas-json>'  (use - to read JSON from stdin)")
		}
		payload := cmdArgs[0]
		if payload == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			payload = string(b)
		}
		return cli.Feel(env, payload)
	case "tick":
		return cli.Tick(env)
	case "reset":
		return cli.Reset(env)
	case "mcp":
		return runMCP(env)
	default:
		return fmt.Errorf("unknown command %q\n%s", cmd, usage)
	}
}

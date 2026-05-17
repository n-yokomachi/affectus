package main

import (
	"fmt"

	"github.com/n-yokomachi/affectus/internal/cli"
)

// runMCP is replaced with the real MCP server wiring in Task 12.
func runMCP(_ cli.Env) error {
	return fmt.Errorf("mcp subcommand not yet implemented")
}

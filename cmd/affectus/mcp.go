package main

import (
	"context"

	"github.com/n-yokomachi/affectus/internal/cli"
	"github.com/n-yokomachi/affectus/internal/mcp"
)

func runMCP(env cli.Env) error {
	return mcp.Serve(context.Background(), env)
}

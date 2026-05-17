package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/n-yokomachi/affectus/internal/cli"
)

// showResult is the structured output of both tools.
type showResult struct {
	Fragment string             `json:"fragment"`
	Axes     map[string]float64 `json:"axes"`
}

// feelInput is the input schema of the emotion_feel tool.
type feelInput struct {
	Deltas map[string]float64 `json:"deltas"`
}

const serverVersion = "0.1.0"

// handleShow computes the current emotion without persisting.
func handleShow(env cli.Env) (showResult, error) {
	fragment, axes, err := cli.ComputeShow(env)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Fragment: fragment, Axes: axes}, nil
}

// handleFeel applies self-reported deltas and persists.
func handleFeel(env cli.Env, in feelInput) (showResult, error) {
	fragment, axes, err := cli.ApplyFeel(env, in.Deltas)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Fragment: fragment, Axes: axes}, nil
}

// Serve runs the affectus MCP server over stdio, exposing emotion_show and
// emotion_feel. Idle decay still requires a separate cron `affectus tick`.
func Serve(ctx context.Context, env cli.Env) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "affectus", Version: serverVersion}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_show",
		Description: "Return the current emotion as a natural-language fragment and raw axis values.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, showResult, error) {
		res, err := handleShow(env)
		return nil, res, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_feel",
		Description: "Apply self-reported emotion deltas and return the updated emotion.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in feelInput) (*mcp.CallToolResult, showResult, error) {
		res, err := handleFeel(env, in)
		return nil, res, err
	})

	return server.Run(ctx, &mcp.StdioTransport{})
}

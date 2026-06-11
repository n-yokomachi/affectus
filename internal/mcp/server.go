package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/n-yokomachi/affectus/internal/cli"
	"github.com/n-yokomachi/affectus/internal/engine"
)

// showResult is the structured output of all tools.
type showResult struct {
	Axes      map[string]float64 `json:"axes"`
	Prospects []engine.Prospect  `json:"prospects,omitempty"`
}

// feelInput is the input schema of the emotion_feel tool.
type feelInput struct {
	Deltas map[string]float64 `json:"deltas"`
}

const serverVersion = "0.3.0"

const showDesc = "Returns the current emotion as a JSON object of axes with float values. Interpret the values relationally per the emotion-model structure documented in your system prompt."

const feelDesc = "Apply self-reported emotion deltas and return the updated emotion as a JSON object of axes with float values. Interpret the values relationally per the emotion-model structure documented in your system prompt."

const appraiseDesc = "Report a cognitive appraisal (OCC model): consequences of events (desirability, optional likelihood for uncertain prospects), actions of agents (praiseworthiness), aspects of objects (appealingness), and resolutions of pending prospects. Include consequence and action together when they belong to the same event — compound emotions (anger, gratitude, gratification, remorse) only arise from that co-occurrence. The engine derives emotion deltas deterministically and returns the updated state including the prospect ledger. Requires an occ-model config."

// handleShow computes the current emotion without persisting.
func handleShow(env cli.Env) (showResult, error) {
	_, s, _, err := cli.ComputeShowFull(env)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Axes: s.Axes, Prospects: s.Prospects}, nil
}

// handleAppraise applies an OCC appraisal and persists.
func handleAppraise(env cli.Env, a engine.Appraisal) (showResult, error) {
	_, s, err := cli.ApplyAppraise(env, a)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Axes: s.Axes, Prospects: s.Prospects}, nil
}

// handleFeel applies self-reported deltas and persists.
func handleFeel(env cli.Env, in feelInput) (showResult, error) {
	_, axes, err := cli.ApplyFeel(env, in.Deltas)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Axes: axes}, nil
}

// Serve runs the affectus MCP server over stdio, exposing emotion_show,
// emotion_feel, and emotion_appraise. Idle decay still requires a separate
// cron `affectus tick`.
func Serve(ctx context.Context, env cli.Env) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "affectus", Version: serverVersion}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_show",
		Description: showDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, showResult, error) {
		res, err := handleShow(env)
		return nil, res, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_feel",
		Description: feelDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in feelInput) (*mcp.CallToolResult, showResult, error) {
		res, err := handleFeel(env, in)
		return nil, res, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_appraise",
		Description: appraiseDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in engine.Appraisal) (*mcp.CallToolResult, showResult, error) {
		res, err := handleAppraise(env, in)
		return nil, res, err
	})

	return server.Run(ctx, &mcp.StdioTransport{})
}

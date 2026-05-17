# affectus

An emotion-state engine for LLM agents. `affectus` holds a multi-axis emotion
vector (Plutchik's 8 emotions by default), relaxes it toward baseline over
time, and renders it as a natural-language fragment you can inject into a
prompt — so an agent's tone can shift with a persistent, decaying mood.

It is framework-agnostic: any agent that can run a shell command can use it.

## Concept: two loops

- **Conversation loop (self-report):** each turn, the agent runs `emotion show`
  to read its current mood, colors its reply, then runs `emotion feel` to
  report how the exchange shifted its emotions.
- **Background loop (cron):** a scheduled `emotion tick` relaxes the vector
  toward baseline so the mood drifts naturally even while the agent is idle.

## Install

```bash
go install github.com/n-yokomachi/affectus/cmd/emotion@latest
```

Or download a binary from the Releases page and place `emotion` on your PATH.

Requires macOS or Linux (affectus uses Unix file locking).

## Quick start

```bash
emotion init                       # write default config + baseline state
emotion show                       # -> "Right now you feel calm and even."
emotion feel '{"joy":0.6,"surprise":0.2}'
emotion show                       # -> reflects the new emotion
```

State and config live under `~/.config/affectus/` by default. Override with
`--config` / `--state` or the `AFFECTUS_CONFIG` / `AFFECTUS_STATE` env vars.

## Commands

| Command | Description |
|---|---|
| `emotion init [--force]` | Write default config and a baseline state file |
| `emotion show [--format text\|json]` | Print the current emotion (read-only) |
| `emotion get` | Print decayed raw axis values as JSON (read-only) |
| `emotion feel '<json>'` | Apply self-reported deltas (use `-` to read stdin) |
| `emotion tick` | Apply time decay only — the cron target |
| `emotion reset` | Return every axis to baseline |
| `emotion mcp` | Run an MCP server (stdio) exposing `emotion_show` / `emotion_feel` |

## Wiring it into an agent

1. Put the `emotion` binary on PATH.
2. `emotion init`.
3. Schedule decay: add `examples/cron.sample` to your crontab.
4. Teach the agent the protocol: paste `examples/system-prompt-snippet.md`
   into its system prompt.
5. MCP-capable agents may instead register `emotion mcp` — see that command.

## Emotion model

Default axes are Plutchik's 8 emotions (joy, sadness, trust, disgust, fear,
anger, surprise, anticipation), each `0.0–1.0`. Axis names, count, baselines,
half-lives, and the natural-language phrasing are all configurable — see the
config written by `emotion init` and `examples/configs/plutchik8-ja.yaml`.

Decay is symmetric across axes by default: no emotion lingers longer than
another.

## License

MIT

# affectus

An emotion-state engine for LLM agents. `affectus` holds a multi-axis emotion
vector (Plutchik's 8 emotions by default), relaxes it toward baseline over
time, and exposes the raw numeric state for an LLM to read and interpret — so
an agent's tone can shift with a persistent, decaying mood.

It is framework-agnostic: any agent that can run a shell command can use it.

## Concept: two loops

- **Conversation loop (self-report):** each turn, the agent runs `affectus show`
  to read its current emotion vector as a JSON object, interprets the values
  relationally per the Plutchik wheel, colors its reply, then runs
  `affectus feel` to report how the exchange shifted its emotions.
- **Background loop (cron):** a scheduled `affectus tick` relaxes the vector
  toward baseline so the mood drifts naturally even while the agent is idle.

## Install

### Prebuilt binary (no Go toolchain needed)

Download the archive for your platform from the
[Releases page](https://github.com/n-yokomachi/affectus/releases), extract it,
and put the `affectus` binary on your PATH:

```bash
# example: macOS (Apple Silicon)
tar -xzf affectus_v0.1.1_darwin_arm64.tar.gz
mv affectus /usr/local/bin/
```

### From source

```bash
go install github.com/n-yokomachi/affectus/cmd/affectus@latest
```

Requires macOS or Linux (affectus uses Unix file locking).

## Quick start

```bash
affectus init                       # write default config + baseline state
affectus show                       # -> {"joy":0.00,"trust":0.00,...}
affectus feel '{"joy":0.6,"surprise":0.2}'
affectus show                       # -> {"joy":0.60,"trust":0.00,...,"surprise":0.20,...}
```

State and config live under `~/.config/affectus/` by default. Override with
`--config` / `--state` or the `AFFECTUS_CONFIG` / `AFFECTUS_STATE` env vars.

## Commands

| Command | Description |
|---|---|
| `affectus init [--force]` | Write default config and a baseline state file |
| `affectus show [--format text\|json]` | Print the current emotion (read-only) |
| `affectus get` | Print decayed raw axis values as JSON (read-only) |
| `affectus feel '<json>'` | Apply self-reported deltas (use `-` to read stdin) |
| `affectus tick` | Apply time decay only — the cron target |
| `affectus reset` | Return every axis to baseline |
| `affectus mcp` | Run an MCP server (stdio) exposing `emotion_show` / `emotion_feel` |
| `affectus viz [--port N]` | Serve a real-time browser visualization of the emotion state (default port 8765) |

## Wiring it into an agent

1. Put the `affectus` binary on PATH.
2. `affectus init`.
3. Schedule decay: add `examples/cron.sample` to your crontab.
4. Teach the agent the protocol: paste `examples/system-prompt-snippet.md`
   into its system prompt.
5. MCP-capable agents may instead register `affectus mcp` — see that command.

## Visualizing the emotion state

`affectus viz` starts a local web server that visualizes the live emotion
state — a Plutchik wheel coloured by intensity and a set of decay bars.

```bash
affectus viz            # then open http://localhost:8765
```

The page polls the state and shows it decayed to the current moment, so the
mood visibly drifts toward baseline between turns. The server is read-only.

## Emotion model

Default axes are Plutchik's 8 emotions (joy, sadness, trust, disgust, fear,
anger, surprise, anticipation), each `0.0–1.0`. Axis names, count, baselines,
and half-lives are all configurable — see the config written by `affectus init`
and `examples/configs/plutchik8-ja.yaml`.

**v0.3 note:** affectus no longer renders emotions as natural-language text.
It emits a one-line JSON object of all axes; the LLM is responsible for
reading the values relationally (per the Plutchik wheel) and modulating its
response accordingly. See `examples/system-prompt-snippet.md` for guidance.

Decay is symmetric across axes by default: no emotion lingers longer than
another.

## License

MIT

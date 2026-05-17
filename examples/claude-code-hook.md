# Tighter integration: Claude Code Stop hook

Agents with lifecycle hooks can refresh idle decay without cron. In Claude
Code, a `Stop` hook fires when a response finishes — a natural place to run
`emotion tick`.

Add to `.claude/settings.json`:

```json
{
  "hooks": {
    "Stop": [
      {
        "matcher": "",
        "hooks": [
          { "type": "command", "command": "emotion tick" }
        ]
      }
    ]
  }
}
```

This keeps the stored state fresh between turns. The conversational
`emotion show` / `emotion feel` protocol from `system-prompt-snippet.md`
is still required — the hook only handles decay.

Note: the `emotion feel` self-report still belongs in the system prompt;
hooks cannot decide emotional deltas.

# examples/strands-eval/src/agent.py
"""Build a conversational agent for a given (personality, affectus_on) cell.

Two backends, selected with the EVAL_BACKEND environment variable:

- ``claude-cli`` (default): headless Claude Code (``claude -p``), billed to
  the local user's Claude subscription instead of a metered API. The
  conversation is threaded turn by turn via ``--resume``. The default system
  prompt is REPLACED with the cell's persona prompt (``--system-prompt``) and
  no setting sources are loaded (``--setting-sources ""``) so the operator's
  personal CLAUDE.md, output styles and project settings cannot leak into the
  experiment. Caveat: the CLI exposes no temperature control, so runs are not
  deterministic.
- ``bedrock``: the original Strands Agents + Amazon Bedrock path
  (temperature=0, max_tokens=512), kept for reproducibility of runs recorded
  before the backend switch.
"""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path


PROMPTS_DIR = Path(__file__).parent.parent / "prompts"
SESSIONS_DIR = Path(__file__).parent.parent / "state" / "claude-sessions"

VALID_PERSONALITIES = {"friendly", "contrarian"}


def _load_prompt(name: str) -> str:
    return (PROMPTS_DIR / f"{name}.md").read_text(encoding="utf-8")


class ClaudeCLIAgent:
    """One conversation over headless ``claude -p``, resumed each turn.

    The first call creates a session; later calls pass ``--resume`` with the
    session id returned by the previous call, so the CLI keeps the full
    conversation history exactly like a chat backend would.
    """

    def __init__(
        self,
        system_prompt: str,
        model: str,
        claude_bin: str = "claude",
        cwd: str | None = None,
        timeout_seconds: int = 600,
    ):
        self.system_prompt = system_prompt
        self.model = model
        self.claude_bin = claude_bin
        self.cwd = cwd
        self.timeout_seconds = timeout_seconds
        self.session_id: str | None = None

    def __call__(self, message: str) -> str:
        cmd = [
            self.claude_bin,
            "-p",
            "--output-format", "json",
            "--model", self.model,
            "--system-prompt", self.system_prompt,
            "--setting-sources", "",
        ]
        if self.session_id:
            cmd += ["--resume", self.session_id]
        cmd.append(message)
        result = subprocess.run(
            cmd, capture_output=True, text=True,
            cwd=self.cwd, timeout=self.timeout_seconds,
        )
        if result.returncode != 0:
            raise RuntimeError(f"claude -p failed: {result.stderr.strip()[:500]}")
        data = json.loads(result.stdout)
        if data.get("is_error"):
            raise RuntimeError(f"claude -p returned error: {str(data.get('result'))[:500]}")
        self.session_id = data.get("session_id", self.session_id)
        return str(data.get("result", ""))


def build_agent(personality: str, affectus_on: bool):
    """Build an agent callable for the (personality, affectus_on) cell.

    When affectus_on is True, the affectus block (Plutchik relational reading
    + delta-reporting protocol) is appended to the system prompt. The agent's
    per-turn emotion state is NOT included in the system prompt; the
    orchestrator (run.py) prepends a `[現在のあなたの感情: ...]` line to each
    user message at run time. This way the LLM sees the live, evolving state
    each turn instead of a frozen snapshot.

    The agent emits `<feel>{...}</feel>` deltas as part of its reply text;
    the orchestrator parses and applies them. No tools are registered — keeps
    a single code path for delta application.
    """
    if personality not in VALID_PERSONALITIES:
        raise ValueError(f"unknown personality: {personality!r}")

    base = _load_prompt(personality)
    if affectus_on:
        system_prompt = base.rstrip() + "\n\n" + _load_prompt("affectus-block")
    else:
        system_prompt = base.rstrip()

    backend = os.environ.get("EVAL_BACKEND", "claude-cli")

    if backend == "claude-cli":
        SESSIONS_DIR.mkdir(parents=True, exist_ok=True)
        return ClaudeCLIAgent(
            system_prompt=system_prompt,
            model=os.environ.get("EVAL_CLAUDE_MODEL", "claude-sonnet-4-6"),
            claude_bin=os.environ.get("EVAL_CLAUDE_BIN", "claude"),
            cwd=str(SESSIONS_DIR),
        )

    if backend == "bedrock":
        from strands import Agent
        from strands.models import BedrockModel

        model = BedrockModel(
            model_id=os.environ.get("BEDROCK_MODEL_ID", "jp.anthropic.claude-sonnet-4-6"),
            region_name=os.environ.get("AWS_REGION", "ap-northeast-1"),
            temperature=0.0,
            max_tokens=512,
        )
        return Agent(model=model, system_prompt=system_prompt, tools=[])

    raise ValueError(f"unknown EVAL_BACKEND: {backend!r}")

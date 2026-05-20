# examples/strands-eval/src/agent.py
"""Build a Strands Agent for a given (personality, affectus_on) cell."""

from __future__ import annotations

import os
from pathlib import Path

from strands import Agent
from strands.models import BedrockModel


PROMPTS_DIR = Path(__file__).parent.parent / "prompts"
DEFAULT_MODEL_ID = os.environ.get(
    "BEDROCK_MODEL_ID",
    "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
)
DEFAULT_REGION = os.environ.get("AWS_REGION", "us-east-1")

VALID_PERSONALITIES = {"friendly", "contrarian"}


def _load_prompt(name: str) -> str:
    return (PROMPTS_DIR / f"{name}.md").read_text(encoding="utf-8")


def build_agent(personality: str, affectus_on: bool):
    """Build a Strands Agent for the (personality, affectus_on) cell.

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

    model = BedrockModel(
        model_id=DEFAULT_MODEL_ID,
        region_name=DEFAULT_REGION,
        temperature=0.0,
        max_tokens=512,
    )
    return Agent(model=model, system_prompt=system_prompt, tools=[])

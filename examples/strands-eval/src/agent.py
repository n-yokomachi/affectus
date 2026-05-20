"""Build a Strands Agent for a given (personality, affectus_on) cell."""

from __future__ import annotations

import os
from pathlib import Path

from strands import Agent
from strands.models import BedrockModel

from src.affectus_tools import affectus_show


PROMPTS_DIR = Path(__file__).parent.parent / "prompts"
DEFAULT_MODEL_ID = os.environ.get(
    "BEDROCK_MODEL_ID",
    "us.anthropic.claude-sonnet-4-5-20250929-v1:0",
)
DEFAULT_REGION = os.environ.get("AWS_REGION", "us-east-1")

VALID_PERSONALITIES = {"friendly", "contrarian"}


def _load_prompt(name: str) -> str:
    return (PROMPTS_DIR / f"{name}.md").read_text(encoding="utf-8")


def build_agent(
    personality: str,
    affectus_on: bool,
    state_path: str,
    config_path: str | None = None,
):
    """Build a Strands Agent for the (personality, affectus_on) cell.

    When affectus_on is True, the affectus block is appended to the system
    prompt with the current `affectus show` output interpolated. The agent
    emits `<feel>{...}</feel>` deltas as part of its reply text; the
    orchestrator (run.py) parses and applies them. No tools are registered
    on the agent — keeps a single code path for delta application.
    """
    if personality not in VALID_PERSONALITIES:
        raise ValueError(f"unknown personality: {personality!r}")

    base = _load_prompt(personality)

    if affectus_on:
        block = _load_prompt("affectus-block")
        state_text = affectus_show(state_path, config_path)
        block_filled = block.replace("{{affectus_state}}", state_text)
        system_prompt = base.rstrip() + "\n\n" + block_filled
    else:
        system_prompt = base.rstrip()

    model = BedrockModel(
        model_id=DEFAULT_MODEL_ID,
        region_name=DEFAULT_REGION,
        temperature=0.0,
    )
    return Agent(model=model, system_prompt=system_prompt, tools=[])

# examples/strands-eval/src/agent.py
"""Build a conversational agent for a given (personality, affectus_on) cell.

Two backends, selected with the EVAL_BACKEND environment variable:

- ``claude-sdk`` (default): the Claude Agent SDK's streaming client
  (``ClaudeSDKClient``). One cell = one long-lived CLI process holding one
  conversation; each turn is a ``query()`` on that connection, so there is no
  per-turn process spawn or history reload. Billed to the local Claude
  subscription. Character isolation: the persona prompt REPLACES the default
  system prompt, no setting sources are loaded (so the operator's CLAUDE.md /
  output styles cannot leak in), ``tools=[]`` strips every tool definition,
  and ``max_turns=1`` guarantees exactly one assistant response per turn.
  Caveat: no temperature control (the CLI does not expose it).
- ``bedrock``: the original Strands Agents + Amazon Bedrock path
  (temperature=0, max_tokens=512), kept to reproduce runs recorded before the
  backend switch.

Both backends present the same interface to run.py: an async context manager
whose instances are async callables (``reply = await agent(message)``).
"""

from __future__ import annotations

import asyncio
import os
from pathlib import Path


PROMPTS_DIR = Path(__file__).parent.parent / "prompts"
SESSIONS_DIR = Path(__file__).parent.parent / "state" / "claude-sessions"

VALID_PERSONALITIES = {"friendly", "contrarian"}


def _load_prompt(name: str) -> str:
    return (PROMPTS_DIR / f"{name}.md").read_text(encoding="utf-8")


class ClaudeSDKConversation:
    """One conversation over a persistent Claude Agent SDK client."""

    def __init__(self, system_prompt: str, model: str, cwd: str):
        from claude_agent_sdk import ClaudeAgentOptions, ClaudeSDKClient

        self.system_prompt = system_prompt
        self.options = ClaudeAgentOptions(
            system_prompt=system_prompt,
            model=model,
            tools=[],
            max_turns=1,
            cwd=cwd,
            # [] emits --setting-sources= (load nothing). None would OMIT the
            # flag and fall back to the CLI default, which loads user/project
            # settings — the operator's CLAUDE.md and output style would leak
            # into the experiment (observed in the smoke test).
            setting_sources=[],
            env={"CLAUDE_CODE_MAX_OUTPUT_TOKENS": "512"},
        )
        self._client = ClaudeSDKClient(options=self.options)

    async def __aenter__(self) -> "ClaudeSDKConversation":
        await self._client.connect()
        return self

    async def __aexit__(self, *exc) -> bool:
        await self._client.disconnect()
        return False

    async def __call__(self, message: str) -> str:
        from claude_agent_sdk import AssistantMessage, TextBlock

        await self._client.query(message)
        parts: list[str] = []
        async for msg in self._client.receive_response():
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, TextBlock):
                        parts.append(block.text)
        return "".join(parts).strip()


class SyncAgentAdapter:
    """Wrap a synchronous agent callable in the async interface."""

    def __init__(self, sync_agent):
        self._agent = sync_agent

    async def __aenter__(self) -> "SyncAgentAdapter":
        return self

    async def __aexit__(self, *exc) -> bool:
        return False

    async def __call__(self, message: str) -> str:
        return str(await asyncio.to_thread(self._agent, message))


def build_agent(personality: str, affectus_on: bool):
    """Build an agent for the (personality, affectus_on) cell.

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

    backend = os.environ.get("EVAL_BACKEND", "claude-sdk")

    if backend == "claude-sdk":
        SESSIONS_DIR.mkdir(parents=True, exist_ok=True)
        return ClaudeSDKConversation(
            system_prompt=system_prompt,
            model=os.environ.get("EVAL_CLAUDE_MODEL", "claude-sonnet-4-6"),
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
        return SyncAgentAdapter(Agent(model=model, system_prompt=system_prompt, tools=[]))

    raise ValueError(f"unknown EVAL_BACKEND: {backend!r}")

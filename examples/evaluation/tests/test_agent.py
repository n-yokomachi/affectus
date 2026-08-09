# examples/evaluation/tests/test_agent.py
from unittest.mock import patch

import pytest

from src.agent import ClaudeSDKConversation, SyncAgentAdapter, build_agent


# ---- system prompt assembly (checked via the default claude-sdk backend) ----

def test_build_agent_friendly_off_uses_baseline_prompt(monkeypatch):
    monkeypatch.delenv("EVAL_BACKEND", raising=False)
    agent = build_agent(personality="friendly", affectus_on=False)
    assert isinstance(agent, ClaudeSDKConversation)
    assert "明るく協力的" in agent.system_prompt
    assert "感情状態の参照" not in agent.system_prompt
    assert "<feel>" not in agent.system_prompt


def test_build_agent_friendly_on_appends_affectus_block(monkeypatch):
    monkeypatch.delenv("EVAL_BACKEND", raising=False)
    agent = build_agent(personality="friendly", affectus_on=True)
    assert "明るく協力的" in agent.system_prompt
    assert "感情状態の参照" in agent.system_prompt
    assert "[現在のあなたの感情:" in agent.system_prompt
    assert "<feel>" in agent.system_prompt
    # State value is NOT in the system prompt; it's injected per turn by run.py.
    assert "{{affectus_state}}" not in agent.system_prompt


def test_build_agent_contrarian_on_uses_contrarian_prompt(monkeypatch):
    monkeypatch.delenv("EVAL_BACKEND", raising=False)
    agent = build_agent(personality="contrarian", affectus_on=True)
    assert "天邪鬼" in agent.system_prompt
    assert "感情状態の参照" in agent.system_prompt


def test_build_agent_rejects_unknown_personality():
    with pytest.raises(ValueError, match="unknown personality"):
        build_agent(personality="grumpy", affectus_on=False)


def test_build_agent_rejects_unknown_backend(monkeypatch):
    monkeypatch.setenv("EVAL_BACKEND", "carrier-pigeon")
    with pytest.raises(ValueError, match="unknown EVAL_BACKEND"):
        build_agent(personality="friendly", affectus_on=False)


# ---- claude-sdk backend: character isolation options ----

def test_claude_sdk_conversation_isolates_the_character(monkeypatch):
    monkeypatch.delenv("EVAL_BACKEND", raising=False)
    monkeypatch.setenv("EVAL_CLAUDE_MODEL", "claude-sonnet-4-6")
    agent = build_agent(personality="friendly", affectus_on=True)
    opts = agent.options
    # persona REPLACES the default system prompt (plain string form)
    assert opts.system_prompt == agent.system_prompt
    # no filesystem settings (CLAUDE.md, output styles) are loaded.
    # [] emits --setting-sources= ; None would omit the flag and let the CLI
    # load user/project settings (the leak observed in the smoke test).
    assert opts.setting_sources == []
    # no tool definitions reach the model; exactly one assistant turn per query
    assert opts.tools == []
    assert opts.max_turns == 1
    assert opts.model == "claude-sonnet-4-6"
    # output cap for rough parity with the bedrock backend's max_tokens=512
    assert opts.env.get("CLAUDE_CODE_MAX_OUTPUT_TOKENS") == "512"


# ---- bedrock backend (kept for reproducibility of earlier runs) ----
# strands-agents is an optional extra (.[bedrock]); skipped when absent.

def test_build_agent_bedrock_backend_uses_temperature_zero_and_no_tools(monkeypatch):
    pytest.importorskip("strands")
    monkeypatch.setenv("EVAL_BACKEND", "bedrock")
    with patch("strands.models.BedrockModel") as mock_BedrockModel, \
         patch("strands.Agent") as mock_Agent:
        agent = build_agent(personality="friendly", affectus_on=False)
    assert isinstance(agent, SyncAgentAdapter)
    model_kwargs = mock_BedrockModel.call_args.kwargs
    assert model_kwargs["temperature"] == 0.0
    assert model_kwargs["max_tokens"] == 512
    agent_kwargs = mock_Agent.call_args.kwargs
    assert "明るく協力的" in agent_kwargs["system_prompt"]
    assert agent_kwargs["tools"] == []

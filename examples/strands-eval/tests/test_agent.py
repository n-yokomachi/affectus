# examples/strands-eval/tests/test_agent.py
import json
from unittest.mock import MagicMock, patch

import pytest

from src.agent import ClaudeCLIAgent, build_agent


# ---- system prompt assembly (backend-independent, checked via claude-cli) ----

def test_build_agent_friendly_off_uses_baseline_prompt(monkeypatch):
    monkeypatch.delenv("EVAL_BACKEND", raising=False)
    agent = build_agent(personality="friendly", affectus_on=False)
    assert isinstance(agent, ClaudeCLIAgent)
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


# ---- claude-cli backend ----

def _cli_result(session_id: str, text: str) -> MagicMock:
    proc = MagicMock()
    proc.returncode = 0
    proc.stdout = json.dumps({"session_id": session_id, "result": text, "is_error": False})
    proc.stderr = ""
    return proc


@patch("src.agent.subprocess.run")
def test_claude_cli_agent_threads_session_across_turns(mock_run):
    mock_run.side_effect = [_cli_result("sess-1", "reply1"), _cli_result("sess-2", "reply2")]
    agent = ClaudeCLIAgent(system_prompt="sp", model="claude-sonnet-4-6")

    assert agent("hi") == "reply1"
    first_cmd = mock_run.call_args_list[0].args[0]
    assert "--resume" not in first_cmd
    assert first_cmd[first_cmd.index("--system-prompt") + 1] == "sp"
    assert first_cmd[first_cmd.index("--setting-sources") + 1] == ""
    assert first_cmd[first_cmd.index("--model") + 1] == "claude-sonnet-4-6"
    assert first_cmd[-1] == "hi"

    assert agent("again") == "reply2"
    second_cmd = mock_run.call_args_list[1].args[0]
    assert second_cmd[second_cmd.index("--resume") + 1] == "sess-1"
    # the returned session id is carried forward for the next turn
    assert agent.session_id == "sess-2"


@patch("src.agent.subprocess.run")
def test_claude_cli_agent_raises_on_nonzero_exit(mock_run):
    proc = MagicMock()
    proc.returncode = 1
    proc.stdout = ""
    proc.stderr = "boom"
    mock_run.return_value = proc
    agent = ClaudeCLIAgent(system_prompt="sp", model="m")
    with pytest.raises(RuntimeError, match="claude -p failed"):
        agent("hi")


@patch("src.agent.subprocess.run")
def test_claude_cli_agent_raises_on_is_error(mock_run):
    proc = MagicMock()
    proc.returncode = 0
    proc.stdout = json.dumps({"session_id": "s", "result": "nope", "is_error": True})
    proc.stderr = ""
    mock_run.return_value = proc
    agent = ClaudeCLIAgent(system_prompt="sp", model="m")
    with pytest.raises(RuntimeError, match="returned error"):
        agent("hi")


# ---- bedrock backend (kept for reproducibility of earlier runs) ----

@patch("strands.models.BedrockModel")
@patch("strands.Agent")
def test_build_agent_bedrock_backend_uses_temperature_zero_and_no_tools(
    mock_Agent, mock_BedrockModel, monkeypatch
):
    monkeypatch.setenv("EVAL_BACKEND", "bedrock")
    build_agent(personality="friendly", affectus_on=False)
    model_kwargs = mock_BedrockModel.call_args.kwargs
    assert model_kwargs["temperature"] == 0.0
    assert model_kwargs["max_tokens"] == 512
    agent_kwargs = mock_Agent.call_args.kwargs
    assert "明るく協力的" in agent_kwargs["system_prompt"]
    assert agent_kwargs["tools"] == []

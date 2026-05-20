# examples/strands-eval/tests/test_agent.py
from unittest.mock import patch

import pytest

from src import agent as agent_mod


@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_friendly_off_uses_baseline_prompt_no_tools(mock_Agent, mock_BedrockModel):
    agent_mod.build_agent(personality="friendly", affectus_on=False)
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "明るく協力的" in sys_prompt
    assert "感情状態の参照" not in sys_prompt
    assert "<feel>" not in sys_prompt
    assert kwargs["tools"] == []


@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_friendly_on_appends_affectus_block(mock_Agent, mock_BedrockModel):
    agent_mod.build_agent(personality="friendly", affectus_on=True)
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "明るく協力的" in sys_prompt
    assert "感情状態の参照" in sys_prompt
    assert "[現在のあなたの感情:" in sys_prompt
    assert "<feel>" in sys_prompt
    # State value is NOT in the system prompt; it's injected per turn by run.py.
    assert "{{affectus_state}}" not in sys_prompt
    assert kwargs["tools"] == []


@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_contrarian_on_uses_contrarian_prompt(mock_Agent, mock_BedrockModel):
    agent_mod.build_agent(personality="contrarian", affectus_on=True)
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "天邪鬼" in sys_prompt
    assert "感情状態の参照" in sys_prompt


def test_build_agent_rejects_unknown_personality():
    with pytest.raises(ValueError, match="unknown personality"):
        agent_mod.build_agent(personality="grumpy", affectus_on=False)

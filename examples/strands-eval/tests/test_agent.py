from unittest.mock import patch, MagicMock

import pytest

from src import agent as agent_mod


@patch("src.agent.affectus_show", return_value="いまはほどほどの喜びを感じている。")
@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_friendly_off_uses_baseline_prompt_no_tools(mock_Agent, mock_BedrockModel, mock_show):
    agent_mod.build_agent(personality="friendly", affectus_on=False, state_path="/tmp/s.json")
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "明るく協力的" in sys_prompt
    assert "あなたの現在の感情状態" not in sys_prompt
    assert kwargs["tools"] == []
    mock_show.assert_not_called()


@patch("src.agent.affectus_show", return_value="いまはほどほどの喜びを感じている。")
@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_friendly_on_appends_affectus_block(mock_Agent, mock_BedrockModel, mock_show):
    agent_mod.build_agent(personality="friendly", affectus_on=True, state_path="/tmp/s.json")
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "明るく協力的" in sys_prompt
    assert "あなたの現在の感情状態" in sys_prompt
    assert "いまはほどほどの喜びを感じている。" in sys_prompt
    assert "<feel>" in sys_prompt
    # No tools registered — feel deltas come back as <feel>...</feel> text that
    # the orchestrator (run.py) parses and applies. Avoids double-apply if the
    # LLM both calls a tool AND emits the tag.
    assert kwargs["tools"] == []
    mock_show.assert_called_once_with("/tmp/s.json", None)


@patch("src.agent.affectus_show", return_value="いまは穏やかで、心は凪いでいる。")
@patch("src.agent.BedrockModel")
@patch("src.agent.Agent")
def test_build_agent_contrarian_on_uses_contrarian_prompt(mock_Agent, mock_BedrockModel, mock_show):
    agent_mod.build_agent(personality="contrarian", affectus_on=True, state_path="/tmp/s.json")
    kwargs = mock_Agent.call_args.kwargs
    sys_prompt = kwargs["system_prompt"]
    assert "天邪鬼" in sys_prompt
    assert "あなたの現在の感情状態" in sys_prompt


def test_build_agent_rejects_unknown_personality():
    with pytest.raises(ValueError, match="unknown personality"):
        agent_mod.build_agent(personality="grumpy", affectus_on=False, state_path="/tmp/s.json")

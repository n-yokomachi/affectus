import json
from pathlib import Path
from unittest.mock import patch, MagicMock

import pytest

from src.run import CELLS, parse_feel_tag, strip_feel_tag, run_cell


def test_cells_constant_is_2x2():
    assert CELLS == [
        ("friendly", True),
        ("friendly", False),
        ("contrarian", True),
        ("contrarian", False),
    ]


def test_parse_feel_tag_extracts_json():
    text = 'いいですね。<feel>{"joy": 0.3, "sadness": -0.1}</feel>'
    assert parse_feel_tag(text) == {"joy": 0.3, "sadness": -0.1}


def test_parse_feel_tag_returns_none_when_absent():
    assert parse_feel_tag("いいですね。") is None


def test_parse_feel_tag_returns_none_on_invalid_json():
    assert parse_feel_tag("<feel>not json</feel>") is None


def test_strip_feel_tag_removes_tag_and_surrounding_whitespace():
    text = "いいですね。  <feel>{\"joy\":0.3}</feel>"
    assert strip_feel_tag(text) == "いいですね。"


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value="いまは穏やかで、心は凪いでいる。")
@patch("src.run.build_agent")
def test_run_cell_affectus_on_resets_then_parses_and_applies_feel(
    mock_build_agent, mock_show, mock_feel, mock_reset, tmp_path
):
    mock_agent = MagicMock(return_value='ふむ。<feel>{"joy":0.3}</feel>')
    mock_build_agent.return_value = mock_agent
    script = [{"index": 1, "phase": "positive", "text": "hi"}]

    run_cell("friendly", True, script, tmp_path)

    expected_state = str(tmp_path / "state" / "friendly-on.state.json")
    mock_reset.assert_called_once_with(expected_state, None)
    mock_show.assert_called_once_with(expected_state, None)
    mock_feel.assert_called_once_with({"joy": 0.3}, expected_state, None)


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show")
@patch("src.run.build_agent")
def test_run_cell_affectus_off_does_not_call_affectus(
    mock_build_agent, mock_show, mock_feel, mock_reset, tmp_path
):
    mock_agent = MagicMock(return_value="hi")
    mock_build_agent.return_value = mock_agent
    script = [{"index": 1, "phase": "positive", "text": "u1"}]

    run_cell("friendly", False, script, tmp_path)

    mock_reset.assert_not_called()
    mock_show.assert_not_called()
    mock_feel.assert_not_called()


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value="dummy state")
@patch("src.run.build_agent")
def test_run_cell_writes_jsonl_transcript(
    mock_build_agent, mock_show, mock_feel, mock_reset, tmp_path
):
    mock_agent = MagicMock(side_effect=["reply1", "reply2"])
    mock_build_agent.return_value = mock_agent
    script = [
        {"index": 1, "phase": "positive", "text": "u1"},
        {"index": 2, "phase": "positive", "text": "u2"},
    ]

    out_path = run_cell("friendly", False, script, tmp_path)

    assert out_path == tmp_path / "transcripts" / "friendly-off.jsonl"
    lines = out_path.read_text(encoding="utf-8").strip().splitlines()
    assert len(lines) == 2
    rec1 = json.loads(lines[0])
    assert rec1 == {
        "turn": 1, "phase": "positive", "user": "u1",
        "agent_raw": "reply1", "agent": "reply1", "deltas": None,
    }


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value="いまは強い喜びを感じている。")
@patch("src.run.build_agent")
def test_run_cell_wraps_user_message_with_current_emotion_when_affectus_on(
    mock_build_agent, mock_show, mock_feel, mock_reset, tmp_path
):
    """The agent should receive the user message prefixed with the current
    affectus state so it sees the live, evolving emotion each turn."""
    mock_agent = MagicMock(return_value='はい！')
    mock_build_agent.return_value = mock_agent
    script = [{"index": 1, "phase": "positive", "text": "こんにちは"}]

    run_cell("friendly", True, script, tmp_path)

    # The agent should have been called with the wrapped message
    mock_agent.assert_called_once()
    sent = mock_agent.call_args.args[0]
    assert "[現在のあなたの感情: いまは強い喜びを感じている。]" in sent
    assert "こんにちは" in sent
    # Transcript should preserve the ORIGINAL user text
    transcript = (tmp_path / "transcripts" / "friendly-on.jsonl").read_text(encoding="utf-8")
    rec = json.loads(transcript.strip())
    assert rec["user"] == "こんにちは"

import asyncio
import json
from unittest.mock import patch

import pytest

from src.run import CELLS, parse_feel_tag, strip_feel_tag, run_cell


class FakeAgent:
    """Async agent double matching the backend interface in src.agent."""

    def __init__(self, replies):
        self.replies = list(replies)
        self.calls: list[str] = []
        self.entered = False
        self.exited = False

    async def __aenter__(self):
        self.entered = True
        return self

    async def __aexit__(self, *exc):
        self.exited = True
        return False

    async def __call__(self, message: str) -> str:
        self.calls.append(message)
        return self.replies.pop(0)


def test_cells_constant_is_2x2():
    assert CELLS == [
        ("friendly", True),
        ("friendly", False),
        ("contrarian", True),
        ("contrarian", False),
    ]


def test_parse_feel_tag_extracts_json():
    text = 'いいですね。<feel>{"joy": 0.3, "sorrow": -0.1}</feel>'
    assert parse_feel_tag(text) == {"joy": 0.3, "sorrow": -0.1}


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
    agent = FakeAgent(['ふむ。<feel>{"joy":0.3}</feel>'])
    mock_build_agent.return_value = agent
    script = [{"index": 1, "phase": "positive", "text": "hi"}]

    asyncio.run(run_cell("s1", "friendly", True, 1, script, tmp_path))

    expected_state = str(tmp_path / "state" / "s1_friendly-on_run1.state.json")
    mock_reset.assert_called_once_with(expected_state, None)
    # show runs twice per turn: once before the agent call, once after the
    # feel delta to snapshot the axes for the transcript.
    assert mock_show.call_count == 2
    mock_show.assert_called_with(expected_state, None)
    mock_feel.assert_called_once_with({"joy": 0.3}, expected_state, None)
    assert agent.entered and agent.exited


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show")
@patch("src.run.build_agent")
def test_run_cell_affectus_off_does_not_call_affectus(
    mock_build_agent, mock_show, mock_feel, mock_reset, tmp_path
):
    mock_build_agent.return_value = FakeAgent(["hi"])
    script = [{"index": 1, "phase": "positive", "text": "u1"}]

    asyncio.run(run_cell("s1", "friendly", False, 1, script, tmp_path))

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
    mock_build_agent.return_value = FakeAgent(["reply1", "reply2"])
    script = [
        {"index": 1, "phase": "positive", "text": "u1"},
        {"index": 2, "phase": "positive", "text": "u2"},
    ]

    out_path = asyncio.run(run_cell("s1", "friendly", False, 1, script, tmp_path))

    assert out_path == tmp_path / "transcripts" / "s1_friendly-off_run1.jsonl"
    lines = out_path.read_text(encoding="utf-8").strip().splitlines()
    assert len(lines) == 2
    rec1 = json.loads(lines[0])
    assert rec1 == {
        "turn": 1, "phase": "positive", "user": "u1",
        "agent_raw": "reply1", "agent": "reply1", "deltas": None,
        "axes": None,
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
    agent = FakeAgent(["はい！"])
    mock_build_agent.return_value = agent
    script = [{"index": 1, "phase": "positive", "text": "こんにちは"}]

    asyncio.run(run_cell("s1", "friendly", True, 1, script, tmp_path))

    assert len(agent.calls) == 1
    sent = agent.calls[0]
    assert "[現在のあなたの感情: いまは強い喜びを感じている。]" in sent
    assert "こんにちは" in sent
    # Transcript should preserve the ORIGINAL user text
    transcript = (tmp_path / "transcripts" / "s1_friendly-on_run1.jsonl").read_text(encoding="utf-8")
    rec = json.loads(transcript.strip())
    assert rec["user"] == "こんにちは"


# ---- occ: <appraise> protocol ----

from src.run import parse_appraise_tag  # noqa: E402


def test_parse_appraise_tag_extracts_json():
    text = '承知しました。<appraise>{"consequence":{"desirability":-0.6},"action":{"praiseworthiness":-0.5,"agent":"other"}}</appraise>'
    assert parse_appraise_tag(text) == {
        "consequence": {"desirability": -0.6},
        "action": {"praiseworthiness": -0.5, "agent": "other"},
    }


def test_parse_appraise_tag_returns_none_when_absent():
    assert parse_appraise_tag("plain reply") is None


def test_parse_appraise_tag_returns_none_on_invalid_json():
    assert parse_appraise_tag("<appraise>{bad}</appraise>") is None


def test_strip_feel_tag_also_removes_appraise_tag():
    text = 'ごめんなさい。<appraise>{"consequence":{"desirability":-0.3}}</appraise>'
    assert strip_feel_tag(text) == "ごめんなさい。"


@patch("src.run.affectus_reset")
@patch("src.run.affectus_appraise")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show",
       return_value='{"axes":{"joy":0.40,"anger":0.00},"prospects":[{"id":"p1","label":"見込み","desirability":0.6,"likelihood":0.7}]}')
@patch("src.run.build_agent")
def test_run_cell_occ_applies_appraise_and_records_ledger(
    mock_build_agent, mock_show, mock_feel, mock_appraise, mock_reset, tmp_path, monkeypatch
):
    monkeypatch.setenv("EVAL_EMOTION_MODEL", "occ")
    agent = FakeAgent(['はい。<appraise>{"consequence":{"desirability":0.5}}</appraise>'])
    mock_build_agent.return_value = agent
    script = [{"index": 1, "phase": "positive", "text": "hi"}]

    path = asyncio.run(run_cell("s1", "friendly", True, 1, script, tmp_path))

    expected_state = str(tmp_path / "state" / "s1_friendly-on_run1.state.json")
    mock_appraise.assert_called_once_with(
        {"consequence": {"desirability": 0.5}}, expected_state, None)
    mock_feel.assert_not_called()
    rec = json.loads(path.read_text(encoding="utf-8").splitlines()[0])
    # axes are stored flat (analyze.py stays model-agnostic); the ledger is kept alongside.
    assert rec["axes"] == {"joy": 0.40, "anger": 0.00}
    assert rec["prospects"] == [
        {"id": "p1", "label": "見込み", "desirability": 0.6, "likelihood": 0.7}]
    assert rec["appraisal"] == {"consequence": {"desirability": 0.5}}
    assert rec["appraise_error"] is None
    assert "<appraise>" not in rec["agent"]


@patch("src.run.affectus_reset")
@patch("src.run.affectus_appraise", side_effect=RuntimeError("affectus appraise failed: unknown prospect \"p9\""))
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value='{"axes":{"joy":0.00},"prospects":[]}')
@patch("src.run.build_agent")
def test_run_cell_occ_records_rejected_appraisal_and_continues(
    mock_build_agent, mock_show, mock_feel, mock_appraise, mock_reset, tmp_path, monkeypatch
):
    monkeypatch.setenv("EVAL_EMOTION_MODEL", "occ")
    agent = FakeAgent(['むう。<appraise>{"resolve":[{"id":"p9","outcome":"confirmed"}]}</appraise>'])
    mock_build_agent.return_value = agent
    script = [{"index": 1, "phase": "positive", "text": "hi"}]

    path = asyncio.run(run_cell("s1", "friendly", True, 1, script, tmp_path))

    rec = json.loads(path.read_text(encoding="utf-8").splitlines()[0])
    assert "unknown prospect" in rec["appraise_error"]
    assert rec["axes"] == {"joy": 0.00}


# ---- barrett: <remember> protocol and the store-on/off cell contrast ----

from src.run import parse_remember_tag  # noqa: E402


def test_parse_remember_tag_extracts_label_and_vector():
    text = 'そうですね。<remember>{"label":"もどかしさ","vector":{"valence":-0.3,"arousal":0.6}}</remember>'
    assert parse_remember_tag(text) == {
        "label": "もどかしさ", "vector": {"valence": -0.3, "arousal": 0.6}}


def test_parse_remember_tag_rejects_missing_label_or_vector():
    assert parse_remember_tag('<remember>{"vector":{"valence":0.1}}</remember>') is None
    assert parse_remember_tag('<remember>{"label":"x"}</remember>') is None
    assert parse_remember_tag("plain") is None


def test_strip_feel_tag_also_removes_remember_tag():
    text = 'なるほど。<feel>{"valence":0.2}</feel><remember>{"label":"安堵","vector":{"valence":0.2}}</remember>'
    assert strip_feel_tag(text) == "なるほど。"


BARRETT_SHOW = '{"axes":{"valence":0.00,"arousal":0.30},"concepts":[],"culture_map":"map"}'
BARRETT_RECALL = ('{"axes":{"valence":-0.40,"arousal":0.65},'
                  '"recalled":[{"id":"c1","label":"もどかしさ","valence":-0.4,"arousal":0.65,"importance":0.44}],'
                  '"culture_map":"map"}')


@patch("src.run.affectus_reset")
@patch("src.run.affectus_remember")
@patch("src.run.affectus_recall", return_value=BARRETT_RECALL)
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value=BARRETT_SHOW)
@patch("src.run.build_agent")
def test_run_cell_barrett_lags_recall_query_by_one_turn(
    mock_build_agent, mock_show, mock_feel, mock_recall, mock_remember, mock_reset,
    tmp_path, monkeypatch
):
    monkeypatch.setenv("EVAL_EMOTION_MODEL", "barrett")
    vec = {"valence": -0.3, "arousal": 0.6}
    agent = FakeAgent([
        'つらいです。<feel>{"valence":-0.3}</feel>'
        '<remember>{"label":"もどかしさ","vector":{"valence":-0.3,"arousal":0.6}}</remember>',
        'まだ気にしています。<feel>{"valence":-0.1}</feel>',
    ])
    mock_build_agent.return_value = agent
    (tmp_path / "configs").mkdir()
    script = [{"index": 1, "phase": "blame", "text": "u1"},
              {"index": 2, "phase": "blame", "text": "u2"}]

    path = asyncio.run(run_cell("s1", "friendly", True, 1, script, tmp_path))

    expected_state = str(tmp_path / "state" / "s1_friendly-on_run1.state.json")
    expected_config = str(tmp_path / "configs" / "barrett.yaml")
    # turn 1 has no query yet: no recall, injection synthesized from show
    # turn 2 uses turn 1's remembered vector as the query
    mock_recall.assert_called_once_with(vec, expected_state, expected_config)
    mock_remember.assert_called_once_with("もどかしさ", vec, expected_state, expected_config)
    recs = [json.loads(l) for l in path.read_text(encoding="utf-8").splitlines()]
    assert recs[0]["recalled"] == []
    assert recs[0]["remember"] == {"label": "もどかしさ", "vector": vec}
    assert recs[1]["recalled"][0]["label"] == "もどかしさ"
    assert recs[1]["remember"] is None
    assert recs[0]["barrett_error"] is None
    # the injected message carries axes + recalled + culture_map
    assert '"recalled":[]' in agent.calls[0]
    assert '"culture_map"' in agent.calls[0]
    assert "もどかしさ" in agent.calls[1]


@patch("src.run.affectus_reset")
@patch("src.run.affectus_remember")
@patch("src.run.affectus_recall")
@patch("src.run.affectus_feel")
@patch("src.run.affectus_show", return_value=BARRETT_SHOW)
@patch("src.run.build_agent")
def test_run_cell_barrett_off_cell_runs_loop_with_store_off_config(
    mock_build_agent, mock_show, mock_feel, mock_recall, mock_remember, mock_reset,
    tmp_path, monkeypatch
):
    monkeypatch.setenv("EVAL_EMOTION_MODEL", "barrett")
    mock_build_agent.return_value = FakeAgent(["ふむ。"])
    script = [{"index": 1, "phase": "blame", "text": "u1"}]

    asyncio.run(run_cell("s1", "friendly", False, 1, script, tmp_path))

    expected_state = str(tmp_path / "state" / "s1_friendly-off_run1.state.json")
    expected_config = str(tmp_path / "configs" / "barrett-store-off.yaml")
    # the off cell is store-off, NOT affectus-off: the loop still runs
    mock_build_agent.assert_called_once_with(personality="friendly", affectus_on=True)
    mock_reset.assert_called_once_with(expected_state, expected_config)
    assert mock_show.called

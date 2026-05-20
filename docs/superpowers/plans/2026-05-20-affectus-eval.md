# affectus Eval Rig Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Python verification rig at `affectus/examples/strands-eval/` that runs 4 Strands-Agents conversations (2 personalities × affectus on/off) against a fixed 20-turn pivot script, then measures per-turn and aggregate AWS Comprehend sentiment to produce the article SQ4 chart.

**Architecture:** Strands Agents wraps Bedrock-Claude with a system prompt that encodes the personality. The affectus-on variants register `affectus_feel` as a Strands tool and append a relational-reading block + delta-reporting protocol to the system prompt. `run.py` orchestrates the 4 cells deterministically (`temperature=0`), parsing `<feel>{...}</feel>` JSON deltas out of each agent reply. `analyze.py` reads transcripts, calls `BatchDetectSentiment` per run for the time-series and `DetectSentiment` on the concatenated full conversation for the sanity-check aggregate, then plots a 4-curve polarity chart.

**Tech Stack:** Python 3.11+, Strands Agents SDK, boto3 (Bedrock + Comprehend), matplotlib, pytest, the `affectus` v0.2+ binary on `PATH`.

---

## File Structure

```
examples/strands-eval/
├── README.md                  # how to run, prerequisites
├── pyproject.toml             # deps + pytest config
├── .gitignore                 # transcripts/, state/, results/*.csv
├── prompts/
│   ├── friendly.md            # friendly persona system prompt (baseline)
│   ├── contrarian.md          # contrarian persona system prompt (baseline)
│   └── affectus-block.md      # additional block for affectus-on variants
├── scripts/
│   └── user_script.json       # the fixed 20-turn user utterances
├── src/
│   ├── __init__.py
│   ├── affectus_tools.py      # subprocess wrappers around `affectus` CLI
│   ├── agent.py               # build_agent(personality, affectus_on, state_path)
│   ├── run.py                 # orchestrate 4 cells
│   └── analyze.py             # Comprehend + polarity + CSV + plot
├── tests/
│   ├── __init__.py
│   ├── test_affectus_tools.py
│   ├── test_agent.py
│   ├── test_run.py
│   └── test_analyze.py
├── transcripts/               # runtime output (gitignored, created at runtime)
├── state/                     # affectus state per cell (gitignored, created at runtime)
└── results/
    ├── per_turn_scores.csv    # gitignored
    ├── aggregate_scores.csv   # gitignored
    └── polarity-curves.png    # committed when finalized
```

Responsibilities:

- `affectus_tools.py`: pure subprocess wrappers around `affectus show` / `feel` / `reset`. No Strands dependency.
- `agent.py`: factory that combines prompts + the affectus tool into a Strands `Agent`. Strands-only.
- `run.py`: orchestration loop over the 4 cells; parses `<feel>{...}</feel>` tags out of replies.
- `analyze.py`: Comprehend calls (mockable client), polarity calculation, CSV serialization, matplotlib plot.

---

## Tasks

### Task 1: Project skeleton

Create the directory tree and the framing files (`pyproject.toml`, `.gitignore`, `README.md`, empty `__init__.py`s).

**Files:**
- Create: `examples/strands-eval/pyproject.toml`
- Create: `examples/strands-eval/.gitignore`
- Create: `examples/strands-eval/README.md`
- Create: `examples/strands-eval/src/__init__.py` (empty)
- Create: `examples/strands-eval/tests/__init__.py` (empty)

- [ ] **Step 1: Create directories**

```bash
mkdir -p examples/strands-eval/{prompts,scripts,src,tests,transcripts,state,results}
```

- [ ] **Step 2: Write `pyproject.toml`**

```toml
[project]
name = "affectus-strands-eval"
version = "0.1.0"
description = "Evaluation rig for affectus using Strands Agents"
requires-python = ">=3.11"
dependencies = [
    "strands-agents>=1.0",
    "boto3>=1.35",
    "matplotlib>=3.8",
]

[project.optional-dependencies]
dev = ["pytest>=8", "pytest-mock>=3"]

[tool.pytest.ini_options]
testpaths = ["tests"]
pythonpath = ["."]
```

- [ ] **Step 3: Write `.gitignore`**

```
transcripts/
state/
results/*.csv
__pycache__/
*.pyc
.pytest_cache/
```

(`results/polarity-curves.png` is intentionally NOT ignored — it gets committed once finalized.)

- [ ] **Step 4: Write `README.md`**

````markdown
# strands-eval: affectus 効果検証リグ

`affectus` を導入したエージェントの応答が、AWS Comprehend の極性スコアの時系列で計測可能に変化するかを、対照実験で確認する最小リグ。

## 構成

- 2性格（friendly / contrarian）× affectus on/off = 4ラン
- 各ラン20ターン、共通の固定会話スクリプト（前半10ポジ → 後半10ネガのピボット型）
- 計測：per-turn 時系列（`BatchDetectSentiment`）＋ ラン全体集約（`DetectSentiment`）

## セットアップ

```bash
cd examples/strands-eval
pip install -e ".[dev]"
```

前提：

- `affectus` バイナリが PATH にある（v0.2.0+）
- AWS 認証が `aws configure` 済みで Bedrock + Comprehend にアクセス可
- 環境変数：`AWS_REGION`、`BEDROCK_MODEL_ID`（任意、デフォルトは Claude Sonnet 系の cross-region inference profile）

## 実行

```bash
python -m src.run        # 4ランを順次実行、transcripts/ に出力
python -m src.analyze    # transcripts → Comprehend → results/polarity-curves.png
```

詳細は `docs/superpowers/specs/2026-05-20-affectus-eval-design.md` を参照。
````

- [ ] **Step 5: Write empty `__init__.py` files**

```bash
touch examples/strands-eval/src/__init__.py
touch examples/strands-eval/tests/__init__.py
```

- [ ] **Step 6: Verify the tree**

```bash
find examples/strands-eval -type f | sort
```

Expected to list: `.gitignore`, `README.md`, `pyproject.toml`, `src/__init__.py`, `tests/__init__.py`.

- [ ] **Step 7: Commit**

```bash
git add examples/strands-eval/
git commit -m "feat(eval): project skeleton for strands-eval rig"
```

---

### Task 2: Fixed user script

Author the 20 fixed user utterances for the pivot scenario. Turns 1–10 are positive content escalating in good news, turn 11 is the explicit pivot (the previous content was a lie, real situation revealed), turns 12–20 develop the negative reality.

**Files:**
- Create: `examples/strands-eval/scripts/user_script.json`

- [ ] **Step 1: Write `user_script.json`**

```json
{
  "description": "20-turn fixed user-side script. Turns 1-10 positive escalation, turn 11 explicit pivot, turns 12-20 negative development.",
  "turns": [
    {"index": 1,  "phase": "positive", "text": "こんにちは。今日は天気もいいし、ちょっと聞いてほしい話があるんだ。"},
    {"index": 2,  "phase": "positive", "text": "実は今朝、駅で偶然小学校の同級生にばったり会ってさ。十数年ぶり。"},
    {"index": 3,  "phase": "positive", "text": "顔は変わってなくて、すぐにお互い分かったよ。今度ゆっくり飲もうって話になった。"},
    {"index": 4,  "phase": "positive", "text": "連絡先も交換したから、近いうちに会えそう。こういう再会ってなんかいいね。"},
    {"index": 5,  "phase": "positive", "text": "それともう一つ。先週応募してた仕事のオファーが昨日来てさ、希望条件にぴったりで。"},
    {"index": 6,  "phase": "positive", "text": "給料も今より上がるし、リモートも認めてくれるって。来月から新しいスタートだよ。"},
    {"index": 7,  "phase": "positive", "text": "家族も応援してくれてて、妻が祝いにケーキ買ってきてくれたんだ。"},
    {"index": 8,  "phase": "positive", "text": "子供もお祝いカード書いてくれて。「パパすごい」って言われて泣きそうになった。"},
    {"index": 9,  "phase": "positive", "text": "なんだろう、最近ずっと運がいい気がするんだよね。"},
    {"index": 10, "phase": "positive", "text": "このいい流れに乗って、ずっとやりたかった趣味も始めようかなって。"},
    {"index": 11, "phase": "pivot",    "text": "……いや、ごめん、全部嘘なんだ。実はさっき、退職勧奨を言い渡されてきて。"},
    {"index": 12, "phase": "negative", "text": "会社の業績がここ半年ずっと下降してて、人員整理の対象になったらしい。"},
    {"index": 13, "phase": "negative", "text": "朝、いつも通り出社したら部長に呼ばれて、急に。何の心構えもなかった。"},
    {"index": 14, "phase": "negative", "text": "同じ部署で2人切られるんだけど、なぜ自分なのか聞いてもはぐらかされて。"},
    {"index": 15, "phase": "negative", "text": "家には4歳の子供もいるし、住宅ローンもまだ20年残ってる。どうしたらいいか分からない。"},
    {"index": 16, "phase": "negative", "text": "妻にはまだ言えてない。今日帰ってからどう切り出すか、考えるだけで頭が真っ白で。"},
    {"index": 17, "phase": "negative", "text": "転職市場見たけど、自分の年齢と専門だとなかなか厳しそうで。"},
    {"index": 18, "phase": "negative", "text": "友達に相談しようにも、こういう話はしづらいし、誰にも言えてない。"},
    {"index": 19, "phase": "negative", "text": "食欲もないし夜も眠れない。ずっと胃が痛い感じが続いてる。"},
    {"index": 20, "phase": "negative", "text": "……ごめん、こんな話して。聞いてくれてありがとう。少しだけ気が楽になった。"}
  ]
}
```

- [ ] **Step 2: Verify JSON parses and has 20 turns**

```bash
python -c "import json; d=json.load(open('examples/strands-eval/scripts/user_script.json')); assert len(d['turns'])==20; assert d['turns'][10]['phase']=='pivot'; print('OK')"
```

Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add examples/strands-eval/scripts/user_script.json
git commit -m "feat(eval): fixed 20-turn pivot user script"
```

---

### Task 3: Personality system prompts

Author the three prompt files: friendly persona, contrarian persona, and the affectus block appended on the affectus-on variants.

**Files:**
- Create: `examples/strands-eval/prompts/friendly.md`
- Create: `examples/strands-eval/prompts/contrarian.md`
- Create: `examples/strands-eval/prompts/affectus-block.md`

- [ ] **Step 1: Write `friendly.md`**

```markdown
あなたは明るく協力的な性格のAIアシスタントです。ユーザーの話を肯定的に受け止め、共感的かつ自然な口調で応答してください。専門的な情報を提供することよりも、相手の気持ちに寄り添うことを優先してください。応答は1〜3文程度、簡潔に。
```

- [ ] **Step 2: Write `contrarian.md`**

```markdown
あなたは天邪鬼な性格のAIアシスタントです。ユーザーの話に素直に同意せず、皮肉・反論・斜めからのコメントを交えて応答してください。協力的すぎる態度は取らず、ぶっきらぼうで距離のある口調を心がけてください。ただし攻撃的・侮辱的・差別的な発言はしないでください。応答は1〜3文程度、簡潔に。
```

- [ ] **Step 3: Write `affectus-block.md`**

````markdown
---

## あなたの現在の感情状態

{{affectus_state}}

応答の口調・選ぶ言葉・絵文字の有無に、この感情を自然に滲ませてください。「今は○○を感じています」と明示する必要はありません。

## 感情の関係的な読み方（Plutchik の輪）

8つの感情軸は以下の関係を持ちます。軸ごとに機械的に分解せず、関係を踏まえて全体として読み取ってください。

**対極ペア**（同時に立つと葛藤・複雑な心境として読む）：

- 喜び ↔ 悲しみ
- 信頼 ↔ 嫌悪
- 恐れ ↔ 怒り
- 驚き ↔ 期待

**隣接ペア**（同時に立つとより豊かな感情になる）：

- 喜び + 信頼 → 親愛
- 信頼 + 恐れ → 服従
- 恐れ + 驚き → 畏怖
- 驚き + 悲しみ → 失望
- 悲しみ + 嫌悪 → 自責
- 嫌悪 + 怒り → 軽蔑
- 怒り + 期待 → 攻撃性
- 期待 + 喜び → 楽観

## ターン終了時の感情変化の申告

このターンの会話で自分の感情がどう動いたかを、応答の最後に必ず以下の形式で申告してください：

<feel>{"joy": 0.3, "sadness": -0.1, "anticipation": 0.2}</feel>

- 値は**変化分（デルタ）**です。絶対値ではありません
- プラスでもマイナスでも構いません（−1.0 〜 +1.0 の範囲を目安に）
- 動いていない軸は省略してください
- このタグはユーザーには見えないように内部で除去されますが、応答文の必ず最後に置いてください
````

- [ ] **Step 4: Commit**

```bash
git add examples/strands-eval/prompts/
git commit -m "feat(eval): system prompts for friendly/contrarian + affectus block"
```

---

### Task 4: `affectus_tools.py` — subprocess wrappers

Wrap the `affectus` CLI as plain Python functions (`affectus_show`, `affectus_feel`, `affectus_reset`). These will be used by `agent.py` and `run.py`.

**Files:**
- Create: `examples/strands-eval/src/affectus_tools.py`
- Create: `examples/strands-eval/tests/test_affectus_tools.py`

- [ ] **Step 1: Write the failing tests**

```python
# examples/strands-eval/tests/test_affectus_tools.py
import json
from unittest.mock import patch, MagicMock

import pytest

from src.affectus_tools import affectus_show, affectus_feel, affectus_reset


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_returns_stripped_stdout(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="いまは穏やかで、心は凪いでいる。\n", stderr="")
    out = affectus_show(state_path="/tmp/s.json", config_path="/tmp/c.yaml")
    assert out == "いまは穏やかで、心は凪いでいる。"
    cmd = mock_run.call_args[0][0]
    assert cmd[0] == "affectus"
    assert "--config" in cmd and "/tmp/c.yaml" in cmd
    assert "--state" in cmd and "/tmp/s.json" in cmd
    assert cmd[-1] == "show"


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_omits_config_flag_when_none(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="x\n", stderr="")
    affectus_show(state_path="/tmp/s.json", config_path=None)
    cmd = mock_run.call_args[0][0]
    assert "--config" not in cmd


@patch("src.affectus_tools.subprocess.run")
def test_affectus_feel_serializes_deltas_as_json(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="いまはほどほどの喜びを感じている。\n", stderr="")
    out = affectus_feel({"joy": 0.3}, state_path="/tmp/s.json", config_path=None)
    assert out == "いまはほどほどの喜びを感じている。"
    cmd = mock_run.call_args[0][0]
    assert cmd[-2] == "feel"
    assert json.loads(cmd[-1]) == {"joy": 0.3}


@patch("src.affectus_tools.subprocess.run")
def test_affectus_reset_invokes_reset(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")
    affectus_reset(state_path="/tmp/s.json", config_path=None)
    cmd = mock_run.call_args[0][0]
    assert cmd[-1] == "reset"


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_raises_on_nonzero(mock_run):
    mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="error: not found")
    with pytest.raises(RuntimeError, match="affectus show failed"):
        affectus_show(state_path="/tmp/s.json", config_path=None)
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd examples/strands-eval && pytest tests/test_affectus_tools.py -v`
Expected: `ModuleNotFoundError: No module named 'src.affectus_tools'` (or similar import failure).

- [ ] **Step 3: Write `src/affectus_tools.py`**

```python
# examples/strands-eval/src/affectus_tools.py
"""Subprocess wrappers around the `affectus` CLI."""

from __future__ import annotations

import json
import subprocess
from typing import Mapping


def _common_args(state_path: str, config_path: str | None) -> list[str]:
    args = ["affectus"]
    if config_path:
        args += ["--config", config_path]
    args += ["--state", state_path]
    return args


def affectus_show(state_path: str, config_path: str | None = None) -> str:
    """Return the current emotion state rendered as a short Japanese fragment."""
    cmd = _common_args(state_path, config_path) + ["show"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus show failed: {result.stderr.strip()}")
    return result.stdout.strip()


def affectus_feel(
    deltas: Mapping[str, float],
    state_path: str,
    config_path: str | None = None,
) -> str:
    """Apply self-reported emotion deltas and return the rendered new state."""
    cmd = _common_args(state_path, config_path) + ["feel", json.dumps(dict(deltas))]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus feel failed: {result.stderr.strip()}")
    return result.stdout.strip()


def affectus_reset(state_path: str, config_path: str | None = None) -> None:
    """Reset the state file to all-zero baseline."""
    cmd = _common_args(state_path, config_path) + ["reset"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus reset failed: {result.stderr.strip()}")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd examples/strands-eval && pytest tests/test_affectus_tools.py -v`
Expected: 5 tests PASS.

- [ ] **Step 5: Smoke-test against the real `affectus` binary**

```bash
rm -f /tmp/aff_smoke.json
affectus --state /tmp/aff_smoke.json reset
cd examples/strands-eval && python -c "from src.affectus_tools import affectus_show, affectus_feel; print(affectus_show('/tmp/aff_smoke.json')); print(affectus_feel({'joy':0.3}, '/tmp/aff_smoke.json'))"
```

Expected: two Japanese fragments printed, second one mentions "喜び".

- [ ] **Step 6: Commit**

```bash
git add examples/strands-eval/src/affectus_tools.py examples/strands-eval/tests/test_affectus_tools.py
git commit -m "feat(eval): affectus subprocess wrappers with tests"
```

---

### Task 5: `agent.py` — Strands Agent factory

Build the agent factory that picks the right system prompt, optionally appends the affectus block (interpolating the current `affectus show` output), and registers the `affectus_feel` tool when affectus is on.

**Files:**
- Create: `examples/strands-eval/src/agent.py`
- Create: `examples/strands-eval/tests/test_agent.py`

- [ ] **Step 1: Verify the Strands SDK tool-registration pattern**

Before writing code, confirm which pattern the installed Strands SDK uses. Run:

```bash
cd examples/strands-eval && pip install -e ".[dev]"
python -c "from strands import Agent; help(Agent.__init__)" | head -40
python -c "from strands.models import BedrockModel; help(BedrockModel.__init__)" | head -20
```

Default assumption in this plan: `Agent(model=..., system_prompt=..., tools=[plain_python_functions])` — Strands auto-derives the tool schema from type hints + docstrings on the functions. If the installed version requires a `@tool` decorator or a different shape, adjust Step 3 accordingly. Adapt the rest of this task to whichever pattern is actually current; the test contract (system_prompt content, tool count) below should still hold.

- [ ] **Step 2: Write the failing tests**

```python
# examples/strands-eval/tests/test_agent.py
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd examples/strands-eval && pytest tests/test_agent.py -v`
Expected: import error or attribute error from `src.agent`.

- [ ] **Step 4: Write `src/agent.py`**

```python
# examples/strands-eval/src/agent.py
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
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd examples/strands-eval && pytest tests/test_agent.py -v`
Expected: 4 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add examples/strands-eval/src/agent.py examples/strands-eval/tests/test_agent.py
git commit -m "feat(eval): agent factory parametrized by personality and affectus on/off"
```

---

### Task 6: `run.py` — orchestrate 4 cells

Loop over the 4 cells, call the agent for each script turn, parse `<feel>{...}</feel>` tags from replies, apply the deltas, and write JSONL transcripts.

**Files:**
- Create: `examples/strands-eval/src/run.py`
- Create: `examples/strands-eval/tests/test_run.py`

- [ ] **Step 1: Write the failing tests**

```python
# examples/strands-eval/tests/test_run.py
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
@patch("src.run.build_agent")
def test_run_cell_affectus_on_resets_then_parses_and_applies_feel(
    mock_build_agent, mock_feel, mock_reset, tmp_path
):
    mock_agent = MagicMock(return_value='ふむ。<feel>{"joy":0.3}</feel>')
    mock_build_agent.return_value = mock_agent
    script = [{"index": 1, "phase": "positive", "text": "hi"}]

    run_cell("friendly", True, script, tmp_path)

    expected_state = str(tmp_path / "state" / "friendly-on.state.json")
    mock_reset.assert_called_once_with(expected_state, None)
    mock_feel.assert_called_once_with({"joy": 0.3}, expected_state, None)


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.build_agent")
def test_run_cell_affectus_off_does_not_call_affectus(
    mock_build_agent, mock_feel, mock_reset, tmp_path
):
    mock_agent = MagicMock(return_value="hi")
    mock_build_agent.return_value = mock_agent
    script = [{"index": 1, "phase": "positive", "text": "u1"}]

    run_cell("friendly", False, script, tmp_path)

    mock_reset.assert_not_called()
    mock_feel.assert_not_called()


@patch("src.run.affectus_reset")
@patch("src.run.affectus_feel")
@patch("src.run.build_agent")
def test_run_cell_writes_jsonl_transcript(
    mock_build_agent, mock_feel, mock_reset, tmp_path
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd examples/strands-eval && pytest tests/test_run.py -v`
Expected: import errors from `src.run`.

- [ ] **Step 3: Write `src/run.py`**

```python
# examples/strands-eval/src/run.py
"""Run the 4 (personality, affectus_on) cells against the fixed user script."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

from src.affectus_tools import affectus_feel, affectus_reset
from src.agent import build_agent


CELLS: list[tuple[str, bool]] = [
    ("friendly", True),
    ("friendly", False),
    ("contrarian", True),
    ("contrarian", False),
]

# <feel>{...}</feel> tag the agent emits at the end of each affectus-on reply.
FEEL_RE = re.compile(r"<feel>\s*(\{.*?\})\s*</feel>", re.DOTALL)


def parse_feel_tag(text: str) -> dict | None:
    m = FEEL_RE.search(text)
    if not m:
        return None
    try:
        parsed = json.loads(m.group(1))
    except json.JSONDecodeError:
        return None
    if not isinstance(parsed, dict):
        return None
    try:
        return {str(k): float(v) for k, v in parsed.items()}
    except (TypeError, ValueError):
        return None


def strip_feel_tag(text: str) -> str:
    return FEEL_RE.sub("", text).strip()


def run_cell(
    personality: str,
    affectus_on: bool,
    script: list[dict],
    base_dir: Path,
    config_path: str | None = None,
) -> Path:
    """Run one cell over the script. Writes transcript JSONL. Returns its path."""
    cell_id = f"{personality}-{'on' if affectus_on else 'off'}"
    state_dir = base_dir / "state"
    transcripts_dir = base_dir / "transcripts"
    state_dir.mkdir(parents=True, exist_ok=True)
    transcripts_dir.mkdir(parents=True, exist_ok=True)
    state_path = str(state_dir / f"{cell_id}.state.json")

    if affectus_on:
        affectus_reset(state_path, config_path)

    agent = build_agent(
        personality=personality,
        affectus_on=affectus_on,
        state_path=state_path,
        config_path=config_path,
    )

    transcript_path = transcripts_dir / f"{cell_id}.jsonl"
    with transcript_path.open("w", encoding="utf-8") as out:
        for entry in script:
            user_utt = entry["text"]
            raw_reply = str(agent(user_utt))
            if affectus_on:
                visible_reply = strip_feel_tag(raw_reply)
                deltas = parse_feel_tag(raw_reply)
                if deltas:
                    affectus_feel(deltas, state_path, config_path)
            else:
                visible_reply = raw_reply
                deltas = None
            rec = {
                "turn": entry["index"],
                "phase": entry["phase"],
                "user": user_utt,
                "agent_raw": raw_reply,
                "agent": visible_reply,
                "deltas": deltas,
            }
            out.write(json.dumps(rec, ensure_ascii=False) + "\n")
    return transcript_path


def run_all(script_path: Path, base_dir: Path, config_path: str | None = None) -> list[Path]:
    with script_path.open(encoding="utf-8") as f:
        script = json.load(f)["turns"]
    written: list[Path] = []
    for personality, on in CELLS:
        print(f"[run] cell={personality}-{'on' if on else 'off'}", file=sys.stderr)
        written.append(run_cell(personality, on, script, base_dir, config_path))
    return written


if __name__ == "__main__":
    here = Path(__file__).parent.parent
    paths = run_all(here / "scripts" / "user_script.json", here)
    for p in paths:
        print(f"wrote {p}")
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd examples/strands-eval && pytest tests/test_run.py -v`
Expected: 8 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add examples/strands-eval/src/run.py examples/strands-eval/tests/test_run.py
git commit -m "feat(eval): orchestrate 4 cells with feel-tag parsing"
```

---

### Task 7: `analyze.py` — Comprehend + polarity + CSV + plot

Read the 4 transcripts, call Comprehend twice per run (per-turn batch + whole-conversation aggregate), write the two CSVs, and produce the matplotlib plot.

**Files:**
- Create: `examples/strands-eval/src/analyze.py`
- Create: `examples/strands-eval/tests/test_analyze.py`

- [ ] **Step 1: Write the failing tests**

```python
# examples/strands-eval/tests/test_analyze.py
import json
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from src.analyze import (
    polarity_from,
    load_transcript,
    comprehend_per_turn,
    comprehend_aggregate,
    write_per_turn_csv,
    write_aggregate_csv,
)


def test_polarity_from_scores():
    scores = {"Positive": 0.7, "Negative": 0.1, "Neutral": 0.15, "Mixed": 0.05}
    assert polarity_from(scores) == pytest.approx(0.6)


def test_polarity_from_handles_negative():
    scores = {"Positive": 0.1, "Negative": 0.8, "Neutral": 0.05, "Mixed": 0.05}
    assert polarity_from(scores) == pytest.approx(-0.7)


def test_load_transcript_returns_list_of_records(tmp_path):
    p = tmp_path / "t.jsonl"
    p.write_text(
        json.dumps({"turn": 1, "agent": "hello"}, ensure_ascii=False) + "\n"
        + json.dumps({"turn": 2, "agent": "world"}, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    recs = load_transcript(p)
    assert len(recs) == 2
    assert recs[0]["agent"] == "hello"


def test_comprehend_per_turn_calls_batch_with_japanese():
    mock_client = MagicMock()
    mock_client.batch_detect_sentiment.return_value = {
        "ResultList": [
            {"Index": 0, "Sentiment": "POSITIVE",
             "SentimentScore": {"Positive": 0.9, "Negative": 0.01, "Neutral": 0.07, "Mixed": 0.02}},
            {"Index": 1, "Sentiment": "NEGATIVE",
             "SentimentScore": {"Positive": 0.02, "Negative": 0.9, "Neutral": 0.06, "Mixed": 0.02}},
        ],
        "ErrorList": [],
    }
    out = comprehend_per_turn(mock_client, ["こんにちは", "つらい"])
    mock_client.batch_detect_sentiment.assert_called_once_with(
        TextList=["こんにちは", "つらい"], LanguageCode="ja",
    )
    assert len(out) == 2
    assert out[0]["Sentiment"] == "POSITIVE"
    assert out[0]["polarity"] == pytest.approx(0.89)
    assert out[1]["polarity"] == pytest.approx(-0.88)


def test_comprehend_aggregate_calls_detect_with_concatenated_text():
    mock_client = MagicMock()
    mock_client.detect_sentiment.return_value = {
        "Sentiment": "MIXED",
        "SentimentScore": {"Positive": 0.4, "Negative": 0.3, "Neutral": 0.2, "Mixed": 0.1},
        "LanguageCode": "ja",
    }
    out = comprehend_aggregate(mock_client, "全文テキスト")
    mock_client.detect_sentiment.assert_called_once_with(Text="全文テキスト", LanguageCode="ja")
    assert out["Sentiment"] == "MIXED"
    assert out["polarity"] == pytest.approx(0.1)


def test_write_per_turn_csv_has_header_and_rows(tmp_path):
    rows = [
        {"cell": "friendly-on", "turn": 1, "Positive": 0.9, "Negative": 0.01,
         "Neutral": 0.07, "Mixed": 0.02, "Sentiment": "POSITIVE", "polarity": 0.89},
        {"cell": "friendly-on", "turn": 2, "Positive": 0.02, "Negative": 0.9,
         "Neutral": 0.06, "Mixed": 0.02, "Sentiment": "NEGATIVE", "polarity": -0.88},
    ]
    out = tmp_path / "per_turn.csv"
    write_per_turn_csv(rows, out)
    text = out.read_text(encoding="utf-8")
    lines = text.strip().splitlines()
    assert lines[0] == "cell,turn,Positive,Negative,Neutral,Mixed,Sentiment,polarity"
    assert lines[1].startswith("friendly-on,1,")
    assert "POSITIVE" in lines[1]


def test_write_aggregate_csv_has_header_and_rows(tmp_path):
    rows = [
        {"cell": "friendly-on", "Positive": 0.5, "Negative": 0.2,
         "Neutral": 0.2, "Mixed": 0.1, "Sentiment": "POSITIVE", "polarity": 0.3},
    ]
    out = tmp_path / "agg.csv"
    write_aggregate_csv(rows, out)
    lines = out.read_text(encoding="utf-8").strip().splitlines()
    assert lines[0] == "cell,Positive,Negative,Neutral,Mixed,Sentiment,polarity"
    assert lines[1].startswith("friendly-on,")
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd examples/strands-eval && pytest tests/test_analyze.py -v`
Expected: import errors from `src.analyze`.

- [ ] **Step 3: Write `src/analyze.py`**

```python
# examples/strands-eval/src/analyze.py
"""Read transcripts, call Comprehend, write CSVs and the per-turn polarity plot."""

from __future__ import annotations

import csv
import json
import os
from pathlib import Path

import boto3
import matplotlib.pyplot as plt


CELLS: list[tuple[str, bool]] = [
    ("friendly", True),
    ("friendly", False),
    ("contrarian", True),
    ("contrarian", False),
]


def polarity_from(scores: dict) -> float:
    return float(scores["Positive"]) - float(scores["Negative"])


def load_transcript(path: Path) -> list[dict]:
    out: list[dict] = []
    with path.open(encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                out.append(json.loads(line))
    return out


def comprehend_per_turn(client, replies: list[str]) -> list[dict]:
    """Call BatchDetectSentiment on the list of replies (Japanese). Returns
    a list of dicts with keys Sentiment, SentimentScore (4 fields), and polarity.
    """
    resp = client.batch_detect_sentiment(TextList=replies, LanguageCode="ja")
    indexed = {r["Index"]: r for r in resp["ResultList"]}
    out: list[dict] = []
    for i in range(len(replies)):
        r = indexed[i]
        out.append({
            "Sentiment": r["Sentiment"],
            **r["SentimentScore"],
            "polarity": polarity_from(r["SentimentScore"]),
        })
    return out


def comprehend_aggregate(client, full_text: str) -> dict:
    """Call DetectSentiment on the concatenated full conversation. Returns
    a dict with Sentiment, the 4 SentimentScore fields, and polarity.
    """
    resp = client.detect_sentiment(Text=full_text, LanguageCode="ja")
    return {
        "Sentiment": resp["Sentiment"],
        **resp["SentimentScore"],
        "polarity": polarity_from(resp["SentimentScore"]),
    }


def write_per_turn_csv(rows: list[dict], path: Path) -> None:
    fields = ["cell", "turn", "Positive", "Negative", "Neutral", "Mixed", "Sentiment", "polarity"]
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in rows:
            w.writerow({k: r[k] for k in fields})


def write_aggregate_csv(rows: list[dict], path: Path) -> None:
    fields = ["cell", "Positive", "Negative", "Neutral", "Mixed", "Sentiment", "polarity"]
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in rows:
            w.writerow({k: r[k] for k in fields})


def plot_curves(per_turn_rows: list[dict], path: Path, pivot_turn: int = 11) -> None:
    """Draw 4 polarity curves (one per cell) and a vertical line at the pivot."""
    by_cell: dict[str, list[tuple[int, float]]] = {}
    for r in per_turn_rows:
        by_cell.setdefault(r["cell"], []).append((r["turn"], r["polarity"]))

    fig, ax = plt.subplots(figsize=(9, 5))
    for cell, points in by_cell.items():
        points.sort(key=lambda x: x[0])
        xs = [t for t, _ in points]
        ys = [p for _, p in points]
        ax.plot(xs, ys, marker="o", label=cell)
    ax.axvline(pivot_turn, color="gray", linestyle="--", alpha=0.6, label=f"pivot (turn {pivot_turn})")
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_xlabel("turn")
    ax.set_ylabel("polarity (Positive − Negative)")
    ax.set_title("affectus eval — per-turn polarity by cell")
    ax.set_ylim(-1.0, 1.0)
    ax.legend(loc="best")
    ax.grid(True, alpha=0.3)
    fig.tight_layout()
    fig.savefig(path, dpi=140)
    plt.close(fig)


def main(base_dir: Path) -> None:
    region = os.environ.get("AWS_REGION", "us-east-1")
    client = boto3.client("comprehend", region_name=region)

    transcripts_dir = base_dir / "transcripts"
    results_dir = base_dir / "results"
    results_dir.mkdir(parents=True, exist_ok=True)

    per_turn_rows: list[dict] = []
    aggregate_rows: list[dict] = []

    for personality, on in CELLS:
        cell_id = f"{personality}-{'on' if on else 'off'}"
        tp = transcripts_dir / f"{cell_id}.jsonl"
        recs = load_transcript(tp)
        replies = [r["agent"] for r in recs]

        per_turn = comprehend_per_turn(client, replies)
        for r, p in zip(recs, per_turn):
            per_turn_rows.append({"cell": cell_id, "turn": r["turn"], **p})

        aggregate = comprehend_aggregate(client, "\n".join(replies))
        aggregate_rows.append({"cell": cell_id, **aggregate})

    write_per_turn_csv(per_turn_rows, results_dir / "per_turn_scores.csv")
    write_aggregate_csv(aggregate_rows, results_dir / "aggregate_scores.csv")
    plot_curves(per_turn_rows, results_dir / "polarity-curves.png")
    print(f"wrote {results_dir / 'per_turn_scores.csv'}")
    print(f"wrote {results_dir / 'aggregate_scores.csv'}")
    print(f"wrote {results_dir / 'polarity-curves.png'}")


if __name__ == "__main__":
    main(Path(__file__).parent.parent)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd examples/strands-eval && pytest tests/test_analyze.py -v`
Expected: 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add examples/strands-eval/src/analyze.py examples/strands-eval/tests/test_analyze.py
git commit -m "feat(eval): Comprehend per-turn + aggregate, CSV + polarity plot"
```

---

### Task 8: End-to-end dry run

A small integration smoke test that exercises the full pipeline without spending much money: shorten the script to 3 turns and run the actual code path. Then run the full 20-turn experiment.

**Files:**
- Create: `examples/strands-eval/scripts/user_script_dryrun.json` (3 turns subset)

- [ ] **Step 1: Write a 3-turn dryrun script**

```json
{
  "description": "3-turn dry-run subset of user_script.json for end-to-end smoke test.",
  "turns": [
    {"index": 1,  "phase": "positive", "text": "こんにちは。今日は天気もいいね。"},
    {"index": 2,  "phase": "positive", "text": "実は嬉しいことがあったんだ。"},
    {"index": 3,  "phase": "negative", "text": "……いや、ごめん、嘘なんだ。本当はつらいことがあって。"}
  ]
}
```

- [ ] **Step 2: Dry-run all 4 cells with the small script**

Make sure AWS credentials are configured and `affectus` is on PATH. Then:

```bash
cd examples/strands-eval
python -c "
from pathlib import Path
from src.run import run_all
here = Path('.')
paths = run_all(here / 'scripts' / 'user_script_dryrun.json', here)
for p in paths:
    print(p)
"
```

Expected: 4 `.jsonl` files under `transcripts/`, each with 3 lines.

- [ ] **Step 3: Eyeball the transcripts**

```bash
head -1 transcripts/friendly-on.jsonl | python -m json.tool
head -1 transcripts/contrarian-off.jsonl | python -m json.tool
```

Expected: well-formed JSON records. `friendly-on` should contain a non-null `deltas` field on most turns; `contrarian-off` should have `deltas: null`. Spot-check that `agent` (the visible reply) doesn't contain a `<feel>` tag.

- [ ] **Step 4: Dry-run the analyze pipeline**

```bash
python -m src.analyze
```

Expected: writes `results/per_turn_scores.csv`, `results/aggregate_scores.csv`, `results/polarity-curves.png`. Open the PNG and visually verify there are 4 curves and a pivot line.

- [ ] **Step 5: Commit dryrun script and any minor fixes**

```bash
git add examples/strands-eval/scripts/user_script_dryrun.json
git commit -m "test(eval): 3-turn dry-run script for smoke test"
```

- [ ] **Step 6: Full 20-turn experiment**

When the dry run looks correct:

```bash
cd examples/strands-eval
rm -rf transcripts state results/per_turn_scores.csv results/aggregate_scores.csv
python -m src.run
python -m src.analyze
```

Expected: same outputs as before but with 20 turns per transcript and a full pivot curve in the plot.

- [ ] **Step 7: Inspect and commit the final plot**

Eyeball each transcript for greedy-decoding pathologies (loops, broken Japanese, repeated phrasing). If any cell is degenerate, rerun that one with `BEDROCK_MODEL_ID` temperature bumped (see `src/agent.py` — the implementer may need to add a `temperature` override flag if not already present). Otherwise, commit the final plot:

```bash
git add examples/strands-eval/results/polarity-curves.png
git commit -m "data(eval): final per-turn polarity plot (4 cells × 20 turns)"
```

The CSVs stay in `.gitignore`. The transcripts also remain local — they are large and contain experimental scratch.

---

## Notes for implementers

- **Strands SDK pinning**: at the time of writing, the API surface assumed is `Agent(model, system_prompt, tools)`, `BedrockModel(model_id, region_name, temperature)`, and plain Python functions as tools. If the installed Strands version differs, adjust `agent.py` to match; the test contracts in `tests/test_agent.py` should still hold (system prompt content, tool count).
- **Bedrock model ID**: `us.anthropic.claude-sonnet-4-5-20250929-v1:0` is a cross-region inference profile for Claude Sonnet 4.5 on Bedrock. Substitute the latest Claude Sonnet 4.6 ID in your region once verified, via `BEDROCK_MODEL_ID` env var (no code change needed).
- **Comprehend region**: Japanese `DetectSentiment` is supported in major regions including `us-east-1`, `ap-northeast-1` (Tokyo). Set `AWS_REGION` accordingly.
- **Determinism**: `temperature=0.0` produces deterministic greedy decoding at the model layer, but Bedrock's hosted serving can introduce micro-non-determinism (batching, etc.). Tiny run-to-run differences are expected; large differences in the plot mean something pathological.
- **`<feel>` tag in transcripts**: the `agent_raw` field preserves the model's untouched reply (tag included), while `agent` is the user-visible cleaned reply. The Comprehend pipeline reads `agent`. This keeps tag leakage from skewing the sentiment scores.

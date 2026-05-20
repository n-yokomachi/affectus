"""Run the 4 (personality, affectus_on) cells against the fixed user script."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

from src.affectus_tools import affectus_feel, affectus_reset, affectus_show
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
    # Protocol says the feel tag is the LAST element of the reply, so pick the
    # last match if the LLM emits multiple tags by mistake.
    matches = FEEL_RE.findall(text)
    if not matches:
        return None
    try:
        parsed = json.loads(matches[-1])
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

    agent = build_agent(personality=personality, affectus_on=affectus_on)

    transcript_path = transcripts_dir / f"{cell_id}.jsonl"
    with transcript_path.open("w", encoding="utf-8") as out:
        for entry in script:
            user_utt = entry["text"]
            if affectus_on:
                current_state = affectus_show(state_path, config_path)
                message = f"[現在のあなたの感情: {current_state}]\n\n{user_utt}"
            else:
                message = user_utt
            raw_reply = str(agent(message))
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
        cell_id = f"{personality}-{'on' if on else 'off'}"
        print(f"[run] cell={cell_id}", file=sys.stderr)
        try:
            written.append(run_cell(personality, on, script, base_dir, config_path))
        except Exception as exc:
            print(f"[run] ERROR in {cell_id}: {exc}", file=sys.stderr)
    return written


if __name__ == "__main__":
    here = Path(__file__).parent.parent
    paths = run_all(here / "scripts" / "user_script.json", here)
    for p in paths:
        print(f"wrote {p}")

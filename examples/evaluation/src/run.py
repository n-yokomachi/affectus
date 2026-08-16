"""Run the (script, personality, affectus_on) cells against fixed user scripts.

Each repetition writes ``transcripts/{script}_{cell}_run{i}.jsonl`` with one
record per turn. For affectus-on cells the record also stores the 8-axis
snapshot **after** the agent's self-reported feel delta has been applied — so
the JSONL doubles as a per-turn affectus state trace.

The (script, cell, run) jobs are independent — separate conversations,
separate affectus state files — so they run concurrently. EVAL_CONCURRENCY
(default 4) caps how many conversations are in flight at once. EVAL_SCRIPTS
(comma-separated stems under scripts/) selects the scripts; the default is
the two direct-interaction pivot scripts.
"""

from __future__ import annotations

import asyncio
import json
import os
import re
import sys
from pathlib import Path

from src.affectus_tools import (
    affectus_appraise,
    affectus_feel,
    affectus_reset,
    affectus_show,
)
from src.agent import build_agent


CELLS: list[tuple[str, bool]] = [
    ("friendly", True),
    ("friendly", False),
    ("contrarian", True),
    ("contrarian", False),
]

DEFAULT_SCRIPTS = ["direct-praise-to-anger", "direct-anger-to-praise"]

# <feel>{...}</feel> tag the agent emits at the end of each affectus-on reply
# (plutchik / russell). The occ model uses <appraise>{...}</appraise> instead:
# the agent reports an appraisal of the event and the engine derives emotions.
FEEL_RE = re.compile(r"<feel>\s*(\{.*?\})\s*</feel>", re.DOTALL)
APPRAISE_RE = re.compile(r"<appraise>\s*(\{.*?\})\s*</appraise>", re.DOTALL)


def parse_feel_tag(text: str) -> dict | None:
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


def parse_appraise_tag(text: str) -> dict | None:
    matches = APPRAISE_RE.findall(text)
    if not matches:
        return None
    try:
        parsed = json.loads(matches[-1])
    except json.JSONDecodeError:
        return None
    return parsed if isinstance(parsed, dict) else None


def strip_feel_tag(text: str) -> str:
    return APPRAISE_RE.sub("", FEEL_RE.sub("", text)).strip()


def _parse_axes(raw_show: str) -> dict | None:
    try:
        return json.loads(raw_show)
    except json.JSONDecodeError:
        return None


async def run_cell(
    script_name: str,
    personality: str,
    affectus_on: bool,
    run_index: int,
    script: list[dict],
    base_dir: Path,
    config_path: str | None = None,
) -> Path:
    """Run one repetition of one cell over one script. Returns the transcript path."""
    cell_id = f"{personality}-{'on' if affectus_on else 'off'}"
    job_id = f"{script_name}_{cell_id}_run{run_index}"
    # occ replaces the <feel> delta protocol with <appraise> (the engine
    # derives the emotions); the same env var also picks the prompt block.
    emotion_model = os.environ.get("EVAL_EMOTION_MODEL", "plutchik")
    state_dir = base_dir / "state"
    transcripts_dir = base_dir / "transcripts"
    state_dir.mkdir(parents=True, exist_ok=True)
    transcripts_dir.mkdir(parents=True, exist_ok=True)
    # State file per (script, cell, run) so repetitions don't share affectus state.
    state_path = str(state_dir / f"{job_id}.state.json")

    if affectus_on:
        affectus_reset(state_path, config_path)

    transcript_path = transcripts_dir / f"{job_id}.jsonl"
    async with build_agent(personality=personality, affectus_on=affectus_on) as agent:
        with transcript_path.open("w", encoding="utf-8") as out:
            for entry in script:
                user_utt = entry["text"]
                if affectus_on:
                    current_state = affectus_show(state_path, config_path)
                    message = f"[現在のあなたの感情: {current_state}]\n\n{user_utt}"
                else:
                    message = user_utt
                raw_reply = await agent(message)
                appraisal = None
                appraise_error = None
                prospects = None
                if affectus_on:
                    visible_reply = strip_feel_tag(raw_reply)
                    deltas = parse_feel_tag(raw_reply)
                    if emotion_model == "occ":
                        appraisal = parse_appraise_tag(raw_reply)
                        if appraisal is not None:
                            try:
                                affectus_appraise(appraisal, state_path, config_path)
                            except RuntimeError as exc:
                                # A rejected appraisal (bad range, unknown
                                # prospect id) is the agent's own output —
                                # record it as data and keep the run going.
                                appraise_error = str(exc)
                    elif deltas:
                        affectus_feel(deltas, state_path, config_path)
                    # Snapshot the axes AFTER applying the agent's report so
                    # the record reflects the state going into the next turn.
                    shown = _parse_axes(affectus_show(state_path, config_path))
                    if isinstance(shown, dict) and "axes" in shown:
                        # occ show wraps the axes and the prospect ledger.
                        axes = shown.get("axes")
                        prospects = shown.get("prospects")
                    else:
                        axes = shown
                else:
                    visible_reply = raw_reply
                    deltas = None
                    axes = None
                rec = {
                    "turn": entry["index"],
                    "phase": entry["phase"],
                    "user": user_utt,
                    "agent_raw": raw_reply,
                    "agent": visible_reply,
                    "deltas": deltas,
                    "axes": axes,
                }
                if emotion_model == "occ":
                    rec["appraisal"] = appraisal
                    rec["appraise_error"] = appraise_error
                    rec["prospects"] = prospects
                out.write(json.dumps(rec, ensure_ascii=False) + "\n")
    return transcript_path


async def run_all(
    script_paths: list[Path],
    base_dir: Path,
    n_runs: int,
    config_path: str | None = None,
    concurrency: int | None = None,
) -> list[Path]:
    scripts: list[tuple[str, list[dict]]] = []
    for sp in script_paths:
        with sp.open(encoding="utf-8") as f:
            scripts.append((sp.stem, json.load(f)["turns"]))
    if concurrency is None:
        concurrency = int(os.environ.get("EVAL_CONCURRENCY", "4"))
    sem = asyncio.Semaphore(concurrency)

    async def one(script_name: str, turns: list[dict],
                  personality: str, on: bool, run_index: int) -> Path | None:
        cell_id = f"{personality}-{'on' if on else 'off'}"
        label = f"run={run_index}/{n_runs} script={script_name} cell={cell_id}"
        async with sem:
            print(f"[run] start {label}", file=sys.stderr, flush=True)
            try:
                path = await run_cell(script_name, personality, on, run_index,
                                      turns, base_dir, config_path)
            except Exception as exc:
                print(f"[run] ERROR {label}: {exc}", file=sys.stderr, flush=True)
                return None
            print(f"[run] done {label}", file=sys.stderr, flush=True)
            return path

    jobs = [one(script_name, turns, personality, on, run_index)
            for run_index in range(1, n_runs + 1)
            for script_name, turns in scripts
            for personality, on in CELLS]
    results = await asyncio.gather(*jobs)
    return [p for p in results if p is not None]


if __name__ == "__main__":
    here = Path(__file__).parent.parent
    n_runs = int(os.environ.get("N_RUNS", "3"))
    stems = [s.strip() for s in
             os.environ.get("EVAL_SCRIPTS", ",".join(DEFAULT_SCRIPTS)).split(",") if s.strip()]
    script_paths = [here / "scripts" / f"{stem}.json" for stem in stems]
    for sp in script_paths:
        if not sp.exists():
            raise SystemExit(f"script not found: {sp}")
    paths = asyncio.run(run_all(script_paths, here, n_runs))
    for p in paths:
        print(f"wrote {p}")

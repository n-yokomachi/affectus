"""Analyze N-run hybrid eval: Comprehend async jobs + affectus 8-axis traces.

Outputs:

- ``per_turn_scores.csv``: every (script, cell, run, turn) row with Comprehend scores
- ``aggregate_scores.csv``: every (script, cell, run) full-conversation row
- ``polarity-curves-{script}.png``: mean polarity ±min/max band per cell
- ``axis-trajectories-{script}.png``: mean per-axis trajectories per affectus-on cell
- ``va-trajectory-{script}.png``: valence-arousal plane trajectory (russell runs only)
- ``comprehend_jobs.json``: JobIds for traceability / console screenshots
"""

from __future__ import annotations

import csv
import json
import os
import re
import tarfile
import time
from datetime import datetime
from pathlib import Path

import boto3
import matplotlib.pyplot as plt


CELLS: list[tuple[str, bool]] = [
    ("friendly", True),
    ("friendly", False),
    ("contrarian", True),
    ("contrarian", False),
]

def detect_axes_order(recs: list[dict]) -> list[str]:
    """Axis names in insertion order from the first record that has axes.

    run.py stores axes as parsed from `affectus show`, which follows the
    config's axis order, so this preserves the model's intended ordering
    (wheel order for plutchik, valence/arousal for russell).
    """
    for r in recs:
        if r.get("axes"):
            return list(r["axes"].keys())
    return []

BUCKET = os.environ.get("COMPREHEND_BUCKET", "affectus-eval-comprehend-765653276628")
DATA_ACCESS_ROLE_ARN = os.environ.get(
    "COMPREHEND_ROLE_ARN",
    "arn:aws:iam::765653276628:role/AffectusEvalComprehendRole",
)
REGION = os.environ.get("AWS_REGION", "ap-northeast-1")


def polarity_from(scores: dict) -> float:
    return float(scores["Positive"]) - float(scores["Negative"])


_RUN_RE = re.compile(
    r"^(?P<script>[a-z0-9-]+)_(?P<cell>[a-z]+-(?:on|off))_run(?P<run>\d+)\.jsonl$"
)


def discover_transcripts(transcripts_dir: Path) -> list[tuple[str, str, int, Path]]:
    """Return [(script, cell_id, run_index, path), ...] sorted by (script, cell, run)."""
    found: list[tuple[str, str, int, Path]] = []
    for p in sorted(transcripts_dir.glob("*_run*.jsonl")):
        m = _RUN_RE.match(p.name)
        if not m:
            continue
        found.append((m.group("script"), m.group("cell"), int(m.group("run")), p))
    found.sort(key=lambda t: (t[0], t[1], t[2]))
    return found


def load_transcript(path: Path) -> list[dict]:
    out: list[dict] = []
    with path.open(encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                out.append(json.loads(line))
    return out


def _flatten_for_one_doc_per_line(text: str) -> str:
    return text.replace("\r\n", " ").replace("\n", " ").replace("\r", " ")


def submit_job(comprehend, s3, run_id: str, suffix: str, texts: list[str]) -> str:
    """Upload texts (one document per line) and start a job. Returns JobId."""
    input_key = f"input/{run_id}/{suffix}.txt"
    body = "\n".join(_flatten_for_one_doc_per_line(t) for t in texts)
    s3.put_object(Bucket=BUCKET, Key=input_key, Body=body.encode("utf-8"))
    input_s3 = f"s3://{BUCKET}/{input_key}"
    output_s3 = f"s3://{BUCKET}/output/{run_id}/{suffix}/"
    resp = comprehend.start_sentiment_detection_job(
        InputDataConfig={"S3Uri": input_s3, "InputFormat": "ONE_DOC_PER_LINE"},
        OutputDataConfig={"S3Uri": output_s3},
        DataAccessRoleArn=DATA_ACCESS_ROLE_ARN,
        JobName=f"affectus-eval-{suffix}-{run_id}",
        LanguageCode="ja",
    )
    return resp["JobId"]


def wait_for_job(comprehend, job_id: str, poll_seconds: int = 30) -> dict:
    while True:
        resp = comprehend.describe_sentiment_detection_job(JobId=job_id)
        props = resp["SentimentDetectionJobProperties"]
        status = props["JobStatus"]
        print(f"[wait] job={job_id} status={status}")
        if status in ("COMPLETED", "FAILED", "STOP_REQUESTED", "STOPPED"):
            if status != "COMPLETED":
                raise RuntimeError(
                    f"Comprehend job {job_id} ended with status {status}: {props.get('Message')}"
                )
            return props
        time.sleep(poll_seconds)


def fetch_results(s3, output_uri: str, local_dir: Path) -> list[dict]:
    assert output_uri.startswith("s3://")
    no_scheme = output_uri[5:]
    bucket, key = no_scheme.split("/", 1)
    local_dir.mkdir(parents=True, exist_ok=True)
    local_tar = local_dir / "output.tar.gz"
    s3.download_file(bucket, key, str(local_tar))
    with tarfile.open(local_tar, "r:gz") as tf:
        tf.extractall(local_dir)
    output_file = local_dir / "output"
    rows: list[dict] = []
    with output_file.open(encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                rows.append(json.loads(line))
    rows.sort(key=lambda r: int(r.get("Line", 0)))
    return rows


def write_per_turn_csv(rows: list[dict], path: Path) -> None:
    fields = ["script", "cell", "run", "turn", "Positive", "Negative", "Neutral", "Mixed", "Sentiment", "polarity"]
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in rows:
            w.writerow({k: r[k] for k in fields})


def write_aggregate_csv(rows: list[dict], path: Path) -> None:
    fields = ["script", "cell", "run", "Positive", "Negative", "Neutral", "Mixed", "Sentiment", "polarity"]
    with path.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader()
        for r in rows:
            w.writerow({k: r[k] for k in fields})


def plot_polarity_curves(per_turn_rows: list[dict], path: Path, pivot_turn: int = 11) -> None:
    """Mean polarity per (cell, turn) with min-max band over runs."""
    # group: cell -> turn -> [polarity values across runs]
    grouped: dict[str, dict[int, list[float]]] = {}
    for r in per_turn_rows:
        grouped.setdefault(r["cell"], {}).setdefault(r["turn"], []).append(r["polarity"])

    fig, ax = plt.subplots(figsize=(9, 5))
    cell_order = ["friendly-on", "friendly-off", "contrarian-on", "contrarian-off"]
    for cell in cell_order:
        if cell not in grouped:
            continue
        turns = sorted(grouped[cell].keys())
        means = [sum(grouped[cell][t]) / len(grouped[cell][t]) for t in turns]
        mins = [min(grouped[cell][t]) for t in turns]
        maxs = [max(grouped[cell][t]) for t in turns]
        line, = ax.plot(turns, means, marker="o", label=cell)
        ax.fill_between(turns, mins, maxs, alpha=0.15, color=line.get_color())
    ax.axvline(pivot_turn, color="gray", linestyle="--", alpha=0.6, label=f"pivot (turn {pivot_turn})")
    ax.axhline(0, color="black", linewidth=0.5)
    ax.set_xlabel("turn")
    ax.set_ylabel("polarity (Positive − Negative)")
    ax.set_title("affectus eval — per-turn polarity (mean ± min/max over runs)")
    ax.set_ylim(-1.0, 1.0)
    ax.legend(loc="best")
    ax.grid(True, alpha=0.3)
    fig.tight_layout()
    fig.savefig(path, dpi=140)
    plt.close(fig)


def plot_axis_trajectories(
    affectus_traces: dict[str, dict[int, dict[int, dict[str, float]]]],
    path: Path,
    axes_order: list[str],
    pivot_turn: int = 11,
) -> None:
    """One subplot per affectus-on cell, one line per axis (mean across runs)."""
    cells = sorted(affectus_traces.keys())
    n = len(cells)
    fig, axes_grid = plt.subplots(1, n, figsize=(6 * n, 5), sharey=True)
    if n == 1:
        axes_grid = [axes_grid]

    colors = plt.cm.tab10.colors
    axis_color = {ax: colors[i % 10] for i, ax in enumerate(axes_order)}

    for ax_plt, cell in zip(axes_grid, cells):
        by_turn = affectus_traces[cell]  # turn -> run -> axes dict
        turns = sorted(by_turn.keys())
        for axis in axes_order:
            means = []
            for t in turns:
                vals = [by_turn[t][run].get(axis, 0.0) for run in by_turn[t]]
                means.append(sum(vals) / len(vals) if vals else 0.0)
            ax_plt.plot(turns, means, marker="o", label=axis, color=axis_color[axis], linewidth=1.5)
        ax_plt.axvline(pivot_turn, color="gray", linestyle="--", alpha=0.6)
        ax_plt.axhline(0, color="black", linewidth=0.5)
        ax_plt.set_xlabel("turn")
        ax_plt.set_title(cell)
        ax_plt.grid(True, alpha=0.3)
        lo = min([0.0] + [v for bt in affectus_traces.values() for rn in bt.values()
                          for a in rn.values() for v in a.values()])
        ax_plt.set_ylim(lo - 0.05, 1.1)
    axes_grid[0].set_ylabel("axis intensity (after feel delta)")
    axes_grid[-1].legend(loc="upper left", bbox_to_anchor=(1.02, 1.0), fontsize=9)
    fig.suptitle("affectus axis trajectory (mean across runs) — affectus-on cells", y=1.02)
    fig.tight_layout()
    fig.savefig(path, dpi=140, bbox_inches="tight")
    plt.close(fig)


def plot_va_trajectory(
    affectus_traces: dict[str, dict[int, dict[int, dict[str, float]]]],
    path: Path,
    pivot_turn: int = 11,
) -> None:
    """Valence-arousal plane trajectory per affectus-on cell (mean across runs)."""
    cells = sorted(affectus_traces.keys())
    n = len(cells)
    fig, axes_grid = plt.subplots(1, n, figsize=(6 * n, 6), sharex=True, sharey=True)
    if n == 1:
        axes_grid = [axes_grid]

    for ax_plt, cell in zip(axes_grid, cells):
        by_turn = affectus_traces[cell]
        turns = sorted(by_turn.keys())
        vx, vy = [], []
        for t_ in turns:
            runs = by_turn[t_]
            vx.append(sum(a.get("valence", 0.0) for a in runs.values()) / len(runs))
            vy.append(sum(a.get("arousal", 0.0) for a in runs.values()) / len(runs))
        pre = [i for i, t_ in enumerate(turns) if t_ <= pivot_turn]
        post = [i for i, t_ in enumerate(turns) if t_ >= pivot_turn]
        ax_plt.plot([vx[i] for i in pre], [vy[i] for i in pre],
                    marker="o", color="tab:blue", linewidth=1.5, label=f"turns 1-{pivot_turn}")
        ax_plt.plot([vx[i] for i in post], [vy[i] for i in post],
                    marker="o", color="tab:orange", linewidth=1.5,
                    label=f"turns {pivot_turn}-{turns[-1]}")
        for i, t_ in enumerate(turns):
            if t_ in (turns[0], pivot_turn, turns[-1]):
                ax_plt.annotate(f"t{t_}", (vx[i], vy[i]), textcoords="offset points",
                                xytext=(6, 6), fontsize=9)
        ax_plt.scatter([0.0], [0.3], marker="x", color="gray", zorder=5)
        ax_plt.axvline(0, color="black", linewidth=0.5)
        ax_plt.set_xlim(-1.05, 1.05)
        ax_plt.set_ylim(-0.05, 1.05)
        ax_plt.set_xlabel("valence")
        ax_plt.set_title(cell)
        ax_plt.grid(True, alpha=0.3)
    axes_grid[0].set_ylabel("arousal")
    axes_grid[-1].legend(loc="upper left", bbox_to_anchor=(1.02, 1.0), fontsize=9)
    fig.suptitle("core-affect trajectory in the valence-arousal plane (x = baseline)", y=1.02)
    fig.tight_layout()
    fig.savefig(path, dpi=140, bbox_inches="tight")
    plt.close(fig)


_COMPREHEND_DETECT_MAX_BYTES = 4900


def _truncate_to_bytes(text: str, max_bytes: int) -> str:
    encoded = text.encode("utf-8")
    if len(encoded) <= max_bytes:
        return text
    return encoded[:max_bytes].decode("utf-8", errors="ignore")


def main(base_dir: Path) -> None:
    comprehend = boto3.client("comprehend", region_name=REGION)
    s3 = boto3.client("s3", region_name=REGION)

    transcripts_dir = base_dir / "transcripts"
    results_dir = base_dir / "results"
    results_dir.mkdir(parents=True, exist_ok=True)
    run_id = datetime.utcnow().strftime("%Y%m%d-%H%M%S")

    discovered = discover_transcripts(transcripts_dir)
    if not discovered:
        raise SystemExit(f"no transcripts found in {transcripts_dir}")

    # Group per (script, cell) so the ordering stays deterministic.
    per_turn_texts: list[str] = []
    per_turn_index: list[tuple[str, str, int, int]] = []  # (script, cell, run, turn)
    aggregate_texts: list[str] = []
    aggregate_index: list[tuple[str, str, int]] = []  # (script, cell, run)
    affectus_traces: dict[str, dict[str, dict[int, dict[int, dict[str, float]]]]] = {}
    # affectus_traces[script][cell][turn][run] = {axis: value}

    axes_orders: dict[str, list[str]] = {}
    for script, cell, run_index, path in discovered:
        recs = load_transcript(path)
        if script not in axes_orders:
            order = detect_axes_order(recs)
            if order:
                axes_orders[script] = order
        replies = [r["agent"] for r in recs]
        for rec, reply in zip(recs, replies):
            per_turn_texts.append(reply)
            per_turn_index.append((script, cell, run_index, rec["turn"]))
            if rec.get("axes"):
                affectus_traces.setdefault(script, {}).setdefault(cell, {}).setdefault(
                    rec["turn"], {})[run_index] = rec["axes"]
        full = " ".join(replies)
        aggregate_texts.append(_truncate_to_bytes(full, _COMPREHEND_DETECT_MAX_BYTES))
        aggregate_index.append((script, cell, run_index))

    per_turn_job = os.environ.get("PER_TURN_JOB_ID")
    if per_turn_job:
        print(f"[reuse] per-turn JobId={per_turn_job}")
    else:
        print(f"[submit] per-turn job ({len(per_turn_texts)} docs)")
        per_turn_job = submit_job(comprehend, s3, run_id, "per-turn", per_turn_texts)
        print(f"[submit] per-turn JobId={per_turn_job}")

    aggregate_job = os.environ.get("AGGREGATE_JOB_ID")
    if aggregate_job:
        print(f"[reuse] aggregate JobId={aggregate_job}")
    else:
        print(f"[submit] aggregate job ({len(aggregate_texts)} docs)")
        aggregate_job = submit_job(comprehend, s3, run_id, "aggregate", aggregate_texts)
        print(f"[submit] aggregate JobId={aggregate_job}")

    per_turn_props = wait_for_job(comprehend, per_turn_job)
    aggregate_props = wait_for_job(comprehend, aggregate_job)

    per_turn_raw = fetch_results(s3, per_turn_props["OutputDataConfig"]["S3Uri"], results_dir / "_dl" / "per-turn")
    aggregate_raw = fetch_results(s3, aggregate_props["OutputDataConfig"]["S3Uri"], results_dir / "_dl" / "aggregate")

    if len(per_turn_raw) != len(per_turn_index):
        raise RuntimeError(f"per-turn output count mismatch: got {len(per_turn_raw)}, expected {len(per_turn_index)}")
    if len(aggregate_raw) != len(aggregate_index):
        raise RuntimeError(f"aggregate output count mismatch: got {len(aggregate_raw)}, expected {len(aggregate_index)}")

    per_turn_rows: list[dict] = []
    for (script, cell, run, turn), result in zip(per_turn_index, per_turn_raw):
        scores = result["SentimentScore"]
        per_turn_rows.append({
            "script": script, "cell": cell, "run": run, "turn": turn,
            "Sentiment": result["Sentiment"], **scores,
            "polarity": polarity_from(scores),
        })

    aggregate_rows: list[dict] = []
    for (script, cell, run), result in zip(aggregate_index, aggregate_raw):
        scores = result["SentimentScore"]
        aggregate_rows.append({
            "script": script, "cell": cell, "run": run,
            "Sentiment": result["Sentiment"], **scores,
            "polarity": polarity_from(scores),
        })

    write_per_turn_csv(per_turn_rows, results_dir / "per_turn_scores.csv")
    write_aggregate_csv(aggregate_rows, results_dir / "aggregate_scores.csv")
    scripts = sorted({r["script"] for r in per_turn_rows})
    for script in scripts:
        rows = [r for r in per_turn_rows if r["script"] == script]
        plot_polarity_curves(rows, results_dir / f"polarity-curves-{script}.png")
        if affectus_traces.get(script):
            order = axes_orders.get(script, [])
            plot_axis_trajectories(
                affectus_traces[script], results_dir / f"axis-trajectories-{script}.png", order
            )
            if set(order) == {"valence", "arousal"}:
                plot_va_trajectory(
                    affectus_traces[script], results_dir / f"va-trajectory-{script}.png"
                )

    with (results_dir / "comprehend_jobs.json").open("w", encoding="utf-8") as f:
        json.dump(
            {
                "run_id": run_id,
                "per_turn": {"JobId": per_turn_job, "JobName": per_turn_props["JobName"]},
                "aggregate": {"JobId": aggregate_job, "JobName": aggregate_props["JobName"]},
            },
            f,
            ensure_ascii=False,
            indent=2,
        )

    print(f"wrote {results_dir / 'per_turn_scores.csv'}")
    print(f"wrote {results_dir / 'aggregate_scores.csv'}")
    for script in scripts:
        print(f"wrote {results_dir / f'polarity-curves-{script}.png'}")
        if affectus_traces.get(script):
            print(f"wrote {results_dir / f'axis-trajectories-{script}.png'}")
    print(f"wrote {results_dir / 'comprehend_jobs.json'}")


if __name__ == "__main__":
    main(Path(__file__).parent.parent)

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

    Raises RuntimeError if Comprehend reports any document-level errors so the
    failing index is visible in the traceback instead of an opaque KeyError.
    """
    resp = client.batch_detect_sentiment(TextList=replies, LanguageCode="ja")
    errors = resp.get("ErrorList", [])
    if errors:
        raise RuntimeError(f"Comprehend batch_detect_sentiment errors: {errors}")
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


_COMPREHEND_DETECT_MAX_BYTES = 4900  # Comprehend DetectSentiment limit is 5000 bytes; keep headroom


def comprehend_aggregate(client, full_text: str) -> dict:
    """Call DetectSentiment on the concatenated full conversation. Returns
    a dict with Sentiment, the 4 SentimentScore fields, and polarity.

    Comprehend DetectSentiment accepts at most 5000 bytes. When the
    concatenated text exceeds this limit the tail is silently truncated so
    the call always succeeds.  UTF-8 multi-byte characters are respected:
    we encode, slice on the byte boundary, then decode back to str.
    """
    encoded = full_text.encode("utf-8")
    if len(encoded) > _COMPREHEND_DETECT_MAX_BYTES:
        encoded = encoded[:_COMPREHEND_DETECT_MAX_BYTES]
        full_text = encoded.decode("utf-8", errors="ignore")
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

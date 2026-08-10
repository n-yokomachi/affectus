import json
import tarfile
from pathlib import Path
from unittest.mock import MagicMock

import pytest

from src.analyze import (
    detect_axes_order,
    BUCKET,
    _flatten_for_one_doc_per_line,
    _truncate_to_bytes,
    discover_transcripts,
    fetch_results,
    load_transcript,
    polarity_from,
    submit_job,
    wait_for_job,
    write_aggregate_csv,
    write_per_turn_csv,
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


def test_discover_transcripts_parses_script_cell_run_and_sorts(tmp_path):
    for name in [
        "direct-praise-to-anger_friendly-on_run2.jsonl",
        "direct-praise-to-anger_friendly-on_run1.jsonl",
        "direct-anger-to-praise_contrarian-off_run1.jsonl",
        "notes.txt",
        "friendly-on_run1.jsonl",  # old naming without a script prefix: ignored
    ]:
        (tmp_path / name).write_text("", encoding="utf-8")
    found = discover_transcripts(tmp_path)
    assert [(s, c, r) for s, c, r, _ in found] == [
        ("direct-anger-to-praise", "contrarian-off", 1),
        ("direct-praise-to-anger", "friendly-on", 1),
        ("direct-praise-to-anger", "friendly-on", 2),
    ]


def test_flatten_replaces_newlines_with_spaces():
    assert _flatten_for_one_doc_per_line("a\nb\r\nc\rd") == "a b c d"


def test_truncate_to_bytes_keeps_short_text():
    assert _truncate_to_bytes("こんにちは", 100) == "こんにちは"


def test_truncate_to_bytes_never_splits_a_multibyte_char():
    # each char is 3 bytes in UTF-8; cutting at 7 bytes must not leave a
    # partial character behind
    assert _truncate_to_bytes("あいう", 7) == "あい"


def test_submit_job_uploads_one_doc_per_line_and_starts_ja_job():
    comprehend = MagicMock()
    s3 = MagicMock()
    comprehend.start_sentiment_detection_job.return_value = {"JobId": "job-123"}

    job_id = submit_job(comprehend, s3, "20260729-000000", "per-turn", ["一行目\nに改行", "二行目"])

    assert job_id == "job-123"
    s3.put_object.assert_called_once()
    put_kwargs = s3.put_object.call_args.kwargs
    assert put_kwargs["Bucket"] == BUCKET
    assert put_kwargs["Body"].decode("utf-8") == "一行目 に改行\n二行目"
    job_kwargs = comprehend.start_sentiment_detection_job.call_args.kwargs
    assert job_kwargs["LanguageCode"] == "ja"
    assert job_kwargs["InputDataConfig"]["InputFormat"] == "ONE_DOC_PER_LINE"


def test_wait_for_job_returns_props_on_completed():
    comprehend = MagicMock()
    comprehend.describe_sentiment_detection_job.return_value = {
        "SentimentDetectionJobProperties": {"JobStatus": "COMPLETED", "JobName": "n"}
    }
    props = wait_for_job(comprehend, "job-123")
    assert props["JobStatus"] == "COMPLETED"


def test_wait_for_job_raises_on_failed():
    comprehend = MagicMock()
    comprehend.describe_sentiment_detection_job.return_value = {
        "SentimentDetectionJobProperties": {"JobStatus": "FAILED", "Message": "boom"}
    }
    with pytest.raises(RuntimeError, match="FAILED"):
        wait_for_job(comprehend, "job-123")


def test_fetch_results_downloads_tar_and_sorts_by_line(tmp_path):
    # Build the tar.gz Comprehend would produce: one file named "output" with
    # one JSON object per line, in arbitrary Line order.
    src_dir = tmp_path / "src"
    src_dir.mkdir()
    output_file = src_dir / "output"
    rows = [
        {"Line": 1, "Sentiment": "NEGATIVE"},
        {"Line": 0, "Sentiment": "POSITIVE"},
    ]
    output_file.write_text("\n".join(json.dumps(r) for r in rows), encoding="utf-8")
    tar_path = tmp_path / "made.tar.gz"
    with tarfile.open(tar_path, "w:gz") as tf:
        tf.add(output_file, arcname="output")

    s3 = MagicMock()

    def fake_download(bucket, key, dest):
        Path(dest).write_bytes(tar_path.read_bytes())

    s3.download_file.side_effect = fake_download

    out = fetch_results(s3, "s3://bucket/output/x/output.tar.gz", tmp_path / "dl")
    assert [r["Sentiment"] for r in out] == ["POSITIVE", "NEGATIVE"]


def test_write_per_turn_csv_has_header_and_rows(tmp_path):
    rows = [
        {"script": "s1", "cell": "friendly-on", "run": 1, "turn": 1, "Positive": 0.9, "Negative": 0.01,
         "Neutral": 0.07, "Mixed": 0.02, "Sentiment": "POSITIVE", "polarity": 0.89},
        {"script": "s1", "cell": "friendly-on", "run": 1, "turn": 2, "Positive": 0.02, "Negative": 0.9,
         "Neutral": 0.06, "Mixed": 0.02, "Sentiment": "NEGATIVE", "polarity": -0.88},
    ]
    out = tmp_path / "per_turn.csv"
    write_per_turn_csv(rows, out)
    lines = out.read_text(encoding="utf-8").strip().splitlines()
    assert lines[0] == "script,cell,run,turn,Positive,Negative,Neutral,Mixed,Sentiment,polarity"
    assert lines[1].startswith("s1,friendly-on,1,1,")
    assert "POSITIVE" in lines[1]


def test_write_aggregate_csv_has_header_and_rows(tmp_path):
    rows = [
        {"script": "s1", "cell": "friendly-on", "run": 1, "Positive": 0.5, "Negative": 0.2,
         "Neutral": 0.2, "Mixed": 0.1, "Sentiment": "POSITIVE", "polarity": 0.3},
    ]
    out = tmp_path / "agg.csv"
    write_aggregate_csv(rows, out)
    lines = out.read_text(encoding="utf-8").strip().splitlines()
    assert lines[0] == "script,cell,run,Positive,Negative,Neutral,Mixed,Sentiment,polarity"
    assert lines[1].startswith("s1,friendly-on,1,")


def test_detect_axes_order_uses_first_record_with_axes():
    recs = [
        {"turn": 1, "axes": None},
        {"turn": 2, "axes": {"valence": 0.1, "arousal": 0.3}},
        {"turn": 3, "axes": {"arousal": 0.2, "valence": 0.0}},
    ]
    assert detect_axes_order(recs) == ["valence", "arousal"]


def test_detect_axes_order_empty_when_no_axes():
    assert detect_axes_order([{"turn": 1, "axes": None}]) == []

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

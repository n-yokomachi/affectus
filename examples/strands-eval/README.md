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

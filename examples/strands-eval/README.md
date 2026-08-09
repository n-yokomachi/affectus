# strands-eval: affectus 効果検証リグ

`affectus` を導入したエージェントの応答が、AWS Comprehend の極性スコアの時系列で計測可能に変化するかを、対照実験で確認する最小リグ。

## 構成

- 2性格（friendly / contrarian）× affectus on/off = 4セル、既定で N=3 反復
- 各ラン20ターン、共通の固定会話スクリプト（前半10ポジ → 後半10ネガのピボット型）
- 計測：Comprehend の非同期バッチジョブ（per-turn 時系列＋ラン全体集約）

## LLM バックエンド

環境変数 `EVAL_BACKEND` で切り替える。

- `claude-sdk`（既定）: Claude Agent SDK のストリーミングクライアント。
  1セル=1会話=1プロセスを保持したままターンを送るので、ターンごとの
  プロセス起動・履歴再読込がない。ローカルの Claude サブスクリプションの
  範囲で動き、API の従量課金が発生しない。キャラクターの分離は
  `system_prompt`（完全置換）、`setting_sources=[]`（個人設定・出力スタイル
  を読まない。⚠ None だとフラグが省略され CLI 既定で読まれてしまう）、
  `tools=[]`（ツール定義を渡さない）、`max_turns=1`（1ターン1応答）で担保。
  (cell, run) のジョブは独立なので並列実行する（`EVAL_CONCURRENCY`、既定 4）。
  ⚠ temperature は指定できないため決定論モードはない
  - 環境変数: `EVAL_CLAUDE_MODEL`（既定 `claude-sonnet-4-6`）、`EVAL_CONCURRENCY`
- `bedrock`: Strands Agents + Amazon Bedrock（temperature=0、max_tokens=512）。
  バックエンド切り替え前に記録したランの再現用
  - 環境変数: `AWS_REGION`、`BEDROCK_MODEL_ID`（既定は Claude Sonnet 系の
    cross-region inference profile）

## セットアップ

```bash
cd examples/strands-eval
pip install -e ".[dev]"
```

前提：

- `affectus` バイナリが PATH にある（軸名改名後のソースからビルドしたもの。
  `AFFECTUS_CONFIG` でリポジトリの `internal/engine/plutchik.default.yaml` を
  明示すると、ローカルの個人設定に影響されない）
- claude-cli バックエンド：`claude` CLI がインストール・ログイン済み
- 計測（`src.analyze`）には AWS 認証が必要（Comprehend + S3。バケットと
  DataAccessRole は `COMPREHEND_BUCKET` / `COMPREHEND_ROLE_ARN` で上書き可）
- bedrock バックエンド利用時のみ Bedrock へのアクセス権が必要

## 実行

```bash
python -m src.run        # 4セル × N_RUNS を順次実行、transcripts/ に出力
python -m src.analyze    # transcripts → Comprehend → results/ に CSV と図
```

詳細は `docs/superpowers/specs/2026-05-20-affectus-eval-design.md` を参照。

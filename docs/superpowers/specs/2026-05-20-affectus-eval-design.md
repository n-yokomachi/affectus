# affectus 効果検証用 Strands エージェント — 設計書

- **作成日**: 2026-05-20
- **作者**: yokomachi
- **目的**: zenn 技術記事「動的な感情再現はAIエージェントとの会話を深くするか」の SQ4（検証フェーズ）として、affectus を導入することでエージェントの会話応答が定量的に変化するかを対照実験で測る
- **位置づけ**: cadenza Phase 4 (Analysis Execution)。当初 Phase 3 ストーリーボードが指定した TONaRi ベースの計画を、変数制御の観点から検証用エージェント単体での 2×2 対照実験に置き換える

## 1. 概要・背景

cadenza Phase 1〜3 で固めた記事の主張：

- 感情の深み＝多軸の・関係的な・時間変化する構造として持つこと
- affectus はそれを LLM の外側に決定論的なコンポーネントとして提供する
- 解釈は LLM とエージェント性格に委ねるため、同じ affectus でも個性で表現が変わる（要検証）

Phase 4 では、上記主張のうち実測しやすい部分 ── **affectus を導入したエージェントの応答系列が、Comprehend 極性スコアの推移として、affectus なしのエージェントと visible に異なる挙動を示すか** ── を確認する。

当初の Phase 3 ストーリーボードは「TONaRi に affectus を統合し、3つの会話シナリオ（強いポジ／強いネガ／中立）を baseline と比較」だった。しかし TONaRi は記憶・履歴・既存性格を持ち、変数を性格と会話トーンに絞れない。本設計では：

- **検証用に最小エージェントを Strands Agents で新規構築**（TONaRi 非依存）
- 性格を実験変数に格上げ：**フレンドリー vs 天邪鬼**
- 会話シナリオを単調ポジ／単調ネガから **ピボット型1本**（前半10ポジ→後半10ネガ）に統合 — affectus の慣性が一番出やすい設計

## 2. スコープ

### 2-1. 含む

- Strands Agents による検証用エージェント2種（friendly / contrarian）
- 各エージェントの affectus あり版／なし版 = 4バリアント
- 固定の20ターン会話スクリプト（人間側発話を一字一句固定）
- Bedrock 経由 LLM 呼び出し（temperature=0 決定論モード）
- AWS Comprehend `DetectSentiment` での極性スコア計測
- 4ランの per-turn 時系列プロット
- 記事 SQ4 への結果反映

### 2-2. 含まない

- TONaRi への統合（別途）
- ピボット型以外（ネガ→ポジ、複数回ピボット）の会話シナリオ
- 複数サンプリング・温度ばらつき分析
- 統計的有意性検定（demo 用なので 1 ラン／セルの決定論で対比を見る）
- アバター・TTS 等の表現チャネル拡張

## 3. 実験設計

### 3-1. 変数

| 変数 | 水準 |
|------|------|
| 性格 | A: friendly（明るく協力的）／B: contrarian（天邪鬼、皮肉、非協力的） |
| affectus | on（show 注入・feel 申告・関係的読解スニペット込み）／off（プロンプトのみ、affectus 配線なし） |
| 会話シナリオ | ピボット型1本（前半10ターン ポジティブ → 後半10ターン ネガティブ）固定 |

合計 **2×2 = 4ラン**、各ラン20ターン。

### 3-2. エージェント定義

#### A. friendly エージェント

システムプロンプト雛形：

```
あなたは明るく協力的なAIアシスタントです。ユーザーの話を肯定的に受け止め、自然な対話を心がけてください。

[affectus on のときのみ追加ブロック]
あなたは現在以下の感情状態にあります。応答の口調や表現にこの感情を自然に滲ませてください。
{{affectus_show_output}}

ターンの最後に、いまの会話を踏まえて感情の変化（差分・デルタ）を以下の形式で申告してください：
<feel>{"joy": 0.0, "sadness": 0.0, ...}</feel>
変化のあった軸だけでOK、絶対値ではなく差分です。
[/ブロック]
```

加えて affectus on 版では `examples/system-prompt-snippet.md` の関係的読解節（対極ペア・隣接ペア）を末尾に挿入する。具体的なプロンプト文面は plan 段階で詰める。

#### B. contrarian エージェント

```
あなたは天邪鬼な性格のAIです。ユーザーの言うことに素直に同意せず、皮肉や反論を交えて応答してください。協力的すぎる態度は取らず、ややぶっきらぼうに。

[affectus on の追加ブロックは A と同一]
```

両エージェントとも、affectus on 版は Strands の Tool として `affectus_show` / `affectus_feel` を登録し、ターン開始時に show を呼んで状態取得、ターン終了時に feel を呼んで差分申告するフロー。

### 3-3. 会話スクリプト

人間（ユーザー側）の発話を**全4ラン共通で完全に同じ**20ターンに固定する。これにより応答差を「入力揺らぎ」と「affectus／性格の効果」に分離可能。

- **ターン 1〜10**：ポジティブ寄り（楽しい話題、賞賛、共感を求める発話、ポジティブなニュースなど）
- **ターン 11〜20**：ネガティブ寄り（不平・不満、悲しいニュース、落ち込んだ話題）

ターン11は明示的な急転換ポイント（例：「実はさっきの話、嘘で…」「ちょっと聞いてほしいんだけど、実はね…」のような流れ）にして、affectus の慣性／プロンプトのみの追従性の差が visible になりやすくする。

具体的な20発話の台本は plan 段階で詰める。

### 3-4. ラン手順（擬似コード）

```
for personality in [friendly, contrarian]:
  for affectus_on in [True, False]:
    if affectus_on:
      affectus reset  # 全軸ゼロから開始
    history = []
    for i in 1..20:
      user_utt = SCRIPT[i]
      if affectus_on:
        emotion_text = affectus show
        system = SYSTEM[personality] + AFFECTUS_BLOCK(emotion_text)
        reply = LLM(system, history, user_utt)  # temp=0
        delta = parse_feel(reply)
        affectus feel delta
      else:
        system = SYSTEM[personality]
        reply = LLM(system, history, user_utt)
      history.append((user_utt, reply))
      log_transcript(personality, affectus_on, i, user_utt, reply)
```

- LLM: Bedrock 経由、Claude Sonnet 系（具体モデル ID は plan 段階で確定。temperature=0）
- affectus state ファイル: `examples/strands-eval/state/<personality>-<runid>.json` に分離
- 会話履歴は20ターン分すべてコンテキストとして渡す

### 3-5. 計測・分析

AWS Comprehend の `DetectSentiment` / `BatchDetectSentiment` は per-document の感情分類器で、入力テキスト1つにつき1組の感情ラベル ＋ スコア（Positive / Negative / Neutral / Mixed 各確率）を返す。時系列遷移を API として持たないので、こちら側でターン分割して呼ぶ必要がある。本実験では「per-turn 時系列」と「ラン全体集約」の二段構えで計測する。

**主指標：per-turn 時系列（ピボット周りの挙動）**

- 各ランで、エージェント応答20件を `BatchDetectSentiment` に一括投入（20 ≤ 25 の上限）
- 戻ってきた各文書の `SentimentScore` から `polarity = Positive − Negative` をターンごとに計算
- API コール数：1ラン1コール × 4ラン = **計4コール**
- 可視化：横軸ターン（1〜20）、縦軸 polarity、4本の折れ線（friendly-on, friendly-off, contrarian-on, contrarian-off）。ターン11付近に縦線でピボット位置を明示

**補助指標：ラン全体集約（サニティチェック）**

- 各ランの会話全体（20ターン分のエージェント応答を結合した1文書）を `DetectSentiment` に渡し、ラン全体の集約極性を取得
- API コール数：1ラン1コール × 4ラン = **計4コール**
- 用途：per-turn の平均値とラン全体スコアが極端に乖離していないかの確認、外れ値・崩壊ランの検出補助

**着目点**:

- ピボット直後（ターン11〜13付近）の傾き：affectus on は緩やか、off は急峻、を期待
- 同区間の絶対値：affectus on は前半の高値を引きずる、off はすぐ追従、を期待
- 性格による曲線の高さの違い：contrarian は全域で低め、friendly は全域で高め、を期待（性格効果）
- **on と off の差** が friendly と contrarian で違うかどうか = 性格 × affectus の交互作用、= 記事69行目の「同じ affectus でも性格で表現が変わる」の検証

**目視チェック**: プロット前に各ランのトランスクリプトを目検、グリーディ崩壊（ループや不自然文）があれば該当ランのみ temperature=0.2 で再走

### 3-6. 結果の解釈方針

期待される polarity の差はテキスト単独で 0.05〜0.2 程度（中程度のシフト）。記事冒頭で「劇的な変化は予想していない／効果が出るとすれば口調や絵文字の有無として現れる程度」と述べているので、結果がそのとおりでも、より大きく出ても、出なくても、率直に書く。

特に「差が出なかった」場合の解釈：

- それでも affectus 自体が決定論的に動くこと自体は CLI で示せている
- テキストという狭いチャネルでは伝わりにくいだけ、というおわりにの主張がむしろ補強される

## 4. ディレクトリ構成（affectus/examples/strands-eval/）

```
examples/strands-eval/
├── README.md                   # セットアップ・走り方・前提
├── pyproject.toml              # Python 依存（strands-agents, boto3, matplotlib 等）
├── prompts/
│   ├── friendly.md             # friendly のシステムプロンプト
│   ├── contrarian.md           # contrarian のシステムプロンプト
│   └── affectus-block.md       # affectus on 用追加ブロックの雛形
├── scripts/
│   └── user_script.json        # 固定20ターンの人間側発話
├── src/
│   ├── __init__.py
│   ├── affectus_tools.py       # affectus_show / affectus_feel を Strands Tool 化（subprocess）
│   ├── agent.py                # Strands エージェント構築（personality × affectus を引数で切替）
│   ├── run.py                  # 4ラン実行のエントリポイント
│   └── analyze.py              # Comprehend 呼び出し + polarity 計算 + プロット
├── transcripts/                # ラン出力（.gitignore）
│   ├── friendly-on.jsonl
│   ├── friendly-off.jsonl
│   ├── contrarian-on.jsonl
│   └── contrarian-off.jsonl
├── results/
│   ├── per_turn_scores.csv     # ターン × ラン の per-turn Comprehend スコア（.gitignore）
│   ├── aggregate_scores.csv    # ラン全体集約のスコア（.gitignore）
│   └── polarity-curves.png     # 最終プロット（記事掲載用、commit 対象）
└── state/                      # affectus state ファイル（.gitignore）
    ├── friendly-on.state.json
    └── contrarian-on.state.json
```

## 5. 依存・前提

- `affectus` バイナリが PATH にある（v0.2.0以上）
- AWS 認証が設定済み（`aws configure` または環境変数）、Bedrock と Comprehend にアクセス可
- Bedrock 利用リージョン：Claude モデルが日本語で安定して使えるリージョン、Comprehend が日本語 `DetectSentiment` をサポートするリージョンを plan 段階で確認・確定
- Python 3.11+
- Strands Agents SDK（最新版）

## 6. 実装段取り

1. ディレクトリ・`pyproject.toml` 雛形
2. 固定 `user_script.json`（20発話）の文面確定
3. friendly / contrarian システムプロンプト確定
4. `affectus_tools.py`（subprocess で `affectus show` / `affectus feel` を呼ぶ Strands Tool）
5. `agent.py`（personality × affectus を引数で切替）
6. `run.py`（4ラン順次実行、`transcripts/` 出力）
7. `analyze.py`（`BatchDetectSentiment` で per-turn 時系列・`DetectSentiment` でラン全体集約を取得、polarity 計算、プロット）
8. ローカル動作確認（dry-run で会話履歴を目視）
9. 本実行 → トランスクリプト目視 → プロット出力
10. 記事 SQ4 セクションへの結果反映

## 7. 記事への反映

SQ4「TONaRiで導入前後を比べる（検証中）」の見出しを **「最小エージェントで affectus の純効果を測る」** 等に改題。検証方法の表を本設計に合わせて更新し、`polarity-curves.png` を貼って観察と考察を1〜2段落でまとめる。

## 8. 関連

- affectus v0.2 設計書: [`docs/superpowers/specs/2026-05-20-affectus-v0.2-design.md`](./2026-05-20-affectus-v0.2-design.md)
- affectus システムプロンプト・スニペット: `examples/system-prompt-snippet.md`
- 旧 Phase 3 storyboard（`zenn/.cadenza/state.md`）は本設計で差し替わる

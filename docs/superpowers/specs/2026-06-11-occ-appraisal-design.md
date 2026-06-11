# affectus OCC（認知的評価）対応 — 設計仕様

- 日付: 2026-06-11
- ステータス: 設計確定（実装計画待ち）
- 関連: garden `docs/research/ai-emotion-synthesis/OCC 方式 — appraisal Phase 実装案`、`affectus 進化ロードマップ — Plutchik → Russell → Barrett`（Phase 3）

## 背景と目的

Plutchik（8軸カテゴリカル）・Russell（Valence×Arousal 2軸）はいずれも **LLM が感情そのものをデルタ申告する**モデルだった。Phase 3 では OCC モデル（Ortony, Clore & Collins 1988）を導入し、**LLM は出来事への「評価（appraisal）」だけを申告し、感情は決定論的ルールが導出する**構成に切り替える。

```
LLM ── appraise '<評価JSON>' ──▶ DeriveDeltas（OCC ルール・Go 実装）
                                     │ 感情デルタ {anger:+0.4, …}
                                     ▼
                     既存パイプライン: Decay → ApplyDeltas → SaveState → Render
```

感情の保持・減衰・clamp は既存エンジンを完全再利用し、前段に「評価→感情の変換」を1段足すだけの構成とする。

### このフェーズのゴール（スコープ確定事項）

- **OCC の22感情をフル実装する**（subset にしない）。
- **見込み台帳（prospect ledger）はエンジンが永続化する**。「見込みを覚えておく」のは状態永続化でありエンジンの責務。「この出来事はあの見込みの確定だ」という解釈・紐付け判断は LLM の責務（ID 参照で申告）。
- **対象（人物）への好意 liking は都度申告**。エンジンは人物概念を持たない。「誰をどう思うか」はエージェントメモリーの領分として分離する。
- **導出ルールは Go コードに書く**。yaml には感情軸定義と gain 係数のみ。OCC のロジックは理論の写経でありユーザーが差し替えるものではない。
- **強度変数は central（desirability / praiseworthiness / appealingness）＋ likelihood ＋ liking のみ**。global / local 変数（意外性・努力量・当然さ等）はスコープ外。
- **後方互換を厳守**: plutchik / russell の config・state・挙動は完全に不変。`feel`（直接デルタ申告）は occ モデルでも従来通り動く（汎用機構のまま。対照実験のスイッチにもなる）。

## 設計

### 1. OCC の22感情軸

`occ.default.yaml` に定義する軸（すべて baseline 0.0、halflife 90分、グローバル clamp [0,1]）:

| 分枝 | 軸名 |
|---|---|
| well-being（2） | `joy` `distress` |
| prospect-based（6） | `hope` `fear` `satisfaction` `disappointment` `relief` `fears-confirmed` |
| fortunes-of-others（4） | `happy-for` `pity` `resentment` `gloating` |
| attribution（4） | `pride` `shame` `admiration` `reproach` |
| attraction（2） | `love` `hate` |
| 複合（4） | `gratification` `gratitude` `remorse` `anger` |

既存の `AxisConfig` をそのまま使う。エンジン本体（Decay / ApplyDeltas / Render）に変更は不要。

### 2. 評価入力スキーマ（appraise JSON）

1回の申告に最大4セクション。すべて省略可（ただし全省略はエラー）。

```json
{
  "consequence": {
    "desirability": 0.6,
    "for": "self",
    "likelihood": 0.7,
    "label": "PRがマージされそう"
  },
  "action": { "praiseworthiness": -0.5, "agent": "other" },
  "object": { "appealingness": 0.3 },
  "resolve": [ { "id": "p1", "outcome": "confirmed" } ]
}
```

| フィールド | 型・値域 | 意味 |
|---|---|---|
| `consequence.desirability` | −1..+1 | 出来事の望ましさ。`for: "other"` のときは**相手にとっての**望ましさ |
| `consequence.for` | `"self"` \| `"other"` | 結果が誰に降りかかるか。省略時 `"self"` |
| `consequence.likelihood` | 0 < x < 1 | 指定時は**見込み**（prospect）。省略または 1.0 は確定事象 |
| `consequence.label` | string | 見込みの短い説明文（台帳に保存。likelihood 指定時は必須） |
| `consequence.liking` | −1..+1 | `for: "other"` のときの相手への好意（都度申告）。`for: "other"` で必須 |
| `action.praiseworthiness` | −1..+1 | 行為の称賛性（負は非難） |
| `action.agent` | `"self"` \| `"other"` | 行為の主体 |
| `object.appealingness` | −1..+1 | 対象の魅力 |
| `resolve[].id` | string | 台帳の見込み ID |
| `resolve[].outcome` | `"confirmed"` \| `"disconfirmed"` \| `"dropped"` | 確定／否認／破棄（dropped は感情を出さず台帳から消すだけ） |

バリデーション: 値域外はエラー。未知の `id` はエラー。`for:"other"` かつ `likelihood` 指定（他者見込み）は本フェーズではエラー（OCC では fortunes-of-others は確定事象のみ扱う）。

### 3. 導出ルール（`internal/engine/occ.go`、Go 実装）

各分枝の gain は config の `occ.gains` から取る。導出されるデルタはすべて非負（軸の値を上げる方向のみ。減衰は時間が担う）。

| # | 条件 | 導出 | 強度 |
|---|---|---|---|
| 1 | consequence(self, 確定) | desirability > 0 → `joy`、< 0 → `distress` | `gains.wellbeing × |des|` |
| 2 | consequence(self, likelihood < 1) | des > 0 → `hope`、< 0 → `fear`。同時に台帳へエントリ発行 | `gains.prospect × |des| × likelihood` |
| 3 | resolve(confirmed) | 台帳 des > 0 → `satisfaction`、< 0 → `fears-confirmed` | `gains.prospect × |des|` |
| 4 | resolve(disconfirmed) | 台帳 des > 0 → `disappointment`、< 0 → `relief` | `gains.prospect × |des|` |
| 5 | consequence(other) × liking | (des,liking) = (+,+) → `happy-for`、(−,+) → `pity`、(+,−) → `resentment`、(−,−) → `gloating` | `gains.fortunes × |des| × |liking|` |
| 6 | action | (praise, agent) = (+,self) → `pride`、(−,self) → `shame`、(+,other) → `admiration`、(−,other) → `reproach` | `gains.attribution × |praise|` |
| 7 | object | appealingness > 0 → `love`、< 0 → `hate` | `gains.attraction × |app|` |
| 8 | 複合: consequence(self, 確定) と action が同一申告に共存し符号が整合 | (des+, self, praise+) → `gratification`、(des+, other, praise+) → `gratitude`、(des−, self, praise−) → `remorse`、(des−, other, praise−) → `anger` | `gains.compound × min(|des|, |praise|)` |

- **複合は加算**: 構成感情（ルール1・6）も発火した上で、複合感情を追加で発火する（anger の事象では distress と reproach も上がる）。OCC の「複合 = 共起」の定式化に忠実で、ルールが合成的に書ける。
- 符号が整合しない組（des > 0 かつ praise < 0 等）は複合を発火しない（OCC に定義がない）。
- desirability = 0 等のゼロ値は該当ルールをスキップ（no-op）。
- 導出後のデルタは既存の `ApplyDeltas` に渡す（delta_clamp・axis clamp は既存処理）。

### 4. 見込み台帳（state 拡張）

```json
{
  "version": 1,
  "updated_at": "…",
  "axes": { "joy": 0.1, "…": 0 },
  "prospect_seq": 3,
  "prospects": [
    { "id": "p3", "label": "PRがマージされそう", "desirability": 0.6, "likelihood": 0.7, "created_at": "…" }
  ]
}
```

- `State` 構造体に `ProspectSeq int` と `Prospects []Prospect` を追加。JSON に無ければゼロ値（既存 plutchik / russell state はそのまま読める。保存時も空なら `omitempty` で出力しない＝**既存モデルの state ファイル形式は不変**）。
- ID は `p<連番>`（`prospect_seq` をインクリメント）。
- 台帳エントリ自体は減衰させない（`hope` / `fear` **軸**は減衰する）。
- 上限 `occ.max_prospects`（既定 20）。超過時は最古のエントリを自動 drop（感情は発火しない）。
- resolve されたエントリは台帳から除去。

### 5. occ デフォルト config（embed 新規追加）

```yaml
version: 1
model: occ
clamp:       { min: 0.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  - { name: joy,      baseline: 0.0, halflife_minutes: 90 }
  # …22軸ぶん…

occ:
  gains:
    wellbeing:   0.8
    prospect:    0.8
    fortunes:    0.6
    attribution: 0.8
    attraction:  0.6
    compound:    0.5
  max_prospects: 20

fragment_file: ""
```

- `Config` に `OCC *OCCConfig` セクションを追加（`yaml:"occ"`）。plutchik / russell では nil。
- Validate: `model: occ` のとき `occ:` セクション必須、22軸が揃っていることを検証。gain は非負。
- `engine.Models` レジストリに `"occ"` を追加（`init --model occ` で導入可能に）。

### 6. CLI: `appraise` サブコマンド

```
affectus appraise '<評価JSON>'
```

- 処理順: config ロード → state ロック → Decay → **DeriveDeltas（occ.go）＋台帳更新** → ApplyDeltas → SaveState → Render 出力。`ApplyFeel` と同じ read-modify-write サイクル（`internal/cli/write.go` に `ApplyAppraise` を追加）。
- `model != "occ"` の config で `appraise` を打ったらエラー（「occ モデルでのみ使用可能」）。
- `feel` / `tick` / `reset` / `get` は occ モデルでもそのまま動く（22軸の汎用操作として）。

### 7. `show` の出力拡張（occ のみ）

- occ モデルの `show` は、従来の軸 JSON に台帳を加えた1行 JSON を出力する:
  `{"axes":{"joy":0.10,…},"prospects":[{"id":"p3","label":"…","desirability":0.6,"likelihood":0.7}]}`
- **台帳を毎ターン LLM に見せることが要**: 文脈を失った後続セッションでも「何に hope していたか」をエンジンが思い出させ、resolve を可能にする。
- plutchik / russell の `show` 出力（軸のみの1行 JSON）は不変。fragment_file も同じ形式で書く。

### 8. MCP

`internal/mcp/server.go` に `appraise` ツールを追加（occ モデル時のみ登録、または常時登録でモデル不一致時エラー）。入力スキーマは §2 の JSON。既存ツールの説明文はモデル非依存化済みなので変更最小。

### 9. 可視化: バーチャート

22軸は円環（wheel）にも散布図にも乗らないため、occ 用は**横バーチャート**の簡易ビュー。

- `/state` レスポンスは Russell 対応時の拡張（`model` フィールド・per-axis `range`）をそのまま使い、`model == "occ"` でフロントが分岐。
- 値が 0 の軸はグレーアウト（22本並ぶため、動いている軸を目立たせる）。
- 台帳（prospects）はバーの下にリスト表示。
- `/state` に `prospects` を追加（occ のみ）。

### 10. examples

- `examples/system-prompt-snippet-occ.md` 新規: LLM に教える内容 —
  - 感情を直接申告しない。出来事への**評価**を `appraise` で申告し、感情はエンジンが導出する。
  - 評価 JSON の4セクション（consequence / action / object / resolve）の書き方と値域。
  - 見込み（likelihood < 1）には `label` を付ける。`show` の `prospects` を見て、実現／否認に気づいたら `resolve` で申告する。
  - 状態の読み方は従来と同じ（数値を読んで応答のトーンに反映）。
- `examples/configs/occ-ja.yaml` 新規: 日本語コメント付きサンプル config。

## テスト方針

- **occ.go（中核・table-driven）**: 22感情すべての導出経路（8ルール × 符号の組合せ）、複合の加算（anger 事象で distress / reproach も上がる）、符号不整合で複合が出ないこと、ゼロ値スキップ、gain の反映。
- **台帳ライフサイクル**: 発行（ID 採番・label 保存）→ confirmed / disconfirmed / dropped の各経路 → 除去。未知 ID エラー。max_prospects 超過時の最古 drop。state の JSON round-trip（既存モデルの state に `prospects` が現れないこと）。
- **config**: occ.default.yaml のパース＆Validate（22軸・gains 検証）。occ セクション無しの `model: occ` がエラー。
- **CLI**: `init --model occ` → `appraise` → `show` の一連動作。plutchik config で `appraise` がエラー。バリデーション（値域外・全セクション省略・`for:"other"` で liking 欠落）。
- **viz / MCP**: `/state` に occ の `prospects` が含まれること。appraise ツールの正常系・異常系。
- 既存テストスイート全体の green を維持（plutchik / russell の回帰なし）。

## 非対象（このスペックでやらないこと）

- strands-eval への OCC variant 組み込みと対照実験（「直接申告 vs 評価導出」比較は別スペック。基盤が動いてから設計する）。
- OCC の global / local 強度変数（意外性・努力量・当然さ・馴染み等）。
- 対象（人物）ごとの態度の永続化（attitude 台帳）。エージェントメモリーの領分。
- 他者見込み（fortunes-of-others × prospect の合成）。
- 見込み台帳エントリ自体の減衰・忘却。
- Phase 4（Barrett）の概念ストア等。

## 影響を受けるファイル（想定）

| ファイル | 変更内容 |
|---|---|
| `internal/engine/occ.go` | 新規（Appraisal 型・DeriveDeltas・台帳操作） |
| `internal/engine/occ_test.go` | 新規（中核テスト） |
| `internal/engine/occ.default.yaml` | 新規（22軸＋gains） |
| `internal/engine/config.go` | `Config.OCC` セクション追加、Validate 拡張 |
| `internal/engine/state.go` | `State.ProspectSeq` / `State.Prospects` 追加（omitempty） |
| `internal/engine/defaults.go` | `Models` に `"occ"` 追加 |
| `internal/cli/write.go` | `ApplyAppraise` 追加 |
| `internal/cli/cli.go` | （必要なら）ヘルプ文言 |
| `cmd/affectus/main.go` | `appraise` サブコマンド配線 |
| `internal/mcp/server.go` | appraise ツール追加 |
| `internal/viz/server.go` | `/state` に `prospects`（occ のみ） |
| `internal/viz/assets/index.html` | occ バーチャートビュー |
| `examples/system-prompt-snippet-occ.md` | 新規 |
| `examples/configs/occ-ja.yaml` | 新規 |
| 各 `*_test.go` | 上記のテスト追加 |

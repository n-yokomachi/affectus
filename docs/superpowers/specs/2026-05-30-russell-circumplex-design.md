# affectus Russell（感情円環）対応 — 設計仕様

- 日付: 2026-05-30
- ステータス: 設計確定（実装計画待ち）
- 関連: garden `docs/research/ai-emotion-synthesis/Russell 方式 — core affect 2軸への拡張案`、`affectus 進化ロードマップ — Plutchik → Russell → Barrett`（Phase 2）

## 背景と目的

affectus はこれまで Plutchik の感情の輪（8軸カテゴリカル感情モデル）をベースに、感情状態を決定論的に管理し解釈は LLM に委ねる設計で開発してきた（v0.3 で完了）。

本仕様は次フェーズとして、Russell の感情円環モデル（Valence × Arousal の2軸連続体）を affectus 本体で動かせるようにする。コア原則「状態管理アーキテクチャはそのまま踏襲し、軸の本数・意味・値域だけを差し替える」に従う。

### このフェーズのゴール（スコープ確定事項）

- **主目的は研究比較の基盤づくり**。最終的に strands-eval で Plutchik と Russell を対照実験するのがゴールだが、**比較実験そのものは別スペック**とする（基盤が動いてから、実挙動を見て評価軸を設計する方が手戻りが少ないため）。
- `init --model` でモデルを切り替える**プロファイル機構を導入**し、将来の Phase 3（Barrett）の基盤も兼ねる。
- **後方互換を厳守**：`--model` 省略時は `plutchik`。既存 Plutchik config の挙動は完全に不変。
- strands-eval は Plutchik run と Russell run を**別々に回して後で比較**する（同一会話での並走比較はしない）。

## 設計

### 1. エンジン: per-axis clamp

Russell の2軸は値域が非対称（valence: −1〜+1、arousal: 0〜+1）。現状はクランプがグローバル1本（`Config.Clamp`）のため、軸ごとの値域を表現できない。

`AxisConfig` に**オプションの** `range` フィールドを追加する。

```go
type AxisConfig struct {
    Name            string  `yaml:"name"`
    Baseline        float64 `yaml:"baseline"`
    HalflifeMinutes float64 `yaml:"halflife_minutes"`
    Opposite        string  `yaml:"opposite"`
    Range           *Range  `yaml:"range"` // nil ならグローバル clamp にフォールバック
}
```

- `Range` 型はすでに `internal/engine/config.go:11` に存在するため新規型は不要。
- `ApplyDeltas`（`internal/engine/apply.go`）: 軸ごとのクランプ値域を解決するヘルパー（例 `axisClamp(ax, cfg) Range` — `ax.Range != nil` ならそれ、nil なら `cfg.Clamp`）を導入し、`clamp(s.Axes[ax.Name]+d, r.Min, r.Max)` を使う。
- `delta_clamp` はグローバルのまま（1回の `<feel>` デルタ上限は両モデル共通で十分）。
- **Plutchik config は `range` 無し → グローバル clamp [0,1] で従来通り動作**。

#### Validate の追加検証（`Config.Validate`）

- `Range` 設定時に `min < max` を検証。
- `Baseline` が有効値域内にあることを検証（per-axis `range` があればその範囲、なければグローバル `clamp` の範囲）。例: arousal baseline=0.3 が [0,1] 内であること。

### 2. config: `model` フィールド

`Config` にモデル識別子を追加する。

```go
Model string `yaml:"model"` // "plutchik" | "russell"（将来 "barrett"）
```

- viz の描画分岐（レーダー vs 散布図）と、プロファイルの自己記述に使う。
- 既存 `plutchik8.default.yaml` に `model: plutchik` を追記する。

### 3. Russell デフォルト config（embed 新規追加）

`internal/engine/russell.default.yaml` を新規作成しバイナリに embed する。

```yaml
version: 1
model: russell
clamp:       { min: -1.0, max: 1.0 }   # フォールバック（per-axis range が優先される）
delta_clamp: { min: -1.0, max: 1.0 }
axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }
fragment_file: ""
```

- **減衰中心点**: valence=0.0（中立）、arousal=0.3（低〜中程度の覚醒へ沈静化）。garden メモの「中程度の覚醒が自然状態」を踏まえた確定値。
- halflife は Plutchik と揃えて 90 分。
- 対極（opposite）・隣接の関係構造は持たない（連続体）。`opposite` フィールドは省略（空）。

### 4. モデルレジストリ ＋ CLI `init --model`

embed を単一からレジストリへ拡張する（`internal/engine/defaults.go`）。

```go
//go:embed plutchik8.default.yaml
var plutchikYAML []byte

//go:embed russell.default.yaml
var russellYAML []byte

// Models maps a model identifier to its embedded default config YAML.
var Models = map[string][]byte{
    "plutchik": plutchikYAML,
    "russell":  russellYAML,
}

// DefaultConfigYAML は plutchik を指し続ける（後方互換）。
var DefaultConfigYAML = plutchikYAML
```

CLI（`cmd/affectus/main.go` の init サブコマンド ＋ `internal/cli/cli.go` の `Init`）:

- `affectus init --model {plutchik|russell}`（デフォルト `plutchik`）。
- 指定モデルの embed YAML を `config.yaml` に書き出し、整合する初期 state を生成する。
- 未知の `--model` 値はエラー（有効値リストを提示）。
- `cli.Init` のシグネチャに model 引数を追加（`DefaultConfigYAML` 直参照を `Models[model]` 経由に変更）。

### 5. 可視化: Russell 散布図（B+C 折衷）

#### サーバー側（`internal/viz/server.go`）

`/state` レスポンスを拡張する。

- トップレベルに `model` フィールドを追加（フロントの描画分岐用）。
- 各 `axisValue` に `range {min,max}` を追加（散布図の軸スケール用。per-axis range があればそれ、なければグローバル clamp）。

サーバーは引き続き **stateless**（状態履歴は持たない）。

#### フロント側（`internal/viz/assets/index.html`）

`model` で描画を分岐する。

- `model == "plutchik"`（または既定 / 軸数が2でない）→ 従来のレーダーチャート（wheel）。「8-axis wheel」のハードコード文言は軸数に応じて動的化（または中立表現に）。
- `model == "russell"` → **散布図ビュー**（B+C 折衷）:
  - valence（X軸: range から −1〜+1）× arousal（Y軸: range から 0〜+1）の平面。
  - 背景に Russell 円環の感情語ラベル（excited / happy / content / calm / sad / upset / tense / alert）を配置。
  - 現在地を大ドット、**過去の軌跡を残像トレイル**で描画。
  - 右に valence / arousal の生値パネル。
- **軌跡 = クライアント側でポーリング点を蓄積**（リングバッファ、セッション内のみ・リロードでリセット）。サーバーには状態履歴を持たせない（YAGNI）。

### 6. Russell 用 system-prompt-snippet

既存 `examples/system-prompt-snippet.md`（Plutchik 用）に対応する Russell 版を新規追加する（例 `examples/system-prompt-snippet-russell.md`）。LLM に教える内容:

- 状態が valence(−1〜+1)・arousal(0〜+1) の2軸であること。
- `<feel>{"valence": +0.3, "arousal": +0.2}</feel>` 形式でデルタを自己申告すること。
- 離散ラベルは持たず、数値の言語化（"valence高+arousal高 → excited" など）は LLM 側の解釈に委ねること。
- 関係構造（対極・隣接）の説明は無し（連続体のため）。

### 7. MCP の文言修正（軽微）

`internal/mcp/server.go`（行 47, 55 付近）の tool description にある "Plutchik wheel structure" 等のハードコード文言を、ロード中の config から動的に取るか、モデル非依存の表現に修正する。

## テスト方針

- **エンジン**: per-axis clamp のユニットテスト（valence が [−1,+1]、arousal が [0,+1] にクランプされること／各 baseline へ正しく減衰すること）。既存 Plutchik テストの回帰がないこと。
- **config**: Russell デフォルト config のパース＆Validate。`range` バリデーション（min ≥ max でエラー、baseline が値域外でエラー）。
- **CLI**: `init --model russell` が russell config を書き出すこと。`feel '{"valence":0.5,"arousal":0.4}'` が通り、未知軸 `joy` がエラーになること。未知 `--model` 値がエラーになること。
- **viz**: `/state` に `model` と per-axis `range` が含まれること。
- 既存テストスイート全体の green を維持。

## 非対象（このスペックでやらないこと）

- strands-eval への Russell variant 組み込みと、valence-arousal 軌跡の解析・作図（別スペック）。
- Plutchik 8軸を Valence×Arousal 平面に投影して同一会話で比較する実験。
- 同一会話で複数モデルを並走させる仕組み。
- サーバー側での状態履歴の永続化。
- Phase 3（Barrett）の概念ストア等。

## 影響を受けるファイル（想定）

| ファイル | 変更内容 |
|---|---|
| `internal/engine/config.go` | `AxisConfig.Range` 追加、`Config.Model` 追加、`Validate` 拡張 |
| `internal/engine/apply.go` | per-axis clamp 解決ヘルパー導入、`ApplyDeltas` 修正 |
| `internal/engine/defaults.go` | embed をレジストリ化（`Models` map） |
| `internal/engine/plutchik8.default.yaml` | `model: plutchik` 追記 |
| `internal/engine/russell.default.yaml` | 新規（Russell デフォルト config） |
| `internal/cli/cli.go` | `Init` に model 引数 |
| `cmd/affectus/main.go` | init サブコマンドに `--model` フラグ |
| `internal/viz/server.go` | `/state` に `model` と per-axis `range` |
| `internal/viz/assets/index.html` | model 分岐、Russell 散布図ビュー、文言動的化 |
| `internal/mcp/server.go` | tool description の文言をモデル非依存化 |
| `examples/system-prompt-snippet-russell.md` | 新規（Russell 用プロンプト断片） |
| `examples/configs/russell-ja.yaml` | 新規（ユーザー向けサンプル、任意） |
| 各 `*_test.go` | 上記のテスト追加 |

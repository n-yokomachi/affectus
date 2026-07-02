# affectus Barrett（構成主義的情動）対応 — 設計仕様

- 日付: 2026-06-22
- ステータス: 設計確定（実装計画待ち）
- 関連: garden `docs/research/ai-emotion-synthesis/Barrett 方式 — 構成主義的情動の実装案`、`affectus 進化ロードマップ — Plutchik → Russell → OCC → Barrett`（Phase 4）

## 背景と目的

Phase 1〜3（Plutchik / Russell / OCC）はいずれも入力→一意出力の決定論的変換だった。Barrett の構成主義的情動理論（TCE）の核心は「感情カテゴリが文脈・文化・過去経験から **その場で構成される**」動的過程であり、本質的に反・決定論的。

本フェーズの背骨（確定事項）：**動的な構成（カテゴリ化）は LLM が担い、決定論エンジンの貢献は「概念ストア（永続・減衰・差し替え可能）と、その決定論的検索」に限定する。** これは affectus の設計方針（決定論に徹し、解釈は LLM に委ねる）に沿う。Russell の core affect（valence×arousal）を下層に据え、その上に概念ストアを重ねる構造は TCE に忠実（Barrett 2017 SCAN：core affect は内受容予測の低次元投影）。

```
LLM ── recall '<属性JSON>' ──▶ 概念ストアから決定論的に想起（top-K）
   ◀── {axes, recalled[], culture_map}
   （構成＝カテゴリ化）
   ── feel '<core affectデルタ>' ──▶ 既存パイプライン（Decay → ApplyDeltas → Save）
   ── remember '<{label, vector}>' ──▶ 経験を概念ストアに永続化（importance導出・上限適用）
```

### スコープ確定事項（ブレストでの決定）

- **MVP 実働モデル**: core affect 下層＋概念ストア＋決定論的検索＋忘却＋構成ループ＋config/CLI/viz 配線。既存 eval rig で「ストア on/off」の対照実験が回る状態をゴールとする。
- **表現方式＝表現非依存ストア**: v1 は属性スキーマ規約（Li 2024 の14属性）。`Vector []float64` の generic store とし、embedding は**書き換えではなく規約変更**で到達できる受け皿だけ用意する。affectus 自身は embedding を計算せず、ベクトル値はエージェントが申告する（delta/appraisal と同じ）。よって embedding モードでも affectus バイナリは zero-dependency のまま。
- **モデルの位置づけ＝新モデル `barrett`**: Russell 同型の valence/arousal 2軸を自身の state に持ち core affect 下層とする。`feel` を再利用。OCC の `appraise` は独立モデルのままで Barrett は使わない。
- **概念ストアのエントリ単位＝個々の経験**: 1エントリ=過去の情動経験。検索は「今ターンの自己申告ベクトル」との類似上位 K を返す。忘却は経験単位で効く（差別化軸）。
- **検索・忘却スコア＝relevance + recency + 導出 importance**: importance は経験の core affect 強度から決定論的に導出（追加申告なし）。Park 型の重み付き和・min-max 正規化、すべて純 Go。
- **文化マップ＝最小 config 提供**: config が提供するカテゴリ-語彙マップを recall/show 出力に echo。エンジン state ではなく config 扱い。「差し替え→出力変化→応答変化」で防衛線の第一節を成立させる（異文化対照 eval そのものは後続）。

## 設計

### 1. モデルの位置づけ

`Config.Model` に新値 `"barrett"` を追加。core affect 下層は Russell と同型（valence ∈ [-1,1] / arousal ∈ [0,1] の2軸、各 `range` override）。`feel`・`tick`・`Decay`・`ApplyDeltas`・`Render` は既存をそのまま再利用し、core affect の保持・減衰・描画を担う。

**重要な分離**：core affect（常に valence/arousal の2次元）と、検索表現ベクトル（v1=14属性、将来 embedding）は別物として持つ。importance は常に core affect 強度から導出するため、検索表現を embedding に差し替えても importance ロジックは不変。これが「表現非依存ストア」を成立させる。

### 2. データモデル

既存 `engine.State` に、`Prospects`/`ProspectSeq` と同じ `omitempty` パターンで概念ストアを追加する。plutchik/russell/occ の state 出力はバイト不変。

```go
// Concept is one stored emotional experience (barrett model).
type Concept struct {
    ID           string    `json:"id"`            // "c1", "c2", ...
    Label        string    `json:"label"`         // category the LLM applied, e.g. "frustration"
    Vector       []float64 `json:"vector"`        // retrieval representation; v1 = 14-attribute schema
    Valence      float64   `json:"valence"`       // core-affect snapshot at write time
    Arousal      float64   `json:"arousal"`
    Importance   float64   `json:"importance"`    // derived from core-affect intensity, [0,1]
    CreatedAt    time.Time `json:"created_at"`
    LastRecalled time.Time `json:"last_recalled"` // recency basis; init = CreatedAt, bumped on recall
}

// State additions (omitempty keeps other models byte-identical):
//   ConceptSeq int        `json:"concept_seq,omitempty"`
//   Concepts   []Concept  `json:"concepts,omitempty"`
```

- `Vector` はスライス（14属性も embedding も同一コード）。次元名は config の `vector_dims` で宣言し、(a) 検査可能性（「なぜこのカテゴリか」の追跡＝防衛線第二節）、(b) エージェントが何を申告すべきかの契約、を兼ねる。
- `Valence`/`Arousal` は core affect スナップショット（importance 導出と検査に使う）。検索表現ベクトルとは独立に常に保持する。

### 3. 検索と忘却

#### 検索（recall）

エージェントが申告したクエリベクトル `q` に対し、各エントリ `e` のスコアを計算：

- `relevance(e)` = 既定では `q` と `e.Vector` のコサイン類似度 `cos ∈ [-1,1]` を `(cos + 1) / 2` で [0,1] に写像。`distance` config が cosine 以外を指すときはその距離を [0,1] の類似度に写像する。いずれかが零ベクトルのときは relevance=0。
- `recency(e)` = `0.5 ^ (age / concept_halflife_minutes)`、`age = now - e.LastRecalled`（分）。既存 half-life 哲学を流用。**減衰するのは検索の重みであり、ベクトル値・core affect スナップショットは不変**（概念は baseline に向かわない）。
- `importance(e)` = 書込時に core affect 強度から導出済みの固定値（次節）。
- `score(e)` = `w_rel·relevance + w_rec·recency + w_imp·importance`。各成分は候補集合内で min-max 正規化してから加重（Park 型）。縮退ケース（候補全件で max=min の成分）はその成分を全候補 0 として扱う（全員同値なので順位に影響しない）。重み既定 1/1/1（config）。
- 上位 `recall_k` 件を返す。返したエントリの `LastRecalled` を `now` に更新（Park 型の想起強化）。よって recall は state を変更する＝lock を取る書き込み操作。

`recall_k: 0` は store-off モード＝eval の対照群スイッチ。`recalled` は常に `[]` だが axes と culture_map は通常どおり返し、`LastRecalled` は更新しない（実質 read-only）。

#### importance の導出

`importance` ∈ [0,1] は、経験の core affect スナップショットが中立点（valence・arousal **各軸の baseline**）からどれだけ離れているかのユークリッド距離を、最大距離（中立点から各軸 range の遠い側の端までの距離のノルム）で割って得る（中立点から遠い＝強い情動経験ほど高い）。激しい経験ほど忘れにくく、想起されやすい。

#### 忘却（eviction）

- `max_concepts` のハード上限（`max_prospects` 同様）。
- 超過時は **最低スコアのエントリから退去**。退去判定はクエリが無いので `w_rec·recency + w_imp·importance`（relevance 抜き）を全エントリで正規化して用いる＝古く・些末な経験から忘れる。
- 減衰基点は**時間ベース**（経験数ベースではない＝affectus 全体と一貫）。上限は件数のハード天井として併用。
- 退去した経験は state から消え、エンジンからは未経験と同一視する。**MVP の既知の割り切り**（「忘却された経験 ≠ 未経験」という心理学的疑問は将来課題）。

### 4. プロトコルと CLI 表面（構成ループ）

`feel`・`tick`・`show` は既存・汎用のまま。barrett 専用に `recall` と `remember` を追加。1ターンの流れ：

```
① affectus recall '<14属性JSON>'
     → {axes(core affect), recalled:[上位K経験], culture_map}
     （副作用: 返した経験の LastRecalled を更新。lock取得）
       ↓ LLM が「以前こう感じた時は X と構えた」を読んで構成・応答
② affectus feel '<core affectデルタ>'     （既存・汎用。valence/arousal 更新）
③ affectus remember '<{label, vector}>'
     （エンジンが post-feel の valence/arousal をスナップショット、importance導出、
      id採番、append、max_concepts上限適用。lock取得）
```

設計判断：

- **recall は新コマンド**（`show` は read-only を維持。recall は `LastRecalled` を更新するため別立て）。
- **feel と remember を分離**。feel を汎用のまま残すことで、eval の store-off 対照群が素の feel で動く。順序は feel→remember を規約とし、remember は更新後 core affect をスナップショットする。
- **`show`**（既存・read-only）は core affect ＋ 概念ストア全体 ＋ culture_map を返す（`RenderOCC` が axes+prospects を返すのと同型のレンダラ `RenderBarrett`。culture_map の echo はスコープ確定事項「recall/show 出力に echo」に対応）。検査・viz 用。ランキングも `LastRecalled` 更新もしない。
- **`tick`** は core affect のみ減衰。概念の recency は `LastRecalled` から都度算出するので tick 変更は不要。
- **入力フォーマット**：recall のクエリも remember の `vector` も、`vector_dims` の全次元名をキーに持つ JSON オブジェクト（`feel` と同じ named-key 形式・順序非依存）。エンジンが `vector_dims` 順の `[]float64` に写像して保存・比較する。欠落・未知の次元名はエラー。将来の embedding 規約では named-key でなく生配列を受ける（規約変更のみ、エンジンの検索・忘却ロジックは不変）。
- バリデーション：要素ごとの値域は v1 では強制しない（cosine は尺度に寛容なため。将来 vector_dims に値域を付すのは拡張）。`label` 空・不正 JSON はエラー。`barrett` 以外のモデルで recall/remember はエラー。

### 5. config（model=barrett）

`OCCConfig` と同じく `Config` に `Barrett *BarrettConfig` を追加（他モデルでは nil）。

```go
type BarrettConfig struct {
    VectorDims       []string           `yaml:"vector_dims"`
    RecallK          int                `yaml:"recall_k"`
    MaxConcepts      int                `yaml:"max_concepts"`
    HalflifeMinutes  float64            `yaml:"concept_halflife_minutes"`
    Weights          BarrettWeights     `yaml:"weights"`   // relevance/recency/importance
    Distance         string             `yaml:"distance"`  // "cosine"（既定）
    CultureMap       string             `yaml:"culture_map"`
}
```

```yaml
version: 1
model: barrett
clamp: {min: -1.0, max: 1.0}
delta_clamp: {min: -1.0, max: 1.0}
axes:                              # core affect = Russell 2軸（流用）
  - {name: valence, baseline: 0.0, halflife_minutes: 90, range: {min: -1, max: 1}}
  - {name: arousal, baseline: 0.3, halflife_minutes: 90, range: {min: 0,  max: 1}}
barrett:
  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]   # v1 = 14属性（Li 2024）
  recall_k: 5
  max_concepts: 200
  concept_halflife_minutes: 10080          # recency減衰（例: 7日）
  weights: {relevance: 1.0, recency: 1.0, importance: 1.0}
  distance: cosine
  culture_map: |                           # 差し替え→出力変化→応答変化（防衛線第一節）
    高覚醒・不快: 怒り / 苛立ち / 焦り
    低覚醒・不快: 悲しみ / 侘しさ / 気だるさ
    高覚醒・快: 歓喜 / 昂揚 / わくわく
    低覚醒・快: 安らぎ / 満足 / 懐かしさ
fragment_file: ""
```

Validate（`Config.Validate` を拡張）：`model: barrett` は barrett セクション必須／core affect 2軸（valence・arousal）必須／`vector_dims` 非空／`recall_k` は **0 以上**（0 は store-off モード、§3・§7）／`max_concepts`・`concept_halflife_minutes` 正／weights 非負／`distance` は既知の値（**省略時は cosine 既定**）。`barrett` 以外のモデルでは既存挙動を一切変えない。

### 6. viz

既存の model 別レンダリングに barrett ビューを追加：

- 上段：Russell の 2D core affect 平面を流用（valence×arousal の点＋軌跡）。
- 下段：概念ストア一覧（id / label / valence・arousal / importance / age）。OCC の bar＋ledger ビューと同型。想起（`LastRecalled` 更新）と忘却（退去）が時間で動くのが見える。read-only サーバのまま。

### 7. eval 配線

`examples/strands-eval` の N 反復ハイブリッドを流用：

- 対照軸＝**store on/off**：barrett（recall が経験を返す）vs barrett-store-off（`recall_k: 0` で `recalled` が常に空＝core affect のみ）。「概念ストアが応答の質感を変えるか」を測る。
- 性格 × store on/off × N 反復で既存リグに乗せる。Comprehend 極性と内部状態の両チャネルで比較。
- **異文化対照 eval（マップ差し替え実験）は MVP スコープ外**。配線だけ残し実験は後続。

### 8. テスト（既存 TDD パターン・各 `_test.go` に倣う）

- ベクトル距離 / cosine（ゼロベクトル含む）
- 検索ランキング（relevance+recency+importance の min-max 正規化・縮退ケース・top-K・recall_k=0）
- importance 導出（中立点からの距離正規化）
- recall の `LastRecalled` 更新
- remember（`LastRecalled`=`CreatedAt` 初期化・post-feel スナップショット・ID 採番・named-key→配列写像）
- 忘却（上限超過時に最低スコア退去、recency·importance のみで判定）
- 概念ストア render（`RenderBarrett`）
- config validation（barrett 必須要件・他モデル不変）
- **後方互換**（plutchik/russell/occ の state がバイト不変・feel/tick 不変）

### 9. 後方互換

- `Concepts`/`ConceptSeq` は `omitempty` で既存 state に非出現。
- `feel`/`tick`/`show` は汎用のまま。barrett 固有ロジックは `model: barrett` でゲート。
- 新規ファイル想定：`internal/engine/barrett.go`（+ `barrett_test.go`）、`internal/engine/barrett.default.yaml`、`examples/configs/barrett-ja.yaml`、`examples/system-prompt-snippet-barrett.md`、CLI に recall/remember 配線、viz に barrett ビュー。

## 既知の割り切り（MVP）

- **embedding は v1 では受け皿のみ**：属性スキーマ規約で動かし、ストアは表現非依存（generic vector store）。embedding は将来の規約変更で到達。state JSON 肥大（embedding 次元）は将来の検討事項。
- **異文化対照 eval は後続**：文化マップ機構（config 提供・swap 可能）は v1 に入れるが、実験は MVP 外。
- **退去経験は未経験と同一視**：忘却＝state から消去。心理学的により正確な「忘却された記憶の残響」は将来課題。

## 表現方式の将来拡張（embedding パス）

`Vector []float64` と `distance`（cosine）は embedding をそのまま受けられる。embedding モードへの移行に必要な変更は：(a) `vector_dims` を次元数指定（または省略可）に緩める、(b) エージェント側が属性自己申告でなく embedding を申告する規約に変える、の2点で、エンジンの検索・忘却ロジックは不変。affectus は依然 embedding を計算しない（zero-dependency 維持）。

## 未解決 / 後続

- 異文化対照 eval の設計（マップ差し替えで応答が「らしく」変わるかの測り方）。
- 「構成の質感」の数値指標（Plutchik/Russell より格段に難しい）。
- 退去経験の心理学的により正確な扱い。
- embedding 規約の本格設計（state 肥大対策・申告プロトコル）。
- MCP surface（emotion_recall / emotion_remember 相当）の追加要否。MVP は CLI/viz のみ（OCC で確立した「サーフェス毎の露出差は意図として文書化する」方針に沿って後続判断）。

## 参考

- garden: `Barrett 方式 — 構成主義的情動の実装案` / `感情モデルの全体地図` / `affectus 進化ロードマップ`
- papers: Barrett 2017 SCAN（理論基盤）/ Seth & Friston 2016（身体なし LLM の数値的内受容）/ Li et al. 2024（14属性スキーマ）/ Croissant et al. 2024（構成ループ）/ Park et al. 2023（検索ランキング recency+importance+relevance）/ Anthropic 2026（LLM は持続感情を内部に持たない＝外部ストアの根拠）
- 既存実装の前例：`internal/engine/occ.go` の prospect ledger（エンジンが永続・毎ターン提示、LLM が ID 参照で更新）は概念ストアの直系の前例。

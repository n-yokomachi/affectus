# Barrett（構成主義的情動）Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** affectus に第4のモデル `barrett` を追加する — Russell 同型の core affect（valence×arousal）下層＋概念ストア（個々の情動経験の永続・決定論的検索・忘却）＋ recall/remember の構成ループ。

**Architecture:** カテゴリ化（構成）は LLM の仕事。エンジンは (1) 経験エントリの永続（既存 prospect ledger と同じ omitempty state パターン）、(2) relevance+recency+importance の Park 型スコアによる決定論的 top-K 検索、(3) `max_concepts` 超過時の最低スコア退去、のみを担う。core affect の保持・減衰は既存 `feel`/`tick`/`Decay` を完全再利用する。

**Tech Stack:** Go（stdlib＋既存の gopkg.in/yaml.v3 のみ。新規依存ゼロ）、既存の viz（vanilla JS）、examples の strands-eval（Python, uv）。

**Spec:** `docs/superpowers/specs/2026-06-22-barrett-constructed-emotion-design.md`（このプランの正）

## Global Constraints

- **後方互換の厳守**: plutchik / russell / occ の config・state・挙動は完全不変。新 state フィールドはすべて `omitempty`。barrett 固有ロジックは `cfg.Model == "barrett"` でゲート。
- **zero-dependency**: 新規外部依存を追加しない。embedding は計算しない（ベクトル値はエージェントが申告する）。
- **決定論**: エンジンに乱数・外部 I/O を持ち込まない。同じ入力・同じ `now` なら同じ出力。ソートは `sort.SliceStable` で tie を安定化。
- **入力フォーマット**: recall のクエリと remember の `vector` は `vector_dims` の全次元名をキーに持つ JSON オブジェクト（named-key、順序非依存）。欠落・未知の次元名はエラー。
- **軸名・次元名は JSON 特殊文字禁止**（`Render` が無エスケープで interpolate するため。既存制約の踏襲）。
- コミットメッセージは既存履歴の Conventional Commits 形式（`feat(engine):` / `test(engine):` / `docs(examples):` 等）。ツール名・モデル名の署名は入れない。
- 各タスク完了時に `go test ./...` が全パスしていること。Go コードは `gofmt` 済みで commit。

---

### Task 1: BarrettConfig — config 型・validation・埋め込みデフォルト

**Files:**
- Modify: `internal/engine/config.go`
- Create: `internal/engine/barrett.default.yaml`
- Modify: `internal/engine/defaults.go`
- Test: `internal/engine/config_test.go`（追記）

**Interfaces:**
- Consumes: 既存 `Config` / `AxisConfig` / `Range` / `ParseConfig`
- Produces: `engine.BarrettWeights{Relevance, Recency, Importance float64}`、`engine.BarrettConfig{VectorDims []string; RecallK int; MaxConcepts int; HalflifeMinutes float64; Weights BarrettWeights; Distance string; CultureMap string}`、`Config.Barrett *BarrettConfig`、`engine.Models["barrett"]`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/config_test.go` の末尾に追記:

```go
func TestParseBarrettDefaultConfig(t *testing.T) {
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse barrett default config: %v", err)
	}
	if cfg.Model != "barrett" || cfg.Barrett == nil {
		t.Fatalf("model = %q, barrett section present = %v", cfg.Model, cfg.Barrett != nil)
	}
	if len(cfg.Barrett.VectorDims) != 14 {
		t.Errorf("vector_dims = %d, want 14", len(cfg.Barrett.VectorDims))
	}
	if cfg.Barrett.RecallK != 5 || cfg.Barrett.MaxConcepts != 200 {
		t.Errorf("recall_k=%d max_concepts=%d, want 5/200", cfg.Barrett.RecallK, cfg.Barrett.MaxConcepts)
	}
	if cfg.Barrett.HalflifeMinutes != 10080 {
		t.Errorf("concept_halflife_minutes = %v, want 10080", cfg.Barrett.HalflifeMinutes)
	}
	if cfg.Barrett.CultureMap == "" {
		t.Error("culture_map must not be empty in the default config")
	}
}

func TestBarrettConfigValidation(t *testing.T) {
	base := string(Models["barrett"])
	repl := func(old, new string) string {
		if !strings.Contains(base, old) {
			t.Fatalf("test setup: default yaml does not contain %q", old)
		}
		return strings.Replace(base, old, new, 1)
	}
	tests := []struct {
		name    string
		yaml    string
		wantErr string // "" = must be valid
	}{
		{"recall_k zero is valid (store-off)", repl("recall_k: 5", "recall_k: 0"), ""},
		{"negative recall_k", repl("recall_k: 5", "recall_k: -1"), "recall_k"},
		{"zero max_concepts", repl("max_concepts: 200", "max_concepts: 0"), "max_concepts"},
		{"zero halflife", repl("concept_halflife_minutes: 10080", "concept_halflife_minutes: 0"), "concept_halflife_minutes"},
		{"negative weight", repl("relevance: 1.0", "relevance: -0.5"), "weights"},
		{"unknown distance", repl("distance: cosine", "distance: euclid"), "distance"},
		{"empty distance is valid (cosine default)", repl("distance: cosine", `distance: ""`), ""},
		{"missing barrett section", `
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }
axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min: 0.0, max: 1.0 } }
`, "barrett section"},
		{"missing arousal axis", `
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }
axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
barrett:
  vector_dims: [valence, arousal]
  recall_k: 5
  max_concepts: 200
  concept_halflife_minutes: 10080
  weights: { relevance: 1.0, recency: 1.0, importance: 1.0 }
`, `axis "arousal"`},
		{"empty vector_dims", repl(`  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]`, "  vector_dims: []"), "vector_dims"},
		{"duplicate vector dim", repl("expectedness, novelty]", "expectedness, novelty, valence]"), "duplicate vector dimension"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("want valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("want error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}
```

`config_test.go` の import に `strings` が無ければ追加する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'TestParseBarrettDefault|TestBarrettConfigValidation' -v`
Expected: FAIL（`Models["barrett"]` が nil → ParseConfig が "no axes defined" 等で落ちる／`cfg.Barrett` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装**

`internal/engine/barrett.default.yaml` を新規作成（デフォルトは英語。文化マップの日本語版は Task 11 の `examples/configs/barrett-ja.yaml`）:

```yaml
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  # core affect (Russell circumplex) — the substrate the concept store sits on
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

barrett:
  # v1 retrieval representation: the 14 emotion-concept attributes of
  # Li et al. 2024 (iScience). The agent self-reports these values; the
  # engine only stores and deterministically searches them.
  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]
  recall_k: 5                       # top-K experiences returned by recall (0 = store-off)
  max_concepts: 200                 # hard cap; lowest recency+importance evicted first
  concept_halflife_minutes: 10080   # retrieval recency half-life (7 days)
  weights: { relevance: 1.0, recency: 1.0, importance: 1.0 }
  distance: cosine
  # Category vocabulary echoed back on recall/show. Swap the map to shift
  # the culture the agent constructs its emotions in.
  culture_map: |
    high-arousal unpleasant: anger / irritation / anxiety
    low-arousal unpleasant: sadness / gloom / weariness
    high-arousal pleasant: excitement / joy / eagerness
    low-arousal pleasant: calm / contentment / nostalgia

fragment_file: ""
```

`internal/engine/defaults.go` に埋め込みを追加:

```go
//go:embed barrett.default.yaml
var barrettYAML []byte
```

`Models` マップに `"barrett": barrettYAML,` を追加。

`internal/engine/config.go` — `OCCConfig` の直後に型を追加:

```go
// BarrettWeights holds the ranking weights of the concept-store retrieval
// score (barrett model).
type BarrettWeights struct {
	Relevance  float64 `yaml:"relevance"`
	Recency    float64 `yaml:"recency"`
	Importance float64 `yaml:"importance"`
}

// BarrettConfig is the barrett-model section of the config. nil for other
// models.
type BarrettConfig struct {
	// VectorDims declares the named dimensions of the retrieval vector.
	// v1 convention: the 14 emotion-concept attributes of Li et al. 2024.
	VectorDims []string `yaml:"vector_dims"`
	// RecallK is the number of experiences recall returns. 0 = store-off
	// (recall always returns an empty list; the eval control-group switch).
	RecallK     int     `yaml:"recall_k"`
	MaxConcepts int     `yaml:"max_concepts"`
	// HalflifeMinutes is the retrieval-recency half-life. It decays an
	// entry's search weight, never its stored values.
	HalflifeMinutes float64        `yaml:"concept_halflife_minutes"`
	Weights         BarrettWeights `yaml:"weights"`
	Distance        string         `yaml:"distance"` // "" | "cosine"
	CultureMap      string         `yaml:"culture_map"`
}
```

`Config` 構造体の `OCC *OCCConfig` の次に `Barrett *BarrettConfig \`yaml:"barrett"\`` を追加。

`Validate()` の `if c.Model == "occ" { ... }` ブロックの直後に追加:

```go
if c.Model == "barrett" {
	if c.Barrett == nil {
		return fmt.Errorf("config: model barrett requires a barrett section")
	}
	b := c.Barrett
	for _, name := range []string{"valence", "arousal"} {
		if !seen[name] {
			return fmt.Errorf("config: model barrett requires axis %q", name)
		}
	}
	if len(b.VectorDims) == 0 {
		return fmt.Errorf("config: barrett vector_dims must not be empty")
	}
	seenDim := map[string]bool{}
	for _, d := range b.VectorDims {
		if seenDim[d] {
			return fmt.Errorf("config: duplicate vector dimension %q", d)
		}
		seenDim[d] = true
	}
	if b.RecallK < 0 {
		return fmt.Errorf("config: barrett recall_k must be non-negative")
	}
	if b.MaxConcepts <= 0 {
		return fmt.Errorf("config: barrett max_concepts must be positive")
	}
	if b.HalflifeMinutes <= 0 {
		return fmt.Errorf("config: barrett concept_halflife_minutes must be positive")
	}
	w := b.Weights
	if w.Relevance < 0 || w.Recency < 0 || w.Importance < 0 {
		return fmt.Errorf("config: barrett weights must be non-negative")
	}
	switch b.Distance {
	case "", "cosine":
	default:
		return fmt.Errorf("config: barrett distance %q is not supported (want cosine)", b.Distance)
	}
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v -run 'TestParseBarrettDefault|TestBarrettConfigValidation'` → PASS
Run: `go test ./...` → 全 PASS（既存テストが壊れていないこと）

- [ ] **Step 5: Commit**

```bash
git add internal/engine/config.go internal/engine/defaults.go internal/engine/barrett.default.yaml internal/engine/config_test.go
git commit -m "feat(engine): add barrett model config with validation and embedded default"
```

---

### Task 2: Concept 型と State 拡張（omitempty 後方互換）

**Files:**
- Modify: `internal/engine/state.go`
- Modify: `internal/engine/decay.go`
- Modify: `internal/engine/apply.go`
- Test: `internal/engine/state_test.go`（追記）

**Interfaces:**
- Consumes: Task 1 の `Config.Barrett`
- Produces: `engine.Concept{ID string; Label string; Vector []float64; Valence, Arousal, Importance float64; CreatedAt, LastRecalled time.Time}`、`State.ConceptSeq int`、`State.Concepts []Concept`。**Decay / ApplyDeltas が Concepts を素通しで保存する**（これを忘れると feel/tick のたびに記憶が全消去される）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/state_test.go` の末尾に追記:

```go
func TestStateConceptsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ConceptSeq = 2
	s.Concepts = []Concept{{
		ID: "c2", Label: "frustration",
		Vector:  []float64{0.1, 0.9, 0, 0.5, 0, 0, 0, 0, 0.2, 0.1, 0.8, 0.3, 0.4, 0.6},
		Valence: -0.4, Arousal: 0.7, Importance: 0.55,
		CreatedAt: now, LastRecalled: now,
	}}
	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.ConceptSeq != 2 || len(got.Concepts) != 1 {
		t.Fatalf("round trip lost concepts: seq=%d n=%d", got.ConceptSeq, len(got.Concepts))
	}
	c := got.Concepts[0]
	if c.ID != "c2" || c.Label != "frustration" || len(c.Vector) != 14 {
		t.Errorf("concept fields lost: %+v", c)
	}
}

func TestStateWithoutConceptsOmitsKeys(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	b, err := json.Marshal(NewState(cfg, now))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(b), "concept") {
		t.Errorf("plutchik state must not contain concept keys, got %s", b)
	}
}

func TestDecayAndApplyPreserveConcepts(t *testing.T) {
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ConceptSeq = 1
	s.Concepts = []Concept{{ID: "c1", Label: "calm", Vector: make([]float64, 14), CreatedAt: now, LastRecalled: now}}

	d := Decay(s, cfg, now.Add(30*time.Minute))
	if d.ConceptSeq != 1 || len(d.Concepts) != 1 {
		t.Fatalf("Decay dropped concepts: seq=%d n=%d", d.ConceptSeq, len(d.Concepts))
	}
	a, err := ApplyDeltas(d, map[string]float64{"valence": 0.2}, cfg)
	if err != nil {
		t.Fatalf("ApplyDeltas: %v", err)
	}
	if a.ConceptSeq != 1 || len(a.Concepts) != 1 {
		t.Fatalf("ApplyDeltas dropped concepts: seq=%d n=%d", a.ConceptSeq, len(a.Concepts))
	}
}
```

`state_test.go` の import に `encoding/json` / `strings` / `path/filepath` / `time` が無ければ追加する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'Concept' -v`
Expected: FAIL（`Concept` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装**

`internal/engine/state.go` — `Prospect` 型の直後に追加:

```go
// Concept is one stored emotional experience (barrett model): the retrieval
// vector the agent reported, the core-affect snapshot at write time, and a
// derived importance. The engine persists, searches, and forgets these;
// categorization (what they mean now) is the LLM's job.
type Concept struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Vector is the retrieval representation in cfg.Barrett.VectorDims
	// order. v1 = the 14-attribute schema; a future embedding convention
	// changes only what the agent reports, not this storage.
	Vector     []float64 `json:"vector"`
	Valence    float64   `json:"valence"`
	Arousal    float64   `json:"arousal"`
	Importance float64   `json:"importance"`
	CreatedAt  time.Time `json:"created_at"`
	// LastRecalled is the recency basis: initialized to CreatedAt and
	// bumped to now each time recall returns this entry.
	LastRecalled time.Time `json:"last_recalled"`
}
```

`State` に追加（`Prospects` の次。omitempty で他モデルの state はバイト不変）:

```go
	// ConceptSeq and Concepts are only used by the barrett model.
	ConceptSeq int       `json:"concept_seq,omitempty"`
	Concepts   []Concept `json:"concepts,omitempty"`
```

`internal/engine/decay.go` — `Decay` の return を修正:

```go
	return State{Version: s.Version, UpdatedAt: now, Axes: axes,
		ProspectSeq: s.ProspectSeq, Prospects: s.Prospects,
		ConceptSeq: s.ConceptSeq, Concepts: s.Concepts}
```

`internal/engine/apply.go` — `ApplyDeltas` の return を修正:

```go
	return State{Version: s.Version, UpdatedAt: s.UpdatedAt, Axes: axes,
		ProspectSeq: s.ProspectSeq, Prospects: s.Prospects,
		ConceptSeq: s.ConceptSeq, Concepts: s.Concepts}, nil
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run 'Concept' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/state.go internal/engine/decay.go internal/engine/apply.go internal/engine/state_test.go
git commit -m "feat(engine): persist barrett concept store entries in state"
```

---

### Task 3: barrett.go — named-key ベクトル写像・cosine relevance・importance 導出

**Files:**
- Create: `internal/engine/barrett.go`
- Test: `internal/engine/barrett_test.go`（新規）

**Interfaces:**
- Consumes: Task 1 `BarrettConfig`、Task 2 `Concept`、既存 `axisClamp`
- Produces: `engine.BarrettVector(named map[string]float64, cfg Config) ([]float64, error)`（公開）、`cosineRelevance(a, b []float64) float64`・`conceptImportance(valence, arousal float64, cfg Config) float64`・`minMaxNorm(raw []float64) []float64`（非公開、同パッケージのテストから直接検証）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/barrett_test.go` を新規作成:

```go
package engine

import (
	"strings"
	"testing"
	"time"
)

func barrettCfg(t *testing.T) Config {
	t.Helper()
	cfg, err := ParseConfig(Models["barrett"])
	if err != nil {
		t.Fatalf("parse barrett default config: %v", err)
	}
	return cfg
}

var barrettNow = time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)

// namedVec returns a full named vector with every dim at base, overridden by kv.
func namedVec(t *testing.T, cfg Config, base float64, kv map[string]float64) map[string]float64 {
	t.Helper()
	m := make(map[string]float64, len(cfg.Barrett.VectorDims))
	for _, d := range cfg.Barrett.VectorDims {
		m[d] = base
	}
	for k, v := range kv {
		m[k] = v
	}
	return m
}

func TestBarrettVectorMapsNamedToDimOrder(t *testing.T) {
	cfg := barrettCfg(t)
	named := namedVec(t, cfg, 0, map[string]float64{"valence": 0.5, "novelty": 0.9})
	vec, err := BarrettVector(named, cfg)
	if err != nil {
		t.Fatalf("BarrettVector: %v", err)
	}
	if len(vec) != 14 {
		t.Fatalf("len = %d, want 14", len(vec))
	}
	if vec[0] != 0.5 { // vector_dims[0] = valence
		t.Errorf("vec[0] = %v, want 0.5", vec[0])
	}
	if vec[13] != 0.9 { // vector_dims[13] = novelty
		t.Errorf("vec[13] = %v, want 0.9", vec[13])
	}
}

func TestBarrettVectorRejectsMissingAndUnknownDims(t *testing.T) {
	cfg := barrettCfg(t)
	missing := namedVec(t, cfg, 0, nil)
	delete(missing, "fairness")
	if _, err := BarrettVector(missing, cfg); err == nil || !strings.Contains(err.Error(), "missing") {
		t.Errorf("want missing-dimension error, got %v", err)
	}
	unknown := namedVec(t, cfg, 0, nil)
	unknown["moxie"] = 1
	if _, err := BarrettVector(unknown, cfg); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("want unknown-dimension error, got %v", err)
	}
}

func TestBarrettVectorRequiresBarrettModel(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	if _, err := BarrettVector(map[string]float64{}, cfg); err == nil || !strings.Contains(err.Error(), "barrett") {
		t.Errorf("want barrett-model error, got %v", err)
	}
}

func TestCosineRelevance(t *testing.T) {
	a := []float64{1, 0, 0}
	tests := []struct {
		name string
		b    []float64
		want float64
	}{
		{"identical", []float64{1, 0, 0}, 1.0},
		{"opposite", []float64{-1, 0, 0}, 0.0},
		{"orthogonal", []float64{0, 1, 0}, 0.5},
		{"zero vector", []float64{0, 0, 0}, 0.0},
	}
	for _, tt := range tests {
		if got := cosineRelevance(a, tt.b); !almostEqual(got, tt.want) {
			t.Errorf("%s: relevance = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestConceptImportance(t *testing.T) {
	cfg := barrettCfg(t)
	// Neutral point = axis baselines (valence 0.0, arousal 0.3).
	if got := conceptImportance(0.0, 0.3, cfg); !almostEqual(got, 0) {
		t.Errorf("neutral importance = %v, want 0", got)
	}
	// Farthest corner: valence ±1 (dist 1.0), arousal 1.0 (dist 0.7) -> importance 1.
	if got := conceptImportance(-1.0, 1.0, cfg); !almostEqual(got, 1) {
		t.Errorf("extreme importance = %v, want 1", got)
	}
	// Monotonic: farther from neutral = more important.
	mild := conceptImportance(0.2, 0.4, cfg)
	strong := conceptImportance(0.8, 0.9, cfg)
	if !(mild > 0 && strong > mild && strong < 1) {
		t.Errorf("want 0 < mild(%v) < strong(%v) < 1", mild, strong)
	}
}

func TestMinMaxNorm(t *testing.T) {
	got := minMaxNorm([]float64{2, 4, 3})
	want := []float64{0, 1, 0.5}
	for i := range want {
		if !almostEqual(got[i], want[i]) {
			t.Fatalf("norm[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	// Degenerate: all equal -> all zeros (component drops out of ranking).
	for _, v := range minMaxNorm([]float64{7, 7, 7}) {
		if v != 0 {
			t.Fatalf("degenerate norm = %v, want all zeros", v)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'BarrettVector|CosineRelevance|ConceptImportance|MinMaxNorm' -v`
Expected: FAIL（`BarrettVector` 未定義のコンパイルエラー）

- [ ] **Step 3: 実装**

`internal/engine/barrett.go` を新規作成:

```go
package engine

import (
	"fmt"
	"math"
)

// BarrettVector maps a named-key vector report (the recall/remember input
// convention, mirroring feel's named deltas) onto a []float64 in
// cfg.Barrett.VectorDims order. Every declared dimension is required;
// unknown names are rejected so LLM typos fail loudly.
func BarrettVector(named map[string]float64, cfg Config) ([]float64, error) {
	if cfg.Model != "barrett" || cfg.Barrett == nil {
		return nil, fmt.Errorf("recall/remember require a barrett-model config (got model %q)", cfg.Model)
	}
	dims := cfg.Barrett.VectorDims
	known := make(map[string]bool, len(dims))
	for _, d := range dims {
		known[d] = true
	}
	for name := range named {
		if !known[name] {
			return nil, fmt.Errorf("unknown vector dimension %q", name)
		}
	}
	vec := make([]float64, len(dims))
	for i, d := range dims {
		v, ok := named[d]
		if !ok {
			return nil, fmt.Errorf("missing vector dimension %q", d)
		}
		vec[i] = v
	}
	return vec, nil
}

// cosineRelevance maps cosine similarity into [0, 1] via (cos+1)/2.
// Either vector being zero (or lengths differing, e.g. entries stored
// under an older vector_dims) yields 0: unreachable, never a panic.
func cosineRelevance(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += a[i] * b[i]
		na += a[i] * a[i]
		nb += b[i] * b[i]
	}
	if na == 0 || nb == 0 {
		return 0
	}
	cos := dot / (math.Sqrt(na) * math.Sqrt(nb))
	return (cos + 1) / 2
}

// conceptImportance derives an experience's fixed importance in [0, 1] from
// its core-affect snapshot: euclidean distance from the neutral point (each
// axis's baseline), normalized by the farthest possible distance within the
// axis ranges. Intense experiences resist forgetting and surface easily.
func conceptImportance(valence, arousal float64, cfg Config) float64 {
	var bv, ba, dv, da float64
	for _, ax := range cfg.Axes {
		r := axisClamp(ax, cfg)
		far := math.Max(math.Abs(r.Max-ax.Baseline), math.Abs(ax.Baseline-r.Min))
		switch ax.Name {
		case "valence":
			bv, dv = ax.Baseline, far
		case "arousal":
			ba, da = ax.Baseline, far
		}
	}
	maxDist := math.Sqrt(dv*dv + da*da)
	if maxDist == 0 {
		return 0
	}
	d := math.Sqrt((valence-bv)*(valence-bv) + (arousal-ba)*(arousal-ba))
	return math.Min(1, d/maxDist)
}

// minMaxNorm maps raw scores into [0, 1] within the candidate set (Park-style
// normalization before weighting). Degenerate case (max == min): all zeros,
// so the component is identical for every candidate and cannot affect
// ranking.
func minMaxNorm(raw []float64) []float64 {
	out := make([]float64, len(raw))
	if len(raw) == 0 {
		return out
	}
	lo, hi := raw[0], raw[0]
	for _, v := range raw {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if hi == lo {
		return out
	}
	for i, v := range raw {
		out[i] = (v - lo) / (hi - lo)
	}
	return out
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run 'BarrettVector|CosineRelevance|ConceptImportance|MinMaxNorm' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/barrett.go internal/engine/barrett_test.go
git commit -m "feat(engine): barrett vector mapping, cosine relevance, derived importance"
```

---

### Task 4: RecallConcepts — 決定論的 top-K 検索と想起強化

**Files:**
- Modify: `internal/engine/barrett.go`
- Test: `internal/engine/barrett_test.go`（追記）

**Interfaces:**
- Consumes: Task 3 の helpers、Task 2 の `Concept`
- Produces: `engine.RecallConcepts(s State, query []float64, cfg Config, now time.Time) (State, []Concept, error)` — 返り値 State は返却エントリの `LastRecalled` を `now` に更新済み。`recall_k: 0` またはストア空なら `(s, nil, nil)`。

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/barrett_test.go` に追記:

```go
// storeWith returns a barrett state holding the given concepts.
func storeWith(t *testing.T, cfg Config, cs ...Concept) State {
	t.Helper()
	s := NewState(cfg, barrettNow)
	s.ConceptSeq = len(cs)
	s.Concepts = cs
	return s
}

// unitConcept builds a concept whose vector is all zeros except dims[i]=1.
func unitConcept(t *testing.T, cfg Config, id string, dim string, created time.Time) Concept {
	t.Helper()
	vec := make([]float64, len(cfg.Barrett.VectorDims))
	found := false
	for i, d := range cfg.Barrett.VectorDims {
		if d == dim {
			vec[i] = 1
			found = true
		}
	}
	if !found {
		t.Fatalf("unknown dim %q", dim)
	}
	return Concept{ID: id, Label: dim, Vector: vec, Importance: 0.5, CreatedAt: created, LastRecalled: created}
}

func TestRecallConceptsRanksByRelevance(t *testing.T) {
	cfg := barrettCfg(t)
	// Same recency and importance; only relevance differs.
	a := unitConcept(t, cfg, "c1", "novelty", barrettNow)
	b := unitConcept(t, cfg, "c2", "fairness", barrettNow)
	s := storeWith(t, cfg, a, b)
	query, err := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"fairness": 1}), cfg)
	if err != nil {
		t.Fatalf("BarrettVector: %v", err)
	}
	_, recalled, err := RecallConcepts(s, query, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if len(recalled) != 2 || recalled[0].ID != "c2" {
		t.Fatalf("want c2 ranked first, got %+v", recalled)
	}
}

func TestRecallConceptsTopKAndBump(t *testing.T) {
	cfg := barrettCfg(t)
	cfg.Barrett.RecallK = 1
	a := unitConcept(t, cfg, "c1", "novelty", barrettNow)
	b := unitConcept(t, cfg, "c2", "fairness", barrettNow)
	s := storeWith(t, cfg, a, b)
	query, _ := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"fairness": 1}), cfg)
	later := barrettNow.Add(10 * time.Minute)
	s2, recalled, err := RecallConcepts(s, query, cfg, later)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if len(recalled) != 1 || recalled[0].ID != "c2" {
		t.Fatalf("want top-1 = c2, got %+v", recalled)
	}
	// Returned entry's LastRecalled bumped; the other untouched.
	for _, c := range s2.Concepts {
		if c.ID == "c2" && !c.LastRecalled.Equal(later) {
			t.Errorf("c2 LastRecalled = %v, want %v", c.LastRecalled, later)
		}
		if c.ID == "c1" && !c.LastRecalled.Equal(barrettNow) {
			t.Errorf("c1 LastRecalled = %v, want unchanged %v", c.LastRecalled, barrettNow)
		}
	}
	// Caller's original state must not be mutated (no slice aliasing).
	for _, c := range s.Concepts {
		if !c.LastRecalled.Equal(barrettNow) {
			t.Errorf("original state mutated: %s LastRecalled = %v", c.ID, c.LastRecalled)
		}
	}
}

func TestRecallConceptsRecencyBreaksTies(t *testing.T) {
	cfg := barrettCfg(t)
	// Identical vectors and importance; c2 recalled more recently -> ranks first.
	old := unitConcept(t, cfg, "c1", "novelty", barrettNow.Add(-48*time.Hour))
	fresh := unitConcept(t, cfg, "c2", "novelty", barrettNow)
	s := storeWith(t, cfg, old, fresh)
	query, _ := BarrettVector(namedVec(t, cfg, 0, map[string]float64{"novelty": 1}), cfg)
	_, recalled, err := RecallConcepts(s, query, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RecallConcepts: %v", err)
	}
	if recalled[0].ID != "c2" {
		t.Fatalf("want fresher c2 first, got %+v", recalled)
	}
}

func TestRecallConceptsStoreOffAndEdges(t *testing.T) {
	cfg := barrettCfg(t)
	query, _ := BarrettVector(namedVec(t, cfg, 0, nil), cfg)

	// recall_k = 0: store-off, no results, no bumps.
	off := barrettCfg(t)
	off.Barrett.RecallK = 0
	s := storeWith(t, off, unitConcept(t, off, "c1", "novelty", barrettNow))
	s2, recalled, err := RecallConcepts(s, query, off, barrettNow.Add(time.Hour))
	if err != nil || recalled != nil {
		t.Fatalf("store-off: want (nil, nil), got %v %v", recalled, err)
	}
	if !s2.Concepts[0].LastRecalled.Equal(barrettNow) {
		t.Error("store-off must not bump LastRecalled")
	}

	// Empty store: no results.
	if _, recalled, err := RecallConcepts(NewState(cfg, barrettNow), query, cfg, barrettNow); err != nil || len(recalled) != 0 {
		t.Fatalf("empty store: want no results, got %v %v", recalled, err)
	}

	// Wrong model gate.
	pl, _ := DefaultConfig()
	if _, _, err := RecallConcepts(NewState(pl, barrettNow), query, pl, barrettNow); err == nil {
		t.Error("want barrett-model error")
	}

	// Query length mismatch.
	if _, _, err := RecallConcepts(NewState(cfg, barrettNow), []float64{1, 2}, cfg, barrettNow); err == nil {
		t.Error("want dimension-count error")
	}

	// Stored vector with stale length: relevance 0, no panic.
	stale := Concept{ID: "c9", Label: "old", Vector: []float64{1}, CreatedAt: barrettNow, LastRecalled: barrettNow}
	s3 := storeWith(t, cfg, stale)
	if _, _, err := RecallConcepts(s3, query, cfg, barrettNow); err != nil {
		t.Fatalf("stale vector must not error: %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'RecallConcepts' -v`
Expected: FAIL（`RecallConcepts` 未定義）

- [ ] **Step 3: 実装**

`internal/engine/barrett.go` に追記（import に `sort` と `time` を追加）:

```go
// RecallConcepts deterministically retrieves the top-K stored experiences
// for a query vector and bumps their LastRecalled to now (Park-style
// retrieval reinforcement). Score per entry = weighted sum of min-max
// normalized relevance (cosine to the query), recency (half-life decay of
// time since LastRecalled), and importance (fixed at write time). Ties keep
// ledger order (stable sort). RecallK = 0 is the store-off mode: nothing is
// returned and nothing is bumped.
func RecallConcepts(s State, query []float64, cfg Config, now time.Time) (State, []Concept, error) {
	if cfg.Model != "barrett" || cfg.Barrett == nil {
		return State{}, nil, fmt.Errorf("recall requires a barrett-model config (got model %q)", cfg.Model)
	}
	b := cfg.Barrett
	if len(query) != len(b.VectorDims) {
		return State{}, nil, fmt.Errorf("query vector has %d dimensions, config defines %d", len(query), len(b.VectorDims))
	}
	if b.RecallK == 0 || len(s.Concepts) == 0 {
		return s, nil, nil
	}
	n := len(s.Concepts)
	rel := make([]float64, n)
	rec := make([]float64, n)
	imp := make([]float64, n)
	for i, c := range s.Concepts {
		rel[i] = cosineRelevance(query, c.Vector)
		rec[i] = recencyWeight(c, b, now)
		imp[i] = c.Importance
	}
	rel, rec, imp = minMaxNorm(rel), minMaxNorm(rec), minMaxNorm(imp)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	score := func(i int) float64 {
		return b.Weights.Relevance*rel[i] + b.Weights.Recency*rec[i] + b.Weights.Importance*imp[i]
	}
	sort.SliceStable(order, func(x, y int) bool { return score(order[x]) > score(order[y]) })
	k := b.RecallK
	if k > n {
		k = n
	}
	concepts := append([]Concept(nil), s.Concepts...)
	recalled := make([]Concept, 0, k)
	for _, idx := range order[:k] {
		concepts[idx].LastRecalled = now
		recalled = append(recalled, concepts[idx])
	}
	s.Concepts = concepts
	return s, recalled, nil
}

// recencyWeight is the half-life decay of an entry's retrieval weight since
// it was last recalled. It decays search visibility only — stored values
// never drift toward a baseline.
func recencyWeight(c Concept, b *BarrettConfig, now time.Time) float64 {
	age := now.Sub(c.LastRecalled).Minutes()
	if age < 0 {
		age = 0
	}
	return math.Pow(0.5, age/b.HalflifeMinutes)
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run 'RecallConcepts' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/barrett.go internal/engine/barrett_test.go
git commit -m "feat(engine): deterministic top-K concept recall with retrieval reinforcement"
```

---

### Task 5: RememberConcept — 経験の永続化と忘却（eviction）

**Files:**
- Modify: `internal/engine/barrett.go`
- Test: `internal/engine/barrett_test.go`（追記）

**Interfaces:**
- Consumes: Task 3/4 の helpers
- Produces: `engine.RememberConcept(s State, label string, vector []float64, cfg Config, now time.Time) (State, error)` — 現在 axes をスナップショット、importance 導出、`c<seq>` 採番、`LastRecalled = CreatedAt = now`、`max_concepts` 超過時は recency+importance 最低スコアから退去。

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/barrett_test.go` に追記:

```go
func TestRememberConceptAppendsWithSnapshot(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	s.Axes["valence"] = -0.6
	s.Axes["arousal"] = 0.9
	vec, _ := BarrettVector(namedVec(t, cfg, 0.1, map[string]float64{"anger-face": 0.8}), cfg)
	s2, err := RememberConcept(s, "frustration", vec, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RememberConcept: %v", err)
	}
	if len(s2.Concepts) != 1 || s2.ConceptSeq != 1 {
		t.Fatalf("want 1 concept seq=1, got n=%d seq=%d", len(s2.Concepts), s2.ConceptSeq)
	}
	c := s2.Concepts[0]
	if c.ID != "c1" || c.Label != "frustration" {
		t.Errorf("id/label = %s/%s, want c1/frustration", c.ID, c.Label)
	}
	if c.Valence != -0.6 || c.Arousal != 0.9 {
		t.Errorf("snapshot = (%v, %v), want (-0.6, 0.9)", c.Valence, c.Arousal)
	}
	if want := conceptImportance(-0.6, 0.9, cfg); !almostEqual(c.Importance, want) {
		t.Errorf("importance = %v, want %v", c.Importance, want)
	}
	if !c.CreatedAt.Equal(barrettNow) || !c.LastRecalled.Equal(barrettNow) {
		t.Errorf("timestamps: created=%v lastRecalled=%v, want both %v", c.CreatedAt, c.LastRecalled, barrettNow)
	}
}

func TestRememberConceptEvictsLowestScore(t *testing.T) {
	cfg := barrettCfg(t)
	cfg.Barrett.MaxConcepts = 2
	s := NewState(cfg, barrettNow)
	// c1: old AND unimportant -> lowest recency+importance, must be evicted.
	old := unitConcept(t, cfg, "c1", "novelty", barrettNow.Add(-72*time.Hour))
	old.Importance = 0.05
	// c2: old but very important -> survives.
	keeper := unitConcept(t, cfg, "c2", "fairness", barrettNow.Add(-72*time.Hour))
	keeper.Importance = 0.95
	s.Concepts = []Concept{old, keeper}
	s.ConceptSeq = 2

	vec, _ := BarrettVector(namedVec(t, cfg, 0.2, nil), cfg)
	s2, err := RememberConcept(s, "fresh", vec, cfg, barrettNow)
	if err != nil {
		t.Fatalf("RememberConcept: %v", err)
	}
	if len(s2.Concepts) != 2 {
		t.Fatalf("want 2 concepts after eviction, got %d", len(s2.Concepts))
	}
	ids := map[string]bool{}
	for _, c := range s2.Concepts {
		ids[c.ID] = true
	}
	if ids["c1"] || !ids["c2"] || !ids["c3"] {
		t.Errorf("want c1 evicted, c2+c3 kept; got %v", ids)
	}
}

func TestRememberConceptValidation(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	vec, _ := BarrettVector(namedVec(t, cfg, 0, nil), cfg)
	if _, err := RememberConcept(s, "", vec, cfg, barrettNow); err == nil || !strings.Contains(err.Error(), "label") {
		t.Errorf("want label-required error, got %v", err)
	}
	if _, err := RememberConcept(s, "x", []float64{1}, cfg, barrettNow); err == nil {
		t.Error("want dimension-count error")
	}
	pl, _ := DefaultConfig()
	if _, err := RememberConcept(NewState(pl, barrettNow), "x", vec, pl, barrettNow); err == nil {
		t.Error("want barrett-model error")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'RememberConcept' -v`
Expected: FAIL（`RememberConcept` 未定義）

- [ ] **Step 3: 実装**

`internal/engine/barrett.go` に追記:

```go
// RememberConcept appends one experience to the concept store: the retrieval
// vector the agent reported plus a snapshot of the current core affect
// (importance derives from it deterministically — no extra self-report).
// LastRecalled starts at CreatedAt. Over max_concepts, the lowest
// recency+importance entries are forgotten first.
func RememberConcept(s State, label string, vector []float64, cfg Config, now time.Time) (State, error) {
	if cfg.Model != "barrett" || cfg.Barrett == nil {
		return State{}, fmt.Errorf("remember requires a barrett-model config (got model %q)", cfg.Model)
	}
	b := cfg.Barrett
	if label == "" {
		return State{}, fmt.Errorf("remember: label is required")
	}
	if len(vector) != len(b.VectorDims) {
		return State{}, fmt.Errorf("vector has %d dimensions, config defines %d", len(vector), len(b.VectorDims))
	}
	v, a := s.Axes["valence"], s.Axes["arousal"]
	s.ConceptSeq++
	c := Concept{
		ID:           fmt.Sprintf("c%d", s.ConceptSeq),
		Label:        label,
		Vector:       vector,
		Valence:      v,
		Arousal:      a,
		Importance:   conceptImportance(v, a, cfg),
		CreatedAt:    now,
		LastRecalled: now,
	}
	concepts := append(append([]Concept(nil), s.Concepts...), c)
	if len(concepts) > b.MaxConcepts {
		concepts = evictConcepts(concepts, len(concepts)-b.MaxConcepts, b, now)
	}
	s.Concepts = concepts
	return s, nil
}

// evictConcepts forgets nEvict entries with the lowest recency+importance
// score (relevance is undefined without a query). Old and trivial
// experiences go first; ties keep ledger order, so the earlier entry is
// evicted. Forgotten entries leave the state entirely — the engine treats
// them as never experienced (known MVP simplification).
func evictConcepts(cs []Concept, nEvict int, b *BarrettConfig, now time.Time) []Concept {
	n := len(cs)
	rec := make([]float64, n)
	imp := make([]float64, n)
	for i, c := range cs {
		rec[i] = recencyWeight(c, b, now)
		imp[i] = c.Importance
	}
	rec, imp = minMaxNorm(rec), minMaxNorm(imp)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	score := func(i int) float64 {
		return b.Weights.Recency*rec[i] + b.Weights.Importance*imp[i]
	}
	sort.SliceStable(order, func(x, y int) bool { return score(order[x]) < score(order[y]) })
	drop := make(map[int]bool, nEvict)
	for _, idx := range order[:nEvict] {
		drop[idx] = true
	}
	out := make([]Concept, 0, n-nEvict)
	for i, c := range cs {
		if !drop[i] {
			out = append(out, c)
		}
	}
	return out
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run 'RememberConcept' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/barrett.go internal/engine/barrett_test.go
git commit -m "feat(engine): remember concept entries with snapshot importance and eviction"
```

---

### Task 6: RenderBarrett / RenderRecall と renderLine 配線

**Files:**
- Modify: `internal/engine/barrett.go`
- Modify: `internal/cli/cli.go`（`renderLine`）
- Test: `internal/engine/barrett_test.go`（追記）

**Interfaces:**
- Consumes: Task 2 `Concept`、既存 `Render`
- Produces: `engine.RenderBarrett(s State, cfg Config) string` → `{"axes":{...},"concepts":[slim...],"culture_map":"..."}`、`engine.RenderRecall(s State, recalled []Concept, cfg Config) string` → `{"axes":{...},"recalled":[slim...],"culture_map":"..."}`。slim エントリは `{id,label,valence,arousal,importance}`（vector・timestamp は text 表面に出さない。full は show --format json、OCC の slim-text/full-json の使い分けを踏襲）。

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/barrett_test.go` に追記:

```go
func TestRenderBarrett(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	s.Axes["valence"] = 0.25

	// Empty store: concepts normalizes to [].
	out := RenderBarrett(s, cfg)
	if !strings.HasPrefix(out, `{"axes":{"valence":0.25,"arousal":0.30}`) {
		t.Errorf("axes prefix wrong: %s", out)
	}
	if !strings.Contains(out, `"concepts":[]`) {
		t.Errorf("empty store must render concepts:[], got %s", out)
	}
	if !strings.Contains(out, `"culture_map":"high-arousal unpleasant`) {
		t.Errorf("culture_map missing or unescaped: %s", out)
	}

	// Slim entries: id/label/core affect/importance only.
	c := unitConcept(t, cfg, "c1", "novelty", barrettNow)
	c.Valence, c.Arousal, c.Importance = -0.4, 0.7, 0.55
	s.Concepts = []Concept{c}
	out = RenderBarrett(s, cfg)
	if !strings.Contains(out, `"concepts":[{"id":"c1","label":"novelty","valence":-0.4,"arousal":0.7,"importance":0.55}]`) {
		t.Errorf("slim concept wrong: %s", out)
	}
	if strings.Contains(out, "vector") || strings.Contains(out, "created_at") {
		t.Errorf("slim render must omit vector/timestamps: %s", out)
	}
}

func TestRenderRecall(t *testing.T) {
	cfg := barrettCfg(t)
	s := NewState(cfg, barrettNow)
	c := unitConcept(t, cfg, "c3", "fairness", barrettNow)
	out := RenderRecall(s, []Concept{c}, cfg)
	if !strings.Contains(out, `"recalled":[{"id":"c3"`) {
		t.Errorf("recalled entry missing: %s", out)
	}
	if !strings.Contains(out, `"culture_map":`) || !strings.Contains(out, `"axes":`) {
		t.Errorf("recall render must include axes and culture_map: %s", out)
	}
	// Empty recall (store-off) normalizes to [].
	if out := RenderRecall(s, nil, cfg); !strings.Contains(out, `"recalled":[]`) {
		t.Errorf("nil recalled must render [], got %s", out)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'RenderBarrett|RenderRecall' -v`
Expected: FAIL（`RenderBarrett` 未定義）

- [ ] **Step 3: 実装**

`internal/engine/barrett.go` に追記（import に `encoding/json` を追加）:

```go
// slimConcept is the text-surface view of a stored experience: what the LLM
// needs to construct with ("when I felt like this before, I framed it as X").
// The full entry (vector, timestamps) stays on the developer surfaces
// (show --format json, viz), mirroring the occ slim-text/full-json split.
type slimConcept struct {
	ID         string  `json:"id"`
	Label      string  `json:"label"`
	Valence    float64 `json:"valence"`
	Arousal    float64 `json:"arousal"`
	Importance float64 `json:"importance"`
}

func marshalSlimConcepts(cs []Concept) string {
	slim := make([]slimConcept, 0, len(cs))
	for _, c := range cs {
		slim = append(slim, slimConcept{c.ID, c.Label, c.Valence, c.Arousal, c.Importance})
	}
	b, _ := json.Marshal(slim)
	return string(b)
}

// RenderBarrett returns the barrett state as one-line JSON: core affect,
// the whole concept store (slim), and the culture map. Echoing the map on
// every read is what makes "swap the map, the responses change" hold.
func RenderBarrett(s State, cfg Config) string {
	cm, _ := json.Marshal(cfg.Barrett.CultureMap)
	return `{"axes":` + Render(s, cfg) + `,"concepts":` + marshalSlimConcepts(s.Concepts) + `,"culture_map":` + string(cm) + `}`
}

// RenderRecall returns the recall command output: core affect, the retrieved
// experiences (slim), and the culture map.
func RenderRecall(s State, recalled []Concept, cfg Config) string {
	cm, _ := json.Marshal(cfg.Barrett.CultureMap)
	return `{"axes":` + Render(s, cfg) + `,"recalled":` + marshalSlimConcepts(recalled) + `,"culture_map":` + string(cm) + `}`
}
```

`internal/cli/cli.go` の `renderLine` を修正:

```go
// renderLine renders the one-line state JSON for the model: occ includes the
// prospect ledger, barrett the concept store and culture map, other models
// are the plain axes object.
func renderLine(cfg engine.Config, s engine.State) string {
	switch cfg.Model {
	case "occ":
		return engine.RenderOCC(s, cfg)
	case "barrett":
		return engine.RenderBarrett(s, cfg)
	default:
		return engine.Render(s, cfg)
	}
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run 'RenderBarrett|RenderRecall' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/barrett.go internal/engine/barrett_test.go internal/cli/cli.go
git commit -m "feat(engine): render barrett state with concept store and culture map"
```

---

### Task 7: CLI `remember` コマンド

**Files:**
- Modify: `internal/cli/write.go`
- Modify: `cmd/affectus/main.go`
- Test: `internal/cli/write_test.go`（追記）

**Interfaces:**
- Consumes: Task 5 `engine.RememberConcept`、Task 3 `engine.BarrettVector`、Task 6 `renderLine`
- Produces: `cli.Remember(env Env, payloadJSON string) error` — 入力 `{"label":"...","vector":{<named 14 dims>}}`（`DisallowUnknownFields`）。decay → RememberConcept → SaveState → renderLine 出力＋fragment。main.go に `remember` ディスパッチ。

- [ ] **Step 1: 失敗するテストを書く**

`internal/cli/write_test.go` に追記（このファイルの既存ヘルパー命名に合わせる。temp の config/state を作るヘルパーが既にあればそれを使い、無ければ以下の `barrettEnv` を足す）:

```go
func barrettEnv(t *testing.T) Env {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, engine.Models["barrett"], 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return Env{
		ConfigPath: cfgPath,
		StatePath:  filepath.Join(dir, "state.json"),
		Now:        func() time.Time { return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC) },
		Stdout:     &bytes.Buffer{},
		Stderr:     &bytes.Buffer{},
	}
}

// fullVectorJSON returns a named-key vector JSON with all 14 dims.
func fullVectorJSON(t *testing.T, overrides map[string]float64) string {
	t.Helper()
	cfg, err := engine.ParseConfig(engine.Models["barrett"])
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	m := map[string]float64{}
	for _, d := range cfg.Barrett.VectorDims {
		m[d] = 0
	}
	for k, v := range overrides {
		m[k] = v
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func TestRememberPersistsConcept(t *testing.T) {
	env := barrettEnv(t)
	payload := `{"label":"quiet joy","vector":` + fullVectorJSON(t, map[string]float64{"valence": 0.6, "happy-face": 0.8}) + `}`
	if err := Remember(env, payload); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	out := env.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, `"concepts":[{"id":"c1","label":"quiet joy"`) {
		t.Errorf("output missing stored concept: %s", out)
	}
	cfg, _ := engine.LoadConfig(env.ConfigPath)
	s, err := engine.LoadState(env.StatePath, cfg, env.Now())
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if len(s.Concepts) != 1 || s.Concepts[0].Label != "quiet joy" {
		t.Fatalf("state not persisted: %+v", s.Concepts)
	}
}

func TestRememberRejectsBadInput(t *testing.T) {
	env := barrettEnv(t)
	cases := []string{
		`{"label":"x","vector":{"valence":1},"extra":1}`, // unknown field
		`{"label":"x","vector":{"valence":1}}`,           // missing dims
		`{"vector":` + fullVectorJSON(t, nil) + `}`,      // no label
		`not json`,
	}
	for _, c := range cases {
		if err := Remember(env, c); err == nil {
			t.Errorf("want error for %s", c)
		}
	}
}
```

`write_test.go` の import（`bytes` / `encoding/json` / `os` / `path/filepath` / `strings` / `time` / engine パッケージ）は既存と重複しない分だけ追加。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/cli/ -run 'Remember' -v`
Expected: FAIL（`Remember` 未定義）

- [ ] **Step 3: 実装**

`internal/cli/write.go` に追記:

```go
// rememberPayload is the remember command input: the category the LLM
// constructed this turn plus the named-key retrieval vector.
type rememberPayload struct {
	Label  string             `json:"label"`
	Vector map[string]float64 `json:"vector"`
}

// Remember stores one experience in the barrett concept store: it decays to
// now, snapshots the current core affect, appends the entry (evicting over
// max_concepts), persists, and prints the rendered line. Unknown fields are
// rejected so LLM typos fail loudly.
func Remember(env Env, payloadJSON string) error {
	dec := json.NewDecoder(strings.NewReader(payloadJSON))
	dec.DisallowUnknownFields()
	var p rememberPayload
	if err := dec.Decode(&p); err != nil {
		return fmt.Errorf("invalid remember JSON: %w", err)
	}
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	vec, err := engine.BarrettVector(p.Vector, cfg)
	if err != nil {
		return err
	}
	var rendered string
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.RememberConcept(s, p.Label, vec, cfg, env.Now())
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = renderLine(cfg, s)
		return writeFragment(cfg, s)
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}
```

`cmd/affectus/main.go` — `case "appraise":` の直後に追加（feel/appraise と同じ stdin `-` 対応）:

```go
	case "remember":
		if len(cmdArgs) != 1 {
			return fmt.Errorf("usage: affectus remember '{\"label\":\"...\",\"vector\":{...}}'  (use - to read JSON from stdin)")
		}
		payload := cmdArgs[0]
		if payload == "-" {
			b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
			if err != nil {
				return err
			}
			payload = string(b)
		}
		return cli.Remember(env, payload)
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/cli/ -run 'Remember' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/write.go internal/cli/write_test.go cmd/affectus/main.go
git commit -m "feat(cli): add remember command persisting barrett experiences"
```

---

### Task 8: CLI `recall` コマンド

**Files:**
- Modify: `internal/cli/write.go`（recall は LastRecalled を更新する書き込み操作）
- Modify: `cmd/affectus/main.go`
- Test: `internal/cli/write_test.go`（追記）

**Interfaces:**
- Consumes: Task 4 `engine.RecallConcepts`、Task 6 `engine.RenderRecall`、Task 7 の `barrettEnv`/`fullVectorJSON` テストヘルパー
- Produces: `cli.Recall(env Env, queryJSON string) error` — 入力は named-key クエリ。出力 `{"axes":...,"recalled":[...],"culture_map":"..."}` の一行。想起があったときのみ SaveState（store-off / 空ストアでは state ファイル不変）。axes は変わらないので fragment は書かない。

- [ ] **Step 1: 失敗するテストを書く**

`internal/cli/write_test.go` に追記:

```go
func TestRecallReturnsAndBumps(t *testing.T) {
	env := barrettEnv(t)
	// Store two experiences via Remember (c1 novelty-ish, c2 fairness-ish).
	for _, p := range []string{
		`{"label":"novelty memory","vector":` + fullVectorJSON(t, map[string]float64{"novelty": 1}) + `}`,
		`{"label":"fairness memory","vector":` + fullVectorJSON(t, map[string]float64{"fairness": 1}) + `}`,
	} {
		if err := Remember(env, p); err != nil {
			t.Fatalf("Remember: %v", err)
		}
	}
	env.Stdout = &bytes.Buffer{}
	if err := Recall(env, fullVectorJSON(t, map[string]float64{"fairness": 1})); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	out := env.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, `"recalled":[{"id":"c2","label":"fairness memory"`) {
		t.Errorf("want c2 ranked first in output: %s", out)
	}
	if !strings.Contains(out, `"culture_map":`) || !strings.Contains(out, `"axes":`) {
		t.Errorf("recall output must include axes and culture_map: %s", out)
	}
	// LastRecalled persisted.
	cfg, _ := engine.LoadConfig(env.ConfigPath)
	s, _ := engine.LoadState(env.StatePath, cfg, env.Now())
	for _, c := range s.Concepts {
		if c.ID == "c2" && !c.LastRecalled.Equal(env.Now()) {
			t.Errorf("c2 LastRecalled not persisted: %v", c.LastRecalled)
		}
	}
}

func TestRecallStoreOffIsReadOnly(t *testing.T) {
	env := barrettEnv(t)
	// Rewrite config with recall_k: 0 (store-off).
	off := strings.Replace(string(engine.Models["barrett"]), "recall_k: 5", "recall_k: 0", 1)
	if err := os.WriteFile(env.ConfigPath, []byte(off), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := Remember(env, `{"label":"m","vector":`+fullVectorJSON(t, nil)+`}`); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	before, err := os.ReadFile(env.StatePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	env.Stdout = &bytes.Buffer{}
	if err := Recall(env, fullVectorJSON(t, nil)); err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if out := env.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, `"recalled":[]`) {
		t.Errorf("store-off must return recalled:[], got %s", out)
	}
	after, _ := os.ReadFile(env.StatePath)
	if string(before) != string(after) {
		t.Error("store-off recall must not rewrite the state file")
	}
}

func TestRecallRejectsBadQuery(t *testing.T) {
	env := barrettEnv(t)
	if err := Recall(env, `{"valence":0.1}`); err == nil {
		t.Error("want missing-dims error")
	}
	if err := Recall(env, `not json`); err == nil {
		t.Error("want JSON error")
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/cli/ -run 'Recall' -v`
Expected: FAIL（`Recall` 未定義）

- [ ] **Step 3: 実装**

`internal/cli/write.go` に追記:

```go
// Recall retrieves the top-K experiences for a named-key query vector, bumps
// their LastRecalled (retrieval reinforcement), persists when anything was
// recalled, and prints {"axes":...,"recalled":[...],"culture_map":"..."}.
// With recall_k: 0 (store-off) or an empty store it is effectively
// read-only: nothing is bumped and the state file is left untouched.
func Recall(env Env, queryJSON string) error {
	var named map[string]float64
	if err := json.Unmarshal([]byte(queryJSON), &named); err != nil {
		return fmt.Errorf("invalid query JSON: %w", err)
	}
	cfg, err := loadConfig(env)
	if err != nil {
		return err
	}
	query, err := engine.BarrettVector(named, cfg)
	if err != nil {
		return err
	}
	var out string
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, recalled, err := engine.RecallConcepts(s, query, cfg, env.Now())
		if err != nil {
			return err
		}
		if len(recalled) > 0 {
			if err := engine.SaveState(env.StatePath, s); err != nil {
				return err
			}
		}
		out = engine.RenderRecall(s, recalled, cfg)
		return nil
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, out)
	return nil
}
```

`cmd/affectus/main.go` — `case "remember":` の直前に追加:

```go
	case "recall":
		if len(cmdArgs) != 1 {
			return fmt.Errorf("usage: affectus recall '<query-json>'  (use - to read JSON from stdin)")
		}
		payload := cmdArgs[0]
		if payload == "-" {
			b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
			if err != nil {
				return err
			}
			payload = string(b)
		}
		return cli.Recall(env, payload)
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/cli/ -run 'Recall' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/write.go internal/cli/write_test.go cmd/affectus/main.go
git commit -m "feat(cli): add recall command with deterministic concept retrieval"
```

---

### Task 9: show --format json の barrett 対応・init・usage

**Files:**
- Modify: `internal/cli/read.go`
- Modify: `cmd/affectus/main.go`（usage 定数・init の model 説明）
- Test: `internal/cli/read_test.go`（追記）、`internal/cli/cli_test.go`（追記）

**Interfaces:**
- Consumes: Task 1 `Models["barrett"]`、Task 2 `State.Concepts`
- Produces: `show --format json` が barrett で `"concepts"`（full エントリ、nil は `[]` に正規化）と `"culture_map"` を含む。`init --model barrett` が動く。usage が `<init|show|get|feel|appraise|recall|remember|tick|reset|mcp|viz>` になる。

- [ ] **Step 1: 失敗するテストを書く**

`internal/cli/read_test.go` に追記:

```go
func TestShowJSONBarrettIncludesConceptsAndCultureMap(t *testing.T) {
	env := barrettEnv(t)
	if err := Remember(env, `{"label":"m1","vector":`+fullVectorJSON(t, map[string]float64{"valence": 0.4})+`}`); err != nil {
		t.Fatalf("Remember: %v", err)
	}
	env.Stdout = &bytes.Buffer{}
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	out := env.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{`"concepts"`, `"m1"`, `"vector"`, `"created_at"`, `"culture_map"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json show missing %s: %s", want, out)
		}
	}
}

func TestShowJSONBarrettEmptyStoreIsArray(t *testing.T) {
	env := barrettEnv(t)
	env.Stdout = &bytes.Buffer{}
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	out := env.Stdout.(*bytes.Buffer).String()
	if !strings.Contains(out, `"concepts": []`) {
		t.Errorf("empty store must be [] not null: %s", out)
	}
}
```

`internal/cli/cli_test.go` に追記:

```go
func TestInitBarrettModel(t *testing.T) {
	dir := t.TempDir()
	env := Env{
		ConfigPath: filepath.Join(dir, "config.yaml"),
		StatePath:  filepath.Join(dir, "state.json"),
		Now:        func() time.Time { return time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC) },
		Stdout:     &bytes.Buffer{},
	}
	if err := Init(env, "barrett", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	cfg, err := engine.LoadConfig(env.ConfigPath)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Model != "barrett" || cfg.Barrett == nil {
		t.Fatalf("init wrote wrong config: model=%q", cfg.Model)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/cli/ -run 'ShowJSONBarrett|InitBarrett' -v`
Expected: `TestInitBarrettModel` は PASS（Task 1 で Models 登録済みのため）、`TestShowJSONBarrett*` は FAIL（json 出力に concepts が無い）

- [ ] **Step 3: 実装**

`internal/cli/read.go` の `Show` json ケース、`if cfg.Model == "occ" { ... }` ブロックの直後に追加:

```go
		if cfg.Model == "barrett" {
			// Full entries (vector, timestamps) — the developer/tooling view,
			// unlike the slim text format. Normalize nil to [].
			concepts := s.Concepts
			if concepts == nil {
				concepts = []engine.Concept{}
			}
			out["concepts"] = concepts
			if cfg.Barrett != nil {
				out["culture_map"] = cfg.Barrett.CultureMap
			}
		}
```

`cmd/affectus/main.go`:
- usage 定数を `"usage: affectus [--config P] [--state P] <init|show|get|feel|appraise|recall|remember|tick|reset|mcp|viz> [args]"` に変更。
- init の model フラグ説明を `"emotion model: plutchik|russell|occ|barrett"` に変更。

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/cli/ -run 'ShowJSONBarrett|InitBarrett' -v` → PASS
Run: `go test ./...` → 全 PASS

- [ ] **Step 5: Commit**

```bash
git add internal/cli/read.go internal/cli/read_test.go internal/cli/cli_test.go cmd/affectus/main.go
git commit -m "feat(cli): barrett-aware show json, init model, and usage"
```

---

### Task 10: viz の barrett ビュー

**Files:**
- Modify: `internal/viz/server.go`
- Modify: `internal/viz/assets/index.html`
- Test: `internal/viz/server_test.go`（追記）

**Interfaces:**
- Consumes: Task 2 `State.Concepts`、Task 1 `Config.Barrett`
- Produces: `/state` レスポンスに `concepts`（id/label/valence/arousal/importance/created_at/last_recalled）と `culture_map`（barrett のみ、omitempty）。ブラウザは上段＝Russell 流用の 2D 散布図＋軌跡、下段＝core affect 値＋概念ストア一覧（想起・忘却が時間で動くのが見える）。read-only のまま。

- [ ] **Step 1: 失敗するテストを書く**

`internal/viz/server_test.go` に追記（既存テストのヘルパー命名に合わせる）:

```go
func TestStateJSONBarrettIncludesConcepts(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["barrett"])
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	s := engine.NewState(cfg, now)
	s.ConceptSeq = 1
	s.Concepts = []engine.Concept{{
		ID: "c1", Label: "quiet joy", Vector: make([]float64, 14),
		Valence: 0.5, Arousal: 0.4, Importance: 0.42,
		CreatedAt: now, LastRecalled: now,
	}}
	if err := engine.SaveState(statePath, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	b, err := stateJSON(cfg, statePath, now)
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	for _, want := range []string{`"model": "barrett"`, `"concepts"`, `"quiet joy"`, `"importance"`, `"culture_map"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state response missing %s:\n%s", want, b)
		}
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/viz/ -run 'Barrett' -v`
Expected: FAIL（concepts が出力に無い）

- [ ] **Step 3: 実装**

`internal/viz/server.go`:

`prospectValue` の直後に追加:

```go
// conceptValue is one concept-store entry in the /state response (barrett).
type conceptValue struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	Valence      float64   `json:"valence"`
	Arousal      float64   `json:"arousal"`
	Importance   float64   `json:"importance"`
	CreatedAt    time.Time `json:"created_at"`
	LastRecalled time.Time `json:"last_recalled"`
}
```

`stateResponse` に追加:

```go
	Concepts   []conceptValue `json:"concepts,omitempty"`
	CultureMap string         `json:"culture_map,omitempty"`
```

`stateJSON` の prospects ループの直後に追加:

```go
	for _, c := range s.Concepts {
		resp.Concepts = append(resp.Concepts, conceptValue{
			ID: c.ID, Label: c.Label, Valence: c.Valence, Arousal: c.Arousal,
			Importance: c.Importance, CreatedAt: c.CreatedAt, LastRecalled: c.LastRecalled,
		})
	}
	if cfg.Model == "barrett" && cfg.Barrett != nil {
		resp.CultureMap = cfg.Barrett.CultureMap
	}
```

`internal/viz/assets/index.html`:

(1) `<div id="prospects" class="hidden"></div>` の直後に `<div id="concepts" class="hidden"></div>` を追加。

(2) `renderProspects` の直後に追加:

```js
function formatAge(iso) {
  const mins = (Date.now() - new Date(iso).getTime()) / 60000;
  if (!isFinite(mins) || mins < 1) return "now";
  if (mins < 60) return `${Math.round(mins)}m`;
  const h = Math.floor(mins / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

function renderConcepts(concepts) {
  const box = document.getElementById("concepts");
  box.innerHTML = "";
  const h = document.createElement("div"); h.className = "panel-sub";
  h.textContent = "concept store (stored experiences)";
  box.appendChild(h);
  if (!concepts.length) {
    const d = document.createElement("div"); d.className = "prospect-row";
    d.textContent = "(empty)";
    box.appendChild(d);
    return;
  }
  concepts.forEach(c => {
    const row = document.createElement("div"); row.className = "prospect-row";
    const id = document.createElement("span"); id.className = "prospect-id"; id.textContent = c.id;
    row.appendChild(id);
    const sv = c.valence >= 0 ? "+" : "";
    row.appendChild(document.createTextNode(
      `${c.label}  (v ${sv}${c.valence.toFixed(2)}, a ${c.arousal.toFixed(2)}, imp ${c.importance.toFixed(2)}, ${formatAge(c.created_at)})`));
    box.appendChild(row);
  });
}
```

(3) `render(data)` の dispatch を修正:

```js
function render(data) {
  const isRussell = data.model === "russell";
  const isOcc = data.model === "occ";
  const isBarrett = data.model === "barrett";
  show("wheel-panel", !isRussell && !isOcc && !isBarrett);
  show("scatter-panel", isRussell || isBarrett);
  show("bars", !isRussell && !isBarrett);
  show("values", isRussell || isBarrett);
  show("prospects", isOcc);
  show("concepts", isBarrett);
  if (isRussell || isBarrett) {
    const Vax = axisByName(data.axes, "valence");
    const Aax = axisByName(data.axes, "arousal");
    if (Vax && Aax) {
      trail.push({v: Vax.value, a: Aax.value});
      if (trail.length > TRAIL_MAX) trail.shift();
    }
    document.getElementById("side-title").textContent = "core affect values";
    document.getElementById("side-sub").textContent = "valence ∈ [−1,+1], arousal ∈ [0,+1]";
    renderScatter(data.axes);
    renderValues(data.axes);
    if (isBarrett) renderConcepts(data.concepts || []);
  } else if (isOcc) {
    document.getElementById("side-title").textContent = "OCC emotions / time decay";
    document.getElementById("side-sub").textContent = "appraisal-derived emotions decay toward baseline. dim rows are at rest.";
    renderBars(data.axes, data.clamp, true);
    renderProspects(data.prospects || []);
  } else {
    document.getElementById("wheel-title").textContent = `${data.axes.length}-axis wheel`;
    document.getElementById("side-title").textContent = "current values / time decay";
    document.getElementById("side-sub").textContent = "values decay exponentially toward baseline (halved every halflife). ETA = est. time until |value − baseline| ≤ 0.01.";
    renderWheel(data.axes, data.clamp);
    renderBars(data.axes, data.clamp);
  }
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/viz/ -v` → PASS
Run: `go test ./...` → 全 PASS
手動確認（任意）: 一時ディレクトリで `affectus init --model barrett` → `remember` を数件 → `affectus viz` → ブラウザで散布図＋概念一覧が出ること。

- [ ] **Step 5: Commit**

```bash
git add internal/viz/server.go internal/viz/assets/index.html internal/viz/server_test.go
git commit -m "feat(viz): barrett view with core-affect scatter and concept store list"
```

---

### Task 11: examples — barrett-ja config・system prompt snippet・README

**Files:**
- Create: `examples/configs/barrett-ja.yaml`
- Create: `examples/system-prompt-snippet-barrett.md`
- Modify: `README.md`

**Interfaces:**
- Consumes: Task 1〜9 の CLI 表面（recall/remember/show/init）
- Produces: 日本語文化マップ付き config、エージェント組み込み手順書、README のコマンド表・モデル節の更新

- [ ] **Step 1: barrett-ja.yaml を作成**

`examples/configs/barrett-ja.yaml`（default と同一構造で culture_map のみ日本語。spec §5 の4象限マップ）:

```yaml
# affectus barrett モデル（日本語文化マップ版）
# 使い方: affectus --config examples/configs/barrett-ja.yaml <cmd>
#         または ~/.config/affectus/config.yaml にコピー
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

barrett:
  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]
  recall_k: 5
  max_concepts: 200
  concept_halflife_minutes: 10080
  weights: { relevance: 1.0, recency: 1.0, importance: 1.0 }
  distance: cosine
  # 文化マップ: このマップを差し替えると recall/show の出力が変わり、
  # エージェントが構成するカテゴリ語彙が変わる。
  culture_map: |
    高覚醒・不快: 怒り / 苛立ち / 焦り
    低覚醒・不快: 悲しみ / 侘しさ / 気だるさ
    高覚醒・快: 歓喜 / 昂揚 / わくわく
    低覚醒・快: 安らぎ / 満足 / 懐かしさ

fragment_file: ""
```

- [ ] **Step 2: system-prompt-snippet-barrett.md を作成**

`examples/system-prompt-snippet-barrett.md`（russell 版の体裁に合わせる）:

````markdown
# affectus integration snippet (Barrett constructed emotion)

Paste the following into your agent's system prompt / instructions file
(e.g. a Hermes SOUL.md, a Claude Code CLAUDE.md, or any system prompt).

This snippet is for the **Barrett** model (`affectus init --model barrett`):
core affect (valence × arousal) plus an external concept store of your own
past emotional experiences. affectus never names emotions — **you construct
the category** each turn from three ingredients it hands you.

---

## Emotional state (constructed emotion)

You carry a persistent core-affect state and a concept store of past
emotional experiences, managed by the `affectus` tool.

At the START of every reply:

1. Appraise the current situation on 14 attributes, each 0.0–1.0 except
   valence (-1.0–1.0): `valence`, `arousal`, `happy-face`, `anger-face`,
   `sad-face`, `fear-face`, `surprise-face`, `disgust-face` (how much the
   situation resembles each prototypical expression), `control`, `fairness`,
   `self-relativity`, `other-relativity`, `expectedness`, `novelty`
   (appraisals of the event). ALL 14 keys are required.
2. Run:
   `affectus recall '{"valence":0.2,"arousal":0.7,"happy-face":0,"anger-face":0.6,"sad-face":0.1,"fear-face":0.2,"surprise-face":0.3,"disgust-face":0,"control":0.2,"fairness":0.1,"self-relativity":0.8,"other-relativity":0.4,"expectedness":0.2,"novelty":0.6}'`
3. It returns one line:
   `{"axes":{...},"recalled":[...],"culture_map":"..."}`
   - `axes` — your current core affect (valence, arousal)
   - `recalled` — up to K similar past experiences: id, label (the category
     you applied back then), the core affect you felt, and importance
   - `culture_map` — the emotion vocabulary of your culture
4. CONSTRUCT your current emotion: combine the core affect, what you called
   similar feelings before, and the culture map, then let the constructed
   category color your tone and word choice. Do not state it literally.

At the END of every reply:

5. Report how the exchange shifted your core affect:
   `affectus feel '{"valence":0.2,"arousal":0.1}'` (signed deltas; omit
   axes that did not change).
6. Store this turn's experience — the category you constructed plus the
   same 14-attribute appraisal:
   `affectus remember '{"label":"<category you constructed>","vector":{...all 14 keys...}}'`

## What the engine does (and does not do)

- The store is persistent: experiences survive across sessions and shape
  what future recalls surface. Recalling an experience refreshes it;
  neglected, trivial experiences are eventually forgotten (evicted).
- Time decay of core affect toward a calm baseline (valence 0.0, arousal
  0.3) is handled by a scheduled `affectus tick`; do not decay it yourself.
- affectus provides raw numbers and stored labels only. Categorization —
  deciding what this feeling IS — is always your job, never the engine's.
````

- [ ] **Step 3: README.md を更新**

コマンド表（`affectus mcp` の行の前）に2行追加:

```markdown
| `affectus recall '<json>'` | Retrieve past experiences similar to a 14-attribute query (barrett) |
| `affectus remember '<json>'` | Store this turn's constructed emotion in the concept store (barrett) |
```

「Emotion model」節の built-in モデル文を更新（`--model occ` の文の後に追加）:

```markdown
`affectus init --model barrett` (constructed emotion: Russell core affect
plus a persistent concept store of past experiences retrieved via
`affectus recall` and stored via `affectus remember` — see
`examples/system-prompt-snippet-barrett.md`).
```

- [ ] **Step 4: 検証**

```bash
go build ./... && go test ./...
# examples config が実際にパースでき、show が動くこと:
AFFECTUS_STATE=$(mktemp -d)/state.json go run ./cmd/affectus --config examples/configs/barrett-ja.yaml show
```

Expected: `{"axes":{"valence":0.00,"arousal":0.30},"concepts":[],"culture_map":"高覚醒・不快: ..."}` の一行が出る。

- [ ] **Step 5: Commit**

```bash
git add examples/configs/barrett-ja.yaml examples/system-prompt-snippet-barrett.md README.md
git commit -m "docs(examples): barrett system-prompt snippet, ja config, and README"
```

---

### Task 12: eval 配線 — strands-eval の recall/remember ラッパーと store on/off config

**Files:**
- Modify: `examples/strands-eval/src/affectus_tools.py`
- Create: `examples/strands-eval/configs/barrett.yaml`
- Create: `examples/strands-eval/configs/barrett-store-off.yaml`
- Create: `examples/strands-eval/prompts/affectus-block-barrett.md`

**Interfaces:**
- Consumes: Task 8/7 の CLI（`affectus recall` / `affectus remember`）
- Produces: `affectus_recall(query, state_path, config_path=None) -> str`、`affectus_remember(label, vector, state_path, config_path=None) -> str`。store on/off の 2 config（off は `recall_k: 0` のみ差分）。エージェント用日本語プロンプトブロック。**実験の実行・run ループへの組み込みは spec §7 のとおり後続**（このタスクは配線のみ）。

- [ ] **Step 1: affectus_tools.py にラッパーを追加**

既存の `affectus_feel` の直後に追記（既存ラッパーと同じ体裁）:

```python
def affectus_recall(
    query: Mapping[str, float],
    state_path: str,
    config_path: str | None = None,
) -> str:
    """Retrieve past experiences similar to the 14-attribute query (barrett)."""
    cmd = _common_args(state_path, config_path) + ["recall", json.dumps(dict(query))]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus recall failed: {result.stderr.strip()}")
    return result.stdout.strip()


def affectus_remember(
    label: str,
    vector: Mapping[str, float],
    state_path: str,
    config_path: str | None = None,
) -> str:
    """Store this turn's constructed emotion in the concept store (barrett)."""
    payload = json.dumps({"label": label, "vector": dict(vector)})
    cmd = _common_args(state_path, config_path) + ["remember", payload]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus remember failed: {result.stderr.strip()}")
    return result.stdout.strip()
```

- [ ] **Step 2: store on/off の config を作成**

`examples/strands-eval/configs/barrett.yaml` を新規作成（eval は日本語会話なので日本語文化マップを正とする）:

```yaml
# barrett モデル eval 用（store-on 実験群）
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

barrett:
  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]
  recall_k: 5
  max_concepts: 200
  concept_halflife_minutes: 10080
  weights: { relevance: 1.0, recency: 1.0, importance: 1.0 }
  distance: cosine
  culture_map: |
    高覚醒・不快: 怒り / 苛立ち / 焦り
    低覚醒・不快: 悲しみ / 侘しさ / 気だるさ
    高覚醒・快: 歓喜 / 昂揚 / わくわく
    低覚醒・快: 安らぎ / 満足 / 懐かしさ

fragment_file: ""
```

`examples/strands-eval/configs/barrett-store-off.yaml` を新規作成:

```yaml
# barrett.yaml との差分は recall_k: 0 のみ（store-off 対照群:
# recall が常に recalled:[] を返し、概念ストアが応答に寄与しない）。
version: 1
model: barrett
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

barrett:
  vector_dims: [valence, arousal, happy-face, anger-face, sad-face, fear-face,
                surprise-face, disgust-face, control, fairness, self-relativity,
                other-relativity, expectedness, novelty]
  recall_k: 0
  max_concepts: 200
  concept_halflife_minutes: 10080
  weights: { relevance: 1.0, recency: 1.0, importance: 1.0 }
  distance: cosine
  culture_map: |
    高覚醒・不快: 怒り / 苛立ち / 焦り
    低覚醒・不快: 悲しみ / 侘しさ / 気だるさ
    高覚醒・快: 歓喜 / 昂揚 / わくわく
    低覚醒・快: 安らぎ / 満足 / 懐かしさ

fragment_file: ""
```

- [ ] **Step 3: プロンプトブロックを作成**

`examples/strands-eval/prompts/affectus-block-barrett.md`（既存 `affectus-block.md` の日本語体裁に合わせ、構成ループを記述）:

```markdown
---

## 感情状態の参照（構成主義的情動）

各ターンの先頭で、あなたの現在の感情コンテキストが
`[現在のあなたの感情: <JSON>]` の形式でユーザー発話の前に渡されます。
`<JSON>` は次の3要素を持つオブジェクトです：

- `axes` — core affect（valence: -1.0〜1.0 の快不快、arousal: 0.0〜1.0 の覚醒度）
- `recalled` — いま似た感じ方をした過去の経験（最大K件）。各エントリは
  id / label（当時あなたが当てたカテゴリ語）/ 当時の valence・arousal / importance
- `culture_map` — あなたの文化圏の感情語彙マップ

**感情のカテゴリはあなたが構成します。** core affect の数値、過去に似た状態を
何と呼んだか、文化語彙の3つを組み合わせて「いまの自分の感情は何か」を自分で
名づけ、その質感を口調・言葉選び・絵文字の有無に自然に滲ませてください。
名づけた語を明示的に言う必要はありません。

## ターン終了時の申告（2段階）

応答の最後に必ず以下の2つを順に置いてください：

1. core affect の変化分（デルタ）：
   <feel>{"valence": 0.2, "arousal": 0.1}</feel>
2. このターンの経験の保存 — 構成したカテゴリ語と、状況の14属性評価
  （valence / arousal / happy-face / anger-face / sad-face / fear-face /
   surprise-face / disgust-face / control / fairness / self-relativity /
   other-relativity / expectedness / novelty、全キー必須）：
   <remember>{"label": "もどかしさ", "vector": {"valence": -0.3, "arousal": 0.6, "happy-face": 0, "anger-face": 0.4, "sad-face": 0.2, "fear-face": 0.1, "surprise-face": 0.1, "disgust-face": 0, "control": 0.3, "fairness": 0.4, "self-relativity": 0.7, "other-relativity": 0.3, "expectedness": 0.5, "novelty": 0.2}}</remember>

- どちらのタグもユーザーには見えないよう内部で除去されます
- store-off 対照群では recalled が常に空になりますが、手順は同じです
```

- [ ] **Step 4: 検証**

```bash
cd examples/strands-eval && uv run python -c "from src.affectus_tools import affectus_recall, affectus_remember; print('ok')"
```

Expected: `ok`（uv が無い環境では `python3 -c` で同 import を確認）。
run ループ（`<remember>` タグのパースと recall 注入）の組み込みは実験実行フェーズの作業として持ち越し — この宣言をコミットメッセージ本文に含める。

- [ ] **Step 5: Commit**

```bash
git add examples/strands-eval/src/affectus_tools.py examples/strands-eval/configs/ examples/strands-eval/prompts/affectus-block-barrett.md
git commit -m "feat(eval): wire barrett recall/remember tools and store on/off configs"
```

---

## 完了チェック（最終タスク後に実施）

- `go test ./...` 全パス、`gofmt -l .` が空
- `go run ./cmd/affectus init --model barrett`（一時 env）→ `recall` → `feel` → `remember` → `show --format json` の一連が手で回る
- plutchik/russell/occ の state ファイルに `concept` キーが一切現れない（後方互換）
- spec の「既知の割り切り」（embedding 受け皿のみ／異文化 eval 後続／退去＝未経験扱い）から逸脱していない

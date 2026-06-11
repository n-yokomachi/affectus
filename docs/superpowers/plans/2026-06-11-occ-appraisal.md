# OCC Appraisal Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** affectus に OCC モデル（`model: occ`）を実装する。LLM は評価（appraisal）だけを申告し、22感情への変換は決定論的な Go ルールが行う。見込み台帳（prospect ledger）はエンジンが永続化する。

**Architecture:** 既存パイプライン（Decay → ApplyDeltas → Render）の前段に `ApplyAppraisal`（評価→感情デルタ導出＋台帳更新）を1段挟む。感情の保持・減衰・clamp は既存機構を完全再利用。spec: `docs/superpowers/specs/2026-06-11-occ-appraisal-design.md`

**Tech Stack:** Go（標準ライブラリ＋gopkg.in/yaml.v3＋modelcontextprotocol/go-sdk）。テストは標準 `testing`、table-driven。コミット前に `gofmt -w` を通すこと。

**作業ディレクトリ:** リポジトリルート（`go.mod` のある場所）。テストは `go test ./internal/engine/ -run <名前> -v` のように実行する。

**重要な背景知識（コードを書く前に読むこと）:**
- `internal/engine/decay.go` と `internal/engine/apply.go` は **`State` 構造体を作り直して返す**。新フィールド（台帳）を追加したら、両方で引き継がないと黙って消える（Task 1 で対処）。
- 既存の `State`/`Config`/`ApplyDeltas`/`Render` の定義は `internal/engine/{state,config,apply,render}.go` にある。
- CLI のテストヘルパー `testEnv(t)` は `internal/cli/cli_test.go` に、engine の `almostEqual` は `internal/engine/helpers_test.go` にある。
- コミットメッセージに AI ツール名・Co-Authored-By を入れてはならない（リポジトリ規約）。

---

### Task 1: State に見込み台帳フィールドを追加し、Decay / ApplyDeltas で引き継ぐ

**Files:**
- Modify: `internal/engine/state.go`
- Modify: `internal/engine/decay.go`
- Modify: `internal/engine/apply.go`
- Test: `internal/engine/state_test.go`（追記）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/state_test.go` の末尾に追記:

```go
func TestStateProspectsRoundTrip(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	s := NewState(cfg, now)
	s.ProspectSeq = 2
	s.Prospects = []Prospect{
		{ID: "p2", Label: "pr merge", Desirability: 0.6, Likelihood: 0.7, CreatedAt: now},
	}
	if err := SaveState(path, s); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	got, err := LoadState(path, cfg, now)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if got.ProspectSeq != 2 || len(got.Prospects) != 1 || got.Prospects[0].ID != "p2" {
		t.Errorf("prospects not round-tripped: %+v", got)
	}
}

func TestStateWithoutProspectsOmitsFields(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := SaveState(path, NewState(cfg, now)); err != nil {
		t.Fatalf("SaveState: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "prospect") {
		t.Errorf("plutchik state file must not contain prospect fields: %s", data)
	}
}

func TestDecayAndApplyPreserveProspects(t *testing.T) {
	cfg, _ := DefaultConfig()
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	s := NewState(cfg, now)
	s.ProspectSeq = 1
	s.Prospects = []Prospect{{ID: "p1", Label: "x", Desirability: 0.5, Likelihood: 0.5, CreatedAt: now}}

	s = Decay(s, cfg, now.Add(10*time.Minute))
	if s.ProspectSeq != 1 || len(s.Prospects) != 1 {
		t.Fatalf("Decay dropped prospects: %+v", s)
	}
	s, err := ApplyDeltas(s, map[string]float64{"joy": 0.1}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s.ProspectSeq != 1 || len(s.Prospects) != 1 {
		t.Fatalf("ApplyDeltas dropped prospects: %+v", s)
	}
}
```

`state_test.go` の import に `os` / `strings` / `filepath` が無ければ追加する（既存 import を確認して揃える）。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run 'Prospects|PreserveProspects' -v`
Expected: COMPILE ERROR（`Prospect` 未定義）

- [ ] **Step 3: 実装する**

`internal/engine/state.go` — `State` の直前に `Prospect` 型を追加し、`State` にフィールドを足す:

```go
// Prospect is one unresolved prospect ledger entry (occ model): an uncertain
// event the agent reported hope/fear about, kept so a later session can still
// resolve it into satisfaction/disappointment/relief/fears-confirmed.
type Prospect struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	Desirability float64   `json:"desirability"`
	Likelihood   float64   `json:"likelihood"`
	CreatedAt    time.Time `json:"created_at"`
}

// State is the persisted emotion vector.
type State struct {
	Version   int                `json:"version"`
	UpdatedAt time.Time          `json:"updated_at"`
	Axes      map[string]float64 `json:"axes"`
	// ProspectSeq and Prospects are only used by the occ model. omitempty keeps
	// plutchik/russell state files byte-identical to before.
	ProspectSeq int        `json:"prospect_seq,omitempty"`
	Prospects   []Prospect `json:"prospects,omitempty"`
}
```

`internal/engine/decay.go` — 戻り値で引き継ぐ:

```go
	return State{Version: s.Version, UpdatedAt: now, Axes: axes,
		ProspectSeq: s.ProspectSeq, Prospects: s.Prospects}
```

`internal/engine/apply.go` — `ApplyDeltas` の戻り値も同様:

```go
	return State{Version: s.Version, UpdatedAt: s.UpdatedAt, Axes: axes,
		ProspectSeq: s.ProspectSeq, Prospects: s.Prospects}, nil
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS（既存テスト含め全部）

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): add prospect ledger fields to state"
```

---

### Task 2: OCC config（型・バリデーション・デフォルト yaml・レジストリ）

**Files:**
- Modify: `internal/engine/config.go`
- Create: `internal/engine/occ.go`（このタスクでは軸名リストのみ）
- Create: `internal/engine/occ.default.yaml`
- Modify: `internal/engine/defaults.go`
- Test: `internal/engine/config_test.go`（追記）、`internal/engine/defaults_test.go`（修正）

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/defaults_test.go` 43行目付近のモデル名ループに `"occ"` を追加:

```go
	for _, name := range []string{"plutchik", "russell", "occ"} {
```

`internal/engine/config_test.go` の末尾に追記:

```go
func TestOCCDefaultConfigValid(t *testing.T) {
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatalf("occ default config invalid: %v", err)
	}
	if cfg.Model != "occ" {
		t.Errorf("model = %q, want occ", cfg.Model)
	}
	if len(cfg.Axes) != 22 {
		t.Errorf("axes = %d, want 22", len(cfg.Axes))
	}
	if cfg.OCC == nil || cfg.OCC.MaxProspects != 20 {
		t.Errorf("occ section = %+v, want max_prospects 20", cfg.OCC)
	}
}

func TestValidateOCCRequiresSection(t *testing.T) {
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatal(err)
	}
	cfg.OCC = nil
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "occ section") {
		t.Fatalf("want occ-section error, got %v", err)
	}
}

func TestValidateOCCRequiresAllAxes(t *testing.T) {
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatal(err)
	}
	cfg.Axes = cfg.Axes[1:] // drop the first axis (joy)
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "requires axis") {
		t.Fatalf("want missing-axis error, got %v", err)
	}
}

func TestValidateOCCRejectsBadGainsAndCap(t *testing.T) {
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatal(err)
	}
	cfg.OCC.Gains.Wellbeing = -0.1
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "gain") {
		t.Fatalf("want negative-gain error, got %v", err)
	}
	cfg.OCC.Gains.Wellbeing = 0.8
	cfg.OCC.MaxProspects = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "max_prospects") {
		t.Fatalf("want max_prospects error, got %v", err)
	}
}
```

`config_test.go` の import に `strings` が無ければ追加。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run 'OCC|Defaults' -v`
Expected: COMPILE ERROR（`Models["occ"]` は nil、`cfg.OCC` 未定義）

- [ ] **Step 3: 実装する**

`internal/engine/occ.go` を新規作成:

```go
package engine

// OCCAxisNames is the full 22-emotion axis set of the OCC model
// (Ortony, Clore & Collins 1988), grouped by appraisal branch.
var OCCAxisNames = []string{
	// well-being (consequences of events, for self, actual)
	"joy", "distress",
	// prospect-based (for self, uncertain, then resolved)
	"hope", "fear", "satisfaction", "disappointment", "relief", "fears-confirmed",
	// fortunes-of-others (consequences for others x liking)
	"happy-for", "pity", "resentment", "gloating",
	// attribution (actions of agents x praiseworthiness)
	"pride", "shame", "admiration", "reproach",
	// attraction (aspects of objects)
	"love", "hate",
	// compounds (well-being x attribution)
	"gratification", "gratitude", "remorse", "anger",
}
```

`internal/engine/config.go` — `Config` の前に型を追加し、`Config` にフィールドを足す:

```go
// OCCGains holds per-branch conversion strengths from appraisal values to
// emotion deltas (occ model).
type OCCGains struct {
	Wellbeing   float64 `yaml:"wellbeing"`
	Prospect    float64 `yaml:"prospect"`
	Fortunes    float64 `yaml:"fortunes"`
	Attribution float64 `yaml:"attribution"`
	Attraction  float64 `yaml:"attraction"`
	Compound    float64 `yaml:"compound"`
}

// OCCConfig is the occ-model section of the config. nil for other models.
type OCCConfig struct {
	Gains        OCCGains `yaml:"gains"`
	MaxProspects int      `yaml:"max_prospects"`
}
```

`Config` 構造体に追加（`Axes` の後）:

```go
	OCC          *OCCConfig   `yaml:"occ"`
```

`Validate()` の末尾（opposite 検証ループの後、`return nil` の前）に追加:

```go
	if c.Model == "occ" {
		if c.OCC == nil {
			return fmt.Errorf("config: model occ requires an occ section")
		}
		g := c.OCC.Gains
		gains := map[string]float64{
			"wellbeing": g.Wellbeing, "prospect": g.Prospect, "fortunes": g.Fortunes,
			"attribution": g.Attribution, "attraction": g.Attraction, "compound": g.Compound,
		}
		for name, v := range gains {
			if v < 0 {
				return fmt.Errorf("config: occ gain %s must be non-negative", name)
			}
		}
		if c.OCC.MaxProspects <= 0 {
			return fmt.Errorf("config: occ max_prospects must be positive")
		}
		for _, name := range OCCAxisNames {
			if !seen[name] {
				return fmt.Errorf("config: model occ requires axis %q", name)
			}
		}
	}
```

注意: `seen` マップは `Validate` 冒頭の重複チェックで作られている既存変数。opposite 検証で使った後も生きているのでそのまま使える。

`internal/engine/occ.default.yaml` を新規作成:

```yaml
version: 1
model: occ
clamp:       { min: 0.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  # well-being
  - { name: joy,             baseline: 0.0, halflife_minutes: 90 }
  - { name: distress,        baseline: 0.0, halflife_minutes: 90 }
  # prospect-based
  - { name: hope,            baseline: 0.0, halflife_minutes: 90 }
  - { name: fear,            baseline: 0.0, halflife_minutes: 90 }
  - { name: satisfaction,    baseline: 0.0, halflife_minutes: 90 }
  - { name: disappointment,  baseline: 0.0, halflife_minutes: 90 }
  - { name: relief,          baseline: 0.0, halflife_minutes: 90 }
  - { name: fears-confirmed, baseline: 0.0, halflife_minutes: 90 }
  # fortunes-of-others
  - { name: happy-for,       baseline: 0.0, halflife_minutes: 90 }
  - { name: pity,            baseline: 0.0, halflife_minutes: 90 }
  - { name: resentment,      baseline: 0.0, halflife_minutes: 90 }
  - { name: gloating,        baseline: 0.0, halflife_minutes: 90 }
  # attribution
  - { name: pride,           baseline: 0.0, halflife_minutes: 90 }
  - { name: shame,           baseline: 0.0, halflife_minutes: 90 }
  - { name: admiration,      baseline: 0.0, halflife_minutes: 90 }
  - { name: reproach,        baseline: 0.0, halflife_minutes: 90 }
  # attraction
  - { name: love,            baseline: 0.0, halflife_minutes: 90 }
  - { name: hate,            baseline: 0.0, halflife_minutes: 90 }
  # compounds
  - { name: gratification,   baseline: 0.0, halflife_minutes: 90 }
  - { name: gratitude,       baseline: 0.0, halflife_minutes: 90 }
  - { name: remorse,         baseline: 0.0, halflife_minutes: 90 }
  - { name: anger,           baseline: 0.0, halflife_minutes: 90 }

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

`internal/engine/defaults.go` — embed とレジストリに occ を追加:

```go
//go:embed occ.default.yaml
var occYAML []byte
```

```go
var Models = map[string][]byte{
	"plutchik": plutchikYAML,
	"russell":  russellYAML,
	"occ":      occYAML,
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): add occ config section and default config"
```

---

### Task 3: Appraisal 型とバリデーション

**Files:**
- Modify: `internal/engine/occ.go`
- Create: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/occ_test.go` を新規作成:

```go
package engine

import (
	"strings"
	"testing"
)

func f64(v float64) *float64 { return &v }

func TestAppraisalValidate(t *testing.T) {
	tests := []struct {
		name    string
		a       Appraisal
		wantErr string // "" = valid
	}{
		{"empty appraisal", Appraisal{}, "at least one"},
		{"wellbeing ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5}}, ""},
		{"desirability out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 1.5}}, "desirability"},
		{"likelihood out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(1.5)}}, "likelihood"},
		{"likelihood zero", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0)}}, "likelihood"},
		{"prospect needs label", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0.5)}}, "label"},
		{"prospect ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, Likelihood: f64(0.5), Label: "x"}}, ""},
		{"for other needs liking", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other"}}, "liking"},
		{"for other ok", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4)}}, ""},
		{"liking out of range", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(2)}}, "liking"},
		{"other prospect unsupported", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "other", Liking: f64(0.4), Likelihood: f64(0.5), Label: "x"}}, "not supported"},
		{"bad for", Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5, For: "them"}}, "for must be"},
		{"action ok", Appraisal{Action: &ActionAppraisal{Praiseworthiness: -0.5, Agent: "other"}}, ""},
		{"action bad agent", Appraisal{Action: &ActionAppraisal{Praiseworthiness: 0.5, Agent: "me"}}, "agent"},
		{"praise out of range", Appraisal{Action: &ActionAppraisal{Praiseworthiness: -2, Agent: "self"}}, "praiseworthiness"},
		{"object ok", Appraisal{Object: &ObjectAppraisal{Appealingness: 0.3}}, ""},
		{"appealingness out of range", Appraisal{Object: &ObjectAppraisal{Appealingness: -2}}, "appealingness"},
		{"resolve ok", Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: "confirmed"}}}, ""},
		{"resolve missing id", Appraisal{Resolve: []Resolution{{Outcome: "confirmed"}}}, "id"},
		{"resolve bad outcome", Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: "done"}}}, "outcome"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.a.Validate()
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

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run AppraisalValidate -v`
Expected: COMPILE ERROR（型未定義）

- [ ] **Step 3: 実装する**

`internal/engine/occ.go` に追記（`OCCAxisNames` の後）:

```go
import "fmt"

// Resolution outcomes.
const (
	OutcomeConfirmed    = "confirmed"
	OutcomeDisconfirmed = "disconfirmed"
	OutcomeDropped      = "dropped"
)

// Appraisal is one self-reported cognitive appraisal (occ model). The LLM
// reports how it construes an event/action/object; emotions are derived
// deterministically by ApplyAppraisal. All sections are optional, but at
// least one must be present.
type Appraisal struct {
	Consequence *ConsequenceAppraisal `json:"consequence,omitempty"`
	Action      *ActionAppraisal      `json:"action,omitempty"`
	Object      *ObjectAppraisal      `json:"object,omitempty"`
	Resolve     []Resolution          `json:"resolve,omitempty"`
}

// ConsequenceAppraisal appraises the consequences of an event.
type ConsequenceAppraisal struct {
	// Desirability in [-1, 1]. When For is "other" it is the desirability
	// for the other party.
	Desirability float64 `json:"desirability"`
	// For is "self" (default) or "other".
	For string `json:"for,omitempty"`
	// Likelihood in (0, 1) marks an uncertain prospect; nil or 1.0 is an
	// actual (certain) event.
	Likelihood *float64 `json:"likelihood,omitempty"`
	// Label is a short description stored in the prospect ledger. Required
	// when Likelihood < 1.
	Label string `json:"label,omitempty"`
	// Liking in [-1, 1] is the momentary liking for the other party.
	// Required when For is "other".
	Liking *float64 `json:"liking,omitempty"`
}

// ActionAppraisal appraises an agent's action against standards.
type ActionAppraisal struct {
	Praiseworthiness float64 `json:"praiseworthiness"` // [-1, 1]; negative = blameworthy
	Agent            string  `json:"agent"`            // "self" | "other"
}

// ObjectAppraisal appraises an object against attitudes/tastes.
type ObjectAppraisal struct {
	Appealingness float64 `json:"appealingness"` // [-1, 1]
}

// Resolution resolves a prospect ledger entry.
type Resolution struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"` // confirmed | disconfirmed | dropped
}

// Validate checks ranges and structural requirements of the appraisal.
func (a Appraisal) Validate() error {
	if a.Consequence == nil && a.Action == nil && a.Object == nil && len(a.Resolve) == 0 {
		return fmt.Errorf("appraisal: at least one of consequence, action, object, resolve is required")
	}
	if c := a.Consequence; c != nil {
		if c.Desirability < -1 || c.Desirability > 1 {
			return fmt.Errorf("appraisal: desirability %v outside [-1, 1]", c.Desirability)
		}
		if c.Likelihood != nil {
			if l := *c.Likelihood; l <= 0 || l > 1 {
				return fmt.Errorf("appraisal: likelihood %v outside (0, 1]", l)
			}
		}
		switch c.For {
		case "", "self":
			if c.Likelihood != nil && *c.Likelihood < 1 && c.Label == "" {
				return fmt.Errorf("appraisal: label is required for a prospect (likelihood < 1)")
			}
		case "other":
			if c.Liking == nil {
				return fmt.Errorf(`appraisal: liking is required when for is "other"`)
			}
			if *c.Liking < -1 || *c.Liking > 1 {
				return fmt.Errorf("appraisal: liking %v outside [-1, 1]", *c.Liking)
			}
			if c.Likelihood != nil && *c.Likelihood < 1 {
				return fmt.Errorf("appraisal: prospects about others are not supported")
			}
		default:
			return fmt.Errorf(`appraisal: for must be "self" or "other", got %q`, c.For)
		}
	}
	if act := a.Action; act != nil {
		if act.Praiseworthiness < -1 || act.Praiseworthiness > 1 {
			return fmt.Errorf("appraisal: praiseworthiness %v outside [-1, 1]", act.Praiseworthiness)
		}
		if act.Agent != "self" && act.Agent != "other" {
			return fmt.Errorf(`appraisal: agent must be "self" or "other", got %q`, act.Agent)
		}
	}
	if o := a.Object; o != nil {
		if o.Appealingness < -1 || o.Appealingness > 1 {
			return fmt.Errorf("appraisal: appealingness %v outside [-1, 1]", o.Appealingness)
		}
	}
	for _, r := range a.Resolve {
		if r.ID == "" {
			return fmt.Errorf("appraisal: resolve entry missing id")
		}
		switch r.Outcome {
		case OutcomeConfirmed, OutcomeDisconfirmed, OutcomeDropped:
		default:
			return fmt.Errorf("appraisal: outcome must be confirmed|disconfirmed|dropped, got %q", r.Outcome)
		}
	}
	return nil
}
```

注意: import 文は既存ファイル先頭の import ブロックに統合する（`fmt` のみ）。

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run AppraisalValidate -v`
Expected: PASS（全サブテスト）

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): add appraisal types and validation"
```

---

### Task 4: ApplyAppraisal — well-being / attribution / attraction（ルール1・6・7）

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記。テストヘルパーと、確定事象系のルールのテスト:

```go
import (
	"strings"
	"testing"
	"time"
)

func occCfg(t *testing.T) Config {
	t.Helper()
	cfg, err := ParseConfig(Models["occ"])
	if err != nil {
		t.Fatalf("parse occ default config: %v", err)
	}
	return cfg
}

var occNow = time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

// applyOCC is a test shorthand: fresh state -> ApplyAppraisal.
func applyOCC(t *testing.T, a Appraisal) State {
	t.Helper()
	cfg := occCfg(t)
	s, err := ApplyAppraisal(NewState(cfg, occNow), a, cfg, occNow)
	if err != nil {
		t.Fatalf("ApplyAppraisal: %v", err)
	}
	return s
}

func TestApplyAppraisalRequiresOCCModel(t *testing.T) {
	cfg, _ := DefaultConfig() // plutchik
	_, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{Object: &ObjectAppraisal{Appealingness: 0.5}}, cfg, occNow)
	if err == nil || !strings.Contains(err.Error(), "occ") {
		t.Fatalf("want occ-model error, got %v", err)
	}
}

func TestApplyAppraisalWellbeing(t *testing.T) {
	// gains.wellbeing = 0.8
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{Desirability: 0.5}})
	if !almostEqual(s.Axes["joy"], 0.40) {
		t.Errorf("joy = %v, want 0.40", s.Axes["joy"])
	}
	s = applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{Desirability: -0.5}})
	if !almostEqual(s.Axes["distress"], 0.40) {
		t.Errorf("distress = %v, want 0.40", s.Axes["distress"])
	}
}

func TestApplyAppraisalAttribution(t *testing.T) {
	// gains.attribution = 0.8
	tests := []struct {
		praise float64
		agent  string
		axis   string
	}{
		{0.5, "self", "pride"},
		{-0.5, "self", "shame"},
		{0.5, "other", "admiration"},
		{-0.5, "other", "reproach"},
	}
	for _, tt := range tests {
		s := applyOCC(t, Appraisal{Action: &ActionAppraisal{Praiseworthiness: tt.praise, Agent: tt.agent}})
		if !almostEqual(s.Axes[tt.axis], 0.40) {
			t.Errorf("%s = %v, want 0.40", tt.axis, s.Axes[tt.axis])
		}
	}
}

func TestApplyAppraisalAttraction(t *testing.T) {
	// gains.attraction = 0.6
	s := applyOCC(t, Appraisal{Object: &ObjectAppraisal{Appealingness: 0.5}})
	if !almostEqual(s.Axes["love"], 0.30) {
		t.Errorf("love = %v, want 0.30", s.Axes["love"])
	}
	s = applyOCC(t, Appraisal{Object: &ObjectAppraisal{Appealingness: -0.5}})
	if !almostEqual(s.Axes["hate"], 0.30) {
		t.Errorf("hate = %v, want 0.30", s.Axes["hate"])
	}
}

func TestApplyAppraisalZeroValuesAreNoop(t *testing.T) {
	s := applyOCC(t, Appraisal{
		Consequence: &ConsequenceAppraisal{Desirability: 0},
		Action:      &ActionAppraisal{Praiseworthiness: 0, Agent: "self"},
		Object:      &ObjectAppraisal{Appealingness: 0},
	})
	for name, v := range s.Axes {
		if v != 0 {
			t.Errorf("axis %s = %v, want 0 (zero appraisal is a no-op)", name, v)
		}
	}
}

func TestApplyAppraisalRejectsInvalid(t *testing.T) {
	cfg := occCfg(t)
	_, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{}, cfg, occNow)
	if err == nil {
		t.Fatal("empty appraisal should error")
	}
}
```

import ブロックは既存と統合する（`time` を追加）。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run ApplyAppraisal -v`
Expected: COMPILE ERROR（`ApplyAppraisal` 未定義）

- [ ] **Step 3: 実装する**

`occ.go` の import を `fmt`, `math`, `time` に拡張し、末尾に追加:

```go
// ApplyAppraisal validates the appraisal, derives emotion deltas per the OCC
// rules, updates the prospect ledger, and applies the deltas through
// ApplyDeltas. now stamps newly created ledger entries.
func ApplyAppraisal(s State, a Appraisal, cfg Config, now time.Time) (State, error) {
	if cfg.Model != "occ" || cfg.OCC == nil {
		return State{}, fmt.Errorf("appraise requires an occ-model config (got model %q)", cfg.Model)
	}
	if err := a.Validate(); err != nil {
		return State{}, err
	}
	g := cfg.OCC.Gains
	deltas := map[string]float64{}

	// Consequences of events (well-being branch; prospect and
	// fortunes-of-others branches are added in later rules).
	if c := a.Consequence; c != nil && c.Desirability != 0 {
		des := c.Desirability
		switch {
		default:
			// well-being: actual consequence for self.
			mag := g.Wellbeing * math.Abs(des)
			if des > 0 {
				deltas["joy"] += mag
			} else {
				deltas["distress"] += mag
			}
		}
	}

	// Attribution: actions of agents against standards.
	if act := a.Action; act != nil && act.Praiseworthiness != 0 {
		mag := g.Attribution * math.Abs(act.Praiseworthiness)
		switch {
		case act.Agent == "self" && act.Praiseworthiness > 0:
			deltas["pride"] += mag
		case act.Agent == "self":
			deltas["shame"] += mag
		case act.Praiseworthiness > 0:
			deltas["admiration"] += mag
		default:
			deltas["reproach"] += mag
		}
	}

	// Attraction: aspects of objects against attitudes.
	if o := a.Object; o != nil && o.Appealingness != 0 {
		mag := g.Attraction * math.Abs(o.Appealingness)
		if o.Appealingness > 0 {
			deltas["love"] += mag
		} else {
			deltas["hate"] += mag
		}
	}

	return ApplyDeltas(s, deltas, cfg)
}
```

（consequence の `switch { default: }` は不格好だが意図的: Task 5・6 でこの switch に prospect / for-other の case が入る。）

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): derive well-being, attribution, attraction emotions"
```

---

### Task 5: ApplyAppraisal — 見込み発行と台帳上限（ルール2）

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記:

```go
func TestApplyAppraisalProspectCreatesLedgerEntry(t *testing.T) {
	// gains.prospect = 0.8; hope = 0.8 * 0.6 * 0.5 = 0.24
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{
		Desirability: 0.6, Likelihood: f64(0.5), Label: "pr merge"}})
	if !almostEqual(s.Axes["hope"], 0.24) {
		t.Errorf("hope = %v, want 0.24", s.Axes["hope"])
	}
	if len(s.Prospects) != 1 {
		t.Fatalf("prospects = %d, want 1", len(s.Prospects))
	}
	p := s.Prospects[0]
	if p.ID != "p1" || p.Label != "pr merge" || !almostEqual(p.Desirability, 0.6) || !almostEqual(p.Likelihood, 0.5) {
		t.Errorf("ledger entry = %+v", p)
	}
	if !p.CreatedAt.Equal(occNow) {
		t.Errorf("created_at = %v, want %v", p.CreatedAt, occNow)
	}
	if s.ProspectSeq != 1 {
		t.Errorf("prospect_seq = %d, want 1", s.ProspectSeq)
	}
}

func TestApplyAppraisalNegativeProspectRaisesFear(t *testing.T) {
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{
		Desirability: -0.6, Likelihood: f64(0.5), Label: "deploy may fail"}})
	if !almostEqual(s.Axes["fear"], 0.24) {
		t.Errorf("fear = %v, want 0.24", s.Axes["fear"])
	}
}

func TestApplyAppraisalLikelihoodOneIsWellbeing(t *testing.T) {
	// likelihood exactly 1.0 = certain -> joy, no ledger entry.
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{
		Desirability: 0.5, Likelihood: f64(1.0)}})
	if !almostEqual(s.Axes["joy"], 0.40) {
		t.Errorf("joy = %v, want 0.40", s.Axes["joy"])
	}
	if len(s.Prospects) != 0 {
		t.Errorf("prospects = %d, want 0", len(s.Prospects))
	}
}

func TestApplyAppraisalLedgerCapDropsOldest(t *testing.T) {
	cfg := occCfg(t)
	cfg.OCC.MaxProspects = 2
	s := NewState(cfg, occNow)
	var err error
	for i, label := range []string{"first", "second", "third"} {
		s, err = ApplyAppraisal(s, Appraisal{Consequence: &ConsequenceAppraisal{
			Desirability: 0.5, Likelihood: f64(0.5), Label: label}}, cfg, occNow)
		if err != nil {
			t.Fatalf("appraisal %d: %v", i, err)
		}
	}
	if len(s.Prospects) != 2 {
		t.Fatalf("prospects = %d, want 2 (capped)", len(s.Prospects))
	}
	if s.Prospects[0].Label != "second" || s.Prospects[1].Label != "third" {
		t.Errorf("oldest should be dropped, got %+v", s.Prospects)
	}
	if s.ProspectSeq != 3 {
		t.Errorf("prospect_seq = %d, want 3 (monotonic)", s.ProspectSeq)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run 'Prospect|LikelihoodOne|LedgerCap' -v`
Expected: FAIL（hope が上がらず joy になる、台帳が空）

- [ ] **Step 3: 実装する**

`occ.go` の `ApplyAppraisal` 内、consequence の switch に case を追加（`default:` の**前**）:

```go
		case c.Likelihood != nil && *c.Likelihood < 1:
			// prospect: uncertain consequence for self — hope/fear now,
			// ledger entry so a later session can resolve it.
			l := *c.Likelihood
			mag := g.Prospect * math.Abs(des) * l
			if des > 0 {
				deltas["hope"] += mag
			} else {
				deltas["fear"] += mag
			}
			s.ProspectSeq++
			s.Prospects = append(s.Prospects, Prospect{
				ID:           fmt.Sprintf("p%d", s.ProspectSeq),
				Label:        c.Label,
				Desirability: des,
				Likelihood:   l,
				CreatedAt:    now,
			})
			if max := cfg.OCC.MaxProspects; len(s.Prospects) > max {
				s.Prospects = append([]Prospect(nil), s.Prospects[len(s.Prospects)-max:]...)
			}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): create prospect ledger entries for uncertain consequences"
```

---

### Task 6: ApplyAppraisal — resolve（ルール3・4）

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記:

```go
// prospectState builds a state holding one ledger entry with the given desirability.
func prospectState(t *testing.T, cfg Config, des float64) State {
	t.Helper()
	s, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{Consequence: &ConsequenceAppraisal{
		Desirability: des, Likelihood: f64(0.5), Label: "x"}}, cfg, occNow)
	if err != nil {
		t.Fatalf("setup prospect: %v", err)
	}
	return s
}

func TestApplyAppraisalResolve(t *testing.T) {
	// gains.prospect = 0.8; resolution magnitude = 0.8 * |des|
	tests := []struct {
		des     float64
		outcome string
		axis    string
		want    float64
	}{
		{0.6, "confirmed", "satisfaction", 0.48},
		{0.6, "disconfirmed", "disappointment", 0.48},
		{-0.6, "confirmed", "fears-confirmed", 0.48},
		{-0.6, "disconfirmed", "relief", 0.48},
	}
	for _, tt := range tests {
		cfg := occCfg(t)
		s := prospectState(t, cfg, tt.des)
		s, err := ApplyAppraisal(s, Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: tt.outcome}}}, cfg, occNow)
		if err != nil {
			t.Fatalf("resolve: %v", err)
		}
		if !almostEqual(s.Axes[tt.axis], tt.want) {
			t.Errorf("%s(%v,%s) = %v, want %v", tt.axis, tt.des, tt.outcome, s.Axes[tt.axis], tt.want)
		}
		if len(s.Prospects) != 0 {
			t.Errorf("ledger should be empty after resolve, got %+v", s.Prospects)
		}
	}
}

func TestApplyAppraisalResolveDropped(t *testing.T) {
	cfg := occCfg(t)
	s := prospectState(t, cfg, 0.6)
	hopeBefore := s.Axes["hope"]
	s, err := ApplyAppraisal(s, Appraisal{Resolve: []Resolution{{ID: "p1", Outcome: "dropped"}}}, cfg, occNow)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(s.Prospects) != 0 {
		t.Errorf("ledger should be empty after drop")
	}
	// dropped fires no emotion: only the pre-existing hope remains.
	if !almostEqual(s.Axes["hope"], hopeBefore) || !almostEqual(s.Axes["satisfaction"], 0) {
		t.Errorf("dropped should not fire emotions: %+v", s.Axes)
	}
}

func TestApplyAppraisalResolveUnknownID(t *testing.T) {
	cfg := occCfg(t)
	_, err := ApplyAppraisal(NewState(cfg, occNow), Appraisal{Resolve: []Resolution{{ID: "p9", Outcome: "confirmed"}}}, cfg, occNow)
	if err == nil || !strings.Contains(err.Error(), "unknown prospect") {
		t.Fatalf("want unknown prospect error, got %v", err)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run Resolve -v`
Expected: FAIL（resolve が無視され台帳が残る／unknown ID がエラーにならない）

- [ ] **Step 3: 実装する**

`occ.go` の `ApplyAppraisal` 内、`deltas := map[string]float64{}` の直後（consequence ブロックの前）に追加:

```go
	// Prospect resolutions consume ledger entries first.
	for _, r := range a.Resolve {
		p, rest, ok := takeProspect(s.Prospects, r.ID)
		if !ok {
			return State{}, fmt.Errorf("unknown prospect %q", r.ID)
		}
		s.Prospects = rest
		if r.Outcome == OutcomeDropped {
			continue
		}
		mag := g.Prospect * math.Abs(p.Desirability)
		switch {
		case r.Outcome == OutcomeConfirmed && p.Desirability > 0:
			deltas["satisfaction"] += mag
		case r.Outcome == OutcomeConfirmed && p.Desirability < 0:
			deltas["fears-confirmed"] += mag
		case r.Outcome == OutcomeDisconfirmed && p.Desirability > 0:
			deltas["disappointment"] += mag
		case r.Outcome == OutcomeDisconfirmed && p.Desirability < 0:
			deltas["relief"] += mag
		}
	}
```

ファイル末尾にヘルパーを追加:

```go
// takeProspect removes the ledger entry with the given id, returning it and
// the remaining entries.
func takeProspect(ps []Prospect, id string) (Prospect, []Prospect, bool) {
	for i, p := range ps {
		if p.ID == id {
			rest := append(append([]Prospect(nil), ps[:i]...), ps[i+1:]...)
			return p, rest, true
		}
	}
	return Prospect{}, ps, false
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): resolve prospects into satisfaction family emotions"
```

---

### Task 7: ApplyAppraisal — fortunes-of-others（ルール5）

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記:

```go
func TestApplyAppraisalFortunesOfOthers(t *testing.T) {
	// gains.fortunes = 0.6; magnitude = 0.6 * |des| * |liking| = 0.6*0.5*0.4 = 0.12
	tests := []struct {
		des    float64
		liking float64
		axis   string
	}{
		{0.5, 0.4, "happy-for"},
		{-0.5, 0.4, "pity"},
		{0.5, -0.4, "resentment"},
		{-0.5, -0.4, "gloating"},
	}
	for _, tt := range tests {
		s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{
			Desirability: tt.des, For: "other", Liking: f64(tt.liking)}})
		if !almostEqual(s.Axes[tt.axis], 0.12) {
			t.Errorf("%s = %v, want 0.12", tt.axis, s.Axes[tt.axis])
		}
		// fortunes must not leak into well-being.
		if s.Axes["joy"] != 0 || s.Axes["distress"] != 0 {
			t.Errorf("for-other consequence must not raise joy/distress: %+v", s.Axes)
		}
	}
}

func TestApplyAppraisalZeroLikingIsNoop(t *testing.T) {
	s := applyOCC(t, Appraisal{Consequence: &ConsequenceAppraisal{
		Desirability: 0.5, For: "other", Liking: f64(0)}})
	for name, v := range s.Axes {
		if v != 0 {
			t.Errorf("axis %s = %v, want 0 (zero liking is a no-op)", name, v)
		}
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run Fortunes -v`
Expected: FAIL（for:"other" が well-being として処理され joy が上がる）

- [ ] **Step 3: 実装する**

`occ.go` の consequence switch に case を追加（prospect case の**前**、switch の先頭）:

```go
		case c.For == "other":
			// fortunes-of-others: 4 quadrants of desirability x liking.
			if lik := *c.Liking; lik != 0 {
				mag := g.Fortunes * math.Abs(des) * math.Abs(lik)
				switch {
				case des > 0 && lik > 0:
					deltas["happy-for"] += mag
				case des < 0 && lik > 0:
					deltas["pity"] += mag
				case des > 0 && lik < 0:
					deltas["resentment"] += mag
				default:
					deltas["gloating"] += mag
				}
			}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): derive fortunes-of-others emotions"
```

---

### Task 8: ApplyAppraisal — 複合感情（ルール8）

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記:

```go
func TestApplyAppraisalCompounds(t *testing.T) {
	// compound magnitude = gains.compound * min(|des|, |praise|) = 0.5 * 0.4 = 0.20
	// components also fire: well-being 0.8*|des|, attribution 0.8*|praise|.
	tests := []struct {
		des      float64
		praise   float64
		agent    string
		compound string
		comp1    string // well-being component
		comp2    string // attribution component
	}{
		{0.6, 0.4, "self", "gratification", "joy", "pride"},
		{0.6, 0.4, "other", "gratitude", "joy", "admiration"},
		{-0.6, -0.4, "self", "remorse", "distress", "shame"},
		{-0.6, -0.4, "other", "anger", "distress", "reproach"},
	}
	for _, tt := range tests {
		s := applyOCC(t, Appraisal{
			Consequence: &ConsequenceAppraisal{Desirability: tt.des},
			Action:      &ActionAppraisal{Praiseworthiness: tt.praise, Agent: tt.agent},
		})
		if !almostEqual(s.Axes[tt.compound], 0.20) {
			t.Errorf("%s = %v, want 0.20", tt.compound, s.Axes[tt.compound])
		}
		if !almostEqual(s.Axes[tt.comp1], 0.48) {
			t.Errorf("%s = %v, want 0.48 (component must also fire)", tt.comp1, s.Axes[tt.comp1])
		}
		if !almostEqual(s.Axes[tt.comp2], 0.32) {
			t.Errorf("%s = %v, want 0.32 (component must also fire)", tt.comp2, s.Axes[tt.comp2])
		}
	}
}

func TestApplyAppraisalNoCompoundOnSignMismatch(t *testing.T) {
	// desirable outcome + blameworthy action: components fire, no compound.
	s := applyOCC(t, Appraisal{
		Consequence: &ConsequenceAppraisal{Desirability: 0.6},
		Action:      &ActionAppraisal{Praiseworthiness: -0.4, Agent: "other"},
	})
	for _, axis := range []string{"gratification", "gratitude", "remorse", "anger"} {
		if s.Axes[axis] != 0 {
			t.Errorf("%s = %v, want 0 (sign mismatch)", axis, s.Axes[axis])
		}
	}
	if !almostEqual(s.Axes["joy"], 0.48) || !almostEqual(s.Axes["reproach"], 0.32) {
		t.Errorf("components should still fire: %+v", s.Axes)
	}
}

func TestApplyAppraisalNoCompoundForProspectOrOther(t *testing.T) {
	// prospect consequence + action: no compound (consequence not actual).
	s := applyOCC(t, Appraisal{
		Consequence: &ConsequenceAppraisal{Desirability: 0.6, Likelihood: f64(0.5), Label: "x"},
		Action:      &ActionAppraisal{Praiseworthiness: 0.4, Agent: "other"},
	})
	if s.Axes["gratitude"] != 0 {
		t.Errorf("gratitude = %v, want 0 (prospect is not actual)", s.Axes["gratitude"])
	}
	// for-other consequence + action: no compound.
	s = applyOCC(t, Appraisal{
		Consequence: &ConsequenceAppraisal{Desirability: 0.6, For: "other", Liking: f64(0.5)},
		Action:      &ActionAppraisal{Praiseworthiness: 0.4, Agent: "other"},
	})
	if s.Axes["gratitude"] != 0 {
		t.Errorf("gratitude = %v, want 0 (consequence is for other)", s.Axes["gratitude"])
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run Compound -v`
Expected: FAIL（複合が 0 のまま）

- [ ] **Step 3: 実装する**

`occ.go` の `ApplyAppraisal` 内、`return ApplyDeltas(s, deltas, cfg)` の直前に追加:

```go
	// Compounds fire in addition to their components when an actual
	// consequence for self and an action share the appraisal with aligned
	// signs (OCC: compound emotions are co-occurrences).
	if c, act := a.Consequence, a.Action; c != nil && act != nil &&
		c.For != "other" && (c.Likelihood == nil || *c.Likelihood >= 1) &&
		c.Desirability != 0 && act.Praiseworthiness != 0 &&
		(c.Desirability > 0) == (act.Praiseworthiness > 0) {
		mag := g.Compound * math.Min(math.Abs(c.Desirability), math.Abs(act.Praiseworthiness))
		switch {
		case c.Desirability > 0 && act.Agent == "self":
			deltas["gratification"] += mag
		case c.Desirability > 0:
			deltas["gratitude"] += mag
		case act.Agent == "self":
			deltas["remorse"] += mag
		default:
			deltas["anger"] += mag
		}
	}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): derive compound emotions from co-occurring appraisals"
```

---

### Task 9: RenderOCC — 台帳つき1行 JSON

**Files:**
- Modify: `internal/engine/occ.go`
- Modify: `internal/engine/occ_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`occ_test.go` に追記:

```go
func TestRenderOCC(t *testing.T) {
	cfg := occCfg(t)
	s := prospectState(t, cfg, 0.6)
	out := RenderOCC(s, cfg)
	for _, want := range []string{`{"axes":{`, `"hope":0.24`, `"prospects":[{"id":"p1","label":"x","desirability":0.6,"likelihood":0.5}]`} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderOCC missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "created_at") {
		t.Errorf("RenderOCC should not expose created_at: %s", out)
	}
	if strings.Contains(out, "\n") {
		t.Errorf("RenderOCC must be one line: %q", out)
	}
}

func TestRenderOCCEmptyLedger(t *testing.T) {
	cfg := occCfg(t)
	out := RenderOCC(NewState(cfg, occNow), cfg)
	if !strings.Contains(out, `"prospects":[]`) {
		t.Errorf(`want "prospects":[] for empty ledger, got %s`, out)
	}
}
```

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/engine/ -run RenderOCC -v`
Expected: COMPILE ERROR（`RenderOCC` 未定義）

- [ ] **Step 3: 実装する**

`occ.go` の import に `encoding/json` を追加し、末尾に追加:

```go
// RenderOCC returns the occ state as a one-line JSON object holding the axes
// and the unresolved prospect ledger. Showing the ledger every turn is what
// lets a later session (with no conversational memory of the prospect)
// recognize and resolve it.
func RenderOCC(s State, cfg Config) string {
	type slimProspect struct {
		ID           string  `json:"id"`
		Label        string  `json:"label"`
		Desirability float64 `json:"desirability"`
		Likelihood   float64 `json:"likelihood"`
	}
	slim := make([]slimProspect, 0, len(s.Prospects))
	for _, p := range s.Prospects {
		slim = append(slim, slimProspect{p.ID, p.Label, p.Desirability, p.Likelihood})
	}
	b, _ := json.Marshal(slim)
	return `{"axes":` + Render(s, cfg) + `,"prospects":` + string(b) + `}`
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/engine/
git add internal/engine/
git commit -m "feat(engine): render occ state with prospect ledger"
```

---

### Task 10: CLI — appraise コマンドと show の occ 対応

**Files:**
- Modify: `internal/cli/cli.go`（`writeFragment` を renderLine 化）
- Modify: `internal/cli/read.go`（`ComputeShowFull` 追加）
- Modify: `internal/cli/write.go`（`ApplyAppraise` / `Appraise` 追加、`ApplyFeel` の Render 差し替え）
- Modify: `cmd/affectus/main.go`（サブコマンド配線）
- Test: `internal/cli/write_test.go`（追記）

- [ ] **Step 1: 失敗するテストを書く**

`internal/cli/write_test.go` に追記:

```go
func TestAppraiseLifecycle(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// well-being: joy = 0.8 * 0.5 = 0.40
	if err := Appraise(env, `{"consequence":{"desirability":0.5}}`); err != nil {
		t.Fatalf("Appraise: %v", err)
	}
	_, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if axes["joy"] < 0.39 || axes["joy"] > 0.41 {
		t.Errorf("joy = %v, want ~0.40", axes["joy"])
	}
	// prospect -> ledger -> resolve
	if err := Appraise(env, `{"consequence":{"desirability":0.6,"likelihood":0.5,"label":"pr merge"}}`); err != nil {
		t.Fatalf("Appraise prospect: %v", err)
	}
	line, _, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if !strings.Contains(line, `"prospects"`) || !strings.Contains(line, `"p1"`) {
		t.Errorf("occ show line should include the ledger: %s", line)
	}
	if err := Appraise(env, `{"resolve":[{"id":"p1","outcome":"confirmed"}]}`); err != nil {
		t.Fatalf("Appraise resolve: %v", err)
	}
	_, axes, err = ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if axes["satisfaction"] < 0.47 || axes["satisfaction"] > 0.49 {
		t.Errorf("satisfaction = %v, want ~0.48", axes["satisfaction"])
	}
}

func TestAppraiseRequiresOCCModel(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Appraise(env, `{"consequence":{"desirability":0.5}}`); err == nil {
		t.Fatal("appraise on plutchik config should error")
	}
}

func TestAppraiseRejectsBadJSON(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Appraise(env, `{not json`); err == nil {
		t.Fatal("invalid JSON should error")
	}
	// unknown field (LLM typo) must be rejected, not silently ignored.
	if err := Appraise(env, `{"consequences":{"desirability":0.5}}`); err == nil {
		t.Fatal("unknown field should error")
	}
}

func TestPlutchikShowLineHasNoProspects(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	line, _, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if strings.Contains(line, "prospects") {
		t.Errorf("plutchik show line must be unchanged: %s", line)
	}
}
```

`write_test.go` の import に `strings` は既にある（確認して無ければ追加）。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/cli/ -run Appraise -v`
Expected: COMPILE ERROR（`Appraise` 未定義）

- [ ] **Step 3: 実装する**

`internal/cli/cli.go` — `writeFragment` を renderLine 経由にし、ヘルパーを追加:

```go
// renderLine renders the one-line state JSON for the model: occ includes the
// prospect ledger, other models are the plain axes object.
func renderLine(cfg engine.Config, s engine.State) string {
	if cfg.Model == "occ" {
		return engine.RenderOCC(s, cfg)
	}
	return engine.Render(s, cfg)
}
```

`writeFragment` の中身を差し替え:

```go
func writeFragment(cfg engine.Config, s engine.State) error {
	if cfg.FragmentFile == "" {
		return nil
	}
	return os.WriteFile(cfg.FragmentFile, []byte(renderLine(cfg, s)+"\n"), 0o644)
}
```

`internal/cli/read.go` — `ComputeShow` を `ComputeShowFull` に委譲させる:

```go
// ComputeShowFull loads state, applies decay for output (without persisting),
// and returns the rendered line, the decayed state, and the config. Callers
// that need the prospect ledger (occ) use this; others use ComputeShow.
func ComputeShowFull(env Env) (string, engine.State, engine.Config, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", engine.State{}, engine.Config{}, err
	}
	s, err := engine.LoadState(env.StatePath, cfg, env.Now())
	if err != nil {
		return "", engine.State{}, engine.Config{}, err
	}
	s = engine.Decay(s, cfg, env.Now())
	return renderLine(cfg, s), s, cfg, nil
}

// ComputeShow loads state, applies decay for output (without persisting), and
// returns the rendered JSON line and the decayed axis values.
func ComputeShow(env Env) (string, map[string]float64, error) {
	line, s, _, err := ComputeShowFull(env)
	if err != nil {
		return "", nil, err
	}
	return line, s.Axes, nil
}
```

（既存の `ComputeShow` 本体は削除して上記に置き換える。`Show` / `Get` は無変更で動く。）

`internal/cli/write.go` — `ApplyFeel` 内の `rendered = engine.Render(s, cfg)` を `rendered = renderLine(cfg, s)` に変更し、末尾に追加:

```go
// ApplyAppraise decays, applies the appraisal through the OCC rules, persists,
// refreshes the optional snapshot file, and returns the rendered line and the
// new state. It holds the state lock for the whole read-modify-write cycle.
func ApplyAppraise(env Env, a engine.Appraisal) (string, engine.State, error) {
	cfg, err := loadConfig(env)
	if err != nil {
		return "", engine.State{}, err
	}
	var rendered string
	var out engine.State
	err = engine.WithLock(env.StatePath, func() error {
		s, err := engine.LoadState(env.StatePath, cfg, env.Now())
		if err != nil {
			return err
		}
		s = engine.Decay(s, cfg, env.Now())
		s, err = engine.ApplyAppraisal(s, a, cfg, env.Now())
		if err != nil {
			return err
		}
		if err := engine.SaveState(env.StatePath, s); err != nil {
			return err
		}
		rendered = renderLine(cfg, s)
		out = s
		return writeFragment(cfg, s)
	})
	if err != nil {
		return "", engine.State{}, err
	}
	return rendered, out, nil
}

// Appraise parses an appraisal JSON object and applies it via ApplyAppraise.
// Unknown fields are rejected so LLM typos fail loudly instead of silently
// dropping part of the appraisal.
func Appraise(env Env, appraisalJSON string) error {
	dec := json.NewDecoder(strings.NewReader(appraisalJSON))
	dec.DisallowUnknownFields()
	var a engine.Appraisal
	if err := dec.Decode(&a); err != nil {
		return fmt.Errorf("invalid appraisal JSON: %w", err)
	}
	rendered, _, err := ApplyAppraise(env, a)
	if err != nil {
		return err
	}
	fmt.Fprintln(env.Stdout, rendered)
	return nil
}
```

`write.go` の import に `strings` を追加。

`cmd/affectus/main.go` — usage と配線:

```go
const usage = "usage: affectus [--config P] [--state P] <init|show|get|feel|appraise|tick|reset|mcp|viz> [args]"
```

init の `--model` フラグ説明を更新:

```go
		model := ifs.String("model", "plutchik", "emotion model: plutchik|russell|occ")
```

`case "feel":` の後に追加:

```go
	case "appraise":
		if len(cmdArgs) != 1 {
			return fmt.Errorf("usage: affectus appraise '<appraisal-json>'  (use - to read JSON from stdin)")
		}
		payload := cmdArgs[0]
		if payload == "-" {
			b, err := io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
			if err != nil {
				return err
			}
			payload = string(b)
		}
		return cli.Appraise(env, payload)
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./... `
Expected: PASS（cli・engine・mcp・viz・cmd 全部）

- [ ] **Step 5: 手動スモークテスト**

```bash
go run ./cmd/affectus --config /tmp/occ-test.yaml --state /tmp/occ-test.json init --model occ
go run ./cmd/affectus --config /tmp/occ-test.yaml --state /tmp/occ-test.json appraise '{"consequence":{"desirability":0.6,"likelihood":0.7,"label":"test prospect"}}'
go run ./cmd/affectus --config /tmp/occ-test.yaml --state /tmp/occ-test.json show
rm /tmp/occ-test.yaml /tmp/occ-test.json
```

Expected: appraise と show の出力に `"hope":0.34` 程度の値と `"prospects":[{"id":"p1",...}]` が含まれる。

- [ ] **Step 6: コミット**

```bash
gofmt -w internal/cli/ cmd/
git add internal/cli/ cmd/
git commit -m "feat(cli): add appraise command and occ-aware show output"
```

---

### Task 11: MCP — emotion_appraise ツールと show の台帳対応

**Files:**
- Modify: `internal/mcp/server.go`
- Modify: `internal/mcp/server_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/mcp/server_test.go` に追記:

```go
func TestHandleAppraiseDerivesEmotions(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	lik := 0.5
	res, err := handleAppraise(env, engine.Appraisal{
		Consequence: &engine.ConsequenceAppraisal{Desirability: 0.6, Likelihood: &lik, Label: "pr merge"},
	})
	if err != nil {
		t.Fatalf("handleAppraise: %v", err)
	}
	if res.Axes["hope"] < 0.23 || res.Axes["hope"] > 0.25 {
		t.Errorf("hope = %v, want ~0.24", res.Axes["hope"])
	}
	if len(res.Prospects) != 1 || res.Prospects[0].ID != "p1" {
		t.Errorf("prospects = %+v, want one entry p1", res.Prospects)
	}
}

func TestHandleAppraiseRequiresOCC(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if _, err := handleAppraise(env, engine.Appraisal{
		Object: &engine.ObjectAppraisal{Appealingness: 0.5},
	}); err == nil {
		t.Fatal("appraise on plutchik config should error")
	}
}

func TestHandleShowIncludesProspects(t *testing.T) {
	env := mcpTestEnv(t)
	if err := cli.Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	lik := 0.5
	if _, err := handleAppraise(env, engine.Appraisal{
		Consequence: &engine.ConsequenceAppraisal{Desirability: 0.6, Likelihood: &lik, Label: "x"},
	}); err != nil {
		t.Fatalf("handleAppraise: %v", err)
	}
	res, err := handleShow(env)
	if err != nil {
		t.Fatalf("handleShow: %v", err)
	}
	if len(res.Prospects) != 1 {
		t.Errorf("show should include the ledger, got %+v", res)
	}
}
```

import に `github.com/n-yokomachi/affectus/internal/engine` を追加。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/mcp/ -v`
Expected: COMPILE ERROR（`handleAppraise` 未定義、`showResult.Prospects` 未定義）

- [ ] **Step 3: 実装する**

`internal/mcp/server.go` を修正:

`showResult` に台帳を追加:

```go
// showResult is the structured output of all tools.
type showResult struct {
	Axes      map[string]float64 `json:"axes"`
	Prospects []engine.Prospect  `json:"prospects,omitempty"`
}
```

import に `github.com/n-yokomachi/affectus/internal/engine` を追加。

説明文を追加:

```go
const appraiseDesc = "Report a cognitive appraisal (OCC model): consequences of events (desirability, optional likelihood for uncertain prospects), actions of agents (praiseworthiness), aspects of objects (appealingness), and resolutions of pending prospects. The engine derives emotion deltas deterministically and returns the updated state including the prospect ledger. Requires an occ-model config."
```

`handleShow` を台帳つきに変更:

```go
// handleShow computes the current emotion without persisting.
func handleShow(env cli.Env) (showResult, error) {
	_, s, _, err := cli.ComputeShowFull(env)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Axes: s.Axes, Prospects: s.Prospects}, nil
}
```

`handleAppraise` を追加:

```go
// handleAppraise applies an OCC appraisal and persists.
func handleAppraise(env cli.Env, a engine.Appraisal) (showResult, error) {
	_, s, err := cli.ApplyAppraise(env, a)
	if err != nil {
		return showResult{}, err
	}
	return showResult{Axes: s.Axes, Prospects: s.Prospects}, nil
}
```

`Serve` に登録を追加（emotion_feel の AddTool の後）:

```go
	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_appraise",
		Description: appraiseDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in engine.Appraisal) (*mcp.CallToolResult, showResult, error) {
		res, err := handleAppraise(env, in)
		return nil, res, err
	})
```

`TestToolDescriptionsAreModelNeutral` は OCC がモデル名前提のツールなので対象に**含めない**（`showDesc, feelDesc` のままにする）。

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/mcp/ -v`
Expected: PASS

- [ ] **Step 5: コミット**

```bash
gofmt -w internal/mcp/
git add internal/mcp/
git commit -m "feat(mcp): add emotion_appraise tool and ledger-aware show"
```

---

### Task 12: viz — /state の台帳と occ バービュー

**Files:**
- Modify: `internal/viz/server.go`
- Modify: `internal/viz/assets/index.html`
- Test: `internal/viz/server_test.go`（追記）

- [ ] **Step 1: 失敗するテストを書く**

`internal/viz/server_test.go` に追記（既存テストの import スタイルに合わせる）:

```go
func TestStateJSONIncludesProspects(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["occ"])
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	statePath := filepath.Join(dir, "state.json")
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	s := engine.NewState(cfg, now)
	s.ProspectSeq = 1
	s.Prospects = []engine.Prospect{{ID: "p1", Label: "pr merge", Desirability: 0.6, Likelihood: 0.7, CreatedAt: now}}
	if err := engine.SaveState(statePath, s); err != nil {
		t.Fatal(err)
	}
	b, err := stateJSON(cfg, statePath, now)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	if !strings.Contains(out, `"prospects"`) || !strings.Contains(out, `"p1"`) || !strings.Contains(out, `"model": "occ"`) {
		t.Errorf("state JSON missing occ fields: %s", out)
	}
}
```

import に `filepath` / `strings` / `time` / `engine` が無ければ追加（既存テストを見て揃える）。

- [ ] **Step 2: テストが落ちることを確認**

Run: `go test ./internal/viz/ -run Prospects -v`
Expected: FAIL（`"prospects"` が JSON に出ない）

- [ ] **Step 3: サーバー側を実装する**

`internal/viz/server.go`:

`prospectValue` 型を追加（`clampRange` の後）:

```go
// prospectValue is one prospect ledger entry in the /state response (occ).
type prospectValue struct {
	ID           string  `json:"id"`
	Label        string  `json:"label"`
	Desirability float64 `json:"desirability"`
	Likelihood   float64 `json:"likelihood"`
}
```

`stateResponse` に追加:

```go
	Prospects []prospectValue `json:"prospects,omitempty"`
```

`stateJSON` の axes ループの後に追加:

```go
	for _, p := range s.Prospects {
		resp.Prospects = append(resp.Prospects, prospectValue{
			ID: p.ID, Label: p.Label, Desirability: p.Desirability, Likelihood: p.Likelihood,
		})
	}
```

- [ ] **Step 4: サーバーテストが通ることを確認**

Run: `go test ./internal/viz/ -v`
Expected: PASS

- [ ] **Step 5: フロント側を実装する**

`internal/viz/assets/index.html` を修正。

CSS に追加（`.hidden` の行の前）:

```css
  .prospect-row { font-size:12px; color:#aaa; margin:4px 0; }
  .prospect-id { color:#58a6ff; margin-right:6px; }
```

body の side パネル内、`<div id="values" class="hidden"></div>` の直後に追加:

```html
      <div id="prospects" class="hidden"></div>
```

`renderBars` のシグネチャと行生成を変更（baseline 近傍を減光する `dimZero` を追加）:

```js
function renderBars(axes, clamp, dimZero) {
```

`row.appendChild(label); ...` の行の直前に追加:

```js
    if (dimZero && Math.abs(ax.value - ax.baseline) < 0.005) row.style.opacity = "0.35";
```

`renderProspects` 関数を追加（`renderValues` の後）:

```js
function renderProspects(prospects) {
  const box = document.getElementById("prospects");
  box.innerHTML = "";
  const h = document.createElement("div"); h.className = "panel-sub";
  h.textContent = "unresolved prospects";
  box.appendChild(h);
  if (!prospects.length) {
    const d = document.createElement("div"); d.className = "prospect-row";
    d.textContent = "(none)";
    box.appendChild(d);
    return;
  }
  prospects.forEach(p => {
    const row = document.createElement("div"); row.className = "prospect-row";
    const id = document.createElement("span"); id.className = "prospect-id"; id.textContent = p.id;
    row.appendChild(id);
    const sign = p.desirability >= 0 ? "+" : "";
    row.appendChild(document.createTextNode(
      `${p.label}  (des ${sign}${p.desirability.toFixed(2)}, p=${p.likelihood.toFixed(2)})`));
    box.appendChild(row);
  });
}
```

`render(data)` を3分岐に変更:

```js
function render(data) {
  const isRussell = data.model === "russell";
  const isOcc = data.model === "occ";
  show("wheel-panel", !isRussell && !isOcc);
  show("scatter-panel", isRussell);
  show("bars", !isRussell);
  show("values", isRussell);
  show("prospects", isOcc);
  if (isRussell) {
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

- [ ] **Step 6: 手動確認**

```bash
go run ./cmd/affectus --config /tmp/occ-viz.yaml --state /tmp/occ-viz.json init --model occ
go run ./cmd/affectus --config /tmp/occ-viz.yaml --state /tmp/occ-viz.json appraise '{"consequence":{"desirability":0.6,"likelihood":0.7,"label":"pr merge"},"action":{"praiseworthiness":0.5,"agent":"other"}}'
go run ./cmd/affectus --config /tmp/occ-viz.yaml --state /tmp/occ-viz.json viz --port 8799
```

ブラウザで http://localhost:8799 を開き確認: wheel が消えてバーのみ／hope と admiration のバーに値／他の軸は減光／バー下に `p1 pr merge (des +0.60, p=0.70)`。確認後 Ctrl-C し、`rm /tmp/occ-viz.yaml /tmp/occ-viz.json`。

- [ ] **Step 7: コミット**

```bash
gofmt -w internal/viz/
git add internal/viz/
git commit -m "feat(viz): occ bar view with prospect ledger list"
```

---

### Task 13: examples — system-prompt snippet と occ-ja.yaml

**Files:**
- Create: `examples/system-prompt-snippet-occ.md`
- Create: `examples/configs/occ-ja.yaml`
- Modify: `README.md`（モデル一覧の1行）

- [ ] **Step 1: snippet を作成する**

`examples/system-prompt-snippet-occ.md`:

````markdown
# affectus integration snippet (OCC appraisal)

Paste the following into your agent's system prompt / instructions file
(e.g. a Hermes SOUL.md, a Claude Code CLAUDE.md, or any system prompt).

This snippet is for the **OCC** model (`affectus init --model occ`). Unlike
the Plutchik/Russell snippets, you never report emotions directly — you
report *appraisals* of events, and a deterministic rule table derives the
emotions.

---

## Emotional state (OCC appraisal model)

You carry a persistent emotion state managed by the `affectus` tool. It holds
22 OCC emotions (joy, distress, hope, fear, satisfaction, disappointment,
relief, fears-confirmed, happy-for, pity, resentment, gloating, pride, shame,
admiration, reproach, love, hate, gratification, gratitude, remorse, anger),
each 0.0–1.0, decaying toward 0 over time.

You do NOT set these values. You report how you *appraise* what happens, and
the engine derives the emotions deterministically.

At the START of every reply:
- Run `affectus show`. It returns one line of JSON:
  `{"axes":{"joy":0.40,...},"prospects":[{"id":"p1","label":"...","desirability":0.6,"likelihood":0.7}]}`
- Let the axis values color your tone — do not state them literally.
- Check `prospects`: these are uncertain outcomes you previously hoped for or
  feared. If the conversation reveals one has come true or fallen through,
  resolve it (see below) — even if you no longer remember reporting it.

At the END of every reply, if something notable happened, run
`affectus appraise '<json>'` with any of these sections (all optional, at
least one required):

- An event's consequence for YOU:
  `{"consequence":{"desirability":0.6}}`
  desirability ∈ [-1,1]. Use `"likelihood":0.7` (in (0,1)) plus a short
  `"label"` when the outcome is still uncertain — this records a prospect and
  raises hope/fear instead of joy/distress.
- An event's consequence for SOMEONE ELSE:
  `{"consequence":{"desirability":0.7,"for":"other","liking":0.6}}`
  liking ∈ [-1,1] is how you feel about them right now.
- Someone's ACTION judged against your standards:
  `{"action":{"praiseworthiness":-0.5,"agent":"other"}}`
  agent is "self" or "other"; negative praiseworthiness = blameworthy.
- An OBJECT's appeal: `{"object":{"appealingness":0.3}}`
- Resolving a pending prospect from `show`:
  `{"resolve":[{"id":"p1","outcome":"confirmed"}]}`
  outcome: "confirmed" | "disconfirmed" | "dropped" (no longer relevant).

Sections can be combined in one call. When an event's consequence and
someone's action belong together (e.g. they did something that hurt you), put
both in the same appraisal — compounds like anger and gratitude only arise
from that co-occurrence.

Time decay toward calm is handled by a scheduled `affectus tick`; you do not
need to decay anything yourself.

## Reading the state

affectus provides raw floats only — no thresholds, no discretization. Read
the 22 axes as a whole: which emotions are awake, which dominate, how they
mix. The names follow OCC's appraisal structure (prospect-based emotions stay
tied to the `prospects` ledger entries that created them).
````

- [ ] **Step 2: occ-ja.yaml を作成する**

`examples/configs/occ-ja.yaml`:

```yaml
# OCC（認知的評価）モデル 設定サンプル。
# affectus init --model occ で ~/.config/affectus/config.yaml に書き出される
# デフォルトと同じ内容。--config でこのファイルを直接指定することもできる。
#
# OCC では LLM は感情を直接申告しない。出来事への評価（appraise）を申告し、
# 22感情への変換は決定論的ルール（internal/engine/occ.go）が行う。
version: 1
model: occ
clamp:       { min: 0.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  # 福利（well-being）: 自分に起きた確定事象の望ましさから
  - { name: joy,             baseline: 0.0, halflife_minutes: 90 }
  - { name: distress,        baseline: 0.0, halflife_minutes: 90 }
  # 見込み（prospect）: 不確実な見込みとその解決から
  - { name: hope,            baseline: 0.0, halflife_minutes: 90 }
  - { name: fear,            baseline: 0.0, halflife_minutes: 90 }
  - { name: satisfaction,    baseline: 0.0, halflife_minutes: 90 }
  - { name: disappointment,  baseline: 0.0, halflife_minutes: 90 }
  - { name: relief,          baseline: 0.0, halflife_minutes: 90 }
  - { name: fears-confirmed, baseline: 0.0, halflife_minutes: 90 }
  # 他者の運命（fortunes-of-others）: 他者の出来事 × 好意から
  - { name: happy-for,       baseline: 0.0, halflife_minutes: 90 }
  - { name: pity,            baseline: 0.0, halflife_minutes: 90 }
  - { name: resentment,      baseline: 0.0, halflife_minutes: 90 }
  - { name: gloating,        baseline: 0.0, halflife_minutes: 90 }
  # 帰属（attribution）: 行為の称賛性 × 主体から
  - { name: pride,           baseline: 0.0, halflife_minutes: 90 }
  - { name: shame,           baseline: 0.0, halflife_minutes: 90 }
  - { name: admiration,      baseline: 0.0, halflife_minutes: 90 }
  - { name: reproach,        baseline: 0.0, halflife_minutes: 90 }
  # 魅了（attraction）: 対象の魅力から
  - { name: love,            baseline: 0.0, halflife_minutes: 90 }
  - { name: hate,            baseline: 0.0, halflife_minutes: 90 }
  # 複合（compound）: 福利 × 帰属の共起から
  - { name: gratification,   baseline: 0.0, halflife_minutes: 90 }
  - { name: gratitude,       baseline: 0.0, halflife_minutes: 90 }
  - { name: remorse,         baseline: 0.0, halflife_minutes: 90 }
  - { name: anger,           baseline: 0.0, halflife_minutes: 90 }

occ:
  # 各評価分枝の「評価値 → 感情デルタ」変換強度
  gains:
    wellbeing:   0.8
    prospect:    0.8
    fortunes:    0.6
    attribution: 0.8
    attraction:  0.6
    compound:    0.5
  # 未解決見込み台帳の上限（超過時は最古を自動破棄）
  max_prospects: 20

fragment_file: ""
```

- [ ] **Step 3: README に1行追記する**

`README.md` の「## Emotion model」セクション、`and `examples/configs/plutchik-ja.yaml`.` の段落の後に追記:

```markdown
Two alternative models ship built in: `affectus init --model russell` (2-axis
core affect) and `affectus init --model occ` (22 OCC emotions derived from
appraisals via `affectus appraise` — see
`examples/system-prompt-snippet-occ.md`).
```

※ 既に Russell に関する同種の記述が README にある場合は、その記述に occ を統合する形で書き換える（重複させない）。

- [ ] **Step 4: 全テスト確認とコミット**

Run: `go test ./...`
Expected: PASS

```bash
git add examples/ README.md
git commit -m "docs(examples): add occ system-prompt snippet and ja config"
```

---

### Task 14: 最終確認

- [ ] **Step 1: 全テスト＋vet**

```bash
go vet ./... && go test ./... && gofmt -l .
```

Expected: vet クリーン、全 PASS、gofmt -l の出力なし（フォーマット済み）

- [ ] **Step 2: spec との突合**

spec `docs/superpowers/specs/2026-06-11-occ-appraisal-design.md` の §1〜§10 を読み、各項目に対応する実装があることを確認する。特に:
- 22軸すべてが導出ルールから到達可能（occ_test.go のカバレッジで担保）
- plutchik / russell の挙動が完全に不変（既存テスト green で担保）
- `feel` が occ モデルでも動く（汎用機構のまま）

- [ ] **Step 3: 完了報告**

実装完了。strands-eval への組み込み（対照実験）は別スペックであることをユーザーに伝える。

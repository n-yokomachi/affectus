# Russell 感情円環 対応 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** affectus に Russell の感情円環モデル（Valence × Arousal の2軸連続体）を、`init --model` プロファイル機構と専用散布図ビューつきで追加する。

**Architecture:** 既存の決定論的エンジン（config 駆動・状態を JSON で持ち・解釈は LLM に委ねる）をそのまま踏襲し、(1) 軸ごとの値域 `range` をオプションで持てるようにし、(2) config に `model` 識別子を足し、(3) embed config をモデルレジストリ化し、(4) viz を `model` で分岐させて Russell 用散布図を描く。後方互換を厳守（`--model` 省略時 `plutchik`、既存 Plutchik config の挙動不変）。

**Tech Stack:** Go 1.x（標準ライブラリ ＋ `gopkg.in/yaml.v3`、`//go:embed`）、素の HTML/SVG/JS（viz フロント）、`go test`。

**設計仕様:** `docs/superpowers/specs/2026-05-30-russell-circumplex-design.md`

---

## File Structure

| ファイル | 責務 | 変更種別 |
|---|---|---|
| `internal/engine/config.go` | `AxisConfig.Range`・`Config.Model` 追加、`Validate` 拡張 | Modify |
| `internal/engine/apply.go` | per-axis clamp 解決ヘルパー＋ `ApplyDeltas` 修正 | Modify |
| `internal/engine/defaults.go` | embed をモデルレジストリ `Models` 化 | Modify |
| `internal/engine/plutchik8.default.yaml` | `model: plutchik` 追記 | Modify |
| `internal/engine/russell.default.yaml` | Russell デフォルト config | Create |
| `internal/cli/cli.go` | `Init` に `model` 引数 | Modify |
| `cmd/affectus/main.go` | init サブコマンドに `--model` フラグ | Modify |
| `internal/viz/server.go` | `/state` に `model` と per-axis `range` | Modify |
| `internal/viz/assets/index.html` | `model` 分岐・Russell 散布図ビュー・wheel タイトル動的化 | Modify |
| `internal/mcp/server.go` | tool description をモデル非依存の定数へ | Modify |
| `examples/system-prompt-snippet-russell.md` | Russell 用プロンプト断片 | Create |
| `examples/configs/russell-ja.yaml` | ユーザー向け Russell サンプル config | Create |
| 各 `*_test.go` | 上記のテスト | Modify |

実装は engine（Task 1–4）→ CLI（Task 5）→ viz（Task 6–7）→ MCP（Task 8）→ docs（Task 9）の順。各 Task は単体で `go test ./...` が green になるよう設計する。

---

## Task 1: per-axis `range` で軸ごとにクランプする

**Files:**
- Modify: `internal/engine/config.go` (`AxisConfig` 構造体)
- Modify: `internal/engine/apply.go`
- Test: `internal/engine/apply_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/apply_test.go` の末尾に追記する。`buildRangeCfg` は valence(-1..1)/arousal(0..1) の per-axis range を持つ最小 config を組む。

```go
func buildRangeCfg() Config {
	v := Range{Min: -1.0, Max: 1.0}
	a := Range{Min: 0.0, Max: 1.0}
	return Config{
		Version:    1,
		Clamp:      Range{Min: -1.0, Max: 1.0},
		DeltaClamp: Range{Min: -1.0, Max: 1.0},
		Axes: []AxisConfig{
			{Name: "valence", Baseline: 0.0, HalflifeMinutes: 90, Range: &v},
			{Name: "arousal", Baseline: 0.3, HalflifeMinutes: 90, Range: &a},
		},
	}
}

func TestApplyDeltasPerAxisRange(t *testing.T) {
	cfg := buildRangeCfg()
	s := NewState(cfg, time.Now())
	s.Axes["valence"] = -0.8
	s.Axes["arousal"] = 0.1
	// valence -0.8 + -0.5 = -1.3 -> clamp to -1.0 (range allows negative)
	// arousal 0.1 + -0.5 = -0.4 -> clamp to 0.0 (range floor is 0)
	out, err := ApplyDeltas(s, map[string]float64{"valence": -0.5, "arousal": -0.5}, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !almostEqual(out.Axes["valence"], -1.0) {
		t.Errorf("valence = %v, want clamped to -1.0", out.Axes["valence"])
	}
	if !almostEqual(out.Axes["arousal"], 0.0) {
		t.Errorf("arousal = %v, want clamped to 0.0", out.Axes["arousal"])
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run TestApplyDeltasPerAxisRange -v`
Expected: コンパイルエラー（`AxisConfig` に `Range` フィールドがない）。

- [ ] **Step 3: `AxisConfig` に `Range` を追加**

`internal/engine/config.go` の `AxisConfig` を次のようにする（`Range` 型はファイル先頭に既存）。

```go
// AxisConfig defines one emotion axis.
type AxisConfig struct {
	Name            string  `yaml:"name"`
	Baseline        float64 `yaml:"baseline"`
	HalflifeMinutes float64 `yaml:"halflife_minutes"`
	// Opposite names the polar-opposite axis (e.g. joy <-> sadness). Reserved
	// for future use: validated for referential integrity but not yet consumed
	// by decay, apply, or render logic.
	Opposite string `yaml:"opposite"`
	// Range optionally overrides the global Clamp for this axis. nil means the
	// axis uses Config.Clamp. Used by models with asymmetric axis domains
	// (e.g. Russell: valence -1..1, arousal 0..1).
	Range *Range `yaml:"range"`
}
```

- [ ] **Step 4: `ApplyDeltas` を per-axis クランプにする**

`internal/engine/apply.go` に解決ヘルパーを足し、`ApplyDeltas` のクランプ行を差し替える。

```go
// axisClamp returns the effective value range for one axis: its own Range
// override when set, otherwise the global Clamp.
func axisClamp(ax AxisConfig, cfg Config) Range {
	if ax.Range != nil {
		return *ax.Range
	}
	return cfg.Clamp
}
```

`ApplyDeltas` の最後のループを次に変更する。

```go
	axes := make(map[string]float64, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		d := clamp(deltas[ax.Name], cfg.DeltaClamp.Min, cfg.DeltaClamp.Max)
		r := axisClamp(ax, cfg)
		axes[ax.Name] = clamp(s.Axes[ax.Name]+d, r.Min, r.Max)
	}
```

- [ ] **Step 5: テストが通ることを確認**

Run: `go test ./internal/engine/ -run TestApplyDeltas -v`
Expected: `TestApplyDeltasPerAxisRange` を含む全 ApplyDeltas テストが PASS（既存テストは `Range` nil なので従来通り）。

- [ ] **Step 6: コミット**

```bash
git add internal/engine/config.go internal/engine/apply.go internal/engine/apply_test.go
git commit -m "feat(engine): support per-axis value range (clamp override)"
```

---

## Task 2: `Validate` で range と baseline を検証する

**Files:**
- Modify: `internal/engine/config.go` (`Validate`)
- Test: `internal/engine/config_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/config_test.go` の `TestValidateErrors` のケース表に2件追加し、range を持つ正常系テストも足す。`minimalConfig` は range を持たないので、range 検証用に新しい helper config を使う。

```go
func TestValidateRangeAndBaseline(t *testing.T) {
	good := Config{
		Version:    1,
		Clamp:      Range{Min: -1.0, Max: 1.0},
		DeltaClamp: Range{Min: -1.0, Max: 1.0},
		Axes: []AxisConfig{
			{Name: "valence", Baseline: 0.0, HalflifeMinutes: 90, Range: &Range{Min: -1.0, Max: 1.0}},
			{Name: "arousal", Baseline: 0.3, HalflifeMinutes: 90, Range: &Range{Min: 0.0, Max: 1.0}},
		},
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("valid range config rejected: %v", err)
	}

	badRange := good
	badRange.Axes = []AxisConfig{{Name: "x", Baseline: 0.0, HalflifeMinutes: 90, Range: &Range{Min: 1.0, Max: 1.0}}}
	if err := badRange.Validate(); err == nil || !strings.Contains(err.Error(), "range min") {
		t.Fatalf("want range min error, got %v", err)
	}

	badBaseline := good
	badBaseline.Axes = []AxisConfig{{Name: "arousal", Baseline: -0.2, HalflifeMinutes: 90, Range: &Range{Min: 0.0, Max: 1.0}}}
	if err := badBaseline.Validate(); err == nil || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("want baseline-out-of-range error, got %v", err)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run TestValidateRangeAndBaseline -v`
Expected: FAIL（`badRange`/`badBaseline` がエラーを返さない＝検証未実装）。

- [ ] **Step 3: `Validate` に検証を追加**

`internal/engine/config.go` の `Validate` の最初のループ（dup と halflife を見ている箇所）に、halflife チェックの直後で range と baseline の検証を足す。

```go
	for _, ax := range c.Axes {
		if seen[ax.Name] {
			return fmt.Errorf("config: duplicate axis %q", ax.Name)
		}
		seen[ax.Name] = true
		if ax.HalflifeMinutes <= 0 {
			return fmt.Errorf("config: axis %q has non-positive halflife_minutes", ax.Name)
		}
		lo, hi := c.Clamp.Min, c.Clamp.Max
		if ax.Range != nil {
			if ax.Range.Min >= ax.Range.Max {
				return fmt.Errorf("config: axis %q range min must be less than max", ax.Name)
			}
			lo, hi = ax.Range.Min, ax.Range.Max
		}
		if ax.Baseline < lo || ax.Baseline > hi {
			return fmt.Errorf("config: axis %q baseline %v outside range [%v, %v]", ax.Name, ax.Baseline, lo, hi)
		}
	}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run TestValidate -v`
Expected: `TestValidateRangeAndBaseline` と既存 `TestValidateErrors` がともに PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/engine/config.go internal/engine/config_test.go
git commit -m "feat(engine): validate per-axis range and baseline-in-range"
```

---

## Task 3: `Config.Model` フィールドを追加する

**Files:**
- Modify: `internal/engine/config.go` (`Config`)
- Test: `internal/engine/config_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/engine/config_test.go` に追記。`minimalConfig` には `model:` が無いので空文字になることも確認する。

```go
func TestParseConfigModel(t *testing.T) {
	withModel := "model: russell\n" + minimalConfig
	cfg, err := ParseConfig([]byte(withModel))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Model != "russell" {
		t.Errorf("Model = %q, want russell", cfg.Model)
	}

	base, err := ParseConfig([]byte(minimalConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if base.Model != "" {
		t.Errorf("Model = %q, want empty for config without model field", base.Model)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run TestParseConfigModel -v`
Expected: コンパイルエラー（`Config` に `Model` フィールドがない）。

- [ ] **Step 3: `Config` に `Model` を追加**

`internal/engine/config.go` の `Config` 構造体を次にする。

```go
// Config is the full library configuration.
type Config struct {
	Version int `yaml:"version"`
	// Model identifies the emotion model ("plutchik" | "russell"). Optional;
	// empty means the legacy default (Plutchik). Used by viz to choose a
	// rendering and as profile self-description.
	Model        string       `yaml:"model"`
	Clamp        Range        `yaml:"clamp"`
	DeltaClamp   Range        `yaml:"delta_clamp"`
	Axes         []AxisConfig `yaml:"axes"`
	FragmentFile string       `yaml:"fragment_file"`
}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/engine/ -run TestParseConfigModel -v`
Expected: PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/engine/config.go internal/engine/config_test.go
git commit -m "feat(engine): add optional model identifier to config"
```

---

## Task 4: Russell デフォルト config とモデルレジストリ

**Files:**
- Create: `internal/engine/russell.default.yaml`
- Modify: `internal/engine/plutchik8.default.yaml` (`model: plutchik` 追記)
- Modify: `internal/engine/defaults.go`
- Test: `internal/engine/defaults_test.go`

- [ ] **Step 1: Russell デフォルト config を作成**

`internal/engine/russell.default.yaml`:

```yaml
version: 1
model: russell
clamp:       { min: -1.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

fragment_file: ""
```

- [ ] **Step 2: Plutchik config に model を追記**

`internal/engine/plutchik8.default.yaml` の先頭を次にする（`version: 1` の直後に `model: plutchik` を足す）。

```yaml
version: 1
model: plutchik
clamp:       { min: 0.0, max: 1.0 }
delta_clamp: { min: -1.0, max: 1.0 }
```

- [ ] **Step 3: 失敗するテストを書く**

`internal/engine/defaults_test.go` に追記。

```go
func TestModelsRegistry(t *testing.T) {
	for _, name := range []string{"plutchik", "russell"} {
		yaml, ok := Models[name]
		if !ok {
			t.Fatalf("Models[%q] missing", name)
		}
		if len(yaml) == 0 {
			t.Fatalf("Models[%q] is empty; embed failed", name)
		}
		cfg, err := ParseConfig(yaml)
		if err != nil {
			t.Fatalf("Models[%q] invalid config: %v", name, err)
		}
		if cfg.Model != name {
			t.Errorf("Models[%q] declares model %q", name, cfg.Model)
		}
	}
}

func TestRussellConfigShape(t *testing.T) {
	cfg, err := ParseConfig(Models["russell"])
	if err != nil {
		t.Fatalf("russell config: %v", err)
	}
	if len(cfg.Axes) != 2 {
		t.Fatalf("russell axes = %d, want 2", len(cfg.Axes))
	}
	want := []string{"valence", "arousal"}
	for i, name := range want {
		if cfg.Axes[i].Name != name {
			t.Errorf("axis %d = %q, want %q", i, cfg.Axes[i].Name, name)
		}
		if cfg.Axes[i].Range == nil {
			t.Errorf("axis %q should have an explicit range", name)
		}
	}
	if !almostEqual(cfg.Axes[1].Baseline, 0.3) {
		t.Errorf("arousal baseline = %v, want 0.3", cfg.Axes[1].Baseline)
	}
}
```

- [ ] **Step 4: テストが失敗することを確認**

Run: `go test ./internal/engine/ -run 'TestModelsRegistry|TestRussellConfigShape' -v`
Expected: コンパイルエラー（`Models` が未定義）。

- [ ] **Step 5: `defaults.go` をレジストリ化**

`internal/engine/defaults.go` を次にする。

```go
package engine

import _ "embed"

//go:embed plutchik8.default.yaml
var plutchikYAML []byte

//go:embed russell.default.yaml
var russellYAML []byte

// Models maps a model identifier to its embedded default configuration YAML.
var Models = map[string][]byte{
	"plutchik": plutchikYAML,
	"russell":  russellYAML,
}

// DefaultConfigYAML is the embedded Plutchik-8 default configuration. Kept as
// the package default for backward compatibility.
var DefaultConfigYAML = plutchikYAML

// DefaultConfig parses the embedded default configuration.
func DefaultConfig() (Config, error) {
	return ParseConfig(DefaultConfigYAML)
}
```

- [ ] **Step 6: テストが通ることを確認**

Run: `go test ./internal/engine/ -v`
Expected: 新規2テストを含む engine パッケージ全テストが PASS。`TestDefaultConfigAxisOrder` 等の既存テストも plutchik のまま green。

- [ ] **Step 7: コミット**

```bash
git add internal/engine/russell.default.yaml internal/engine/plutchik8.default.yaml internal/engine/defaults.go internal/engine/defaults_test.go
git commit -m "feat(engine): add russell default config and model registry"
```

---

## Task 5: CLI `init --model {plutchik|russell}`

**Files:**
- Modify: `internal/cli/cli.go` (`Init`)
- Modify: `cmd/affectus/main.go` (init サブコマンド)
- Test: `internal/cli/cli_test.go`, `cmd/affectus/main_test.go`

- [ ] **Step 1: 失敗するテストを書く（cli）**

`internal/cli/cli_test.go` に追記。`engine` import が必要なら足す。

```go
func TestInitRussellWritesRussellConfig(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "russell", false); err != nil {
		t.Fatalf("Init russell: %v", err)
	}
	cfg, err := engine.LoadConfig(env.ConfigPath)
	if err != nil {
		t.Fatalf("load written config: %v", err)
	}
	if cfg.Model != "russell" {
		t.Errorf("written model = %q, want russell", cfg.Model)
	}
	if len(cfg.Axes) != 2 {
		t.Errorf("written axes = %d, want 2", len(cfg.Axes))
	}
}

func TestInitUnknownModel(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "freud", false); err == nil {
		t.Fatal("Init with unknown model should error")
	}
}
```

`internal/cli/cli_test.go` の冒頭 import に `"github.com/n-yokomachi/affectus/internal/engine"` を追加する。
**既存の呼び出しを更新する**: 同ファイル内の `Init(env, false)` を `Init(env, "plutchik", false)`、`Init(env, true)` を `Init(env, "plutchik", true)` に置換する（`TestInitCreatesFiles`, `TestInitRefusesOverwriteWithoutForce` の3か所）。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/cli/ -run TestInit -v`
Expected: コンパイルエラー（`Init` の引数が合わない）。

- [ ] **Step 3: `Init` のシグネチャを変更**

`internal/cli/cli.go` の `Init` を次にする。

```go
// Init writes the config for the given model and a baseline state file.
// Existing files are only overwritten when force is true.
func Init(env Env, model string, force bool) error {
	yamlBytes, ok := engine.Models[model]
	if !ok {
		return fmt.Errorf("unknown model %q (valid: plutchik, russell)", model)
	}
	for _, p := range []string{env.ConfigPath, env.StatePath} {
		if _, err := os.Stat(p); err == nil && !force {
			return fmt.Errorf("%s already exists (use --force to overwrite)", p)
		}
	}
	if err := os.MkdirAll(filepath.Dir(env.ConfigPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(env.ConfigPath, yamlBytes, 0o644); err != nil {
		return err
	}
	cfg, err := engine.ParseConfig(yamlBytes)
	if err != nil {
		return err
	}
	if err := engine.SaveState(env.StatePath, engine.NewState(cfg, env.Now())); err != nil {
		return err
	}
	out := env.Stdout
	if out == nil {
		out = io.Discard
	}
	fmt.Fprintf(out, "initialized model=%s config=%s state=%s\n", model, env.ConfigPath, env.StatePath)
	return nil
}
```

- [ ] **Step 4: main.go の init サブコマンドに `--model` を追加**

`cmd/affectus/main.go` の `case "init":` を次にする。

```go
	case "init":
		ifs := flag.NewFlagSet("init", flag.ContinueOnError)
		force := ifs.Bool("force", false, "overwrite existing files")
		model := ifs.String("model", "plutchik", "emotion model: plutchik|russell")
		if err := ifs.Parse(cmdArgs); err != nil {
			return err
		}
		return cli.Init(env, *model, *force)
```

- [ ] **Step 5: 失敗するテストを書く（main）＋ usage 文言**

`cmd/affectus/main_test.go` に追記。

```go
func TestRunInitRussell(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	state := filepath.Join(dir, "state.json")
	if err := run([]string{"--config", cfg, "--state", state, "init", "--model", "russell"}); err != nil {
		t.Fatalf("run init --model russell: %v", err)
	}
	if err := run([]string{"--config", cfg, "--state", state, "feel", `{"valence":0.5,"arousal":0.4}`}); err != nil {
		t.Fatalf("feel on russell config: %v", err)
	}
}

func TestRunInitUnknownModel(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	state := filepath.Join(dir, "state.json")
	if err := run([]string{"--config", cfg, "--state", state, "init", "--model", "freud"}); err == nil {
		t.Fatal("unknown model should error")
	}
}
```

- [ ] **Step 6: テストが通ることを確認**

Run: `go test ./internal/cli/ ./cmd/affectus/ -v`
Expected: 新規テストと既存テスト（更新後の `Init` 呼び出し含む）がすべて PASS。

- [ ] **Step 7: コミット**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go cmd/affectus/main.go cmd/affectus/main_test.go
git commit -m "feat(cli): add init --model for plutchik/russell profiles"
```

---

## Task 6: viz `/state` に `model` と per-axis `range` を出す

**Files:**
- Modify: `internal/viz/server.go`
- Test: `internal/viz/server_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/viz/server_test.go` に追記。Russell config を組んで `/state` の JSON を検証する。

```go
func TestStateJSONIncludesModelAndPerAxisRange(t *testing.T) {
	cfg, err := engine.ParseConfig(engine.Models["russell"])
	if err != nil {
		t.Fatalf("russell config: %v", err)
	}
	b, err := stateJSON(cfg, filepath.Join(t.TempDir(), "missing.json"), time.Now())
	if err != nil {
		t.Fatalf("stateJSON: %v", err)
	}
	var resp stateResponse
	if err := json.Unmarshal(b, &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Model != "russell" {
		t.Errorf("model = %q, want russell", resp.Model)
	}
	byName := map[string]axisValue{}
	for _, ax := range resp.Axes {
		byName[ax.Name] = ax
	}
	if byName["valence"].Range.Min != -1.0 || byName["valence"].Range.Max != 1.0 {
		t.Errorf("valence range = %+v, want {-1,1}", byName["valence"].Range)
	}
	if byName["arousal"].Range.Min != 0.0 || byName["arousal"].Range.Max != 1.0 {
		t.Errorf("arousal range = %+v, want {0,1}", byName["arousal"].Range)
	}
}
```

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/viz/ -run TestStateJSONIncludesModelAndPerAxisRange -v`
Expected: コンパイルエラー（`stateResponse.Model` と `axisValue.Range` が無い）。

- [ ] **Step 3: レスポンス構造体とビルダーを拡張**

`internal/viz/server.go`:

`axisValue` に `Range` を追加。

```go
// axisValue is one axis in the /state response.
type axisValue struct {
	Name            string     `json:"name"`
	Value           float64    `json:"value"`
	Baseline        float64    `json:"baseline"`
	HalflifeMinutes float64    `json:"halflife_minutes"`
	Range           clampRange `json:"range"`
}
```

`stateResponse` に `Model` を追加。

```go
// stateResponse is the JSON body of GET /state.
type stateResponse struct {
	UpdatedAt time.Time   `json:"updated_at"`
	Model     string      `json:"model"`
	Clamp     clampRange  `json:"clamp"`
	Axes      []axisValue `json:"axes"`
}
```

`stateJSON` の組み立てを次にする（`Model` を入れ、各軸の effective range を解決）。

```go
	resp := stateResponse{
		UpdatedAt: s.UpdatedAt,
		Model:     cfg.Model,
		Clamp:     clampRange{Min: cfg.Clamp.Min, Max: cfg.Clamp.Max},
		Axes:      make([]axisValue, 0, len(cfg.Axes)),
	}
	for _, ax := range cfg.Axes {
		r := clampRange{Min: cfg.Clamp.Min, Max: cfg.Clamp.Max}
		if ax.Range != nil {
			r = clampRange{Min: ax.Range.Min, Max: ax.Range.Max}
		}
		resp.Axes = append(resp.Axes, axisValue{
			Name:            ax.Name,
			Value:           s.Axes[ax.Name],
			Baseline:        ax.Baseline,
			HalflifeMinutes: ax.HalflifeMinutes,
			Range:           r,
		})
	}
```

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/viz/ -v`
Expected: 新規テストと既存 viz テストがすべて PASS（plutchik では `model: plutchik`、range は全軸 {0,1}）。

- [ ] **Step 5: コミット**

```bash
git add internal/viz/server.go internal/viz/server_test.go
git commit -m "feat(viz): expose model and per-axis range in /state"
```

---

## Task 7: viz フロントを model で分岐し Russell 散布図を描く

**Files:**
- Modify: `internal/viz/assets/index.html`
- 手動検証（このリポジトリに JS テスト基盤はないため、サーバー側は Task 6 で担保済み。フロントは目視確認する）

- [ ] **Step 1: `index.html` を全面更新**

`internal/viz/assets/index.html` を以下の内容で**全置換**する。plutchik（既定）は従来の wheel+bars、russell は散布図+数値パネルを描く。wheel のタイトルは軸数から動的化。軌跡はクライアント側でポーリング点を蓄積する（リングバッファ・リロードでリセット）。

```html
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>affectus viz</title>
<style>
  body { font-family: sans-serif; background:#1a1a1a; color:#eee; margin:0; padding:24px; }
  h1 { font-size:15px; font-weight:normal; color:#888; margin:0 0 16px; }
  .panels { display:flex; gap:48px; flex-wrap:wrap; align-items:flex-start; }
  .panel { display:flex; flex-direction:column; }
  .panel-title { font-size:12px; color:#888; margin:0 0 4px; }
  .panel-sub { font-size:11px; color:#666; margin:0 0 10px; }
  .bar-row { display:flex; align-items:center; margin:7px 0; font-size:13px; }
  .bar-label { width:104px; text-align:right; padding-right:10px; }
  .bar-track { width:200px; height:16px; background:#333; border-radius:3px; }
  .bar-fill { height:100%; border-radius:3px; transition:width .35s linear; }
  .bar-value { width:48px; text-align:right; padding-left:10px; font-variant-numeric:tabular-nums; color:#ddd; }
  .bar-eta { padding-left:12px; font-size:12px; color:#888; font-variant-numeric:tabular-nums; }
  .updated { color:#666; font-size:12px; margin-top:20px; }
  .val-row { display:flex; align-items:baseline; gap:10px; margin:10px 0; }
  .val-name { width:80px; color:#888; font-size:13px; }
  .val-num { font-size:26px; font-variant-numeric:tabular-nums; color:#ddd; }
  .hidden { display:none; }
</style>
</head>
<body>
  <h1>affectus &mdash; emotion state</h1>
  <div class="panels">
    <div class="panel" id="wheel-panel">
      <div class="panel-title" id="wheel-title">wheel</div>
      <div class="panel-sub">petal length = current value per axis</div>
      <svg id="wheel" width="320" height="320" viewBox="0 0 320 320"></svg>
    </div>
    <div class="panel" id="scatter-panel">
      <div class="panel-title">valence &times; arousal (Russell circumplex)</div>
      <div class="panel-sub">dot = current core affect; faded trail = recent trajectory (session only)</div>
      <svg id="scatter" width="340" height="340" viewBox="0 0 340 340"></svg>
    </div>
    <div class="panel">
      <div class="panel-title" id="side-title">current values / time decay</div>
      <div class="panel-sub" id="side-sub">values decay exponentially toward baseline.</div>
      <div id="bars"></div>
      <div id="values" class="hidden"></div>
    </div>
  </div>
  <div class="updated" id="updated"></div>
<script>
const POLL_MS = 1000, CX = 160, CY = 160, R = 130;
const SVGNS = "http://www.w3.org/2000/svg";

function hue(i, n) { return Math.round(360 * i / n); }
function el(name, attrs) {
  const e = document.createElementNS(SVGNS, name);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  return e;
}

/* ---------- Plutchik (wheel + bars) ---------- */
function renderWheel(axes, clamp) {
  const svg = document.getElementById("wheel");
  svg.innerHTML = "";
  const n = axes.length;
  const span = (clamp.max - clamp.min) || 1;
  svg.appendChild(el("circle", {cx:CX, cy:CY, r:R, fill:"none", stroke:"#333", "stroke-width":1}));
  for (let i = 0; i < n; i++) {
    const a = 2*Math.PI*i/n - Math.PI/2;
    svg.appendChild(el("line", {x1:CX, y1:CY, x2:CX+R*Math.cos(a), y2:CY+R*Math.sin(a), stroke:"#333", "stroke-width":1}));
  }
  axes.forEach((ax, i) => {
    const a0 = 2*Math.PI*i/n - Math.PI/2;
    const a1 = 2*Math.PI*(i+1)/n - Math.PI/2;
    const norm = Math.max(0, Math.min(1, (ax.value - clamp.min) / span));
    const r = R * norm;
    if (r > 0.5) {
      const x0 = CX + r*Math.cos(a0), y0 = CY + r*Math.sin(a0);
      const x1 = CX + r*Math.cos(a1), y1 = CY + r*Math.sin(a1);
      const path = el("path", {d:`M${CX},${CY} L${x0},${y0} A${r},${r} 0 0,1 ${x1},${y1} Z`,
        fill:`hsl(${hue(i,n)},70%,55%)`, "fill-opacity":0.85, stroke:"#1a1a1a", "stroke-width":1.5});
      svg.appendChild(path);
    }
    const am = (a0 + a1) / 2;
    const t = el("text", {x:CX+(R+14)*Math.cos(am), y:CY+(R+14)*Math.sin(am),
      "font-size":11, fill:"#aaa", "text-anchor":"middle", "dominant-baseline":"middle"});
    t.textContent = ax.name;
    svg.appendChild(t);
  });
}

const ETA_EPS = 0.01;
function formatEta(ax) {
  const hl = ax.halflife_minutes;
  const baseline = ax.baseline || 0;
  if (!hl || hl <= 0) return "—";
  const diff = Math.abs(ax.value - baseline);
  if (diff <= ETA_EPS) return "—";
  const mins = hl * Math.log2(diff / ETA_EPS);
  if (!isFinite(mins) || mins <= 0) return "—";
  if (mins < 60) return `${Math.round(mins)}m`;
  const h = Math.floor(mins / 60);
  const m = Math.round(mins - h*60);
  if (h >= 24) {
    const d = Math.floor(h / 24);
    const rh = h - d*24;
    return rh > 0 ? `${d}d${rh}h` : `${d}d`;
  }
  return m > 0 ? `${h}h${m}m` : `${h}h`;
}

function renderBars(axes, clamp) {
  const box = document.getElementById("bars");
  box.innerHTML = "";
  const n = axes.length;
  const span = (clamp.max - clamp.min) || 1;
  axes.forEach((ax, i) => {
    const row = document.createElement("div"); row.className = "bar-row";
    const label = document.createElement("div"); label.className = "bar-label";
    label.textContent = ax.name;
    const track = document.createElement("div"); track.className = "bar-track";
    const fill = document.createElement("div"); fill.className = "bar-fill";
    const pct = Math.max(0, Math.min(1, (ax.value - clamp.min) / span));
    fill.style.width = (pct*100).toFixed(1) + "%";
    fill.style.background = `hsl(${hue(i,n)},70%,55%)`;
    track.appendChild(fill);
    const val = document.createElement("div"); val.className = "bar-value";
    val.textContent = ax.value.toFixed(2);
    const eta = document.createElement("div"); eta.className = "bar-eta";
    eta.textContent = formatEta(ax);
    row.appendChild(label); row.appendChild(track); row.appendChild(val); row.appendChild(eta);
    box.appendChild(row);
  });
}

/* ---------- Russell (scatter + values) ---------- */
const TRAIL_MAX = 60;
const trail = [];
// Circumplex word anchors in (valence, arousal) value space.
const WORDS = [
  {t:"alert",   v:0.0,  a:1.0},
  {t:"excited", v:0.6,  a:0.92},
  {t:"happy",   v:1.0,  a:0.6},
  {t:"content", v:0.7,  a:0.12},
  {t:"calm",    v:0.0,  a:0.0},
  {t:"sad",     v:-0.7, a:0.12},
  {t:"upset",   v:-1.0, a:0.55},
  {t:"tense",   v:-0.6, a:0.92},
];

function axisByName(axes, name) { return axes.find(a => a.name === name); }

function renderScatter(axes) {
  const V = axisByName(axes, "valence");
  const A = axisByName(axes, "arousal");
  const svg = document.getElementById("scatter");
  svg.innerHTML = "";
  if (!V || !A) return;
  const W = 340, H = 340, pad = 34;
  const vr = V.range, ar = A.range;
  const vspan = (vr.max - vr.min) || 1, aspan = (ar.max - ar.min) || 1;
  const x = v => pad + (v - vr.min) / vspan * (W - 2*pad);
  const y = a => (H - pad) - (a - ar.min) / aspan * (H - 2*pad);

  svg.appendChild(el("rect", {x:pad, y:pad, width:W-2*pad, height:H-2*pad, fill:"#202020", stroke:"#333"}));
  // valence=0 vertical, mid-arousal horizontal guides
  svg.appendChild(el("line", {x1:x(0), y1:pad, x2:x(0), y2:H-pad, stroke:"#3a3a3a", "stroke-dasharray":"3 3"}));
  const midA = (ar.min + ar.max) / 2;
  svg.appendChild(el("line", {x1:pad, y1:y(midA), x2:W-pad, y2:y(midA), stroke:"#3a3a3a", "stroke-dasharray":"3 3"}));

  // axis end labels
  const axLabel = (tx, ty, txt) => {
    const t = el("text", {x:tx, y:ty, "font-size":10, fill:"#777", "text-anchor":"middle"});
    t.textContent = txt; svg.appendChild(t);
  };
  axLabel(x(vr.max)-8, y(midA)-5, "+V");
  axLabel(x(vr.min)+8, y(midA)-5, "−V");
  axLabel(x(0)+12, pad+10, "↑A");

  // circumplex words
  WORDS.forEach(w => {
    const t = el("text", {x:x(w.v), y:y(w.a), "font-size":11, fill:"#9a8", "text-anchor":"middle", "dominant-baseline":"middle", "fill-opacity":0.7});
    t.textContent = w.t; svg.appendChild(t);
  });

  // trail (oldest faint -> newest bright)
  for (let i = 0; i < trail.length; i++) {
    const p = trail[i];
    const op = 0.08 + 0.5 * (i / Math.max(1, trail.length - 1));
    svg.appendChild(el("circle", {cx:x(p.v), cy:y(p.a), r:3, fill:"#58a6ff", "fill-opacity":op}));
    if (i > 0) {
      const q = trail[i-1];
      svg.appendChild(el("line", {x1:x(q.v), y1:y(q.a), x2:x(p.v), y2:y(p.a), stroke:"#58a6ff", "stroke-opacity":op*0.6}));
    }
  }
  // current point
  svg.appendChild(el("circle", {cx:x(V.value), cy:y(A.value), r:7, fill:"#58a6ff", stroke:"#1a1a1a", "stroke-width":2}));
}

function renderValues(axes) {
  const box = document.getElementById("values");
  box.innerHTML = "";
  ["valence", "arousal"].forEach(name => {
    const ax = axisByName(axes, name);
    if (!ax) return;
    const row = document.createElement("div"); row.className = "val-row";
    const nm = document.createElement("div"); nm.className = "val-name"; nm.textContent = name;
    const num = document.createElement("div"); num.className = "val-num";
    num.textContent = (ax.value >= 0 ? "+" : "") + ax.value.toFixed(2);
    row.appendChild(nm); row.appendChild(num);
    box.appendChild(row);
  });
}

/* ---------- dispatch ---------- */
function show(id, on) { document.getElementById(id).classList.toggle("hidden", !on); }

function render(data) {
  const isRussell = data.model === "russell";
  show("wheel-panel", !isRussell);
  show("scatter-panel", isRussell);
  show("bars", !isRussell);
  show("values", isRussell);
  if (isRussell) {
    trail.push({v: (axisByName(data.axes,"valence")||{}).value || 0,
                a: (axisByName(data.axes,"arousal")||{}).value || 0});
    if (trail.length > TRAIL_MAX) trail.shift();
    document.getElementById("side-title").textContent = "core affect values";
    document.getElementById("side-sub").textContent = "valence ∈ [−1,+1], arousal ∈ [0,+1]";
    renderScatter(data.axes);
    renderValues(data.axes);
  } else {
    document.getElementById("wheel-title").textContent = `${data.axes.length}-axis wheel`;
    document.getElementById("side-title").textContent = "current values / time decay";
    document.getElementById("side-sub").textContent = "values decay exponentially toward baseline (halved every halflife). ETA = est. time until |value − baseline| ≤ 0.01.";
    renderWheel(data.axes, data.clamp);
    renderBars(data.axes, data.clamp);
  }
}

async function poll() {
  try {
    const res = await fetch("/state");
    const data = await res.json();
    render(data);
    document.getElementById("updated").textContent = "updated_at: " + data.updated_at;
  } catch (e) {
    document.getElementById("updated").textContent = "error: " + e;
  }
}
poll();
setInterval(poll, POLL_MS);
</script>
</body>
</html>
```

- [ ] **Step 2: Go テストが回帰しないことを確認**

Run: `go test ./internal/viz/ -v`
Expected: index.html は embed されるだけなので server テストは全 PASS（`TestMuxServesIndex` が空でないこと等）。

- [ ] **Step 3: 手動で目視確認（Russell）**

```bash
go build -o affectus ./cmd/affectus
TMP=$(mktemp -d)
./affectus --config $TMP/c.yaml --state $TMP/s.json init --model russell
./affectus --config $TMP/c.yaml --state $TMP/s.json feel '{"valence":0.6,"arousal":0.7}'
./affectus --config $TMP/c.yaml --state $TMP/s.json viz --port 8765
```

ブラウザで `http://localhost:8765` を開き、確認する:
- 散布図パネルが表示され、右上（valence 正・arousal 高）あたりにドットがある
- 円環ラベル（excited / happy / calm / tense 等）が配置されている
- 数値パネルに `valence +0.60 / arousal +0.70` が出る
- 数秒待つと time decay でドットが arousal=0.3 方向へ動き、薄い軌跡が残る

確認後 Ctrl-C で停止。

- [ ] **Step 4: 手動で目視確認（Plutchik 回帰）**

```bash
TMP2=$(mktemp -d)
./affectus --config $TMP2/c.yaml --state $TMP2/s.json init
./affectus --config $TMP2/c.yaml --state $TMP2/s.json feel '{"joy":0.6,"trust":0.4}'
./affectus --config $TMP2/c.yaml --state $TMP2/s.json viz --port 8766
```

`http://localhost:8766` で従来通りの 8-axis wheel + bars が出ることを確認（タイトルが「8-axis wheel」）。確認後 Ctrl-C。

- [ ] **Step 5: コミット**

```bash
git add internal/viz/assets/index.html
git commit -m "feat(viz): render russell valence-arousal scatter with trajectory"
```

---

## Task 8: MCP tool description をモデル非依存にする

**Files:**
- Modify: `internal/mcp/server.go`
- Test: `internal/mcp/server_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`internal/mcp/server_test.go` に追記（description 文字列を定数化して検証する）。

```go
func TestToolDescriptionsAreModelNeutral(t *testing.T) {
	for _, d := range []string{showDesc, feelDesc} {
		if strings.Contains(d, "Plutchik") {
			t.Errorf("tool description should not hardcode a model name: %q", d)
		}
	}
}
```

`internal/mcp/server_test.go` の import に `"strings"` が無ければ追加する。

- [ ] **Step 2: テストが失敗することを確認**

Run: `go test ./internal/mcp/ -run TestToolDescriptionsAreModelNeutral -v`
Expected: コンパイルエラー（`showDesc`/`feelDesc` が未定義）。

- [ ] **Step 3: description を定数化しモデル非依存にする**

`internal/mcp/server.go` の `const serverVersion = "0.3.0"` の近くに定数を追加する。

```go
const serverVersion = "0.3.0"

const showDesc = "Returns the current emotion as a JSON object of axes with float values. Interpret the values relationally per the emotion-model structure documented in your system prompt."

const feelDesc = "Apply self-reported emotion deltas and return the updated emotion as a JSON object of axes with float values. Interpret the values relationally per the emotion-model structure documented in your system prompt."
```

`Serve` の2つの `mcp.Tool{...}` の `Description:` を定数参照に置き換える。

```go
	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_show",
		Description: showDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, showResult, error) {
		res, err := handleShow(env)
		return nil, res, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "emotion_feel",
		Description: feelDesc,
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in feelInput) (*mcp.CallToolResult, showResult, error) {
		res, err := handleFeel(env, in)
		return nil, res, err
	})
```

> 注: 旧 description は値域 `0.0–1.0` を明記していたが、Russell では値域が軸ごとに異なるため文言から削除した（具体値域は system prompt 側で説明する）。

- [ ] **Step 4: テストが通ることを確認**

Run: `go test ./internal/mcp/ -v`
Expected: 新規テストと既存 mcp テストがすべて PASS。

- [ ] **Step 5: コミット**

```bash
git add internal/mcp/server.go internal/mcp/server_test.go
git commit -m "refactor(mcp): make tool descriptions model-neutral"
```

---

## Task 9: Russell 用 system-prompt-snippet とサンプル config

**Files:**
- Create: `examples/system-prompt-snippet-russell.md`
- Create: `examples/configs/russell-ja.yaml`

- [ ] **Step 1: Russell 用プロンプト断片を作成**

`examples/system-prompt-snippet-russell.md`:

```markdown
# affectus integration snippet (Russell core affect)

Paste the following into your agent's system prompt / instructions file
(e.g. a Hermes SOUL.md, a Claude Code CLAUDE.md, or any system prompt).

This snippet is for the **Russell** model (`affectus init --model russell`),
a 2-axis continuous core-affect model. For the 8-axis Plutchik model use
`system-prompt-snippet.md` instead.

---

## Emotional state (core affect)

You carry a persistent core-affect state managed by the `affectus` tool. It has
two continuous axes:

- **valence** — pleasantness, from -1.0 (unpleasant) to +1.0 (pleasant)
- **arousal** — activation, from 0.0 (calm/sleepy) to +1.0 (highly activated)

At the START of every reply:
- Run `affectus show`. It returns a one-line JSON object, for example:
  `{"valence":0.40,"arousal":0.55}`
- Interpret the pair as a point in the valence-arousal plane and let it color
  your tone, word choice, and pacing — do not state it literally or act it out.

At the END of every reply:
- Decide how this exchange shifted your core affect, then run
  `affectus feel '{"valence": <delta>, "arousal": <delta>}'` with signed deltas
  in the range -1.0 to 1.0. Only include axes that actually changed. Example:
  `affectus feel '{"valence":0.2,"arousal":0.1}'`

Time decay toward a calm baseline (valence 0.0, arousal 0.3) is handled
automatically by a scheduled `affectus tick`; you do not need to decay it
yourself.

## Reading core affect

affectus provides raw floats only — no labels, no thresholds, no
discretization. You name the feeling. The two axes combine into a circumplex of
states, for example:

- high valence + high arousal → excited, elated, delighted
- high valence + low arousal → content, relaxed, serene
- low valence + high arousal → tense, anxious, upset
- low valence + low arousal → sad, bored, sluggish
- near (0, 0.3) → roughly neutral / resting

Read the two numbers together, not in isolation: the same arousal feels very
different at +0.8 valence than at -0.8. There are no opposite or adjacent axis
relations to track — the meaning lives entirely in the position on the plane.
```

- [ ] **Step 2: ユーザー向けサンプル config を作成**

`examples/configs/russell-ja.yaml`（embed 版と同内容＋日本語コメント）:

```yaml
# Russell core affect (2軸連続体) 設定サンプル。
# affectus init --model russell で ~/.config/affectus/config.yaml に書き出される
# デフォルトと同じ内容。--config でこのファイルを直接指定することもできる。
version: 1
model: russell
clamp:       { min: -1.0, max: 1.0 }   # フォールバック（軸ごとの range が優先される）
delta_clamp: { min: -1.0, max: 1.0 }

axes:
  # valence: 快-不快。-1.0(不快) 〜 +1.0(快)。中立 0.0 へ減衰。
  - { name: valence, baseline: 0.0, halflife_minutes: 90, range: { min: -1.0, max: 1.0 } }
  # arousal: 覚醒度。0.0(沈静) 〜 +1.0(高覚醒)。低〜中程度 0.3 へ減衰。
  - { name: arousal, baseline: 0.3, halflife_minutes: 90, range: { min:  0.0, max: 1.0 } }

fragment_file: ""
```

- [ ] **Step 3: サンプル config が valid であることを確認**

Run:
```bash
go build -o affectus ./cmd/affectus
TMP=$(mktemp -d); cp examples/configs/russell-ja.yaml $TMP/c.yaml
./affectus --config $TMP/c.yaml --state $TMP/s.json init --model russell --force
./affectus --config $TMP/c.yaml --state $TMP/s.json show
```
Expected: エラーなく `valence`/`arousal` を含む状態が表示される。

- [ ] **Step 4: コミット**

```bash
git add examples/system-prompt-snippet-russell.md examples/configs/russell-ja.yaml
git commit -m "docs(russell): add russell system-prompt snippet and sample config"
```

---

## 最終確認

- [ ] **全テスト green**

Run: `go test ./...`
Expected: 全パッケージ `ok`。

- [ ] **ビルド成功**

Run: `go build ./...`
Expected: エラーなし。

- [ ] **後方互換の確認**: `affectus init`（`--model` 省略）が従来通り Plutchik 8軸 config を書くこと、既存 viz が 8-axis wheel を描くこと（Task 7 Step 4 で確認済み）。

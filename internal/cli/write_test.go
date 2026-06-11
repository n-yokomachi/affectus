package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFeelAppliesDeltas(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Feel(env, `{"joy":0.6,"anticipation":0.3}`); err != nil {
		t.Fatalf("Feel: %v", err)
	}
	_, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if axes["joy"] < 0.59 || axes["joy"] > 0.61 {
		t.Errorf("joy = %v, want ~0.6", axes["joy"])
	}
}

func TestFeelInvalidJSON(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Feel(env, `{not json`); err == nil {
		t.Fatal("invalid JSON should error")
	}
}

func TestFeelUnknownAxis(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Feel(env, `{"glee":0.5}`); err == nil {
		t.Fatal("unknown axis should error")
	}
}

func TestTickDecaysOverTime(t *testing.T) {
	env, _ := testEnv(t)
	base := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return base }
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Feel(env, `{"joy":0.8}`); err != nil {
		t.Fatalf("Feel: %v", err)
	}
	// advance the clock by one halflife (90 min) and tick
	env.Now = func() time.Time { return base.Add(90 * time.Minute) }
	if err := Tick(env); err != nil {
		t.Fatalf("Tick: %v", err)
	}
	_, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if axes["joy"] < 0.39 || axes["joy"] > 0.41 {
		t.Errorf("joy after one halflife = %v, want ~0.4", axes["joy"])
	}
}

func TestResetReturnsToBaseline(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Feel(env, `{"anger":0.7}`); err != nil {
		t.Fatalf("Feel: %v", err)
	}
	if err := Reset(env); err != nil {
		t.Fatalf("Reset: %v", err)
	}
	_, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if axes["anger"] != 0.0 {
		t.Errorf("anger after reset = %v, want 0.0", axes["anger"])
	}
}

func TestFeelWritesFragmentFile(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// point fragment_file at a temp path by rewriting the config
	fragPath := filepath.Join(filepath.Dir(env.ConfigPath), "fragment.txt")
	cfgData, _ := os.ReadFile(env.ConfigPath)
	updated := []byte(string(cfgData) + "\n")
	// replace the empty fragment_file line
	if err := os.WriteFile(env.ConfigPath, []byte(replaceFragmentFile(string(updated), fragPath)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Feel(env, `{"joy":0.5}`); err != nil {
		t.Fatalf("Feel: %v", err)
	}
	if _, err := os.Stat(fragPath); err != nil {
		t.Fatalf("fragment file not written: %v", err)
	}
	data, err := os.ReadFile(fragPath)
	if err != nil {
		t.Fatalf("read fragment file: %v", err)
	}
	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "{") || !strings.HasSuffix(content, "}") {
		t.Errorf("fragment file should contain JSON, got %q", content)
	}
	if !strings.Contains(content, `"joy"`) {
		t.Errorf("fragment file missing joy axis: %q", content)
	}
}

// replaceFragmentFile rewrites the fragment_file value in a config string.
func replaceFragmentFile(cfg, path string) string {
	out := ""
	for _, line := range splitLines(cfg) {
		if len(line) >= 13 && line[:13] == "fragment_file" {
			out += "fragment_file: \"" + path + "\"\n"
			continue
		}
		out += line + "\n"
	}
	return out
}

func splitLines(s string) []string {
	var lines []string
	cur := ""
	for _, r := range s {
		if r == '\n' {
			lines = append(lines, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

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

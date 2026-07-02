package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestComputeShowAfterInit(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	axesJSON, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	// axesJSON should be a one-line JSON object
	if !strings.HasPrefix(axesJSON, "{") || !strings.HasSuffix(axesJSON, "}") {
		t.Errorf("axesJSON not a JSON object: %q", axesJSON)
	}
	if len(axes) != 8 {
		t.Errorf("expected 8 axes, got %d", len(axes))
	}
}

func TestShowTextFormat(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out.Reset()
	if err := Show(env, "text"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	// text output should be the one-line JSON, all zeros at init
	line := strings.TrimSpace(out.String())
	if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
		t.Errorf("text output not a JSON object: %q", line)
	}
	if !strings.Contains(line, `"joy":0.00`) {
		t.Errorf("text output missing joy:0.00: %q", line)
	}
}

func TestShowJSONFormat(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out.Reset()
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	var parsed struct {
		Axes map[string]float64 `json:"axes"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if len(parsed.Axes) != 8 {
		t.Errorf("json output incomplete: %+v", parsed)
	}
}

func TestShowJSONFormatNoFragmentKey(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out.Reset()
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if strings.Contains(out.String(), `"fragment"`) {
		t.Errorf("json output must not contain fragment key: %s", out.String())
	}
}

func TestShowUnknownFormat(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Show(env, "xml"); err == nil {
		t.Fatal("unknown format should error")
	}
}

func TestShowJSONFormatOCCIncludesProspects(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "occ", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Appraise(env, `{"consequence":{"desirability":0.6,"likelihood":0.5,"label":"pr merge"}}`); err != nil {
		t.Fatalf("Appraise: %v", err)
	}
	out.Reset()
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, `"prospects"`) {
		t.Errorf("occ json output missing prospects: %s", got)
	}
	if !strings.Contains(got, `"p1"`) {
		t.Errorf("occ json output missing p1: %s", got)
	}
}

func TestShowJSONFormatPlutchikNoProspects(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out.Reset()
	if err := Show(env, "json"); err != nil {
		t.Fatalf("Show: %v", err)
	}
	if strings.Contains(out.String(), "prospects") {
		t.Errorf("plutchik json output must not contain prospects: %s", out.String())
	}
}

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

func TestGetOutputsAxesJSON(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, "plutchik", false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	out.Reset()
	if err := Get(env); err != nil {
		t.Fatalf("Get: %v", err)
	}
	var axes map[string]float64
	if err := json.Unmarshal(out.Bytes(), &axes); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if _, ok := axes["joy"]; !ok {
		t.Errorf("axes missing joy: %+v", axes)
	}
}

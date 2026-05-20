package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestComputeShowAfterInit(t *testing.T) {
	env, _ := testEnv(t)
	if err := Init(env, false); err != nil {
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
	if err := Init(env, false); err != nil {
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
	if err := Init(env, false); err != nil {
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
	if err := Init(env, false); err != nil {
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
	if err := Init(env, false); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := Show(env, "xml"); err == nil {
		t.Fatal("unknown format should error")
	}
}

func TestGetOutputsAxesJSON(t *testing.T) {
	env, out := testEnv(t)
	if err := Init(env, false); err != nil {
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

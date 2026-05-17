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
	fragment, axes, err := ComputeShow(env)
	if err != nil {
		t.Fatalf("ComputeShow: %v", err)
	}
	if fragment != "Right now you feel calm and even." {
		t.Errorf("fragment = %q", fragment)
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
	if !strings.Contains(out.String(), "calm and even") {
		t.Errorf("text output = %q", out.String())
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
		Fragment string             `json:"fragment"`
		Axes     map[string]float64 `json:"axes"`
	}
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out.String())
	}
	if parsed.Fragment == "" || len(parsed.Axes) != 8 {
		t.Errorf("json output incomplete: %+v", parsed)
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

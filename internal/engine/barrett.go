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

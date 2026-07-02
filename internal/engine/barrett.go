package engine

import (
	"fmt"
	"math"
	"sort"
	"time"
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

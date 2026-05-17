package engine

import (
	"sort"
	"strings"
)

// Render translates the emotion vector into a natural-language fragment for
// prompt injection. Axes at or above render.threshold are sorted descending,
// truncated to render.max_axes, and each is described with a band adverb and
// its configured phrase. If no axis qualifies, render.empty is returned.
func Render(s State, cfg Config) string {
	type scored struct {
		name string
		val  float64
	}
	var list []scored
	for _, ax := range cfg.Axes {
		v := s.Axes[ax.Name]
		if v >= cfg.Render.Threshold {
			list = append(list, scored{ax.Name, v})
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].val > list[j].val })
	if len(list) > cfg.Render.MaxAxes {
		list = list[:cfg.Render.MaxAxes]
	}
	if len(list) == 0 {
		return cfg.Render.Empty
	}
	clauses := make([]string, 0, len(list))
	for _, sc := range list {
		label := bandLabel(sc.val, cfg.Render.Bands)
		clauses = append(clauses, label+cfg.Render.AxisPhrases[sc.name])
	}
	joined := joinClauses(clauses, cfg.Render.Conjunction, cfg.Render.Separator)
	return strings.Replace(cfg.Render.Template, "{clauses}", joined, 1)
}

// bandLabel returns the label of the first band whose Max is > v. Bands are
// assumed ascending by Max (validated at config load). The boundary value
// itself belongs to the next (higher) band, so the last band catches v == max.
func bandLabel(v float64, bands []Band) string {
	for _, b := range bands {
		if v < b.Max {
			return b.Label
		}
	}
	return bands[len(bands)-1].Label
}

// joinClauses joins clauses using the configured separator between non-final
// clauses and the conjunction before the final clause. English uses
// separator ", " and conjunction " and "; Japanese uses "、" and "と".
func joinClauses(c []string, conjunction, separator string) string {
	switch len(c) {
	case 0:
		return ""
	case 1:
		return c[0]
	case 2:
		return c[0] + conjunction + c[1]
	default:
		return strings.Join(c[:len(c)-1], separator) + conjunction + c[len(c)-1]
	}
}

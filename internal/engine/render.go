package engine

import (
	"fmt"
	"strings"
)

// Render returns the state as a one-line JSON of all axes in config
// order. v0.3: no thresholds, no salience filtering, no band labels —
// verbalization is the LLM's job per the project's core design
// principle. The output is suitable for direct injection into a system
// prompt or wrapping into a user message.
//
// Axis names must not contain JSON-special characters (quotes, backslashes,
// control chars); they are interpolated into the output without escaping.
func Render(s State, cfg Config) string {
	parts := make([]string, 0, len(cfg.Axes))
	for _, ax := range cfg.Axes {
		v := s.Axes[ax.Name]
		parts = append(parts, fmt.Sprintf(`"%s":%.2f`, ax.Name, v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

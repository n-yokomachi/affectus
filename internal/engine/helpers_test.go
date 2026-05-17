package engine

import "math"

// almostEqual reports whether two floats are equal within a small epsilon.
// Shared by all engine test files.
func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

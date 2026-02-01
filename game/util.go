package game

import "math"

// normalizeAngle wraps angle to [-pi, pi] with single-step correction.
// Safe when angle changes are bounded (heading += small_delta per tick).
func normalizeAngle(a float32) float32 {
	if a > math.Pi {
		a -= 2 * math.Pi
	} else if a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// mod returns positive modulo (Go's % can return negative).
func mod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
}

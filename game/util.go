package game

import "math"

// normalizeAngle wraps angle to [-pi, pi].
func normalizeAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// mod returns positive modulo (Go's % can return negative).
func mod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
}

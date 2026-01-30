package systems

import "math"

// Clamp functions for common value ranges

// clampFloat clamps a float32 value between min and max.
func clampFloat(v, minVal, maxVal float32) float32 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

// clamp01 clamps a float32 value to the [0, 1] range.
func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Angle normalization functions

// normalizeAngle wraps an angle to [-Pi, Pi].
func normalizeAngle(angle float32) float32 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// normalizeHeading wraps a heading to [0, 2*Pi].
func normalizeHeading(h float32) float32 {
	const twoPi = 2 * math.Pi
	for h < 0 {
		h += twoPi
	}
	for h >= twoPi {
		h -= twoPi
	}
	return h
}

// Distance functions

// distanceSq returns the squared distance between two points.
func distanceSq(x1, y1, x2, y2 float32) float32 {
	dx := x1 - x2
	dy := y1 - y2
	return dx*dx + dy*dy
}

// distance returns the Euclidean distance between two points.
func distance(x1, y1, x2, y2 float32) float32 {
	return float32(math.Sqrt(float64(distanceSq(x1, y1, x2, y2))))
}

// velocityMagnitude returns the magnitude of a velocity vector.
func velocityMagnitude(vx, vy float32) float32 {
	return float32(math.Sqrt(float64(vx*vx + vy*vy)))
}

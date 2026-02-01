package game

import "math"

// Fast math functions for hot-path physics calculations.
// These avoid float32->float64 conversions that Go's math package requires.

// fastSin approximates sin(x) using a polynomial. Accurate to ~0.001 for all x.
func fastSin(x float32) float32 {
	// Normalize to [-π, π]
	x = normalizeAngle(x)
	// Parabola approximation with correction factor
	const pi = math.Pi
	const pi2 = pi * pi
	ax := x
	if ax < 0 {
		ax = -ax
	}
	y := 4 * x * (pi - ax) / pi2
	// Correction: improves accuracy
	return 0.225*(y*absf(y)-y) + y
}

// fastCos approximates cos(x) using fastSin.
func fastCos(x float32) float32 {
	return fastSin(x + math.Pi/2)
}

// fastExp approximates exp(x) for x in [-4, 4].
func fastExp(x float32) float32 {
	if x > 4 {
		return 54.6 // exp(4) ≈ 54.6
	}
	if x < -4 {
		return 0
	}
	// Padé approximation
	x2 := x * x
	return (12 + 6*x + x2) / (12 - 6*x + x2)
}

// fastSqrt approximates sqrt(x) using fast inverse sqrt.
func fastSqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	i := math.Float32bits(x)
	i = 0x5f375a86 - (i >> 1)
	y := math.Float32frombits(i)
	y = y * (1.5 - 0.5*x*y*y)
	return x * y
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

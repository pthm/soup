package systems

import "math"

// ResourceField represents a continuous food source on a torus.
// Implemented as a sum of Gaussian hotspots.
type ResourceField struct {
	centers []hotspot
	sigma   float32
	width   float32
	height  float32
}

type hotspot struct {
	X, Y float32
}

// RNG is the interface for random number generation.
type RNG interface {
	Float32() float32
}

// NewResourceField creates a resource field with random hotspots.
func NewResourceField(w, h float32, numHotspots int, rng RNG) *ResourceField {
	// Use min dimension for sigma to handle non-square worlds
	minDim := float32(math.Min(float64(w), float64(h)))

	rf := &ResourceField{
		centers: make([]hotspot, numHotspots),
		sigma:   minDim * 0.08,
		width:   w,
		height:  h,
	}

	for i := range rf.centers {
		rf.centers[i].X = rng.Float32() * w
		rf.centers[i].Y = rng.Float32() * h
	}

	return rf
}

// Sample returns the resource density at (x, y) in [0, 1].
// Uses smooth saturation to preserve gradient information.
func (rf *ResourceField) Sample(x, y float32) float32 {
	var sum float32
	sigma2 := 2 * rf.sigma * rf.sigma

	for _, c := range rf.centers {
		dx, dy := ToroidalDelta(x, y, c.X, c.Y, rf.width, rf.height)
		d2 := dx*dx + dy*dy
		sum += float32(math.Exp(-float64(d2) / float64(sigma2)))
	}

	// Smooth saturation: 1 - exp(-sum) maps [0,∞) → [0,1)
	return 1 - float32(math.Exp(-float64(sum)))
}

// Width returns the field width.
func (rf *ResourceField) Width() float32 {
	return rf.width
}

// Height returns the field height.
func (rf *ResourceField) Height() float32 {
	return rf.height
}

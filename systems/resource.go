package systems

// ResourceSampler provides O(1) resource density sampling.
// Implemented by CPUResourceField and GPUResourceField backends.
type ResourceSampler interface {
	Sample(x, y float32) float32
	Width() float32
	Height() float32
}

// DepletableResourceSampler extends ResourceSampler with depletion and evolution.
type DepletableResourceSampler interface {
	ResourceSampler

	// Graze removes resource and returns the amount actually removed.
	Graze(x, y float32, rate, dt float32, radiusCells int) float32

	// Step advances regrowth and diffusion by dt seconds.
	Step(dt float32, evolve bool)

	// ResData returns the current resource grid for visualization/serialization.
	ResData() []float32

	// GridSize returns the grid dimensions.
	GridSize() (int, int)
}

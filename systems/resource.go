package systems

// ResourceSampler provides resource field access for the simulation.
// Implemented by ParticleResourceField.
type ResourceSampler interface {
	// Sample returns the resource density at world coordinates.
	Sample(x, y float32) float32

	// Width returns the world width.
	Width() float32

	// Height returns the world height.
	Height() float32

	// Graze removes resource and returns the amount actually removed.
	Graze(x, y float32, rate, dt float32, radiusCells int) float32

	// Step advances the resource field by dt seconds.
	Step(dt float32, evolve bool)

	// ResData returns the current resource grid for visualization/serialization.
	ResData() []float32

	// GridSize returns the grid dimensions.
	GridSize() (int, int)
}

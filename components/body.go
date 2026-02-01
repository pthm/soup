package components

// Body holds physical properties of an entity.
type Body struct {
	Radius float32
}

// Capabilities defines morphology knobs for an entity.
type Capabilities struct {
	VisionRange float32 // perception distance
	FOV         float32 // field of view in radians
	MaxSpeed    float32 // maximum velocity magnitude
	MaxAccel    float32 // maximum acceleration
	MaxTurnRate float32 // maximum turn rate (rad/s)
	Drag        float32 // velocity damping factor
	BiteRange   float32 // attack distance (predators)
	BiteCost    float32 // energy cost per bite
}

// DefaultCapabilities returns baseline capability values.
func DefaultCapabilities() Capabilities {
	return Capabilities{
		VisionRange: 120,
		FOV:         2.4, // ~140 degrees
		MaxSpeed:    80,
		MaxAccel:    180,
		MaxTurnRate: 3.5,
		Drag:        1.2,
		BiteRange:   10,
		BiteCost:    0.02,
	}
}

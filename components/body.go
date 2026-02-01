package components

// Body holds physical properties of an entity.
type Body struct {
	Radius float32 `inspect:"label,fmt:%.1f"`
}

// Capabilities defines morphology knobs for an entity.
type Capabilities struct {
	VisionRange float32 `inspect:"bar,max:200"` // perception distance
	FOV         float32 `inspect:"angle"`       // field of view in radians
	MaxSpeed    float32 `inspect:"bar,max:200"` // maximum velocity magnitude
	MaxAccel    float32 `inspect:"skip"`        // maximum acceleration
	MaxTurnRate float32 `inspect:"skip"`        // maximum turn rate (rad/s)
	Drag        float32 `inspect:"skip"`        // velocity damping factor
	BiteRange   float32 `inspect:"skip"`        // attack distance (predators)
	BiteCost    float32 `inspect:"skip"`        // energy cost per bite
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

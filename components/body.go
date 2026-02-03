package components

import "github.com/pthm-cable/soup/config"

// Body holds physical properties of an entity.
type Body struct {
	Radius float32 `inspect:"label,fmt:%.1f"`
}

// Capabilities defines morphology knobs for an entity.
// All entities have 360° vision; effectiveness varies by angle and diet (see systems/sensors.go).
type Capabilities struct {
	VisionRange float32 `inspect:"bar,max:200"` // perception distance
	MaxSpeed    float32 `inspect:"bar,max:200"` // maximum velocity magnitude
	MaxAccel    float32 `inspect:"skip"`        // maximum acceleration
	MaxTurnRate float32 `inspect:"skip"`        // maximum turn rate (rad/s)
	Drag        float32 `inspect:"skip"`        // velocity damping factor
	BiteRange   float32 `inspect:"skip"`        // attack distance (predators)
	BiteCost    float32 `inspect:"skip"`        // energy cost per bite
}

// DefaultCapabilities returns baseline capability values from config, interpolating
// vision range between prey and predator settings based on diet (0=herbivore, 1=carnivore).
func DefaultCapabilities(diet float32) Capabilities {
	cfg := config.Cfg().Capabilities
	preyVR := float32(cfg.Prey.VisionRange)
	predVR := float32(cfg.Predator.VisionRange)
	visionRange := preyVR + (predVR-preyVR)*diet
	return Capabilities{
		VisionRange: visionRange,
		MaxSpeed:    float32(cfg.MaxSpeed),
		MaxAccel:    float32(cfg.MaxAccel),
		MaxTurnRate: float32(cfg.MaxTurnRate),
		Drag:        float32(cfg.Drag),
		BiteRange:   float32(cfg.BiteRange),
		BiteCost:    float32(cfg.BiteCost),
	}
}

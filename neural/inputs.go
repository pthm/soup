package neural

import (
	"math"
)

// SensoryInputs holds the raw sensory data before normalization.
// Phase 4b: Uses polar vision cones with intent-based outputs and terrain awareness.
type SensoryInputs struct {
	// Polar vision (Phase 3) - 4 cones × 3 channels = 12 inputs
	// Cones: [Front, Right, Back, Left]
	// Values are already normalized [0, 1] from PolarVision.NormalizeForBrain()
	ConeFood   [NumCones]float32 // Food intensity per direction
	ConeThreat [NumCones]float32 // Threat intensity per direction
	ConeFriend [NumCones]float32 // Friend (genetic similarity) intensity per direction

	// Environment
	LightLevel    float32 // 0-1 from shadowmap (ambient)
	FlowAlignment float32 // Phase 4: dot(flow_direction, heading) [-1, 1]
	Openness      float32 // Phase 4b: local free space (1.0 = open, 0 = dense terrain)

	// Directional light (Phase 3b)
	// Gradients indicate where light is brighter (-1 to +1)
	LightFB float32 // Front-back gradient: >0 means brighter ahead
	LightLR float32 // Left-right gradient: >0 means brighter to the right

	// Internal state
	Energy           float32
	MaxEnergy        float32
	CellCount        int
	MaxCells         int
	PerceptionRadius float32

	// Body awareness (sensor capability)
	SensorCount     int     // Number of active sensor cells
	TotalSensorGain float32 // Sum of sensor gains (affects perception quality)

	// Body awareness (actuator capability)
	ActuatorCount    int     // Number of active actuator cells
	TotalActuatorStr float32 // Sum of actuator strengths (affects movement)
}

// ToInputs converts sensory data to normalized neural network inputs.
// Phase 4b layout: 12 cone inputs + 5 environment + 2 light gradients = 19 total
//
// Input mapping:
//
//	[0-3]   ConeFood (front, right, back, left)
//	[4-7]   ConeThreat (front, right, back, left)
//	[8-11]  ConeFriend (front, right, back, left)
//	[12]    Energy ratio
//	[13]    Light level (ambient)
//	[14]    Flow alignment (Phase 4: dot(flow, heading))
//	[15]    Openness (Phase 4b: local free space)
//	[16]    Light gradient front-back (Phase 3b)
//	[17]    Light gradient left-right (Phase 3b)
//	[18]    Bias
func (s *SensoryInputs) ToInputs() []float64 {
	inputs := make([]float64, BrainInputs)

	// [0-3] Food cones (already normalized 0-1)
	for i := 0; i < NumCones; i++ {
		inputs[i] = float64(clampf(s.ConeFood[i], 0, 1))
	}

	// [4-7] Threat cones (already normalized 0-1)
	for i := 0; i < NumCones; i++ {
		inputs[4+i] = float64(clampf(s.ConeThreat[i], 0, 1))
	}

	// [8-11] Friend cones (already normalized 0-1)
	for i := 0; i < NumCones; i++ {
		inputs[8+i] = float64(clampf(s.ConeFriend[i], 0, 1))
	}

	// [12] Energy ratio
	if s.MaxEnergy > 0 {
		inputs[12] = float64(clampf(s.Energy/s.MaxEnergy, 0, 1))
	} else {
		inputs[12] = 0.5
	}

	// [13] Light level (already 0-1)
	inputs[13] = float64(clampf(s.LightLevel, 0, 1))

	// [14] Flow alignment (Phase 4) - range [-1, 1]
	// Positive = flow helping (pushing in direction of heading)
	// Negative = flow hindering (pushing against heading)
	inputs[14] = float64(clampf(s.FlowAlignment, -1, 1))

	// [15] Openness (Phase 4b) - range [0, 1]
	// 1.0 = clear open space, 0 = dense terrain nearby
	inputs[15] = float64(clampf(s.Openness, 0, 1))

	// [16-17] Light gradients (Phase 3b) - range [-1, 1]
	inputs[16] = float64(clampf(s.LightFB, -1, 1))
	inputs[17] = float64(clampf(s.LightLR, -1, 1))

	// [18] Bias (always 1.0)
	inputs[18] = 1.0

	return inputs
}

// FromPolarVision populates cone inputs from a PolarVision scan result.
// Normalizes the values for neural network input.
// Phase 3b: Also extracts directional light gradients.
func (s *SensoryInputs) FromPolarVision(pv *PolarVision) {
	normalized := pv.NormalizeForBrain()
	s.ConeFood = normalized.Food
	s.ConeThreat = normalized.Threat
	s.ConeFriend = normalized.Friend

	// Phase 3b: Extract light gradients
	s.LightFB, s.LightLR = pv.LightGradients()
}

// BehaviorOutputs holds the decoded outputs from the brain network.
// Phase 5: Intent-based system with 6 outputs.
type BehaviorOutputs struct {
	// Movement intent (Phase 4)
	DesireAngle    float32 // -π to +π: where to go relative to heading
	DesireDistance float32 // 0 to 1: urgency (0 = stay, 1 = max pursuit)

	// Action intents
	Eat   float32 // 0 to 1: feeding intent (>0.5 = try to eat)
	Grow  float32 // 0 to 1: growth intent (allocate energy to new cells)
	Breed float32 // 0 to 1: reproduction intent (>0.5 = try to reproduce)
	Glow  float32 // 0 to 1: bioluminescence intent (Phase 5b)
}

// DecodeOutputs converts raw network outputs to intent values.
// Raw outputs are in [0, 1] range from sigmoid activation.
// Phase 5 layout: [DesireAngle, DesireDistance, Eat, Grow, Breed, Glow]
func DecodeOutputs(raw []float64) BehaviorOutputs {
	if len(raw) < BrainOutputs {
		// Return defaults if not enough outputs
		return DefaultOutputs()
	}

	return BehaviorOutputs{
		// DesireAngle: sigmoid [0,1] -> [-π, π]
		DesireAngle: (float32(raw[0]) - 0.5) * 2.0 * math.Pi,
		// DesireDistance: sigmoid [0,1] -> [0,1]
		DesireDistance: float32(raw[1]),
		// Eat: sigmoid [0,1] -> [0,1]
		Eat: float32(raw[2]),
		// Grow: sigmoid [0,1] -> [0,1]
		Grow: float32(raw[3]),
		// Breed: sigmoid [0,1] -> [0,1]
		Breed: float32(raw[4]),
		// Glow: sigmoid [0,1] -> [0,1]
		Glow: float32(raw[5]),
	}
}

// DefaultOutputs returns sensible default outputs for organisms without brains
// or when brain evaluation fails.
func DefaultOutputs() BehaviorOutputs {
	return BehaviorOutputs{
		DesireAngle:    0.0, // No change in direction
		DesireDistance: 0.5, // Medium urgency
		Eat:            0.5, // Neutral eating intent
		Grow:           0.3, // Low growth intent
		Breed:          0.3, // Low breeding intent
		Glow:           0.0, // No glow by default (saves energy)
	}
}

func clampf(v, min, max float32) float32 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

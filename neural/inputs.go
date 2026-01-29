package neural

import (
	"math"
)

// SensoryInputs holds the raw sensory data before normalization.
// Phase 3: Uses polar vision cones instead of nearest-target sensing.
type SensoryInputs struct {
	// Polar vision (Phase 3) - 4 cones × 3 channels = 12 inputs
	// Cones: [Front, Right, Back, Left]
	// Values are already normalized [0, 1] from PolarVision.NormalizeForBrain()
	ConeFood   [NumCones]float32 // Food intensity per direction
	ConeThreat [NumCones]float32 // Threat intensity per direction
	ConeFriend [NumCones]float32 // Friend (genetic similarity) intensity per direction

	// Environment
	LightLevel float32 // 0-1 from shadowmap
	FlowX      float32 // Local flow field
	FlowY      float32

	// Terrain awareness (kept for Phase 5 pathfinding transition)
	TerrainDistance  float32 // Distance to nearest solid terrain
	TerrainGradientX float32 // Direction away from terrain (world X)
	TerrainGradientY float32 // Direction away from terrain (world Y)
	TerrainFound     bool    // True if terrain is within sensing range

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
// Phase 3 layout: 12 cone inputs + 5 environment/state inputs = 17 total
//
// Input mapping:
//
//	[0-3]   ConeFood (front, right, back, left)
//	[4-7]   ConeThreat (front, right, back, left)
//	[8-11]  ConeFriend (front, right, back, left)
//	[12]    Energy ratio
//	[13]    Light level
//	[14-15] Flow field X, Y
//	[16]    Bias
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

	// [14-15] Flow field (small values, clamp to [-1, 1])
	inputs[14] = float64(clampf(s.FlowX, -1, 1))
	inputs[15] = float64(clampf(s.FlowY, -1, 1))

	// [16] Bias (always 1.0)
	inputs[16] = 1.0

	return inputs
}

// FromPolarVision populates cone inputs from a PolarVision scan result.
// Normalizes the values for neural network input.
func (s *SensoryInputs) FromPolarVision(pv *PolarVision) {
	normalized := pv.NormalizeForBrain()
	s.ConeFood = normalized.Food
	s.ConeThreat = normalized.Threat
	s.ConeFriend = normalized.Friend
}

// BehaviorOutputs holds the decoded outputs from the brain network.
// This is the simplified 4-output direct control system.
type BehaviorOutputs struct {
	// Direct control outputs
	Turn   float32 // -1 to +1: heading adjustment (radians/tick)
	Thrust float32 // 0 to 1: forward speed multiplier
	Eat    float32 // 0 to 1: feeding intent (>0.5 = try to eat)
	Mate   float32 // 0 to 1: breeding intent (>0.5 = try to mate)
}

// DecodeOutputs converts raw network outputs to direct control values.
// Raw outputs are in [0, 1] range from sigmoid activation.
func DecodeOutputs(raw []float64) BehaviorOutputs {
	if len(raw) < BrainOutputs {
		// Return defaults if not enough outputs
		return DefaultOutputs()
	}

	return BehaviorOutputs{
		// Turn: sigmoid [0,1] -> tanh-like [-1,1]
		Turn: (float32(raw[0]) - 0.5) * 2.0,
		// Thrust: sigmoid [0,1] -> [0,1]
		Thrust: float32(raw[1]),
		// Eat: sigmoid [0,1] -> [0,1]
		Eat: float32(raw[2]),
		// Mate: sigmoid [0,1] -> [0,1]
		Mate: float32(raw[3]),
	}
}

// DefaultOutputs returns sensible default outputs for organisms without brains
// or when brain evaluation fails.
func DefaultOutputs() BehaviorOutputs {
	return BehaviorOutputs{
		Turn:   0.0, // No turning
		Thrust: 0.5, // Medium speed
		Eat:    0.5, // Neutral eating intent
		Mate:   0.3, // Low mating intent
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

// Legacy conversion helpers for gradual migration.
// These will be removed after Phase 4.

// LegacySensoryInputs holds the old-style sensory data for backward compatibility.
// Deprecated: Use SensoryInputs with polar vision instead.
type LegacySensoryInputs struct {
	// Food detection
	FoodDistance float32 // Distance to nearest food (0 if none)
	FoodAngle    float32 // Angle to food in radians
	FoodFound    bool

	// Predator detection
	PredatorDistance float32
	PredatorAngle    float32
	PredatorFound    bool

	// Mate detection
	MateDistance float32
	MateAngle    float32
	MateFound    bool

	// Social
	HerdCount int // Number of nearby herd members

	// Environment
	LightLevel float32 // 0-1 from shadowmap
	FlowX      float32 // Local flow field
	FlowY      float32

	// Terrain awareness
	TerrainDistance  float32 // Distance to nearest solid terrain
	TerrainGradientX float32 // Direction away from terrain (world X)
	TerrainGradientY float32 // Direction away from terrain (world Y)
	TerrainFound     bool    // True if terrain is within sensing range

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

// ToInputs converts legacy sensory data to neural network inputs.
// Deprecated: Use SensoryInputs.ToInputs() with polar vision instead.
func (s *LegacySensoryInputs) ToInputs() []float64 {
	inputs := make([]float64, BrainInputs)

	// [0-2] Food: distance (normalized), angle sin, angle cos
	if s.FoodFound && s.PerceptionRadius > 0 {
		inputs[0] = float64(clampf(s.FoodDistance/s.PerceptionRadius, 0, 1))
		inputs[1] = math.Sin(float64(s.FoodAngle))
		inputs[2] = math.Cos(float64(s.FoodAngle))
	} else {
		inputs[0] = 1.0 // No food visible = max distance
		inputs[1] = 0.0
		inputs[2] = 0.0
	}

	// [3-5] Predator: distance (normalized), angle sin, angle cos
	if s.PredatorFound && s.PerceptionRadius > 0 {
		// Predator detection range is 1.5x perception
		inputs[3] = float64(clampf(s.PredatorDistance/(s.PerceptionRadius*1.5), 0, 1))
		inputs[4] = math.Sin(float64(s.PredatorAngle))
		inputs[5] = math.Cos(float64(s.PredatorAngle))
	} else {
		inputs[3] = 1.0 // No predator visible = max distance
		inputs[4] = 0.0
		inputs[5] = 0.0
	}

	// [6] Mate distance
	if s.MateFound && s.PerceptionRadius > 0 {
		inputs[6] = float64(clampf(s.MateDistance/s.PerceptionRadius, 0, 1))
	} else {
		inputs[6] = 1.0
	}

	// [7] Herd density (normalized, assume max 10 nearby)
	inputs[7] = float64(clampf(float32(s.HerdCount)/10.0, 0, 1))

	// [8] Light level (already 0-1)
	inputs[8] = float64(clampf(s.LightLevel, 0, 1))

	// [9-10] Flow field (already small values, just pass through)
	inputs[9] = float64(clampf(s.FlowX, -1, 1))
	inputs[10] = float64(clampf(s.FlowY, -1, 1))

	// [11] Energy ratio
	if s.MaxEnergy > 0 {
		inputs[11] = float64(clampf(s.Energy/s.MaxEnergy, 0, 1))
	} else {
		inputs[11] = 0.5
	}

	// [12] Sensor capability (normalized: 0 = no sensors, 1 = 4+ sensor gain)
	// This gives the brain awareness of its perceptual capacity
	inputs[12] = float64(clampf(s.TotalSensorGain/4.0, 0, 1))

	// [13-15] Terrain: distance (normalized), gradient X, gradient Y (heading-relative)
	if s.TerrainFound && s.PerceptionRadius > 0 {
		// Distance: 0 = touching terrain, 1 = far away
		inputs[13] = float64(clampf(s.TerrainDistance/s.PerceptionRadius, 0, 1))
		// Gradient is already in heading-relative coords from behavior system
		inputs[14] = float64(clampf(s.TerrainGradientX, -1, 1))
		inputs[15] = float64(clampf(s.TerrainGradientY, -1, 1))
	} else {
		inputs[13] = 1.0 // No terrain nearby = max distance
		inputs[14] = 0.0 // No gradient
		inputs[15] = 0.0
	}

	// [16] Bias (always 1.0)
	inputs[16] = 1.0

	return inputs
}

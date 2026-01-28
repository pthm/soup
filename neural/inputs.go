package neural

import (
	"math"
)

// SensoryInputs holds the raw sensory data before normalization.
type SensoryInputs struct {
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

	// Internal state
	Energy      float32
	MaxEnergy   float32
	CellCount   int
	MaxCells    int
	PerceptionRadius float32
}

// ToInputs converts sensory data to normalized neural network inputs.
// Returns a slice of BrainInputs (14) float64 values.
func (s *SensoryInputs) ToInputs() []float64 {
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

	// [12] Cell count (normalized by max cells)
	if s.MaxCells > 0 {
		inputs[12] = float64(clampf(float32(s.CellCount)/float32(s.MaxCells), 0, 1))
	} else {
		inputs[12] = 0.5
	}

	// [13] Bias (always 1.0)
	inputs[13] = 1.0

	return inputs
}

// BehaviorOutputs holds the decoded outputs from the brain network.
type BehaviorOutputs struct {
	// Steering weights (0-1 from sigmoid, scaled for behavior system)
	SeekFoodWeight float32
	FleeWeight     float32
	SeekMateWeight float32
	HerdWeight     float32
	WanderWeight   float32

	// Allocation preferences (raw values, pick highest)
	GrowDrive     float32
	BreedDrive    float32
	ConserveDrive float32
}

// DecodeOutputs converts raw network outputs to usable behavior weights.
// Raw outputs are in [0, 1] range from sigmoid activation.
func DecodeOutputs(raw []float64) BehaviorOutputs {
	if len(raw) < BrainOutputs {
		// Return defaults if not enough outputs
		return BehaviorOutputs{
			SeekFoodWeight: 1.5,
			FleeWeight:     3.0,
			SeekMateWeight: 0.5,
			HerdWeight:     1.2,
			WanderWeight:   0.4,
			GrowDrive:      0.5,
			BreedDrive:     0.5,
			ConserveDrive:  0.5,
		}
	}

	return BehaviorOutputs{
		// Scale sigmoid outputs to useful steering weight ranges
		SeekFoodWeight: float32(raw[0]) * 3.0,        // 0-3
		FleeWeight:     float32(raw[1]) * 5.0,        // 0-5
		SeekMateWeight: float32(raw[2]) * 2.0,        // 0-2
		HerdWeight:     float32(raw[3]) * 2.5,        // 0-2.5
		WanderWeight:   float32(raw[4]) * 1.0,        // 0-1

		// Allocation drives (raw 0-1 values)
		GrowDrive:     float32(raw[5]),
		BreedDrive:    float32(raw[6]),
		ConserveDrive: float32(raw[7]),
	}
}

// DefaultOutputs returns sensible default outputs for organisms without brains
// or when brain evaluation fails.
func DefaultOutputs() BehaviorOutputs {
	return BehaviorOutputs{
		SeekFoodWeight: 1.5,
		FleeWeight:     3.0,
		SeekMateWeight: 0.5,
		HerdWeight:     1.2,
		WanderWeight:   0.4,
		GrowDrive:      0.4,
		BreedDrive:     0.3,
		ConserveDrive:  0.3,
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

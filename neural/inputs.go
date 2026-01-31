package neural

import (
	"math"
)

// BodyDescriptor holds normalized body capability metrics.
// These let brains learn policies based on their body type.
type BodyDescriptor struct {
	SizeNorm      float32 // bodyRadius / MaxBodySize [0,1]
	SpeedCapacity float32 // actuatorWeight / (actuatorWeight + drag) [0,1]
	AgilityNorm   float32 // 1 / (1 + drag) [0,1]
	SenseStrength float32 // sensorWeight / MaxSensorWeight [0,1]
	BiteStrength  float32 // mouthSize / MaxMouthSize [0,1]
	ArmorLevel    float32 // structuralArmor [0,1]
}

// BoidFields holds aggregated same-species boid field inputs.
// All directional values are in agent-local frame (fwd/up).
type BoidFields struct {
	CohesionFwd   float32 // Weighted center of mass direction (forward component) [-1,1]
	CohesionUp    float32 // Weighted center of mass direction (lateral component) [-1,1]
	CohesionMag   float32 // Weighted center of mass distance (normalized) [0,1]
	AlignmentFwd  float32 // Average neighbor heading (forward component) [-1,1]
	AlignmentUp   float32 // Average neighbor heading (lateral component) [-1,1]
	SeparationFwd float32 // Repulsion direction (forward component) [-1,1]
	SeparationUp  float32 // Repulsion direction (lateral component) [-1,1]
	SeparationMag float32 // Separation urgency [0,1]
	DensitySame   float32 // Local same-species density [0,1]
}

// FoodFields holds aggregated food attraction fields.
// Plant and meat fields are computed with diet compatibility weighting.
type FoodFields struct {
	PlantFwd float32 // Plant attraction direction (forward) [-1,1]
	PlantUp  float32 // Plant attraction direction (lateral) [-1,1]
	PlantMag float32 // Plant attraction strength [0,1]
	MeatFwd  float32 // Meat attraction direction (forward) [-1,1]
	MeatUp   float32 // Meat attraction direction (lateral) [-1,1]
	MeatMag  float32 // Meat attraction strength [0,1]
}

// ThreatInfo holds nearest predator/threat information.
type ThreatInfo struct {
	Proximity    float32 // Normalized distance to nearest threat [0,1] (0=far, 1=close)
	ClosingSpeed float32 // Rate of approach [-1,1] (negative=approaching, positive=retreating)
}

// SensoryInputs holds the raw sensory data before normalization.
// Layout: self (2) + body (6) + boid (9) + food (6) + threat (2) + bias (1) = 26 total
type SensoryInputs struct {
	// Self state (2 inputs)
	SpeedNorm  float32 // current speed / max speed [0,1]
	EnergyNorm float32 // energy / maxEnergy [0,1]

	// Body descriptor (6 inputs) - body-aware capability metrics
	Body BodyDescriptor

	// Boid fields (9 inputs) - same-species flocking
	Boid BoidFields

	// Food fields (6 inputs) - plant and meat attraction
	Food FoodFields

	// Threat info (2 inputs) - predator awareness
	Threat ThreatInfo

	// Metadata (not inputs)
	MaxSpeed         float32
	MaxEnergy        float32
	PerceptionRadius float32
}

// ToInputs converts sensory data to normalized neural network inputs.
// Layout: self (2) + body (6) + boid (9) + food (6) + threat (2) + bias (1) = 26 total
//
// Input mapping:
//
//	[0]     speed_norm [0,1]
//	[1]     energy_norm [0,1]
//	[2]     size_norm [0,1]
//	[3]     speed_capacity [0,1]
//	[4]     agility_norm [0,1]
//	[5]     sense_strength [0,1]
//	[6]     bite_strength [0,1]
//	[7]     armor_level [0,1]
//	[8]     cohesion_fwd [-1,1]
//	[9]     cohesion_up [-1,1]
//	[10]    cohesion_mag [0,1]
//	[11]    alignment_fwd [-1,1]
//	[12]    alignment_up [-1,1]
//	[13]    separation_fwd [-1,1]
//	[14]    separation_up [-1,1]
//	[15]    separation_mag [0,1]
//	[16]    density_same [0,1]
//	[17]    plant_fwd [-1,1]
//	[18]    plant_up [-1,1]
//	[19]    plant_mag [0,1]
//	[20]    meat_fwd [-1,1]
//	[21]    meat_up [-1,1]
//	[22]    meat_mag [0,1]
//	[23]    threat_proximity [0,1]
//	[24]    threat_closing_speed [-1,1]
//	[25]    bias (1.0)
func (s *SensoryInputs) ToInputs() []float64 {
	inputs := make([]float64, BrainInputs)

	// [0-1] Self state
	inputs[0] = float64(clampf(s.SpeedNorm, 0, 1))
	inputs[1] = float64(clampf(s.EnergyNorm, 0, 1))

	// [2-7] Body descriptor
	inputs[2] = float64(clampf(s.Body.SizeNorm, 0, 1))
	inputs[3] = float64(clampf(s.Body.SpeedCapacity, 0, 1))
	inputs[4] = float64(clampf(s.Body.AgilityNorm, 0, 1))
	inputs[5] = float64(clampf(s.Body.SenseStrength, 0, 1))
	inputs[6] = float64(clampf(s.Body.BiteStrength, 0, 1))
	inputs[7] = float64(clampf(s.Body.ArmorLevel, 0, 1))

	// [8-16] Boid fields
	inputs[8] = float64(clampf(s.Boid.CohesionFwd, -1, 1))
	inputs[9] = float64(clampf(s.Boid.CohesionUp, -1, 1))
	inputs[10] = float64(clampf(s.Boid.CohesionMag, 0, 1))
	inputs[11] = float64(clampf(s.Boid.AlignmentFwd, -1, 1))
	inputs[12] = float64(clampf(s.Boid.AlignmentUp, -1, 1))
	inputs[13] = float64(clampf(s.Boid.SeparationFwd, -1, 1))
	inputs[14] = float64(clampf(s.Boid.SeparationUp, -1, 1))
	inputs[15] = float64(clampf(s.Boid.SeparationMag, 0, 1))
	inputs[16] = float64(clampf(s.Boid.DensitySame, 0, 1))

	// [17-22] Food fields
	inputs[17] = float64(clampf(s.Food.PlantFwd, -1, 1))
	inputs[18] = float64(clampf(s.Food.PlantUp, -1, 1))
	inputs[19] = float64(clampf(s.Food.PlantMag, 0, 1))
	inputs[20] = float64(clampf(s.Food.MeatFwd, -1, 1))
	inputs[21] = float64(clampf(s.Food.MeatUp, -1, 1))
	inputs[22] = float64(clampf(s.Food.MeatMag, 0, 1))

	// [23-24] Threat info
	inputs[23] = float64(clampf(s.Threat.Proximity, 0, 1))
	inputs[24] = float64(clampf(s.Threat.ClosingSpeed, -1, 1))

	// [25] Bias
	inputs[25] = 1.0

	return inputs
}

// BehaviorOutputs holds the decoded outputs from the brain network.
// Simplified to 4 outputs: movement vector + action gates.
type BehaviorOutputs struct {
	// Movement as local velocity (body-limited by pathfinding layer)
	UFwd float32 // Desired forward velocity [-1,1]
	UUp  float32 // Desired lateral velocity [-1,1]

	// Action gates
	AttackIntent float32 // Predation gate [0,1], >0.5 = attack
	MateIntent   float32 // Mating gate [0,1], >0.5 = ready to mate

	// Legacy fields for compatibility (computed from UFwd/UUp)
	DesireAngle    float32 // Computed: angle from UFwd/UUp
	DesireDistance float32 // Computed: magnitude from UFwd/UUp
}

// DecodeOutputs converts raw network outputs to intent values.
// Raw outputs are in [0, 1] range from sigmoid activation.
// Layout: [UFwd, UUp, AttackIntent, MateIntent]
func DecodeOutputs(raw []float64) BehaviorOutputs {
	if len(raw) < BrainOutputs {
		return DefaultOutputs()
	}

	// Convert sigmoid [0,1] to [-1,1] for velocity outputs
	uFwd := float32(raw[0])*2.0 - 1.0
	uUp := float32(raw[1])*2.0 - 1.0

	// Compute legacy angle/distance from velocity
	angle, distance := LocalVelocityToDesire(uFwd, uUp)

	return BehaviorOutputs{
		UFwd:           uFwd,
		UUp:            uUp,
		AttackIntent:   float32(raw[2]),
		MateIntent:     float32(raw[3]),
		DesireAngle:    angle,
		DesireDistance: distance,
	}
}

// LocalVelocityToDesire converts local velocity (uFwd, uUp) to angle and distance.
// Used for compatibility with existing pathfinding layer.
func LocalVelocityToDesire(uFwd, uUp float32) (angle, distance float32) {
	if uFwd < -0.1 {
		// Backward: turn around
		angle = float32(math.Atan2(float64(-uUp), float64(-uFwd))) + math.Pi
		angle = normalizeAngleInputs(angle)
	} else {
		angle = float32(math.Atan2(float64(uUp), float64(uFwd)))
	}
	distance = clampf(float32(math.Sqrt(float64(uFwd*uFwd+uUp*uUp))), 0, 1)
	return angle, distance
}

// normalizeAngleInputs wraps angle to [-π, π].
func normalizeAngleInputs(angle float32) float32 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// DefaultOutputs returns sensible default outputs for organisms without brains
// or when brain evaluation fails.
func DefaultOutputs() BehaviorOutputs {
	return BehaviorOutputs{
		UFwd:           0.0,
		UUp:            0.0,
		AttackIntent:   0.0,
		MateIntent:     0.0,
		DesireAngle:    0.0,
		DesireDistance: 0.0,
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

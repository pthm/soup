package neural

import (
	"math"
)

// Capability matching constants
const (
	CompositionEpsilon = 1e-6 // Avoid division by zero in composition calculation
	DefaultBiteSize    = 0.05 // Fraction of target energy per bite
	FeedingEfficiency  = 0.8  // Energy transfer efficiency when feeding

	// CompatK is the power law exponent for nutrition rewards.
	// Higher values create sharper dietary niches:
	// - k=1: linear (current), generalists viable
	// - k=2: quadratic, specialists favored
	// - k=3-4: strong specialization pressure
	CompatK = 3.0
)

// OrganismCapabilities holds the computed capability values for an organism.
// These are derived from summing cell function strengths.
type OrganismCapabilities struct {
	PhotoWeight     float32 // Total photosynthetic strength
	ActuatorWeight  float32 // Total actuator strength
	SensorWeight    float32 // Total sensor strength
	MouthSize       float32 // Total mouth strength (for feeding)
	DigestiveSum    float32 // Sum of digestive cell strengths
	DigestiveCount  int     // Number of digestive cells
	StructuralArmor float32 // Average structural armor
	StorageCapacity float32 // Average storage capacity
}

// Edibility calculates how edible a target is to the eater.
// Returns 0-1 where higher means more edible.
// Based on how well the eater's digestive spectrum matches the target's composition.
func Edibility(eaterDigestiveSpectrum, targetComposition float32) float32 {
	// Herbivore (spectrum=0) wants flora (composition=1)
	// Carnivore (spectrum=1) wants fauna (composition=0)
	// So we need to invert: edibility = 1 - |spectrum - (1 - composition)|
	// Which simplifies to: edibility = 1 - |spectrum + composition - 1|
	diff := float32(math.Abs(float64(eaterDigestiveSpectrum + targetComposition - 1.0)))
	return clamp01(1.0 - diff)
}

// Penetration calculates the final feeding capability after armor.
// Returns 0-1 where higher means can feed more effectively.
func Penetration(edibility, targetArmor float32) float32 {
	return clamp01(edibility - targetArmor)
}

// NutritionMultiplier applies compat^k power law to penetration for reward calculation.
// This creates sharper dietary niches - specialists get much better nutrition returns.
// Used for actual energy rewards, not for determining if feeding is possible.
func NutritionMultiplier(penetration float32) float32 {
	if penetration <= 0 {
		return 0
	}
	return float32(math.Pow(float64(penetration), CompatK))
}

// ThreatLevel calculates how threatening another organism is to us.
// This is the inverse of Edibility - how well can THEY eat US.
func ThreatLevel(theirDigestiveSpectrum, myComposition, myArmor float32) float32 {
	ed := Edibility(theirDigestiveSpectrum, myComposition)
	return Penetration(ed, myArmor)
}

// clamp01 clamps a value to [0, 1].
func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

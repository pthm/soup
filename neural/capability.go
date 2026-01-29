package neural

import (
	"math"
)

// Capability matching constants
const (
	CompositionEpsilon = 1e-6 // Avoid division by zero in composition calculation
	DefaultBiteSize    = 0.05 // Fraction of target energy per bite
	FeedingEfficiency  = 0.8  // Energy transfer efficiency when feeding
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

// Composition returns the flora/fauna composition ratio.
// 1.0 = pure photosynthetic (flora-like)
// 0.0 = pure actuator (fauna-like)
// 0.5 = neutral or balanced
func (c *OrganismCapabilities) Composition() float32 {
	total := c.PhotoWeight + c.ActuatorWeight
	if total < CompositionEpsilon {
		return 0.5 // Neutral composition for organisms with neither
	}
	return c.PhotoWeight / total
}

// DigestiveSpectrum returns the average digestive spectrum across digestive cells.
// 0.0 = pure herbivore (eats flora)
// 0.5 = omnivore
// 1.0 = pure carnivore (eats fauna)
func (c *OrganismCapabilities) DigestiveSpectrum() float32 {
	if c.DigestiveCount == 0 {
		return 0.5 // Neutral if no digestive cells
	}
	return c.DigestiveSum / float32(c.DigestiveCount)
}

// ComputeCapabilities calculates capability totals from a list of cells.
// Cells should have the new primary/secondary type model.
func ComputeCapabilities(cells []CellSpec) OrganismCapabilities {
	var caps OrganismCapabilities
	var armorSum, storageSum float32
	cellCount := 0

	for _, cell := range cells {
		cellCount++

		// Accumulate function strengths
		caps.PhotoWeight += cell.GetFunctionStrength(CellTypePhotosynthetic)
		caps.ActuatorWeight += cell.GetFunctionStrength(CellTypeActuator)
		caps.SensorWeight += cell.GetFunctionStrength(CellTypeSensor)
		caps.MouthSize += cell.GetFunctionStrength(CellTypeMouth)

		// Digestive cells contribute to spectrum
		digestiveStr := cell.GetFunctionStrength(CellTypeDigestive)
		if digestiveStr > 0 {
			caps.DigestiveSum += cell.DigestiveSpectrum * digestiveStr
			caps.DigestiveCount++
		}

		// Modifiers
		armorSum += cell.StructuralArmor
		storageSum += cell.StorageCapacity
	}

	// Average modifiers
	if cellCount > 0 {
		caps.StructuralArmor = armorSum / float32(cellCount)
		caps.StorageCapacity = storageSum / float32(cellCount)
	}

	return caps
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

// CanFeed returns true if the eater can extract energy from the target.
// Requires positive penetration (edibility > armor).
func CanFeed(eaterDigestiveSpectrum, targetComposition, targetArmor float32) bool {
	ed := Edibility(eaterDigestiveSpectrum, targetComposition)
	return ed > targetArmor
}

// FeedingEffectiveness returns the feeding effectiveness (0-1).
// This determines how much of the bite is actually absorbed.
func FeedingEffectiveness(eaterDigestiveSpectrum, targetComposition, targetArmor float32) float32 {
	ed := Edibility(eaterDigestiveSpectrum, targetComposition)
	return Penetration(ed, targetArmor)
}

// ThreatLevel calculates how threatening another organism is to us.
// This is the inverse of Edibility - how well can THEY eat US.
func ThreatLevel(theirDigestiveSpectrum, myComposition, myArmor float32) float32 {
	ed := Edibility(theirDigestiveSpectrum, myComposition)
	return Penetration(ed, myArmor)
}

// IsThreat returns true if another organism poses a feeding threat.
func IsThreat(theirDigestiveSpectrum, myComposition, myArmor float32) bool {
	return ThreatLevel(theirDigestiveSpectrum, myComposition, myArmor) > 0
}

// IsFood returns true if the target is viable food for the eater.
func IsFood(eaterDigestiveSpectrum, targetComposition, targetArmor float32) bool {
	return CanFeed(eaterDigestiveSpectrum, targetComposition, targetArmor)
}

// FloraCapabilities returns capabilities for a standard flora entity.
// Flora have high photosynthetic weight and no actuator weight.
func FloraCapabilities(armor float32) OrganismCapabilities {
	return OrganismCapabilities{
		PhotoWeight:     1.0,
		ActuatorWeight:  0.0,
		StructuralArmor: armor,
	}
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

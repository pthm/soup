package neural

import (
	"math"
	"testing"
)

func TestComposition(t *testing.T) {
	tests := []struct {
		name     string
		photo    float32
		actuator float32
		expected float32
	}{
		{"pure flora", 1.0, 0.0, 1.0},
		{"pure fauna", 0.0, 1.0, 0.0},
		{"balanced", 0.5, 0.5, 0.5},
		{"mostly flora", 0.8, 0.2, 0.8},
		{"mostly fauna", 0.2, 0.8, 0.2},
		{"no cells (neutral)", 0.0, 0.0, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := OrganismCapabilities{
				PhotoWeight:    tt.photo,
				ActuatorWeight: tt.actuator,
			}
			got := caps.Composition()
			if math.Abs(float64(got-tt.expected)) > 0.01 {
				t.Errorf("Composition() = %.3f, want %.3f", got, tt.expected)
			}
		})
	}
}

func TestDigestiveSpectrum(t *testing.T) {
	tests := []struct {
		name     string
		sum      float32
		count    int
		expected float32
	}{
		{"herbivore", 0.0, 1, 0.0},
		{"carnivore", 1.0, 1, 1.0},
		{"omnivore", 0.5, 1, 0.5},
		{"mixed cells", 1.5, 3, 0.5},
		{"no digestive cells", 0.0, 0, 0.5}, // Neutral
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := OrganismCapabilities{
				DigestiveSum:   tt.sum,
				DigestiveCount: tt.count,
			}
			got := caps.DigestiveSpectrum()
			if math.Abs(float64(got-tt.expected)) > 0.01 {
				t.Errorf("DigestiveSpectrum() = %.3f, want %.3f", got, tt.expected)
			}
		})
	}
}

func TestEdibility(t *testing.T) {
	tests := []struct {
		name              string
		digestiveSpectrum float32
		composition       float32
		expected          float32
	}{
		// Herbivore (spectrum=0) eating flora (composition=1) = perfect match
		{"herbivore eats flora", 0.0, 1.0, 1.0},
		// Herbivore eating fauna (composition=0) = no match
		{"herbivore eats fauna", 0.0, 0.0, 0.0},
		// Carnivore (spectrum=1) eating fauna (composition=0) = perfect match
		{"carnivore eats fauna", 1.0, 0.0, 1.0},
		// Carnivore eating flora = no match
		{"carnivore eats flora", 1.0, 1.0, 0.0},
		// Omnivore (spectrum=0.5) eating flora = partial match
		{"omnivore eats flora", 0.5, 1.0, 0.5},
		// Omnivore eating fauna = partial match
		{"omnivore eats fauna", 0.5, 0.0, 0.5},
		// Omnivore eating balanced = perfect match
		{"omnivore eats balanced", 0.5, 0.5, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Edibility(tt.digestiveSpectrum, tt.composition)
			if math.Abs(float64(got-tt.expected)) > 0.01 {
				t.Errorf("Edibility(%.1f, %.1f) = %.3f, want %.3f",
					tt.digestiveSpectrum, tt.composition, got, tt.expected)
			}
		})
	}
}

func TestPenetration(t *testing.T) {
	tests := []struct {
		name       string
		edibility  float32
		armor      float32
		expected   float32
		canFeed    bool
	}{
		{"high edibility, no armor", 1.0, 0.0, 1.0, true},
		{"high edibility, some armor", 1.0, 0.3, 0.7, true},
		{"high edibility, heavy armor", 1.0, 0.8, 0.2, true},
		{"medium edibility, light armor", 0.5, 0.2, 0.3, true},
		{"low edibility, light armor", 0.3, 0.2, 0.1, true},
		{"armor exceeds edibility", 0.3, 0.5, 0.0, false},
		{"equal armor and edibility", 0.5, 0.5, 0.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Penetration(tt.edibility, tt.armor)
			if math.Abs(float64(got-tt.expected)) > 0.01 {
				t.Errorf("Penetration(%.1f, %.1f) = %.3f, want %.3f",
					tt.edibility, tt.armor, got, tt.expected)
			}
			// Also check CanFeed is consistent
			canFeed := got > 0
			if canFeed != tt.canFeed {
				t.Errorf("CanFeed mismatch: penetration=%.3f, canFeed=%v, want %v",
					got, canFeed, tt.canFeed)
			}
		})
	}
}

func TestCanFeed(t *testing.T) {
	tests := []struct {
		name              string
		digestiveSpectrum float32
		composition       float32
		armor             float32
		canFeed           bool
	}{
		{"herbivore can eat unarmored flora", 0.0, 1.0, 0.0, true},
		{"herbivore can eat lightly armored flora", 0.0, 1.0, 0.3, true},
		{"herbivore cannot eat heavily armored flora", 0.0, 1.0, 1.0, false},
		{"herbivore cannot eat fauna", 0.0, 0.0, 0.0, false},
		{"carnivore can eat unarmored fauna", 1.0, 0.0, 0.0, true},
		{"carnivore cannot eat flora", 1.0, 1.0, 0.0, false},
		{"omnivore can eat both", 0.5, 0.5, 0.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanFeed(tt.digestiveSpectrum, tt.composition, tt.armor)
			if got != tt.canFeed {
				t.Errorf("CanFeed(%.1f, %.1f, %.1f) = %v, want %v",
					tt.digestiveSpectrum, tt.composition, tt.armor, got, tt.canFeed)
			}
		})
	}
}

func TestThreatLevel(t *testing.T) {
	tests := []struct {
		name              string
		theirDigestive    float32
		myComposition     float32
		myArmor           float32
		expectedThreat    float32
		isThreat          bool
	}{
		// A carnivore (digestive=1) is a threat to fauna (composition=0)
		{"carnivore threatens unarmored fauna", 1.0, 0.0, 0.0, 1.0, true},
		{"carnivore threatens armored fauna", 1.0, 0.0, 0.5, 0.5, true},
		{"carnivore blocked by heavy armor", 1.0, 0.0, 1.0, 0.0, false},
		// Herbivore is not a threat to fauna
		{"herbivore no threat to fauna", 0.0, 0.0, 0.0, 0.0, false},
		// Herbivore threatens flora
		{"herbivore threatens flora", 0.0, 1.0, 0.0, 1.0, true},
		// Flora (composition=1) is threatened by herbivores
		{"omnivore partial threat", 0.5, 0.5, 0.0, 1.0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threat := ThreatLevel(tt.theirDigestive, tt.myComposition, tt.myArmor)
			if math.Abs(float64(threat-tt.expectedThreat)) > 0.01 {
				t.Errorf("ThreatLevel(%.1f, %.1f, %.1f) = %.3f, want %.3f",
					tt.theirDigestive, tt.myComposition, tt.myArmor, threat, tt.expectedThreat)
			}
			isThreat := IsThreat(tt.theirDigestive, tt.myComposition, tt.myArmor)
			if isThreat != tt.isThreat {
				t.Errorf("IsThreat() = %v, want %v", isThreat, tt.isThreat)
			}
		})
	}
}

func TestFloraCapabilities(t *testing.T) {
	caps := FloraCapabilities(0.1)

	if caps.PhotoWeight != 1.0 {
		t.Errorf("FloraCapabilities PhotoWeight = %.1f, want 1.0", caps.PhotoWeight)
	}
	if caps.ActuatorWeight != 0.0 {
		t.Errorf("FloraCapabilities ActuatorWeight = %.1f, want 0.0", caps.ActuatorWeight)
	}
	if caps.Composition() != 1.0 {
		t.Errorf("FloraCapabilities Composition() = %.1f, want 1.0", caps.Composition())
	}
	if caps.StructuralArmor != 0.1 {
		t.Errorf("FloraCapabilities StructuralArmor = %.1f, want 0.1", caps.StructuralArmor)
	}
}

func TestComputeCapabilities(t *testing.T) {
	cells := []CellSpec{
		{
			PrimaryType:        CellTypeSensor,
			SecondaryType:      CellTypeActuator,
			EffectivePrimary:   0.8,
			EffectiveSecondary: 0.3,
			StructuralArmor:    0.2,
			StorageCapacity:    0.1,
		},
		{
			PrimaryType:        CellTypePhotosynthetic,
			SecondaryType:      CellTypeNone,
			EffectivePrimary:   0.6,
			EffectiveSecondary: 0.0,
			StructuralArmor:    0.4,
			StorageCapacity:    0.3,
		},
		{
			PrimaryType:        CellTypeDigestive,
			SecondaryType:      CellTypeMouth,
			EffectivePrimary:   0.7,
			EffectiveSecondary: 0.25,
			DigestiveSpectrum:  0.3, // Mostly herbivore
			StructuralArmor:    0.0,
			StorageCapacity:    0.2,
		},
	}

	caps := ComputeCapabilities(cells)

	// Check sensor weight
	expectedSensor := float32(0.8) // Only first cell has sensor as primary
	if math.Abs(float64(caps.SensorWeight-expectedSensor)) > 0.01 {
		t.Errorf("SensorWeight = %.3f, want %.3f", caps.SensorWeight, expectedSensor)
	}

	// Check actuator weight
	expectedActuator := float32(0.3) // Only first cell has actuator as secondary
	if math.Abs(float64(caps.ActuatorWeight-expectedActuator)) > 0.01 {
		t.Errorf("ActuatorWeight = %.3f, want %.3f", caps.ActuatorWeight, expectedActuator)
	}

	// Check photo weight
	expectedPhoto := float32(0.6) // Only second cell has photosynthetic
	if math.Abs(float64(caps.PhotoWeight-expectedPhoto)) > 0.01 {
		t.Errorf("PhotoWeight = %.3f, want %.3f", caps.PhotoWeight, expectedPhoto)
	}

	// Check mouth size
	expectedMouth := float32(0.25) // Only third cell has mouth as secondary
	if math.Abs(float64(caps.MouthSize-expectedMouth)) > 0.01 {
		t.Errorf("MouthSize = %.3f, want %.3f", caps.MouthSize, expectedMouth)
	}

	// Check digestive count
	if caps.DigestiveCount != 1 {
		t.Errorf("DigestiveCount = %d, want 1", caps.DigestiveCount)
	}

	// Check digestive spectrum average (weighted by strength)
	// Third cell: spectrum=0.3, strength=0.7, so sum = 0.3 * 0.7 = 0.21
	expectedDigestiveSum := float32(0.3 * 0.7)
	if math.Abs(float64(caps.DigestiveSum-expectedDigestiveSum)) > 0.01 {
		t.Errorf("DigestiveSum = %.3f, want %.3f", caps.DigestiveSum, expectedDigestiveSum)
	}

	// Average armor: (0.2 + 0.4 + 0.0) / 3 = 0.2
	expectedArmor := float32(0.2)
	if math.Abs(float64(caps.StructuralArmor-expectedArmor)) > 0.01 {
		t.Errorf("StructuralArmor = %.3f, want %.3f", caps.StructuralArmor, expectedArmor)
	}

	// Average storage: (0.1 + 0.3 + 0.2) / 3 = 0.2
	expectedStorage := float32(0.2)
	if math.Abs(float64(caps.StorageCapacity-expectedStorage)) > 0.01 {
		t.Errorf("StorageCapacity = %.3f, want %.3f", caps.StorageCapacity, expectedStorage)
	}
}

func TestFeedingScenarios(t *testing.T) {
	// Scenario 1: Herbivore (digestive=0) trying to eat unarmored flora (composition=1)
	t.Run("herbivore eats flora successfully", func(t *testing.T) {
		herbivoreDigestive := float32(0.0)
		floraComposition := float32(1.0)
		floraArmor := float32(0.0)

		effectiveness := FeedingEffectiveness(herbivoreDigestive, floraComposition, floraArmor)
		if effectiveness != 1.0 {
			t.Errorf("Expected perfect feeding effectiveness, got %.3f", effectiveness)
		}
	})

	// Scenario 2: Carnivore (digestive=1) trying to eat armored fauna (composition=0)
	t.Run("carnivore vs armored fauna", func(t *testing.T) {
		carnivoreDigestive := float32(1.0)
		faunaComposition := float32(0.0)
		faunaArmor := float32(0.4)

		effectiveness := FeedingEffectiveness(carnivoreDigestive, faunaComposition, faunaArmor)
		expectedEffectiveness := float32(0.6) // 1.0 edibility - 0.4 armor
		if math.Abs(float64(effectiveness-expectedEffectiveness)) > 0.01 {
			t.Errorf("Expected effectiveness %.3f, got %.3f", expectedEffectiveness, effectiveness)
		}
	})

	// Scenario 3: Herbivore cannot eat fauna
	t.Run("herbivore cannot eat fauna", func(t *testing.T) {
		herbivoreDigestive := float32(0.0)
		faunaComposition := float32(0.0)

		canFeed := CanFeed(herbivoreDigestive, faunaComposition, 0.0)
		if canFeed {
			t.Error("Herbivore should not be able to eat fauna")
		}
	})

	// Scenario 4: Omnivore can eat both, but less effectively
	t.Run("omnivore feeding efficiency", func(t *testing.T) {
		omnivoreDigestive := float32(0.5)

		// Eating flora
		floraEffectiveness := FeedingEffectiveness(omnivoreDigestive, 1.0, 0.0)
		if floraEffectiveness != 0.5 {
			t.Errorf("Omnivore flora effectiveness = %.3f, want 0.5", floraEffectiveness)
		}

		// Eating fauna
		faunaEffectiveness := FeedingEffectiveness(omnivoreDigestive, 0.0, 0.0)
		if faunaEffectiveness != 0.5 {
			t.Errorf("Omnivore fauna effectiveness = %.3f, want 0.5", faunaEffectiveness)
		}

		// Eating balanced organism (composition=0.5) - perfect match!
		balancedEffectiveness := FeedingEffectiveness(omnivoreDigestive, 0.5, 0.0)
		if balancedEffectiveness != 1.0 {
			t.Errorf("Omnivore balanced effectiveness = %.3f, want 1.0", balancedEffectiveness)
		}
	})
}

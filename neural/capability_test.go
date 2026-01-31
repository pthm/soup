package neural

import (
	"math"
	"testing"
)

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
		name      string
		edibility float32
		armor     float32
		expected  float32
		canFeed   bool
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
			// Also check feeding is consistent
			canFeed := got > 0
			if canFeed != tt.canFeed {
				t.Errorf("Feed mismatch: penetration=%.3f, canFeed=%v, want %v",
					got, canFeed, tt.canFeed)
			}
		})
	}
}

func TestThreatLevel(t *testing.T) {
	tests := []struct {
		name           string
		theirDigestive float32
		myComposition  float32
		myArmor        float32
		expectedThreat float32
	}{
		// A carnivore (digestive=1) is a threat to fauna (composition=0)
		{"carnivore threatens unarmored fauna", 1.0, 0.0, 0.0, 1.0},
		{"carnivore threatens armored fauna", 1.0, 0.0, 0.5, 0.5},
		{"carnivore blocked by heavy armor", 1.0, 0.0, 1.0, 0.0},
		// Herbivore is not a threat to fauna
		{"herbivore no threat to fauna", 0.0, 0.0, 0.0, 0.0},
		// Herbivore threatens flora
		{"herbivore threatens flora", 0.0, 1.0, 0.0, 1.0},
		// Flora (composition=1) is threatened by herbivores
		{"omnivore partial threat", 0.5, 0.5, 0.0, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			threat := ThreatLevel(tt.theirDigestive, tt.myComposition, tt.myArmor)
			if math.Abs(float64(threat-tt.expectedThreat)) > 0.01 {
				t.Errorf("ThreatLevel(%.1f, %.1f, %.1f) = %.3f, want %.3f",
					tt.theirDigestive, tt.myComposition, tt.myArmor, threat, tt.expectedThreat)
			}
		})
	}
}

func TestNutritionMultiplier(t *testing.T) {
	tests := []struct {
		name        string
		penetration float32
		expected    float32
	}{
		// With CompatK=3, the power law creates sharp niches
		{"zero penetration", 0.0, 0.0},
		{"full penetration", 1.0, 1.0},            // 1^3 = 1
		{"half penetration", 0.5, 0.125},          // 0.5^3 = 0.125
		{"high penetration", 0.9, 0.729},          // 0.9^3 = 0.729
		{"low penetration", 0.3, 0.027},           // 0.3^3 = 0.027
		{"negative penetration", -0.5, 0.0},       // Clamped to 0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NutritionMultiplier(tt.penetration)
			if math.Abs(float64(got-tt.expected)) > 0.01 {
				t.Errorf("NutritionMultiplier(%.2f) = %.4f, want %.4f",
					tt.penetration, got, tt.expected)
			}
		})
	}

	// Verify the power law creates sharper niches than linear
	// A specialist (penetration=1.0) vs generalist (penetration=0.5)
	specialist := NutritionMultiplier(1.0)
	generalist := NutritionMultiplier(0.5)
	ratio := specialist / generalist
	if ratio < 5 {
		t.Errorf("Power law should strongly favor specialists: ratio=%.2f, expected >5", ratio)
	}
}

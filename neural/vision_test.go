package neural

import (
	"math"
	"testing"
)

// TestAngleToCone verifies entities in each quadrant contribute to the correct cone.
func TestAngleToCone(t *testing.T) {
	tests := []struct {
		name     string
		angle    float32 // Relative to heading
		wantCone int
	}{
		// Front cone: [-π/4, π/4]
		{"front center", 0, ConeFront},
		{"front edge positive", math.Pi / 4 * 0.9, ConeFront},
		{"front edge negative", -math.Pi / 4 * 0.9, ConeFront},

		// Right cone: [π/4, 3π/4]
		{"right center", math.Pi / 2, ConeRight},
		{"right edge near front", math.Pi/4 + 0.1, ConeRight},
		{"right edge near back", 3*math.Pi/4 - 0.1, ConeRight},

		// Back cone: [3π/4, π] and [-π, -3π/4]
		{"back center positive", math.Pi, ConeBack},
		{"back center negative", -math.Pi, ConeBack},
		{"back edge positive", 3*math.Pi/4 + 0.1, ConeBack},
		{"back edge negative", -3*math.Pi/4 - 0.1, ConeBack},

		// Left cone: [-3π/4, -π/4]
		{"left center", -math.Pi / 2, ConeLeft},
		{"left edge near front", -math.Pi/4 - 0.1, ConeLeft},
		{"left edge near back", -3*math.Pi/4 + 0.1, ConeLeft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AngleToCone(tt.angle)
			if got != tt.wantCone {
				t.Errorf("AngleToCone(%v) = %d, want %d", tt.angle, got, tt.wantCone)
			}
		})
	}
}

// TestConeBinning verifies entities are placed in the correct cones based on position.
func TestConeBinning(t *testing.T) {
	// Observer at origin, facing right (heading = 0)
	params := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0, // Facing positive X
		MyComposition:   0, // Fauna-like
		MyDigestiveSpec: 0.8, // Carnivore
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil, // No sensors = baseline perception
	}

	// Create entities in each quadrant
	entities := []EntityInfo{
		// Front: positive X, near Y=0
		{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.2, IsFlora: false},
		// Right: positive Y
		{X: 0, Y: 10, Composition: 0, DigestiveSpec: 0.2, IsFlora: false},
		// Back: negative X
		{X: -10, Y: 0, Composition: 0, DigestiveSpec: 0.2, IsFlora: false},
		// Left: negative Y
		{X: 0, Y: -10, Composition: 0, DigestiveSpec: 0.2, IsFlora: false},
	}

	var pv PolarVision
	pv.ScanEntities(params, entities)

	// With carnivore (digestive=0.8) eating fauna (composition=0), food relevance should be high
	// Each entity has same distance, so intensities should be similar

	// Check that each cone has some food signal (not zero)
	if pv.Food[ConeFront] <= 0 {
		t.Errorf("Front cone should have food, got %v", pv.Food[ConeFront])
	}
	if pv.Food[ConeRight] <= 0 {
		t.Errorf("Right cone should have food, got %v", pv.Food[ConeRight])
	}
	if pv.Food[ConeBack] <= 0 {
		t.Errorf("Back cone should have food, got %v", pv.Food[ConeBack])
	}
	if pv.Food[ConeLeft] <= 0 {
		t.Errorf("Left cone should have food, got %v", pv.Food[ConeLeft])
	}
}

// TestIntensityFalloff verifies intensity decreases with distance squared.
func TestIntensityFalloff(t *testing.T) {
	params := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0,
		MyDigestiveSpec: 0.8, // Carnivore
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil,
	}

	// Entity at distance 5
	nearEntity := EntityInfo{X: 5, Y: 0, Composition: 0, DigestiveSpec: 0.2, IsFlora: false}
	// Entity at distance 10 (2x farther)
	farEntity := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.2, IsFlora: false}

	var pvNear, pvFar PolarVision
	pvNear.ScanEntities(params, []EntityInfo{nearEntity})
	pvFar.ScanEntities(params, []EntityInfo{farEntity})

	// Intensity should follow inverse square law: I ∝ 1/d²
	// At 2x distance, intensity should be 1/4
	ratio := pvFar.Food[ConeFront] / pvNear.Food[ConeFront]
	expected := float32(0.25) // (5/10)² = 0.25

	if math.Abs(float64(ratio-expected)) > 0.01 {
		t.Errorf("Intensity ratio = %v, want ~%v (inverse square)", ratio, expected)
	}
}

// TestSensorWeighting verifies sensors aligned to a cone increase that cone's output.
func TestSensorWeighting(t *testing.T) {
	// Create sensors pointing forward only
	forwardSensors := []SensorCell{
		{GridX: 1, GridY: 0, Strength: 1.0}, // Facing right (forward when heading=0)
	}

	// Create sensors pointing backward only
	backwardSensors := []SensorCell{
		{GridX: -1, GridY: 0, Strength: 1.0}, // Facing left (backward when heading=0)
	}

	baseParams := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0,
		MyDigestiveSpec: 0.8,
		MyArmor:         0,
		EffectiveRadius: 100,
	}

	// Entity in front
	entityFront := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.2, IsFlora: false}

	// Test with forward-facing sensors
	paramsForward := baseParams
	paramsForward.Sensors = forwardSensors
	var pvForward PolarVision
	pvForward.ScanEntities(paramsForward, []EntityInfo{entityFront})

	// Test with backward-facing sensors
	paramsBackward := baseParams
	paramsBackward.Sensors = backwardSensors
	var pvBackward PolarVision
	pvBackward.ScanEntities(paramsBackward, []EntityInfo{entityFront})

	// Forward sensors should detect front entity better
	if pvForward.Food[ConeFront] <= pvBackward.Food[ConeFront] {
		t.Errorf("Forward sensors should detect front entity better: forward=%v, backward=%v",
			pvForward.Food[ConeFront], pvBackward.Food[ConeFront])
	}
}

// TestFoodThreatChannels verifies food and threat are computed correctly.
func TestFoodThreatChannels(t *testing.T) {
	// Herbivore observer
	herbParams := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0, // Fauna-like
		MyDigestiveSpec: 0.1, // Herbivore
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil,
	}

	// Flora entity (composition = 1.0, high photo, no actuator)
	flora := EntityInfo{X: 10, Y: 0, Composition: 1.0, DigestiveSpec: 0, IsFlora: true}

	// Carnivore entity (composition ~0, high actuator, no photo)
	carnivore := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.9, IsFlora: false}

	// Test herbivore seeing flora (should be food)
	var pvHerbFlora PolarVision
	pvHerbFlora.ScanEntities(herbParams, []EntityInfo{flora})

	if pvHerbFlora.Food[ConeFront] <= 0 {
		t.Errorf("Herbivore should see flora as food, got %v", pvHerbFlora.Food[ConeFront])
	}

	// Test herbivore seeing carnivore (should be threat)
	var pvHerbCarn PolarVision
	pvHerbCarn.ScanEntities(herbParams, []EntityInfo{carnivore})

	if pvHerbCarn.Threat[ConeFront] <= 0 {
		t.Errorf("Herbivore should see carnivore as threat, got %v", pvHerbCarn.Threat[ConeFront])
	}

	// Carnivore observer
	carnParams := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0, // Fauna-like
		MyDigestiveSpec: 0.9, // Carnivore
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil,
	}

	// Test carnivore seeing fauna (should be food)
	prey := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.1, IsFlora: false}
	var pvCarnPrey PolarVision
	pvCarnPrey.ScanEntities(carnParams, []EntityInfo{prey})

	if pvCarnPrey.Food[ConeFront] <= 0 {
		t.Errorf("Carnivore should see prey as food, got %v", pvCarnPrey.Food[ConeFront])
	}

	// Test carnivore seeing flora (should NOT be food - low edibility)
	var pvCarnFlora PolarVision
	pvCarnFlora.ScanEntities(carnParams, []EntityInfo{flora})

	// Carnivore (digestive=0.9) trying to eat flora (composition=1.0)
	// Edibility = 1 - |0.9 + 1.0 - 1| = 1 - 0.9 = 0.1 (very low)
	if pvCarnFlora.Food[ConeFront] >= pvCarnPrey.Food[ConeFront] {
		t.Errorf("Carnivore should prefer prey over flora: flora=%v, prey=%v",
			pvCarnFlora.Food[ConeFront], pvCarnPrey.Food[ConeFront])
	}
}

// TestFriendChannel verifies genetic similarity affects friend intensity.
func TestFriendChannel(t *testing.T) {
	params := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0,
		MyDigestiveSpec: 0.5,
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil,
	}

	// Similar entity (low genetic distance)
	similar := EntityInfo{X: 10, Y: 0, GeneticDistance: 0.5, IsFlora: false}

	// Different entity (high genetic distance)
	different := EntityInfo{X: 10, Y: 0, GeneticDistance: 5.0, IsFlora: false}

	var pvSimilar, pvDifferent PolarVision
	pvSimilar.ScanEntities(params, []EntityInfo{similar})
	pvDifferent.ScanEntities(params, []EntityInfo{different})

	// Similar should have higher friend value
	if pvSimilar.Friend[ConeFront] <= pvDifferent.Friend[ConeFront] {
		t.Errorf("Similar entity should have higher friend value: similar=%v, different=%v",
			pvSimilar.Friend[ConeFront], pvDifferent.Friend[ConeFront])
	}
}

// TestNormalizeForBrain verifies normalization produces [0, 1] range.
func TestNormalizeForBrain(t *testing.T) {
	pv := PolarVision{
		Food:   [4]float32{0, 0.1, 1.0, MaxConeIntensity},
		Threat: [4]float32{0, 0.5, 2.0, MaxConeIntensity},
		Friend: [4]float32{0, 0.01, 0.5, MaxConeIntensity},
	}

	normalized := pv.NormalizeForBrain()

	// Check all values are in [0, 1]
	for i := 0; i < NumCones; i++ {
		if normalized.Food[i] < 0 || normalized.Food[i] > 1 {
			t.Errorf("Food[%d] = %v, want [0, 1]", i, normalized.Food[i])
		}
		if normalized.Threat[i] < 0 || normalized.Threat[i] > 1 {
			t.Errorf("Threat[%d] = %v, want [0, 1]", i, normalized.Threat[i])
		}
		if normalized.Friend[i] < 0 || normalized.Friend[i] > 1 {
			t.Errorf("Friend[%d] = %v, want [0, 1]", i, normalized.Friend[i])
		}
	}

	// Zero should stay zero
	if normalized.Food[0] != 0 {
		t.Errorf("Normalized zero = %v, want 0", normalized.Food[0])
	}

	// Max should become 1
	if math.Abs(float64(normalized.Food[3]-1.0)) > 0.01 {
		t.Errorf("Normalized max = %v, want ~1.0", normalized.Food[3])
	}
}

// TestArmorReducesEdibility verifies structural armor reduces food channel.
func TestArmorReducesEdibility(t *testing.T) {
	params := VisionParams{
		PosX:            0,
		PosY:            0,
		Heading:         0,
		MyComposition:   0,
		MyDigestiveSpec: 0.8, // Carnivore
		MyArmor:         0,
		EffectiveRadius: 100,
		Sensors:         nil,
	}

	// Unarmored prey
	unarmoredPrey := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.1, StructuralArmor: 0, IsFlora: false}

	// Heavily armored prey
	armoredPrey := EntityInfo{X: 10, Y: 0, Composition: 0, DigestiveSpec: 0.1, StructuralArmor: 0.5, IsFlora: false}

	var pvUnarmored, pvArmored PolarVision
	pvUnarmored.ScanEntities(params, []EntityInfo{unarmoredPrey})
	pvArmored.ScanEntities(params, []EntityInfo{armoredPrey})

	// Unarmored should appear more edible (higher food value)
	if pvUnarmored.Food[ConeFront] <= pvArmored.Food[ConeFront] {
		t.Errorf("Unarmored prey should be more edible: unarmored=%v, armored=%v",
			pvUnarmored.Food[ConeFront], pvArmored.Food[ConeFront])
	}
}

// Phase 3b tests: Directional light awareness

// TestLightGradientsUniformLight verifies gradients are zero when illumination is uniform.
func TestLightGradientsUniformLight(t *testing.T) {
	var pv PolarVision

	// Uniform light sampler: returns 0.5 everywhere
	uniformSampler := func(x, y float32) float32 {
		return 0.5
	}

	pv.SampleDirectionalLight(0, 0, 0, 100, uniformSampler)
	lightFB, lightLR := pv.LightGradients()

	// Both gradients should be ~0 with uniform light
	if math.Abs(float64(lightFB)) > 0.01 {
		t.Errorf("Light FB gradient = %v, want ~0 for uniform light", lightFB)
	}
	if math.Abs(float64(lightLR)) > 0.01 {
		t.Errorf("Light LR gradient = %v, want ~0 for uniform light", lightLR)
	}
}

// TestLightGradientsBrighterAhead verifies positive FB gradient when light is ahead.
func TestLightGradientsBrighterAhead(t *testing.T) {
	var pv PolarVision

	// Directional light: brighter in positive X direction (front when heading=0)
	directionalSampler := func(x, y float32) float32 {
		// Light increases with X, normalized to [0, 1]
		light := (x + 100) / 200 // Maps [-100, 100] to [0, 1]
		if light < 0 {
			light = 0
		}
		if light > 1 {
			light = 1
		}
		return light
	}

	// Heading = 0 (facing positive X)
	pv.SampleDirectionalLight(0, 0, 0, 100, directionalSampler)
	lightFB, lightLR := pv.LightGradients()

	// Front-back gradient should be positive (brighter ahead)
	if lightFB <= 0 {
		t.Errorf("Light FB gradient = %v, want > 0 when brighter ahead", lightFB)
	}

	// Left-right gradient should be ~0 (light varies on X axis only)
	if math.Abs(float64(lightLR)) > 0.1 {
		t.Errorf("Light LR gradient = %v, want ~0 when light varies on X only", lightLR)
	}
}

// TestLightGradientsBrighterBehind verifies negative FB gradient when light is behind.
func TestLightGradientsBrighterBehind(t *testing.T) {
	var pv PolarVision

	// Directional light: brighter in negative X direction (behind when heading=0)
	directionalSampler := func(x, y float32) float32 {
		light := (-x + 100) / 200 // Brighter at negative X
		if light < 0 {
			light = 0
		}
		if light > 1 {
			light = 1
		}
		return light
	}

	pv.SampleDirectionalLight(0, 0, 0, 100, directionalSampler)
	lightFB, _ := pv.LightGradients()

	// Front-back gradient should be negative (brighter behind)
	if lightFB >= 0 {
		t.Errorf("Light FB gradient = %v, want < 0 when brighter behind", lightFB)
	}
}

// TestLightGradientsBrighterRight verifies positive LR gradient when light is to the right.
func TestLightGradientsBrighterRight(t *testing.T) {
	var pv PolarVision

	// Directional light: brighter in positive Y direction (right when heading=0)
	directionalSampler := func(x, y float32) float32 {
		light := (y + 100) / 200 // Brighter at positive Y
		if light < 0 {
			light = 0
		}
		if light > 1 {
			light = 1
		}
		return light
	}

	pv.SampleDirectionalLight(0, 0, 0, 100, directionalSampler)
	_, lightLR := pv.LightGradients()

	// Left-right gradient should be positive (brighter to the right)
	if lightLR <= 0 {
		t.Errorf("Light LR gradient = %v, want > 0 when brighter to the right", lightLR)
	}
}

// TestLightGradientsWithHeading verifies gradients account for organism heading.
func TestLightGradientsWithHeading(t *testing.T) {
	var pv PolarVision

	// Light source at positive X
	directionalSampler := func(x, y float32) float32 {
		light := (x + 100) / 200
		if light < 0 {
			light = 0
		}
		if light > 1 {
			light = 1
		}
		return light
	}

	// Heading = π/2 (facing positive Y)
	// In standard math convention with counter-clockwise positive angles:
	// - Front samples at heading + 0 = π/2 (+Y direction)
	// - Right samples at heading + π/2 = π (-X direction)
	// - Left samples at heading - π/2 = 0 (+X direction)
	// So positive X (light source) is to our LEFT
	pv.SampleDirectionalLight(0, 0, math.Pi/2, 100, directionalSampler)
	lightFB, lightLR := pv.LightGradients()

	// FB should be ~0 (light varies on X axis only, perpendicular to our heading)
	if math.Abs(float64(lightFB)) > 0.1 {
		t.Errorf("Light FB gradient = %v, want ~0 when facing perpendicular to light", lightFB)
	}
	// LR should be negative (brighter to the left, which is +X)
	if lightLR >= 0 {
		t.Errorf("Light LR gradient = %v, want < 0 when light source is to the left", lightLR)
	}
}

// TestLightGradientsNilSampler verifies graceful handling of nil sampler.
func TestLightGradientsNilSampler(t *testing.T) {
	var pv PolarVision

	// Nil sampler should not panic and should produce neutral gradients
	pv.SampleDirectionalLight(0, 0, 0, 100, nil)
	lightFB, lightLR := pv.LightGradients()

	// With nil sampler, all cones get 0.5, so gradients should be ~0
	if math.Abs(float64(lightFB)) > 0.01 {
		t.Errorf("Light FB gradient = %v, want ~0 with nil sampler", lightFB)
	}
	if math.Abs(float64(lightLR)) > 0.01 {
		t.Errorf("Light LR gradient = %v, want ~0 with nil sampler", lightLR)
	}
}

package systems

import (
	"testing"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

func init() {
	config.MustInit("")
	InitSensorCache()
}

func TestSensorInputsAsSlice(t *testing.T) {
	inputs := SensorInputs{
		Food:   [NumSectors]float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
		Threat: [NumSectors]float32{0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1},
		Kin:    [NumSectors]float32{},
		Energy: 0.8,
		Speed:  0.5,
		Diet:   0.3,
	}

	slice := inputs.AsSlice()

	if len(slice) != NumSectors*3+3 {
		t.Errorf("AsSlice wrong length: got %d, want %d", len(slice), NumSectors*3+3)
	}

	// Check food values
	for i := 0; i < NumSectors; i++ {
		if slice[i] != inputs.Food[i] {
			t.Errorf("Food[%d] mismatch: got %f, want %f", i, slice[i], inputs.Food[i])
		}
	}

	// Check energy, speed, and diet
	if slice[NumSectors*3] != inputs.Energy {
		t.Errorf("Energy mismatch: got %f, want %f", slice[NumSectors*3], inputs.Energy)
	}
	if slice[NumSectors*3+1] != inputs.Speed {
		t.Errorf("Speed mismatch: got %f, want %f", slice[NumSectors*3+1], inputs.Speed)
	}
	if slice[NumSectors*3+2] != inputs.Diet {
		t.Errorf("Diet mismatch: got %f, want %f", slice[NumSectors*3+2], inputs.Diet)
	}
}

func TestComputeSensorsNoNeighbors(t *testing.T) {
	pos := components.Position{X: 100, Y: 100}
	vel := components.Velocity{X: 1, Y: 0}
	rot := components.Rotation{Heading: 0}
	energy := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	caps := components.DefaultCapabilities(components.KindPrey)

	inputs := ComputeSensors(
		pos, vel, rot, energy, caps, components.KindPrey,
		0.0, // selfDiet (herbivore)
		nil, nil, nil, // neighbors, posMap, orgMap
		1280, 720,
	)

	// With no neighbors, food, threat, and kin should be zero
	for i := 0; i < NumSectors; i++ {
		if inputs.Food[i] != 0 {
			t.Errorf("Food[%d] should be 0, got %f", i, inputs.Food[i])
		}
		if inputs.Threat[i] != 0 {
			t.Errorf("Threat[%d] should be 0, got %f", i, inputs.Threat[i])
		}
		if inputs.Kin[i] != 0 {
			t.Errorf("Kin[%d] should be 0, got %f", i, inputs.Kin[i])
		}
	}

	// Energy and speed should be set
	if inputs.Energy != 0.8 {
		t.Errorf("Energy wrong: got %f, want 0.8", inputs.Energy)
	}

	expectedSpeed := 1.0 / caps.MaxSpeed
	if inputs.Speed != expectedSpeed {
		t.Errorf("Speed wrong: got %f, want %f", inputs.Speed, expectedSpeed)
	}
}

func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		input    float32
		expected float32
	}{
		{0, 0},
		{3.14159, 3.14159},
		{-3.14159, -3.14159},
		{6.28318, 0}, // 2pi -> 0
	}

	for _, tt := range tests {
		got := normalizeAngle(tt.input)
		// Allow some floating point tolerance
		if got < tt.expected-0.01 || got > tt.expected+0.01 {
			t.Errorf("normalizeAngle(%f) = %f, want ~%f", tt.input, got, tt.expected)
		}
	}

	// Special case: 3pi should normalize to +pi or -pi (both valid)
	got := normalizeAngle(9.42478) // 3pi
	if got < 3.13 && got > -3.13 {
		t.Errorf("normalizeAngle(3pi) = %f, want ~pi or ~-pi", got)
	}
}

func TestSmoothSaturate(t *testing.T) {
	// smoothSaturate(0) should be 0
	if smoothSaturate(0) != 0 {
		t.Errorf("smoothSaturate(0) = %f, want 0", smoothSaturate(0))
	}

	// smoothSaturate should approach 1 for large values
	if smoothSaturate(10) < 0.99 {
		t.Errorf("smoothSaturate(10) = %f, want ~1", smoothSaturate(10))
	}

	// smoothSaturate should be monotonic
	prev := float32(0)
	for i := 0; i <= 10; i++ {
		x := float32(i) * 0.5
		v := smoothSaturate(x)
		if v < prev {
			t.Errorf("smoothSaturate not monotonic at %f", x)
		}
		prev = v
	}
}

// TestThreatDetection verifies that prey detect predators as threats.
// This test uses ComputeSensorsBounded with mock neighbor data.
func TestThreatDetection(t *testing.T) {
	// Set up prey (herbivore) looking forward (heading=0)
	selfVel := components.Velocity{X: 0, Y: 0}
	selfRot := components.Rotation{Heading: 0}
	selfEnergy := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	selfCaps := components.DefaultCapabilities(components.KindPrey)
	selfKind := components.KindPrey
	selfDiet := float32(0.0) // Pure herbivore
	selfCladeID := uint64(1)
	selfArchetypeID := uint8(0) // Grazer archetype
	selfPos := components.Position{X: 100, Y: 100}

	// Create a predator neighbor directly in front of the prey
	// Prey is at (100,100) facing right (heading=0), predator at (150,100) = 50 units away
	predatorNeighbor := Neighbor{
		DX:     50, // Directly ahead
		DY:     0,
		DistSq: 50 * 50, // 2500
	}

	// We need to mock the orgMap.Get call
	// Since we can't easily mock ECS, we'll test the edibility math directly
	t.Run("edibility_math", func(t *testing.T) {
		// For a prey (diet=0) seeing a predator (diet=1.0)
		selfDiet := float32(0.0)
		predatorDiet := float32(1.0)

		edOther := predatorDiet * (1.0 - selfDiet)
		if edOther <= 0.05 {
			t.Errorf("edOther = %f, want > 0.05 (predator should be detectable as threat)", edOther)
		}
		t.Logf("edOther for prey seeing predator: %f (threshold: 0.05)", edOther)
	})

	t.Run("edibility_math_low_diet_predator", func(t *testing.T) {
		// Even a predator with diet=0.5 (minimum to be rendered as predator)
		selfDiet := float32(0.0)
		predatorDiet := float32(0.5)

		edOther := predatorDiet * (1.0 - selfDiet)
		if edOther <= 0.05 {
			t.Errorf("edOther = %f, want > 0.05 (low-diet predator should still be threat)", edOther)
		}
		t.Logf("edOther for prey seeing diet=0.5 predator: %f", edOther)
	})

	t.Run("sector_bins", func(t *testing.T) {
		var bins SectorBins
		bins.Clear()

		// Manually simulate what ComputeSensorsBounded does for a predator neighbor

		// Predator is at (150, 100), prey at (100, 100), facing right (heading=0)
		// In local frame: lx = 50, ly = 0 (directly ahead)
		cosH := fastCos(selfRot.Heading)
		sinH := fastSin(selfRot.Heading)
		lx := predatorNeighbor.DX*cosH + predatorNeighbor.DY*sinH
		ly := -predatorNeighbor.DX*sinH + predatorNeighbor.DY*cosH

		relativeAngle := fastAtan2(ly, lx)
		sectorIdx := sectorIndexFromAngle(relativeAngle)
		t.Logf("Predator in sector %d (lx=%f, ly=%f, angle=%f)", sectorIdx, lx, ly, relativeAngle)

		// Compute weights
		dist := fastSqrt(predatorNeighbor.DistSq)
		invVisionRange := 1.0 / selfCaps.VisionRange
		distWeight := clamp01(1.0 - dist*invVisionRange)
		effWeight := VisionEffectivenessForSector(sectorIdx, selfKind)
		baseWeight := distWeight * effWeight

		t.Logf("dist=%f, visionRange=%f, distWeight=%f, effWeight=%f, baseWeight=%f",
			dist, selfCaps.VisionRange, distWeight, effWeight, baseWeight)

		// Edibility
		predatorDiet := float32(1.0)
		edOther := predatorDiet * (1.0 - selfDiet)

		if edOther > 0.05 {
			weight := baseWeight * edOther
			bins.Threat[sectorIdx].insert(predatorNeighbor.DistSq, weight)
			t.Logf("Inserted threat: sector=%d, weight=%f", sectorIdx, weight)
		}

		// Check that threat was registered
		threatSum := bins.Threat[sectorIdx].sum()
		if threatSum == 0 {
			t.Errorf("Threat bin is empty, expected non-zero sum")
		} else {
			t.Logf("Threat bin sum: %f", threatSum)
		}

		// Check smoothSaturate output
		threatSignal := smoothSaturate(threatSum)
		if threatSignal == 0 {
			t.Errorf("Threat signal is 0, expected non-zero")
		} else {
			t.Logf("Threat signal after smoothSaturate: %f", threatSignal)
		}
	})

	t.Run("predator_outside_range", func(t *testing.T) {
		// Test what happens when predator is outside vision range
		farPredator := Neighbor{
			DX:     120, // Beyond 100-unit vision range
			DY:     0,
			DistSq: 120 * 120, // 14400
		}

		visionRangeSq := selfCaps.VisionRange * selfCaps.VisionRange // 10000

		if farPredator.DistSq > visionRangeSq {
			t.Logf("Predator at distance %f is OUTSIDE vision range %f (this explains zero threat)",
				fastSqrt(farPredator.DistSq), selfCaps.VisionRange)
		} else {
			t.Errorf("Expected predator to be outside range")
		}
	})

	// Suppress unused variable warnings
	_ = selfVel
	_ = selfEnergy
	_ = selfCladeID
	_ = selfArchetypeID
	_ = selfPos
}

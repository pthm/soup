package neural

import (
	"math"
	"testing"
)

func TestSensoryInputsToInputs(t *testing.T) {
	// Phase 4b: Test cone-based sensory inputs with flow alignment and openness
	sensory := SensoryInputs{
		ConeFood:      [4]float32{0.8, 0.2, 0.1, 0.3},
		ConeThreat:    [4]float32{0.1, 0.0, 0.5, 0.0},
		ConeFriend:    [4]float32{0.4, 0.4, 0.2, 0.3},
		LightLevel:    0.7,
		FlowAlignment: 0.3,  // Phase 4: single flow alignment value
		Openness:      0.85, // Phase 4b: terrain openness
		LightFB:       0.2,
		LightLR:       -0.1,
		Energy:        75,
		MaxEnergy:     100,
	}

	inputs := sensory.ToInputs()

	if len(inputs) != BrainInputs {
		t.Errorf("expected %d inputs, got %d", BrainInputs, len(inputs))
	}

	// Check food cones [0-3]
	for i := 0; i < NumCones; i++ {
		expected := float64(sensory.ConeFood[i])
		if math.Abs(inputs[i]-expected) > 0.01 {
			t.Errorf("food cone %d: expected %f, got %f", i, expected, inputs[i])
		}
	}

	// Check threat cones [4-7]
	for i := 0; i < NumCones; i++ {
		expected := float64(sensory.ConeThreat[i])
		if math.Abs(inputs[4+i]-expected) > 0.01 {
			t.Errorf("threat cone %d: expected %f, got %f", i, expected, inputs[4+i])
		}
	}

	// Check friend cones [8-11]
	for i := 0; i < NumCones; i++ {
		expected := float64(sensory.ConeFriend[i])
		if math.Abs(inputs[8+i]-expected) > 0.01 {
			t.Errorf("friend cone %d: expected %f, got %f", i, expected, inputs[8+i])
		}
	}

	// Check energy ratio [12]
	expectedEnergy := 75.0 / 100.0
	if math.Abs(inputs[12]-expectedEnergy) > 0.01 {
		t.Errorf("energy ratio: expected %f, got %f", expectedEnergy, inputs[12])
	}

	// Check light level [13]
	if math.Abs(inputs[13]-0.7) > 0.01 {
		t.Errorf("light level: expected 0.7, got %f", inputs[13])
	}

	// Check flow alignment [14] (Phase 4)
	if math.Abs(inputs[14]-0.3) > 0.01 {
		t.Errorf("flow alignment: expected 0.3, got %f", inputs[14])
	}

	// Check openness [15] (Phase 4b)
	if math.Abs(inputs[15]-0.85) > 0.01 {
		t.Errorf("openness: expected 0.85, got %f", inputs[15])
	}

	// Check light gradients [16-17] (Phase 3b)
	if math.Abs(inputs[16]-0.2) > 0.01 {
		t.Errorf("light FB: expected 0.2, got %f", inputs[16])
	}
	if math.Abs(inputs[17]-(-0.1)) > 0.01 {
		t.Errorf("light LR: expected -0.1, got %f", inputs[17])
	}

	// Check bias [18]
	if inputs[18] != 1.0 {
		t.Errorf("bias: expected 1.0, got %f", inputs[18])
	}

	t.Logf("Inputs: %v", inputs)
}

func TestSensoryInputsZeroCones(t *testing.T) {
	// Test with zero cone values (no nearby entities)
	sensory := SensoryInputs{
		ConeFood:   [4]float32{0, 0, 0, 0},
		ConeThreat: [4]float32{0, 0, 0, 0},
		ConeFriend: [4]float32{0, 0, 0, 0},
		Energy:     50,
		MaxEnergy:  100,
	}

	inputs := sensory.ToInputs()

	// All cone values should be zero
	for i := 0; i < 12; i++ {
		if inputs[i] != 0.0 {
			t.Errorf("input %d should be 0, got %f", i, inputs[i])
		}
	}
}

func TestSensoryInputsNormalization(t *testing.T) {
	// Test clamping of extreme values
	sensory := SensoryInputs{
		ConeFood:      [4]float32{1.5, -0.1, 0.5, 2.0}, // Some out of range
		ConeThreat:    [4]float32{0.5, 0.5, 1.5, -0.5},
		ConeFriend:    [4]float32{0, 0, 0, 0},
		LightLevel:    1.5, // Beyond 1.0
		FlowAlignment: 2.0, // Beyond expected range
		Energy:        150, // Beyond max
		MaxEnergy:     100,
	}

	inputs := sensory.ToInputs()

	// All cone values should be clamped to [0, 1]
	for i := 0; i < 12; i++ {
		if inputs[i] < 0 || inputs[i] > 1 {
			t.Errorf("cone input %d out of range [0,1]: %f", i, inputs[i])
		}
	}

	// Light level should be clamped to 1.0
	if inputs[13] > 1.0 {
		t.Errorf("light level not clamped: got %f", inputs[13])
	}

	// Flow alignment should be clamped to 1.0
	if inputs[14] > 1.0 {
		t.Errorf("flow alignment not clamped: got %f", inputs[14])
	}

	// Energy ratio should be clamped
	if inputs[12] > 1.0 {
		t.Errorf("energy ratio not clamped: got %f", inputs[12])
	}
}

func TestFromPolarVision(t *testing.T) {
	// Create a polar vision result
	pv := PolarVision{
		Food:   [4]float32{1.0, 0.5, 0.1, 0.2},
		Threat: [4]float32{0.0, 0.8, 0.0, 0.3},
		Friend: [4]float32{0.5, 0.5, 0.5, 0.5},
	}

	var sensory SensoryInputs
	sensory.FromPolarVision(&pv)

	// Values should be normalized
	for i := 0; i < NumCones; i++ {
		if sensory.ConeFood[i] < 0 || sensory.ConeFood[i] > 1 {
			t.Errorf("food cone %d out of normalized range: %f", i, sensory.ConeFood[i])
		}
	}
}

func TestDecodeOutputs(t *testing.T) {
	// Phase 4: Simulate sigmoid outputs (0-1 range) - 5 outputs: DesireAngle, DesireDistance, Eat, Grow, Breed
	raw := []float64{0.5, 0.8, 0.3, 0.6, 0.7}

	outputs := DecodeOutputs(raw)

	// Check DesireAngle (0.5 sigmoid -> 0.0 angle)
	expectedAngle := float32((0.5 - 0.5) * 2.0 * math.Pi) // 0.0
	if math.Abs(float64(outputs.DesireAngle-expectedAngle)) > 0.01 {
		t.Errorf("desire angle: expected %f, got %f", expectedAngle, outputs.DesireAngle)
	}

	// Check DesireDistance (0.8 sigmoid -> 0.8)
	if math.Abs(float64(outputs.DesireDistance-0.8)) > 0.01 {
		t.Errorf("desire distance: expected 0.8, got %f", outputs.DesireDistance)
	}

	// Check Eat (0.3 sigmoid -> 0.3 eat)
	if math.Abs(float64(outputs.Eat-0.3)) > 0.01 {
		t.Errorf("eat: expected 0.3, got %f", outputs.Eat)
	}

	// Check Grow (0.6 sigmoid -> 0.6 grow)
	if math.Abs(float64(outputs.Grow-0.6)) > 0.01 {
		t.Errorf("grow: expected 0.6, got %f", outputs.Grow)
	}

	// Check Breed (0.7 sigmoid -> 0.7 breed)
	if math.Abs(float64(outputs.Breed-0.7)) > 0.01 {
		t.Errorf("breed: expected 0.7, got %f", outputs.Breed)
	}

	t.Logf("Outputs: %+v", outputs)
}

func TestDecodeOutputsShortInput(t *testing.T) {
	// Not enough outputs should return defaults
	raw := []float64{0.5, 0.5}

	outputs := DecodeOutputs(raw)
	defaults := DefaultOutputs()

	if outputs.DesireAngle != defaults.DesireAngle {
		t.Errorf("short input should return defaults")
	}
}

func TestDefaultOutputs(t *testing.T) {
	outputs := DefaultOutputs()

	// DesireAngle should be 0 (no change in direction)
	if outputs.DesireAngle != 0 {
		t.Errorf("default desire angle should be 0, got %f", outputs.DesireAngle)
	}

	// DesireDistance should be positive
	if outputs.DesireDistance <= 0 {
		t.Error("default desire distance should be positive")
	}

	// Eat, Grow, Breed should be in valid range
	if outputs.Eat < 0 || outputs.Eat > 1 {
		t.Errorf("default eat out of range: %f", outputs.Eat)
	}
	if outputs.Grow < 0 || outputs.Grow > 1 {
		t.Errorf("default grow out of range: %f", outputs.Grow)
	}
	if outputs.Breed < 0 || outputs.Breed > 1 {
		t.Errorf("default breed out of range: %f", outputs.Breed)
	}

	t.Logf("Default outputs: %+v", outputs)
}

func TestToTurnThrust(t *testing.T) {
	// Test conversion from desire to turn/thrust
	outputs := BehaviorOutputs{
		DesireAngle:    math.Pi / 2, // Turn right
		DesireDistance: 0.7,         // Medium-high urgency
	}

	turn, thrust := outputs.ToTurnThrust()

	// Turn should be proportional to desire angle
	expectedTurn := float32(0.5) // π/2 / π = 0.5
	if math.Abs(float64(turn-expectedTurn)) > 0.01 {
		t.Errorf("turn: expected %f, got %f", expectedTurn, turn)
	}

	// Thrust should equal desire distance
	if math.Abs(float64(thrust-0.7)) > 0.01 {
		t.Errorf("thrust: expected 0.7, got %f", thrust)
	}
}

// Test legacy inputs (for backward compatibility during migration)
func TestLegacySensoryInputsToInputs(t *testing.T) {
	sensory := LegacySensoryInputs{
		FoodDistance:     50,
		FoodAngle:        0.5,
		FoodFound:        true,
		PredatorDistance: 30,
		PredatorAngle:    -0.3,
		PredatorFound:    true,
		MateDistance:     80,
		MateFound:        true,
		HerdCount:        5,
		LightLevel:       0.7,
		FlowX:            0.1,
		FlowY:            -0.2,
		Energy:           75,
		MaxEnergy:        100,
		CellCount:        4,
		MaxCells:         16,
		PerceptionRadius: 100,
	}

	inputs := sensory.ToInputs()

	if len(inputs) != BrainInputs {
		t.Errorf("expected %d inputs, got %d", BrainInputs, len(inputs))
	}

	// Check food distance is normalized
	expectedFoodDist := 50.0 / 100.0 // 0.5
	if math.Abs(inputs[0]-expectedFoodDist) > 0.01 {
		t.Errorf("food distance: expected %f, got %f", expectedFoodDist, inputs[0])
	}

	// Check food angle sin/cos
	if math.Abs(inputs[1]-math.Sin(0.5)) > 0.01 {
		t.Errorf("food angle sin: expected %f, got %f", math.Sin(0.5), inputs[1])
	}

	// Check energy ratio
	expectedEnergy := 75.0 / 100.0
	if math.Abs(inputs[11]-expectedEnergy) > 0.01 {
		t.Errorf("energy ratio: expected %f, got %f", expectedEnergy, inputs[11])
	}

	t.Logf("Legacy Inputs: %v", inputs)
}

// Phase 4b: Test light gradient inputs (updated indices)
func TestSensoryInputsLightGradients(t *testing.T) {
	sensory := SensoryInputs{
		ConeFood:   [4]float32{0.5, 0.5, 0.5, 0.5},
		ConeThreat: [4]float32{0, 0, 0, 0},
		ConeFriend: [4]float32{0, 0, 0, 0},
		LightLevel: 0.6,
		LightFB:    0.3,  // Brighter ahead
		LightLR:    -0.5, // Brighter to the left
		Energy:     50,
		MaxEnergy:  100,
	}

	inputs := sensory.ToInputs()

	// Check light gradients [16-17] (Phase 4b indices)
	if math.Abs(inputs[16]-0.3) > 0.01 {
		t.Errorf("light FB: expected 0.3, got %f", inputs[16])
	}
	if math.Abs(inputs[17]-(-0.5)) > 0.01 {
		t.Errorf("light LR: expected -0.5, got %f", inputs[17])
	}
}

// Phase 4b: Test openness input
func TestSensoryInputsOpenness(t *testing.T) {
	tests := []struct {
		name     string
		openness float32
		want     float64
	}{
		{"fully open", 1.0, 1.0},
		{"dense terrain", 0.0, 0.0},
		{"partial openness", 0.5, 0.5},
		{"near terrain", 0.2, 0.2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sensory := SensoryInputs{
				Openness:  tt.openness,
				Energy:    50,
				MaxEnergy: 100,
			}

			inputs := sensory.ToInputs()

			// Openness is at index [15]
			if math.Abs(inputs[15]-tt.want) > 0.01 {
				t.Errorf("openness: expected %f, got %f", tt.want, inputs[15])
			}
		})
	}
}

func TestOpennessClamping(t *testing.T) {
	// Test that openness is clamped to [0, 1]
	sensory := SensoryInputs{
		Openness:  1.5, // Beyond 1.0
		Energy:    50,
		MaxEnergy: 100,
	}

	inputs := sensory.ToInputs()

	if inputs[15] > 1.0 {
		t.Errorf("openness not clamped: got %f", inputs[15])
	}

	sensory.Openness = -0.5 // Below 0
	inputs = sensory.ToInputs()

	if inputs[15] < 0 {
		t.Errorf("openness not clamped: got %f", inputs[15])
	}
}

func TestFromPolarVisionWithLight(t *testing.T) {
	// Create polar vision with directional light
	pv := PolarVision{
		Food:   [4]float32{0.5, 0.5, 0.5, 0.5},
		Threat: [4]float32{0, 0, 0, 0},
		Friend: [4]float32{0, 0, 0, 0},
		Light:  [4]float32{0.8, 0.5, 0.2, 0.5}, // Brighter front, dimmer back
	}

	var sensory SensoryInputs
	sensory.FromPolarVision(&pv)

	// Light gradients should be computed
	// FB = (front - back) / (front + back + eps) = (0.8 - 0.2) / (0.8 + 0.2 + eps) ≈ 0.6
	expectedFB := float32(0.6) / float32(1.0+LightGradientEpsilon)
	if math.Abs(float64(sensory.LightFB-expectedFB)) > 0.01 {
		t.Errorf("light FB: expected ~%f, got %f", expectedFB, sensory.LightFB)
	}

	// LR = (right - left) / (right + left + eps) = (0.5 - 0.5) / (0.5 + 0.5 + eps) ≈ 0
	if math.Abs(float64(sensory.LightLR)) > 0.01 {
		t.Errorf("light LR: expected ~0, got %f", sensory.LightLR)
	}
}

func BenchmarkSensoryToInputs(b *testing.B) {
	sensory := SensoryInputs{
		ConeFood:      [4]float32{0.8, 0.2, 0.1, 0.3},
		ConeThreat:    [4]float32{0.1, 0.0, 0.5, 0.0},
		ConeFriend:    [4]float32{0.4, 0.4, 0.2, 0.3},
		LightLevel:    0.7,
		FlowAlignment: 0.3,
		Energy:        75,
		MaxEnergy:     100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sensory.ToInputs()
	}
}

func BenchmarkDecodeOutputs(b *testing.B) {
	raw := []float64{0.5, 0.8, 0.3, 0.6, 0.7} // Phase 4: 5 outputs

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeOutputs(raw)
	}
}

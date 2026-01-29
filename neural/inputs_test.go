package neural

import (
	"math"
	"testing"
)

func TestSensoryInputsToInputs(t *testing.T) {
	// Phase 3: Test cone-based sensory inputs
	sensory := SensoryInputs{
		ConeFood:   [4]float32{0.8, 0.2, 0.1, 0.3},
		ConeThreat: [4]float32{0.1, 0.0, 0.5, 0.0},
		ConeFriend: [4]float32{0.4, 0.4, 0.2, 0.3},
		LightLevel: 0.7,
		FlowX:      0.1,
		FlowY:      -0.2,
		Energy:     75,
		MaxEnergy:  100,
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

	// Check flow [14-15]
	if math.Abs(inputs[14]-0.1) > 0.01 {
		t.Errorf("flow X: expected 0.1, got %f", inputs[14])
	}
	if math.Abs(inputs[15]-(-0.2)) > 0.01 {
		t.Errorf("flow Y: expected -0.2, got %f", inputs[15])
	}

	// Check bias [16]
	if inputs[16] != 1.0 {
		t.Errorf("bias: expected 1.0, got %f", inputs[16])
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
		ConeFood:   [4]float32{1.5, -0.1, 0.5, 2.0}, // Some out of range
		ConeThreat: [4]float32{0.5, 0.5, 1.5, -0.5},
		ConeFriend: [4]float32{0, 0, 0, 0},
		LightLevel: 1.5, // Beyond 1.0
		FlowX:      2.0, // Beyond expected range
		Energy:     150, // Beyond max
		MaxEnergy:  100,
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

	// Flow should be clamped to 1.0
	if inputs[14] > 1.0 {
		t.Errorf("flow X not clamped: got %f", inputs[14])
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
	// Simulate sigmoid outputs (0-1 range) - 4 outputs: Turn, Thrust, Eat, Mate
	raw := []float64{0.5, 0.8, 0.3, 0.6}

	outputs := DecodeOutputs(raw)

	// Check Turn (0.5 sigmoid -> 0.0 turn)
	expectedTurn := float32((0.5 - 0.5) * 2.0) // 0.0
	if math.Abs(float64(outputs.Turn-expectedTurn)) > 0.01 {
		t.Errorf("turn: expected %f, got %f", expectedTurn, outputs.Turn)
	}

	// Check Thrust (0.8 sigmoid -> 0.8 thrust)
	if math.Abs(float64(outputs.Thrust-0.8)) > 0.01 {
		t.Errorf("thrust: expected 0.8, got %f", outputs.Thrust)
	}

	// Check Eat (0.3 sigmoid -> 0.3 eat)
	if math.Abs(float64(outputs.Eat-0.3)) > 0.01 {
		t.Errorf("eat: expected 0.3, got %f", outputs.Eat)
	}

	// Check Mate (0.6 sigmoid -> 0.6 mate)
	if math.Abs(float64(outputs.Mate-0.6)) > 0.01 {
		t.Errorf("mate: expected 0.6, got %f", outputs.Mate)
	}

	t.Logf("Outputs: %+v", outputs)
}

func TestDecodeOutputsShortInput(t *testing.T) {
	// Not enough outputs should return defaults
	raw := []float64{0.5, 0.5}

	outputs := DecodeOutputs(raw)
	defaults := DefaultOutputs()

	if outputs.Turn != defaults.Turn {
		t.Errorf("short input should return defaults")
	}
}

func TestDefaultOutputs(t *testing.T) {
	outputs := DefaultOutputs()

	// Turn should be 0 (no turning)
	if outputs.Turn != 0 {
		t.Errorf("default turn should be 0, got %f", outputs.Turn)
	}

	// Thrust should be positive
	if outputs.Thrust <= 0 {
		t.Error("default thrust should be positive")
	}

	// Eat and Mate should be in valid range
	if outputs.Eat < 0 || outputs.Eat > 1 {
		t.Errorf("default eat out of range: %f", outputs.Eat)
	}
	if outputs.Mate < 0 || outputs.Mate > 1 {
		t.Errorf("default mate out of range: %f", outputs.Mate)
	}

	t.Logf("Default outputs: %+v", outputs)
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

func BenchmarkSensoryToInputs(b *testing.B) {
	sensory := SensoryInputs{
		ConeFood:   [4]float32{0.8, 0.2, 0.1, 0.3},
		ConeThreat: [4]float32{0.1, 0.0, 0.5, 0.0},
		ConeFriend: [4]float32{0.4, 0.4, 0.2, 0.3},
		LightLevel: 0.7,
		FlowX:      0.1,
		FlowY:      -0.2,
		Energy:     75,
		MaxEnergy:  100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sensory.ToInputs()
	}
}

func BenchmarkDecodeOutputs(b *testing.B) {
	raw := []float64{0.5, 0.8, 0.3, 0.6}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeOutputs(raw)
	}
}

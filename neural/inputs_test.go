package neural

import (
	"math"
	"testing"
)

func TestSensoryInputsToInputs(t *testing.T) {
	sensory := SensoryInputs{
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
		MaxCells:         32,
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

	// Check bias
	if inputs[13] != 1.0 {
		t.Errorf("bias: expected 1.0, got %f", inputs[13])
	}

	t.Logf("Inputs: %v", inputs)
}

func TestSensoryInputsNoFood(t *testing.T) {
	sensory := SensoryInputs{
		FoodFound:        false,
		PerceptionRadius: 100,
		Energy:           50,
		MaxEnergy:        100,
	}

	inputs := sensory.ToInputs()

	// No food should give max distance (1.0)
	if inputs[0] != 1.0 {
		t.Errorf("no food distance: expected 1.0, got %f", inputs[0])
	}

	// Angles should be zero
	if inputs[1] != 0.0 || inputs[2] != 0.0 {
		t.Errorf("no food angles: expected 0,0, got %f,%f", inputs[1], inputs[2])
	}
}

func TestSensoryInputsNormalization(t *testing.T) {
	// Test clamping of extreme values
	sensory := SensoryInputs{
		FoodDistance:     200, // Beyond perception radius
		FoodFound:        true,
		HerdCount:        20, // Beyond expected max
		LightLevel:       1.5, // Beyond 1.0
		FlowX:            2.0, // Beyond expected range
		Energy:           150, // Beyond max
		MaxEnergy:        100,
		PerceptionRadius: 100,
	}

	inputs := sensory.ToInputs()

	// Food distance should be clamped to 1.0
	if inputs[0] > 1.0 {
		t.Errorf("food distance not clamped: got %f", inputs[0])
	}

	// Herd density should be clamped to 1.0
	if inputs[7] > 1.0 {
		t.Errorf("herd density not clamped: got %f", inputs[7])
	}

	// Light level should be clamped to 1.0
	if inputs[8] > 1.0 {
		t.Errorf("light level not clamped: got %f", inputs[8])
	}

	// Flow should be clamped to 1.0
	if inputs[9] > 1.0 {
		t.Errorf("flow X not clamped: got %f", inputs[9])
	}

	// All values should be in valid range
	for i, v := range inputs {
		if v < -1.0 || v > 1.0 {
			// Only sin/cos can be negative, rest should be 0-1
			if i != 1 && i != 2 && i != 4 && i != 5 && i != 9 && i != 10 {
				if v < 0 || v > 1 {
					t.Errorf("input %d out of range [0,1]: %f", i, v)
				}
			}
		}
	}
}

func TestDecodeOutputs(t *testing.T) {
	// Simulate sigmoid outputs (0-1 range) - now 9 outputs including speed
	raw := []float64{0.5, 0.8, 0.3, 0.6, 0.4, 0.7, 0.2, 0.5, 0.6}

	outputs := DecodeOutputs(raw)

	// Check scaling
	expectedSeekFood := float32(0.5 * 3.0) // 1.5
	if math.Abs(float64(outputs.SeekFoodWeight-expectedSeekFood)) > 0.01 {
		t.Errorf("seek food weight: expected %f, got %f", expectedSeekFood, outputs.SeekFoodWeight)
	}

	expectedFlee := float32(0.8 * 5.0) // 4.0
	if math.Abs(float64(outputs.FleeWeight-expectedFlee)) > 0.01 {
		t.Errorf("flee weight: expected %f, got %f", expectedFlee, outputs.FleeWeight)
	}

	// Allocation drives should be raw values
	if math.Abs(float64(outputs.GrowDrive-0.7)) > 0.01 {
		t.Errorf("grow drive: expected 0.7, got %f", outputs.GrowDrive)
	}

	// Speed multiplier should be raw value
	if math.Abs(float64(outputs.SpeedMultiplier-0.6)) > 0.01 {
		t.Errorf("speed multiplier: expected 0.6, got %f", outputs.SpeedMultiplier)
	}

	t.Logf("Outputs: %+v", outputs)
}

func TestDecodeOutputsShortInput(t *testing.T) {
	// Not enough outputs should return defaults
	raw := []float64{0.5, 0.5}

	outputs := DecodeOutputs(raw)
	defaults := DefaultOutputs()

	if outputs.SeekFoodWeight != defaults.SeekFoodWeight {
		t.Errorf("short input should return defaults")
	}
}

func TestDefaultOutputs(t *testing.T) {
	outputs := DefaultOutputs()

	// Should have reasonable defaults
	if outputs.SeekFoodWeight <= 0 {
		t.Error("default seek food weight should be positive")
	}
	if outputs.FleeWeight <= 0 {
		t.Error("default flee weight should be positive")
	}
	if outputs.WanderWeight <= 0 {
		t.Error("default wander weight should be positive")
	}

	t.Logf("Default outputs: %+v", outputs)
}

func BenchmarkSensoryToInputs(b *testing.B) {
	sensory := SensoryInputs{
		FoodDistance:     50,
		FoodAngle:        0.5,
		FoodFound:        true,
		PredatorDistance: 30,
		PredatorAngle:    -0.3,
		PredatorFound:    true,
		HerdCount:        5,
		LightLevel:       0.7,
		FlowX:            0.1,
		FlowY:            -0.2,
		Energy:           75,
		MaxEnergy:        100,
		CellCount:        4,
		MaxCells:         32,
		PerceptionRadius: 100,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sensory.ToInputs()
	}
}

func BenchmarkDecodeOutputs(b *testing.B) {
	raw := []float64{0.5, 0.8, 0.3, 0.6, 0.4, 0.7, 0.2, 0.5, 0.6}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeOutputs(raw)
	}
}

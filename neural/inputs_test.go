package neural

import (
	"math"
	"testing"
)

func TestSensoryInputsToInputs(t *testing.T) {
	// Test boid-field based sensory inputs with approach geometry (30 inputs)
	sensory := SensoryInputs{
		SpeedNorm:  0.5,
		EnergyNorm: 0.75,
		Body: BodyDescriptor{
			SizeNorm:      0.5,
			SpeedCapacity: 0.6,
			AgilityNorm:   0.7,
			SenseStrength: 0.4,
			BiteStrength:  0.3,
			ArmorLevel:    0.2,
		},
		Boid: BoidFields{
			CohesionFwd:   0.5,
			CohesionUp:    -0.3,
			CohesionMag:   0.4,
			AlignmentFwd:  0.8,
			AlignmentUp:   0.2,
			SeparationFwd: -0.4,
			SeparationUp:  0.1,
			SeparationMag: 0.3,
			DensitySame:   0.5,
		},
		Food: FoodFields{
			PlantFwd: 0.6,
			PlantUp:  0.2,
			PlantMag: 0.5,
			MeatFwd:  -0.3,
			MeatUp:   0.7,
			MeatMag:  0.2,
		},
		Threat: ThreatInfo{
			Proximity:    0.3,
			ClosingSpeed: -0.2,
		},
		Approach: ApproachInfo{
			NearestFoodDist:    0.8,
			NearestFoodBearing: 0.2,
			NearestMateDist:    0.6,
			NearestMateBearing: -0.4,
		},
	}

	inputs := sensory.ToInputs()

	if len(inputs) != BrainInputs {
		t.Errorf("expected %d inputs, got %d", BrainInputs, len(inputs))
	}

	// Check self state [0-1]
	if math.Abs(inputs[0]-0.5) > 0.01 {
		t.Errorf("speed norm: expected 0.5, got %f", inputs[0])
	}
	if math.Abs(inputs[1]-0.75) > 0.01 {
		t.Errorf("energy norm: expected 0.75, got %f", inputs[1])
	}

	// Check body descriptor [2-7]
	if math.Abs(inputs[2]-0.5) > 0.01 {
		t.Errorf("size norm: expected 0.5, got %f", inputs[2])
	}
	if math.Abs(inputs[3]-0.6) > 0.01 {
		t.Errorf("speed capacity: expected 0.6, got %f", inputs[3])
	}

	// Check boid fields [8-16]
	if math.Abs(inputs[8]-0.5) > 0.01 {
		t.Errorf("cohesion fwd: expected 0.5, got %f", inputs[8])
	}
	if math.Abs(inputs[9]-(-0.3)) > 0.01 {
		t.Errorf("cohesion up: expected -0.3, got %f", inputs[9])
	}

	// Check food fields [17-22]
	if math.Abs(inputs[17]-0.6) > 0.01 {
		t.Errorf("plant fwd: expected 0.6, got %f", inputs[17])
	}

	// Check threat [23-24]
	if math.Abs(inputs[23]-0.3) > 0.01 {
		t.Errorf("threat proximity: expected 0.3, got %f", inputs[23])
	}
	if math.Abs(inputs[24]-(-0.2)) > 0.01 {
		t.Errorf("closing speed: expected -0.2, got %f", inputs[24])
	}

	// Check approach [25-28]
	if math.Abs(inputs[25]-0.8) > 0.01 {
		t.Errorf("nearest food dist: expected 0.8, got %f", inputs[25])
	}
	if math.Abs(inputs[26]-0.2) > 0.01 {
		t.Errorf("nearest food bearing: expected 0.2, got %f", inputs[26])
	}
	if math.Abs(inputs[27]-0.6) > 0.01 {
		t.Errorf("nearest mate dist: expected 0.6, got %f", inputs[27])
	}
	if math.Abs(inputs[28]-(-0.4)) > 0.01 {
		t.Errorf("nearest mate bearing: expected -0.4, got %f", inputs[28])
	}

	// Check bias [29]
	if inputs[29] != 1.0 {
		t.Errorf("bias: expected 1.0, got %f", inputs[29])
	}

	t.Logf("Inputs: %v", inputs)
}

func TestSensoryInputsZeroValues(t *testing.T) {
	// Test with zero values
	sensory := SensoryInputs{}

	inputs := sensory.ToInputs()

	// All values should be zero except bias
	for i := 0; i < BrainInputs-1; i++ {
		if inputs[i] != 0.0 {
			t.Errorf("input %d should be 0, got %f", i, inputs[i])
		}
	}

	// Bias should be 1
	if inputs[BrainInputs-1] != 1.0 {
		t.Errorf("bias should be 1.0, got %f", inputs[BrainInputs-1])
	}
}

func TestSensoryInputsNormalization(t *testing.T) {
	// Test clamping of extreme values
	sensory := SensoryInputs{
		SpeedNorm:  1.5, // Beyond 1.0
		EnergyNorm: -0.1, // Below 0.0
		Body: BodyDescriptor{
			SizeNorm:      2.0, // Beyond 1.0
			SpeedCapacity: 0.5,
			AgilityNorm:   0.5,
			SenseStrength: 0.5,
			BiteStrength:  0.5,
			ArmorLevel:    0.5,
		},
		Boid: BoidFields{
			CohesionFwd: 2.0, // Beyond 1.0 for [-1,1] range
		},
	}

	inputs := sensory.ToInputs()

	// Self state should be clamped to [0, 1]
	if inputs[0] > 1.0 || inputs[0] < 0 {
		t.Errorf("speed norm not clamped: got %f", inputs[0])
	}
	if inputs[1] > 1.0 || inputs[1] < 0 {
		t.Errorf("energy norm not clamped: got %f", inputs[1])
	}

	// Body descriptor should be clamped
	if inputs[2] > 1.0 {
		t.Errorf("size norm not clamped: got %f", inputs[2])
	}

	// Boid fields [-1, 1] should be clamped
	if inputs[8] > 1.0 || inputs[8] < -1.0 {
		t.Errorf("cohesion fwd not clamped: got %f", inputs[8])
	}
}

func TestDecodeOutputs(t *testing.T) {
	// Test 4-output structure: [UTurn, UThrottle, AttackIntent, MateIntent]
	// Raw outputs are in [0, 1] range from sigmoid
	raw := []float64{0.75, 0.8, 0.8, 0.3}

	outputs := DecodeOutputs(raw)

	// UTurn: 0.75 sigmoid -> (0.75 * 2 - 1) = 0.5
	expectedUTurn := float32(0.75*2.0 - 1.0) // 0.5
	if math.Abs(float64(outputs.UTurn-expectedUTurn)) > 0.01 {
		t.Errorf("UTurn: expected %f, got %f", expectedUTurn, outputs.UTurn)
	}

	// UThrottle: 0.8 raw (stays in [0,1])
	expectedUThrottle := float32(0.8)
	if math.Abs(float64(outputs.UThrottle-expectedUThrottle)) > 0.01 {
		t.Errorf("UThrottle: expected %f, got %f", expectedUThrottle, outputs.UThrottle)
	}

	// AttackIntent: 0.8 raw
	if math.Abs(float64(outputs.AttackIntent-0.8)) > 0.01 {
		t.Errorf("AttackIntent: expected 0.8, got %f", outputs.AttackIntent)
	}

	// MateIntent: 0.3 raw
	if math.Abs(float64(outputs.MateIntent-0.3)) > 0.01 {
		t.Errorf("MateIntent: expected 0.3, got %f", outputs.MateIntent)
	}

	t.Logf("Outputs: %+v", outputs)
}

func TestDecodeOutputsShortInput(t *testing.T) {
	// Not enough outputs should return defaults
	raw := []float64{0.5, 0.5}

	outputs := DecodeOutputs(raw)
	defaults := DefaultOutputs()

	if outputs.UTurn != defaults.UTurn {
		t.Errorf("short input should return defaults")
	}
}

func TestDefaultOutputs(t *testing.T) {
	outputs := DefaultOutputs()

	// All outputs should be 0 for defaults
	if outputs.UTurn != 0 {
		t.Errorf("default UTurn should be 0, got %f", outputs.UTurn)
	}
	if outputs.UThrottle != 0 {
		t.Errorf("default UThrottle should be 0, got %f", outputs.UThrottle)
	}
	if outputs.AttackIntent != 0 {
		t.Errorf("default AttackIntent should be 0, got %f", outputs.AttackIntent)
	}
	if outputs.MateIntent != 0 {
		t.Errorf("default MateIntent should be 0, got %f", outputs.MateIntent)
	}

	t.Logf("Default outputs: %+v", outputs)
}

func BenchmarkSensoryToInputs(b *testing.B) {
	sensory := SensoryInputs{
		SpeedNorm:  0.5,
		EnergyNorm: 0.75,
		Body: BodyDescriptor{
			SizeNorm:      0.5,
			SpeedCapacity: 0.6,
			AgilityNorm:   0.7,
			SenseStrength: 0.4,
			BiteStrength:  0.3,
			ArmorLevel:    0.2,
		},
		Boid: BoidFields{
			CohesionFwd:   0.5,
			CohesionUp:    -0.3,
			CohesionMag:   0.4,
			AlignmentFwd:  0.8,
			AlignmentUp:   0.2,
			SeparationFwd: -0.4,
			SeparationUp:  0.1,
			SeparationMag: 0.3,
			DensitySame:   0.5,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sensory.ToInputs()
	}
}

func BenchmarkDecodeOutputs(b *testing.B) {
	raw := []float64{0.5, 0.8, 0.3, 0.6} // 4 outputs

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = DecodeOutputs(raw)
	}
}

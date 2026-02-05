package neural

import (
	"math/rand"
	"testing"
)

func TestNewFFNN(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	hiddenLayers := []int{16}
	nn := NewFFNN(rng, hiddenLayers, 0)

	if nn == nil {
		t.Fatal("NewFFNN returned nil")
	}

	// Check layer structure
	expectedLayers := []int{NumInputs, 16, NumOutputs}
	if len(nn.Layers) != len(expectedLayers) {
		t.Errorf("wrong number of layers: got %d, want %d", len(nn.Layers), len(expectedLayers))
	}
	for i, size := range expectedLayers {
		if nn.Layers[i] != size {
			t.Errorf("layer %d size wrong: got %d, want %d", i, nn.Layers[i], size)
		}
	}

	// Check weight matrix dimensions
	// Weights[0] connects input (28) to hidden (16)
	if len(nn.Weights[0]) != 16 {
		t.Errorf("Weights[0] fan-out wrong: got %d, want 16", len(nn.Weights[0]))
	}
	if len(nn.Weights[0][0]) != NumInputs {
		t.Errorf("Weights[0][0] fan-in wrong: got %d, want %d", len(nn.Weights[0][0]), NumInputs)
	}
	// Weights[1] connects hidden (16) to output (3)
	if len(nn.Weights[1]) != NumOutputs {
		t.Errorf("Weights[1] fan-out wrong: got %d, want %d", len(nn.Weights[1]), NumOutputs)
	}
	if len(nn.Weights[1][0]) != 16 {
		t.Errorf("Weights[1][0] fan-in wrong: got %d, want 16", len(nn.Weights[1][0]))
	}
}

func TestNewFFNNMultipleHidden(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	hiddenLayers := []int{16, 8}
	nn := NewFFNN(rng, hiddenLayers, 0)

	if nn == nil {
		t.Fatal("NewFFNN returned nil")
	}

	// Check layer structure: 28 -> 16 -> 8 -> 3
	expectedLayers := []int{NumInputs, 16, 8, NumOutputs}
	if len(nn.Layers) != len(expectedLayers) {
		t.Errorf("wrong number of layers: got %d, want %d", len(nn.Layers), len(expectedLayers))
	}
	for i, size := range expectedLayers {
		if nn.Layers[i] != size {
			t.Errorf("layer %d size wrong: got %d, want %d", i, nn.Layers[i], size)
		}
	}

	// Should have 3 weight matrices
	if len(nn.Weights) != 3 {
		t.Errorf("wrong number of weight matrices: got %d, want 3", len(nn.Weights))
	}
}

func TestForward(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16}, 0)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	turn, thrust, bite := nn.Forward(inputs)

	if turn < -1 || turn > 1 {
		t.Errorf("turn out of range [-1,1]: %f", turn)
	}
	if thrust < 0 || thrust > 1 {
		t.Errorf("thrust out of range [0,1]: %f", thrust)
	}
	if bite < 0 || bite > 1 {
		t.Errorf("bite out of range [0,1]: %f", bite)
	}
}

func TestForwardMultipleHidden(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16, 8}, 0)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	turn, thrust, bite := nn.Forward(inputs)

	if turn < -1 || turn > 1 {
		t.Errorf("turn out of range [-1,1]: %f", turn)
	}
	if thrust < 0 || thrust > 1 {
		t.Errorf("thrust out of range [0,1]: %f", thrust)
	}
	if bite < 0 || bite > 1 {
		t.Errorf("bite out of range [0,1]: %f", bite)
	}
}

func TestForwardDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16}, 0)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = float32(i) / float32(NumInputs)
	}

	turn1, thrust1, bite1 := nn.Forward(inputs)
	turn2, thrust2, bite2 := nn.Forward(inputs)

	if turn1 != turn2 || thrust1 != thrust2 || bite1 != bite2 {
		t.Error("Forward is not deterministic")
	}
}

func TestMutate(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16}, 0)

	originalWeight := nn.Weights[0][0][0]

	nn.Mutate(rng, 0.1)

	if nn.Weights[0][0][0] == originalWeight {
		t.Error("Mutate did not change weights")
	}
}

func TestClone(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16}, 0)

	clone := nn.Clone()

	// Clone should have same weights
	if nn.Weights[0][0][0] != clone.Weights[0][0][0] {
		t.Error("Clone has different weights")
	}

	// Modifying clone shouldn't affect original
	clone.Weights[0][0][0] = 999
	if nn.Weights[0][0][0] == 999 {
		t.Error("Clone is not independent")
	}
}

func TestMarshalUnmarshal(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	original := NewFFNN(rng, []int{16, 8}, 0)

	// Marshal
	weights := original.MarshalWeights()

	// Create new network and unmarshal
	restored := &FFNN{}
	restored.UnmarshalWeights(weights)

	// Check layer structure preserved
	if len(restored.Layers) != len(original.Layers) {
		t.Errorf("layer count mismatch: got %d, want %d", len(restored.Layers), len(original.Layers))
	}

	// Run forward pass on both
	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	turn1, thrust1, bite1 := original.Forward(inputs)
	turn2, thrust2, bite2 := restored.Forward(inputs)

	if turn1 != turn2 || thrust1 != thrust2 || bite1 != bite2 {
		t.Errorf("restored network produces different output: got (%f,%f,%f), want (%f,%f,%f)",
			turn2, thrust2, bite2, turn1, thrust1, bite1)
	}
}

func BenchmarkForward(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16}, 0)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nn.Forward(inputs)
	}
}

func BenchmarkForwardMultipleHidden(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng, []int{16, 8}, 0)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nn.Forward(inputs)
	}
}

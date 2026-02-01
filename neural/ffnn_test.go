package neural

import (
	"math/rand"
	"testing"
)

func TestNewFFNN(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng)

	if nn == nil {
		t.Fatal("NewFFNN returned nil")
	}

	// Check dimensions
	if len(nn.W1) != NumHidden {
		t.Errorf("W1 has wrong dimensions: got %d, want %d", len(nn.W1), NumHidden)
	}
	if len(nn.W1[0]) != NumInputs {
		t.Errorf("W1[0] has wrong dimensions: got %d, want %d", len(nn.W1[0]), NumInputs)
	}
	if len(nn.W2) != NumOutputs {
		t.Errorf("W2 has wrong dimensions: got %d, want %d", len(nn.W2), NumOutputs)
	}
	if len(nn.W2[0]) != NumHidden {
		t.Errorf("W2[0] has wrong dimensions: got %d, want %d", len(nn.W2[0]), NumHidden)
	}
}

func TestForward(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng)

	// Create input vector
	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	turn, thrust, bite := nn.Forward(inputs)

	// Check output ranges
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
	nn := NewFFNN(rng)

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
	nn := NewFFNN(rng)

	// Store original weights
	originalW1_0_0 := nn.W1[0][0]

	// Mutate
	nn.Mutate(rng, 0.1)

	// Weights should have changed
	if nn.W1[0][0] == originalW1_0_0 {
		t.Error("Mutate did not change weights")
	}
}

func TestClone(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng)

	clone := nn.Clone()

	// Clone should have same weights
	if nn.W1[0][0] != clone.W1[0][0] {
		t.Error("Clone has different weights")
	}

	// Modifying clone shouldn't affect original
	clone.W1[0][0] = 999
	if nn.W1[0][0] == 999 {
		t.Error("Clone is not independent")
	}
}

func BenchmarkForward(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	nn := NewFFNN(rng)

	inputs := make([]float32, NumInputs)
	for i := range inputs {
		inputs[i] = 0.5
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		nn.Forward(inputs)
	}
}

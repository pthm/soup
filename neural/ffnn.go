// Package neural provides feedforward neural network brains for entities.
package neural

import (
	"math"
	"math/rand"
)

// Network dimensions for Entity v0.
const (
	NumInputs  = 17 // K*3 sectors + 2 self-state (K=5)
	NumHidden  = 12
	NumOutputs = 3 // turn, thrust, bite
)

// FFNN is a simple two-layer feedforward neural network.
type FFNN struct {
	W1 [NumHidden][NumInputs]float32  // input -> hidden weights
	B1 [NumHidden]float32             // hidden biases
	W2 [NumOutputs][NumHidden]float32 // hidden -> output weights
	B2 [NumOutputs]float32            // output biases
}

// NewFFNN creates a randomly initialized network.
func NewFFNN(rng *rand.Rand) *FFNN {
	nn := &FFNN{}
	// Xavier initialization
	scale1 := float32(math.Sqrt(2.0 / float64(NumInputs)))
	scale2 := float32(math.Sqrt(2.0 / float64(NumHidden)))

	for i := range nn.W1 {
		for j := range nn.W1[i] {
			nn.W1[i][j] = float32(rng.NormFloat64()) * scale1
		}
		nn.B1[i] = 0
	}

	for i := range nn.W2 {
		for j := range nn.W2[i] {
			nn.W2[i][j] = float32(rng.NormFloat64()) * scale2
		}
		nn.B2[i] = 0
	}

	return nn
}

// Forward computes the network output.
// Returns: turn [-1,1], thrust [0,1], bite [0,1]
func (nn *FFNN) Forward(inputs []float32) (turn, thrust, bite float32) {
	// Hidden layer with tanh activation
	var hidden [NumHidden]float32
	for i := 0; i < NumHidden; i++ {
		sum := nn.B1[i]
		for j := 0; j < NumInputs; j++ {
			sum += nn.W1[i][j] * inputs[j]
		}
		hidden[i] = tanh(sum)
	}

	// Output layer
	var outputs [NumOutputs]float32
	for i := 0; i < NumOutputs; i++ {
		sum := nn.B2[i]
		for j := 0; j < NumHidden; j++ {
			sum += nn.W2[i][j] * hidden[j]
		}
		outputs[i] = sum
	}

	// Apply output activations
	turn = tanh(outputs[0])         // [-1, 1]
	thrust = sigmoid(outputs[1])    // [0, 1]
	bite = sigmoid(outputs[2])      // [0, 1]

	return turn, thrust, bite
}

// Activations holds captured intermediate layer values.
type Activations struct {
	Inputs  []float32
	Hidden  []float32
	Outputs []float32 // [turn, thrust, bite] after activation
}

// ForwardWithCapture computes the network output and captures all layer activations.
// Returns: turn [-1,1], thrust [0,1], bite [0,1], and activations for visualization.
func (nn *FFNN) ForwardWithCapture(inputs []float32) (turn, thrust, bite float32, act *Activations) {
	act = &Activations{
		Inputs:  make([]float32, len(inputs)),
		Hidden:  make([]float32, NumHidden),
		Outputs: make([]float32, NumOutputs),
	}

	// Capture inputs
	copy(act.Inputs, inputs)

	// Hidden layer with tanh activation
	var hidden [NumHidden]float32
	for i := 0; i < NumHidden; i++ {
		sum := nn.B1[i]
		for j := 0; j < NumInputs; j++ {
			sum += nn.W1[i][j] * inputs[j]
		}
		hidden[i] = tanh(sum)
		act.Hidden[i] = hidden[i]
	}

	// Output layer
	var outputs [NumOutputs]float32
	for i := 0; i < NumOutputs; i++ {
		sum := nn.B2[i]
		for j := 0; j < NumHidden; j++ {
			sum += nn.W2[i][j] * hidden[j]
		}
		outputs[i] = sum
	}

	// Apply output activations
	turn = tanh(outputs[0])      // [-1, 1]
	thrust = sigmoid(outputs[1]) // [0, 1]
	bite = sigmoid(outputs[2])   // [0, 1]

	// Capture activated outputs
	act.Outputs[0] = turn
	act.Outputs[1] = thrust
	act.Outputs[2] = bite

	return turn, thrust, bite, act
}

// Mutate perturbs weights and biases with Gaussian noise.
func (nn *FFNN) Mutate(rng *rand.Rand, strength float32) {
	for i := range nn.W1 {
		for j := range nn.W1[i] {
			nn.W1[i][j] += float32(rng.NormFloat64()) * strength
		}
		nn.B1[i] += float32(rng.NormFloat64()) * strength
	}

	for i := range nn.W2 {
		for j := range nn.W2[i] {
			nn.W2[i][j] += float32(rng.NormFloat64()) * strength
		}
		nn.B2[i] += float32(rng.NormFloat64()) * strength
	}
}

// Clone creates a deep copy of the network.
func (nn *FFNN) Clone() *FFNN {
	clone := &FFNN{}
	for i := range nn.W1 {
		clone.W1[i] = nn.W1[i]
	}
	clone.B1 = nn.B1
	for i := range nn.W2 {
		clone.W2[i] = nn.W2[i]
	}
	clone.B2 = nn.B2
	return clone
}

func tanh(x float32) float32 {
	return float32(math.Tanh(float64(x)))
}

func sigmoid(x float32) float32 {
	return 1.0 / (1.0 + float32(math.Exp(-float64(x))))
}

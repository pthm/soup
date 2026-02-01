// Package neural provides feedforward neural network brains for entities.
package neural

import (
	"math"
	"math/rand"
)

// Network dimensions (compile-time constants for array sizing).
// These values must match the corresponding config values:
// - NumInputs = sensors.num_sectors * 3 + 2
// - NumHidden = neural.num_hidden
// - NumOutputs = neural.num_outputs
const (
	NumInputs  = 26 // K*3 sectors + 2 self-state (K=8)
	NumHidden  = 16
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
	// Hidden layer with fast tanh activation
	// (tanh's |x|>4 branches are rarely taken, good for branch prediction)
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
	// tanh for turn [-1,1], saturating linear for thrust/bite [0,1]
	turn = tanh(outputs[0])
	thrust = saturate01(outputs[1]*0.5 + 0.5)
	bite = saturate01(outputs[2]*0.5 + 0.5)

	return turn, thrust, bite
}

// saturate01 clamps x to [0, 1] - fastest possible [0,1] activation.
func saturate01(x float32) float32 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	return x
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

	// Hidden layer with fast tanh activation
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

	// Apply output activations (same as Forward)
	turn = tanh(outputs[0])
	thrust = saturate01(outputs[1]*0.5 + 0.5)
	bite = saturate01(outputs[2]*0.5 + 0.5)

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

// MutateSparse applies sparse per-weight mutation for stable lineages.
// rate: probability each weight mutates (e.g., 0.05)
// sigma: standard deviation of normal perturbation (e.g., 0.08)
// bigRate: probability of a large mutation (e.g., 0.01)
// bigSigma: sigma for large mutations (e.g., 0.4)
func (nn *FFNN) MutateSparse(rng *rand.Rand, rate, sigma, bigRate, bigSigma float32) {
	biasRate := rate * 0.5 // biases mutate at half the rate

	// Hidden layer weights
	for i := range nn.W1 {
		for j := range nn.W1[i] {
			if rng.Float32() < rate {
				if rng.Float32() < bigRate {
					nn.W1[i][j] += float32(rng.NormFloat64()) * bigSigma
				} else {
					nn.W1[i][j] += float32(rng.NormFloat64()) * sigma
				}
			}
		}
		// Hidden biases
		if rng.Float32() < biasRate {
			if rng.Float32() < bigRate {
				nn.B1[i] += float32(rng.NormFloat64()) * bigSigma
			} else {
				nn.B1[i] += float32(rng.NormFloat64()) * sigma
			}
		}
	}

	// Output layer weights
	for i := range nn.W2 {
		for j := range nn.W2[i] {
			if rng.Float32() < rate {
				if rng.Float32() < bigRate {
					nn.W2[i][j] += float32(rng.NormFloat64()) * bigSigma
				} else {
					nn.W2[i][j] += float32(rng.NormFloat64()) * sigma
				}
			}
		}
		// Output biases
		if rng.Float32() < biasRate {
			if rng.Float32() < bigRate {
				nn.B2[i] += float32(rng.NormFloat64()) * bigSigma
			} else {
				nn.B2[i] += float32(rng.NormFloat64()) * sigma
			}
		}
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

// leakyReLU is the fastest useful activation: no exp, no div.
func leakyReLU(x float32) float32 {
	if x > 0 {
		return x
	}
	return 0.01 * x
}

// softsign: x / (1 + |x|), bounded to [-1, 1], smooth, no exp.
func softsign(x float32) float32 {
	if x >= 0 {
		return x / (1 + x)
	}
	return x / (1 - x)
}

// tanh uses a fast rational approximation avoiding float64 conversion.
// Kept for compatibility but no longer used in Forward().
func tanh(x float32) float32 {
	if x > 4 {
		return 1
	}
	if x < -4 {
		return -1
	}
	x2 := x * x
	return x * (27 + x2) / (27 + 9*x2)
}

// sigmoid uses fast tanh: sigmoid(x) = 0.5 * (1 + tanh(x/2))
// Kept for compatibility but no longer used in Forward().
func sigmoid(x float32) float32 {
	return 0.5 * (1 + tanh(x*0.5))
}

// BrainWeights holds flattened network weights for serialization.
type BrainWeights struct {
	W1 []float32 `json:"w1"` // [NumHidden * NumInputs]
	B1 []float32 `json:"b1"` // [NumHidden]
	W2 []float32 `json:"w2"` // [NumOutputs * NumHidden]
	B2 []float32 `json:"b2"` // [NumOutputs]
}

// MarshalWeights flattens the network weights for JSON serialization.
func (nn *FFNN) MarshalWeights() BrainWeights {
	bw := BrainWeights{
		W1: make([]float32, NumHidden*NumInputs),
		B1: make([]float32, NumHidden),
		W2: make([]float32, NumOutputs*NumHidden),
		B2: make([]float32, NumOutputs),
	}

	// Flatten W1
	for i := 0; i < NumHidden; i++ {
		for j := 0; j < NumInputs; j++ {
			bw.W1[i*NumInputs+j] = nn.W1[i][j]
		}
	}

	// Copy B1
	copy(bw.B1, nn.B1[:])

	// Flatten W2
	for i := 0; i < NumOutputs; i++ {
		for j := 0; j < NumHidden; j++ {
			bw.W2[i*NumHidden+j] = nn.W2[i][j]
		}
	}

	// Copy B2
	copy(bw.B2, nn.B2[:])

	return bw
}

// UnmarshalWeights restores network weights from flattened form.
func (nn *FFNN) UnmarshalWeights(bw BrainWeights) {
	// Restore W1
	for i := 0; i < NumHidden; i++ {
		for j := 0; j < NumInputs; j++ {
			if i*NumInputs+j < len(bw.W1) {
				nn.W1[i][j] = bw.W1[i*NumInputs+j]
			}
		}
	}

	// Restore B1
	for i := 0; i < NumHidden && i < len(bw.B1); i++ {
		nn.B1[i] = bw.B1[i]
	}

	// Restore W2
	for i := 0; i < NumOutputs; i++ {
		for j := 0; j < NumHidden; j++ {
			if i*NumHidden+j < len(bw.W2) {
				nn.W2[i][j] = bw.W2[i*NumHidden+j]
			}
		}
	}

	// Restore B2
	for i := 0; i < NumOutputs && i < len(bw.B2); i++ {
		nn.B2[i] = bw.B2[i]
	}
}

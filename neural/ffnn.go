// Package neural provides feedforward neural network brains for entities.
package neural

import (
	"math"
	"math/rand"
)

// Network dimension constants.
// NumInputs and NumOutputs are fixed based on sensor/action space.
// Hidden layer sizes are configurable via config.
const (
	NumInputs  = 28 // K*3 sectors + 4 self-state (K=8): energy, speed, diet, metabolic_rate
	NumOutputs = 3  // turn, thrust, bite
)

// FFNN is a feedforward neural network with configurable hidden layers.
type FFNN struct {
	// Layer sizes: [input, hidden1, hidden2, ..., output]
	Layers []int

	// Weights[i] connects layer i to layer i+1
	// Weights[i][j][k] = weight from neuron k in layer i to neuron j in layer i+1
	Weights [][][]float32

	// Biases[i] are biases for layer i+1 (no biases for input layer)
	Biases [][]float32
}

// NewFFNN creates a randomly initialized network with the given hidden layer sizes.
// hiddenLayers specifies the size of each hidden layer, e.g. [16, 8] for two hidden layers.
func NewFFNN(rng *rand.Rand, hiddenLayers []int, diet float32) *FFNN {
	// Build layer sizes: input, hidden..., output
	layers := make([]int, len(hiddenLayers)+2)
	layers[0] = NumInputs
	copy(layers[1:], hiddenLayers)
	layers[len(layers)-1] = NumOutputs

	nn := &FFNN{
		Layers:  layers,
		Weights: make([][][]float32, len(layers)-1),
		Biases:  make([][]float32, len(layers)-1),
	}

	// Initialize weights and biases for each layer connection
	for l := 0; l < len(layers)-1; l++ {
		fanIn := layers[l]
		fanOut := layers[l+1]

		// Xavier initialization
		scale := float32(math.Sqrt(2.0 / float64(fanIn)))

		nn.Weights[l] = make([][]float32, fanOut)
		nn.Biases[l] = make([]float32, fanOut)

		for j := 0; j < fanOut; j++ {
			nn.Weights[l][j] = make([]float32, fanIn)
			for k := 0; k < fanIn; k++ {
				nn.Weights[l][j][k] = float32(rng.NormFloat64()) * scale
			}
			nn.Biases[l][j] = 0
		}
	}

	// Bias output layer: bite toward zero
	outputLayer := len(nn.Biases) - 1
	if len(nn.Biases[outputLayer]) >= 3 {
		nn.Biases[outputLayer][1] = 0.0  // thrust: neutral
		nn.Biases[outputLayer][2] = -2.0 // bite: biased toward 0
	}

	return nn
}

// Forward computes the network output.
// Returns: turn [-1,1], thrust [0,1], bite [0,1]
func (nn *FFNN) Forward(inputs []float32) (turn, thrust, bite float32) {
	// Current layer activations
	current := inputs

	// Forward through all layers except output
	for l := 0; l < len(nn.Weights)-1; l++ {
		fanOut := nn.Layers[l+1]
		next := make([]float32, fanOut)

		for j := 0; j < fanOut; j++ {
			sum := nn.Biases[l][j]
			for k := 0; k < len(current); k++ {
				sum += nn.Weights[l][j][k] * current[k]
			}
			next[j] = tanh(sum)
		}
		current = next
	}

	// Output layer (last weight matrix)
	outputLayer := len(nn.Weights) - 1
	outputs := make([]float32, NumOutputs)
	for j := 0; j < NumOutputs; j++ {
		sum := nn.Biases[outputLayer][j]
		for k := 0; k < len(current); k++ {
			sum += nn.Weights[outputLayer][j][k] * current[k]
		}
		outputs[j] = sum
	}

	// Apply output activations
	turn = tanh(outputs[0])
	thrust = saturate01(outputs[1]*0.5 + 0.5)
	bite = saturate01(outputs[2]*0.5 + 0.5)

	return turn, thrust, bite
}

// saturate01 clamps x to [0, 1].
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
	Inputs  []float32   // Input layer
	Hidden  [][]float32 // Hidden layers (one slice per hidden layer)
	Outputs []float32   // Output layer [turn, thrust, bite] after activation
}

// ForwardWithCapture computes the network output and captures all layer activations.
func (nn *FFNN) ForwardWithCapture(inputs []float32) (turn, thrust, bite float32, act *Activations) {
	numHiddenLayers := len(nn.Layers) - 2

	act = &Activations{
		Inputs:  make([]float32, len(inputs)),
		Hidden:  make([][]float32, numHiddenLayers),
		Outputs: make([]float32, NumOutputs),
	}
	copy(act.Inputs, inputs)

	current := inputs

	// Forward through hidden layers
	for l := 0; l < len(nn.Weights)-1; l++ {
		fanOut := nn.Layers[l+1]
		next := make([]float32, fanOut)

		for j := 0; j < fanOut; j++ {
			sum := nn.Biases[l][j]
			for k := 0; k < len(current); k++ {
				sum += nn.Weights[l][j][k] * current[k]
			}
			next[j] = tanh(sum)
		}

		// Capture hidden layer activations
		act.Hidden[l] = make([]float32, fanOut)
		copy(act.Hidden[l], next)

		current = next
	}

	// Output layer
	outputLayer := len(nn.Weights) - 1
	outputs := make([]float32, NumOutputs)
	for j := 0; j < NumOutputs; j++ {
		sum := nn.Biases[outputLayer][j]
		for k := 0; k < len(current); k++ {
			sum += nn.Weights[outputLayer][j][k] * current[k]
		}
		outputs[j] = sum
	}

	// Apply output activations
	turn = tanh(outputs[0])
	thrust = saturate01(outputs[1]*0.5 + 0.5)
	bite = saturate01(outputs[2]*0.5 + 0.5)

	act.Outputs[0] = turn
	act.Outputs[1] = thrust
	act.Outputs[2] = bite

	return turn, thrust, bite, act
}

// Mutate perturbs all weights and biases with Gaussian noise.
func (nn *FFNN) Mutate(rng *rand.Rand, strength float32) {
	for l := range nn.Weights {
		for j := range nn.Weights[l] {
			for k := range nn.Weights[l][j] {
				nn.Weights[l][j][k] += float32(rng.NormFloat64()) * strength
			}
			nn.Biases[l][j] += float32(rng.NormFloat64()) * strength
		}
	}
}

// MutateSparse applies sparse per-weight mutation for stable lineages.
// Returns avgAbsDelta: the average absolute delta of all applied mutations.
func (nn *FFNN) MutateSparse(rng *rand.Rand, rate, sigma, bigRate, bigSigma float32) float32 {
	biasRate := rate * 0.5

	var totalDelta float32
	var count int

	for l := range nn.Weights {
		for j := range nn.Weights[l] {
			// Weights
			for k := range nn.Weights[l][j] {
				if rng.Float32() < rate {
					var delta float32
					if rng.Float32() < bigRate {
						delta = float32(rng.NormFloat64()) * bigSigma
					} else {
						delta = float32(rng.NormFloat64()) * sigma
					}
					nn.Weights[l][j][k] += delta
					totalDelta += abs32(delta)
					count++
				}
			}
			// Biases
			if rng.Float32() < biasRate {
				var delta float32
				if rng.Float32() < bigRate {
					delta = float32(rng.NormFloat64()) * bigSigma
				} else {
					delta = float32(rng.NormFloat64()) * sigma
				}
				nn.Biases[l][j] += delta
				totalDelta += abs32(delta)
				count++
			}
		}
	}

	if count == 0 {
		return 0
	}
	return totalDelta / float32(count)
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// Clone creates a deep copy of the network.
func (nn *FFNN) Clone() *FFNN {
	clone := &FFNN{
		Layers:  make([]int, len(nn.Layers)),
		Weights: make([][][]float32, len(nn.Weights)),
		Biases:  make([][]float32, len(nn.Biases)),
	}
	copy(clone.Layers, nn.Layers)

	for l := range nn.Weights {
		clone.Weights[l] = make([][]float32, len(nn.Weights[l]))
		clone.Biases[l] = make([]float32, len(nn.Biases[l]))
		copy(clone.Biases[l], nn.Biases[l])

		for j := range nn.Weights[l] {
			clone.Weights[l][j] = make([]float32, len(nn.Weights[l][j]))
			copy(clone.Weights[l][j], nn.Weights[l][j])
		}
	}

	return clone
}

// tanh uses a fast rational approximation avoiding float64 conversion.
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

// BrainWeights holds flattened network weights for serialization.
type BrainWeights struct {
	Layers  []int       `json:"layers"`  // Layer sizes [input, hidden..., output]
	Weights [][]float32 `json:"weights"` // Flattened weights per layer connection
	Biases  [][]float32 `json:"biases"`  // Biases per layer
}

// MarshalWeights flattens the network weights for JSON serialization.
func (nn *FFNN) MarshalWeights() BrainWeights {
	bw := BrainWeights{
		Layers:  make([]int, len(nn.Layers)),
		Weights: make([][]float32, len(nn.Weights)),
		Biases:  make([][]float32, len(nn.Biases)),
	}
	copy(bw.Layers, nn.Layers)

	for l := range nn.Weights {
		// Flatten weights for this layer
		fanIn := nn.Layers[l]
		fanOut := nn.Layers[l+1]
		bw.Weights[l] = make([]float32, fanIn*fanOut)
		for j := 0; j < fanOut; j++ {
			for k := 0; k < fanIn; k++ {
				bw.Weights[l][j*fanIn+k] = nn.Weights[l][j][k]
			}
		}

		// Copy biases
		bw.Biases[l] = make([]float32, len(nn.Biases[l]))
		copy(bw.Biases[l], nn.Biases[l])
	}

	return bw
}

// UnmarshalWeights restores network weights from flattened form.
func (nn *FFNN) UnmarshalWeights(bw BrainWeights) {
	nn.Layers = make([]int, len(bw.Layers))
	copy(nn.Layers, bw.Layers)

	nn.Weights = make([][][]float32, len(bw.Weights))
	nn.Biases = make([][]float32, len(bw.Biases))

	for l := range bw.Weights {
		fanIn := nn.Layers[l]
		fanOut := nn.Layers[l+1]

		nn.Weights[l] = make([][]float32, fanOut)
		nn.Biases[l] = make([]float32, fanOut)

		for j := 0; j < fanOut; j++ {
			nn.Weights[l][j] = make([]float32, fanIn)
			for k := 0; k < fanIn; k++ {
				idx := j*fanIn + k
				if idx < len(bw.Weights[l]) {
					nn.Weights[l][j][k] = bw.Weights[l][idx]
				}
			}
		}

		if l < len(bw.Biases) {
			copy(nn.Biases[l], bw.Biases[l])
		}
	}
}

// HiddenLayerSizes returns the sizes of hidden layers (excluding input and output).
func (nn *FFNN) HiddenLayerSizes() []int {
	if len(nn.Layers) <= 2 {
		return nil
	}
	return nn.Layers[1 : len(nn.Layers)-1]
}

// TotalHiddenNeurons returns the total number of hidden neurons across all hidden layers.
func (nn *FFNN) TotalHiddenNeurons() int {
	total := 0
	for _, size := range nn.HiddenLayerSizes() {
		total += size
	}
	return total
}

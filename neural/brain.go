package neural

import (
	"fmt"
	"math/rand"

	"github.com/yaricom/goNEAT/v4/neat/genetics"
	neatmath "github.com/yaricom/goNEAT/v4/neat/math"
	"github.com/yaricom/goNEAT/v4/neat/network"
)

// BrainController wraps a goNEAT network for runtime evaluation.
type BrainController struct {
	Genome  *genetics.Genome
	network *network.Network
}

// NewBrainController creates a controller from a genome.
func NewBrainController(genome *genetics.Genome) (*BrainController, error) {
	phenotype, err := genome.Genesis(genome.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to build network from genome: %w", err)
	}

	return &BrainController{
		Genome:  genome,
		network: phenotype,
	}, nil
}

// Think processes sensory inputs and returns behavior outputs.
// Inputs should be a slice of BrainInputs (14) float64 values.
// Returns a slice of BrainOutputs (8) float64 values.
func (b *BrainController) Think(inputs []float64) ([]float64, error) {
	if len(inputs) != BrainInputs {
		return nil, fmt.Errorf("expected %d inputs, got %d", BrainInputs, len(inputs))
	}

	if err := b.network.LoadSensors(inputs); err != nil {
		return nil, fmt.Errorf("failed to load sensors: %w", err)
	}

	// Activate with depth-based steps for proper signal propagation
	depth, err := b.network.MaxActivationDepth()
	if err != nil || depth < 1 {
		depth = 5 // Fallback for simple networks
	}

	for i := 0; i < depth; i++ {
		if _, err := b.network.Activate(); err != nil {
			return nil, fmt.Errorf("activation failed: %w", err)
		}
	}

	outputs := b.network.ReadOutputs()

	// Flush network state for next tick
	if _, err := b.network.Flush(); err != nil {
		return nil, fmt.Errorf("flush failed: %w", err)
	}

	return outputs, nil
}

// RebuildNetwork recreates the phenotype network from the genome.
// Call this after the genome has been mutated.
func (b *BrainController) RebuildNetwork() error {
	phenotype, err := b.Genome.Genesis(b.Genome.Id)
	if err != nil {
		return fmt.Errorf("failed to rebuild network: %w", err)
	}
	b.network = phenotype
	return nil
}

// NodeCount returns the number of nodes in the network.
func (b *BrainController) NodeCount() int {
	return b.network.NodeCount()
}

// LinkCount returns the number of links (connections) in the network.
func (b *BrainController) LinkCount() int {
	return b.network.LinkCount()
}

// CreateBrainGenome creates a new brain genome with the specified ID.
// The genome starts with sparse random connections between inputs and outputs.
func CreateBrainGenome(id int, connectionProb float64) *genetics.Genome {
	nodes := make([]*network.NNode, 0, BrainInputs+BrainOutputs)

	// Input nodes (IDs 1 to BrainInputs)
	for i := 1; i <= BrainInputs; i++ {
		node := network.NewNNode(i, network.InputNeuron)
		node.ActivationType = neatmath.LinearActivation
		nodes = append(nodes, node)
	}

	// Output nodes (IDs BrainInputs+1 to BrainInputs+BrainOutputs)
	for i := 1; i <= BrainOutputs; i++ {
		node := network.NewNNode(BrainInputs+i, network.OutputNeuron)
		node.ActivationType = neatmath.SigmoidSteepenedActivation
		nodes = append(nodes, node)
	}

	// Create genes (connections) with probability-based connectivity
	genes := make([]*genetics.Gene, 0)
	innovNum := int64(1)

	for i := 0; i < BrainInputs; i++ {
		for j := 0; j < BrainOutputs; j++ {
			// Always increment innovation for consistent tracking
			currentInnov := innovNum
			innovNum++

			if rand.Float64() < connectionProb {
				weight := rand.Float64()*4 - 2 // [-2, 2]
				gene := genetics.NewGeneWithTrait(
					nil,             // trait
					weight,          // weight
					nodes[i],        // input node
					nodes[BrainInputs+j], // output node
					false,           // recurrent
					currentInnov,    // innovation number
					0,               // mutation number
				)
				genes = append(genes, gene)
			}
		}
	}

	// Ensure at least some connections exist
	if len(genes) == 0 {
		// Connect first input to first output as minimum
		gene := genetics.NewGeneWithTrait(
			nil,
			rand.Float64()*2-1,
			nodes[0],
			nodes[BrainInputs],
			false,
			1,
			0,
		)
		genes = append(genes, gene)
	}

	return genetics.NewGenome(id, nil, nodes, genes)
}

// CreateMinimalBrainGenome creates a brain genome with direct input-output connections.
// Useful for testing and as a baseline.
func CreateMinimalBrainGenome(id int) *genetics.Genome {
	nodes := make([]*network.NNode, 0, BrainInputs+BrainOutputs)

	// Input nodes
	for i := 1; i <= BrainInputs; i++ {
		node := network.NewNNode(i, network.InputNeuron)
		node.ActivationType = neatmath.LinearActivation
		nodes = append(nodes, node)
	}

	// Output nodes
	for i := 1; i <= BrainOutputs; i++ {
		node := network.NewNNode(BrainInputs+i, network.OutputNeuron)
		node.ActivationType = neatmath.SigmoidSteepenedActivation
		nodes = append(nodes, node)
	}

	// Fully connected input to output
	genes := make([]*genetics.Gene, 0, BrainInputs*BrainOutputs)
	innovNum := int64(1)

	for i := 0; i < BrainInputs; i++ {
		for j := 0; j < BrainOutputs; j++ {
			weight := rand.Float64()*2 - 1 // [-1, 1]
			gene := genetics.NewGeneWithTrait(
				nil,
				weight,
				nodes[i],
				nodes[BrainInputs+j],
				false,
				innovNum,
				0,
			)
			genes = append(genes, gene)
			innovNum++
		}
	}

	return genetics.NewGenome(id, nil, nodes, genes)
}

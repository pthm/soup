package neural

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"
	neatmath "github.com/yaricom/goNEAT/v4/neat/math"
	"github.com/yaricom/goNEAT/v4/neat/network"
)

// Mutation constants
const (
	perturbProb        = 0.9  // Probability of perturbing vs replacing weights
	maxConnectionWeight = 8.0  // Maximum absolute connection weight
	maxLinkAttempts    = 20   // Maximum attempts to find a new connection
	initialInnovNum    = 1000 // Starting innovation number to avoid conflicts
)

// GenomeIDGenerator generates unique genome IDs.
type GenomeIDGenerator struct {
	nextID       int
	nextInnovNum int64
}

// NewGenomeIDGenerator creates a new ID generator.
func NewGenomeIDGenerator() *GenomeIDGenerator {
	return &GenomeIDGenerator{
		nextID:       1,
		nextInnovNum: initialInnovNum,
	}
}

// NextID returns the next unique genome ID.
func (g *GenomeIDGenerator) NextID() int {
	id := g.nextID
	g.nextID++
	return id
}

// NextInnovation returns the next innovation number.
func (g *GenomeIDGenerator) NextInnovation() int64 {
	num := g.nextInnovNum
	g.nextInnovNum++
	return num
}

// CrossoverGenomes performs NEAT-style crossover between two parent genomes.
// Genes are aligned by innovation number.
// The more fit parent contributes disjoint/excess genes.
func CrossoverGenomes(parent1, parent2 *genetics.Genome, fitness1, fitness2 float64, childID int) (*genetics.Genome, error) {
	if parent1 == nil || parent2 == nil {
		return nil, fmt.Errorf("cannot crossover nil genomes")
	}

	// Determine which parent is more fit (or equal)
	var primary, secondary *genetics.Genome
	if fitness1 >= fitness2 {
		primary, secondary = parent1, parent2
	} else {
		primary, secondary = parent2, parent1
	}

	// Build innovation maps for quick lookup
	primaryGenes := make(map[int64]*genetics.Gene)
	for _, gene := range primary.Genes {
		primaryGenes[gene.InnovationNum] = gene
	}

	secondaryGenes := make(map[int64]*genetics.Gene)
	for _, gene := range secondary.Genes {
		secondaryGenes[gene.InnovationNum] = gene
	}

	// Collect all innovation numbers
	innovSet := make(map[int64]bool)
	for innov := range primaryGenes {
		innovSet[innov] = true
	}
	for innov := range secondaryGenes {
		innovSet[innov] = true
	}

	// Sort innovations for deterministic ordering
	innovations := make([]int64, 0, len(innovSet))
	for innov := range innovSet {
		innovations = append(innovations, innov)
	}
	sort.Slice(innovations, func(i, j int) bool { return innovations[i] < innovations[j] })

	// Build child nodes map (collect all nodes we'll need)
	childNodeMap := make(map[int]*network.NNode)

	// Copy all nodes from primary parent
	for _, node := range primary.Nodes {
		childNode := copyNode(node)
		childNodeMap[childNode.Id] = childNode
	}

	// Add any missing nodes from secondary
	for _, node := range secondary.Nodes {
		if _, exists := childNodeMap[node.Id]; !exists {
			childNode := copyNode(node)
			childNodeMap[childNode.Id] = childNode
		}
	}

	// Build child genes
	childGenes := make([]*genetics.Gene, 0, len(innovations))

	for _, innov := range innovations {
		pGene := primaryGenes[innov]
		sGene := secondaryGenes[innov]

		var selectedGene *genetics.Gene

		if pGene != nil && sGene != nil {
			// Matching gene - randomly select from either parent
			if rand.Float32() < 0.5 {
				selectedGene = pGene
			} else {
				selectedGene = sGene
			}

			// If either parent has it disabled, chance to disable in child
			if !pGene.IsEnabled || !sGene.IsEnabled {
				if rand.Float32() < 0.75 {
					// Keep it disabled
				}
			}
		} else if pGene != nil {
			// Disjoint/excess from more fit parent - always include
			selectedGene = pGene
		} else if fitness1 == fitness2 && sGene != nil {
			// Equal fitness - include disjoint/excess from secondary with 50% chance
			if rand.Float32() < 0.5 {
				selectedGene = sGene
			}
		}
		// Otherwise: disjoint/excess from less fit parent - skip

		if selectedGene != nil {
			// Get nodes for this gene
			inNode := childNodeMap[selectedGene.Link.InNode.Id]
			outNode := childNodeMap[selectedGene.Link.OutNode.Id]

			if inNode != nil && outNode != nil {
				childGene := genetics.NewGeneWithTrait(
					nil,
					selectedGene.Link.ConnectionWeight,
					inNode,
					outNode,
					selectedGene.Link.IsRecurrent,
					selectedGene.InnovationNum,
					selectedGene.MutationNum,
				)
				childGene.IsEnabled = selectedGene.IsEnabled
				childGenes = append(childGenes, childGene)
			}
		}
	}

	// Build sorted node list
	childNodes := make([]*network.NNode, 0, len(childNodeMap))
	for _, node := range childNodeMap {
		childNodes = append(childNodes, node)
	}
	sort.Slice(childNodes, func(i, j int) bool { return childNodes[i].Id < childNodes[j].Id })

	return genetics.NewGenome(childID, nil, childNodes, childGenes), nil
}

func copyNode(node *network.NNode) *network.NNode {
	newNode := network.NewNNode(node.Id, node.NeuronType)
	newNode.ActivationType = node.ActivationType
	return newNode
}

func mutateWeights(genome *genetics.Genome, power float64) {
	for _, gene := range genome.Genes {
		if rand.Float64() < perturbProb {
			// Perturb weight
			gene.Link.ConnectionWeight += (rand.Float64()*2 - 1) * power
		} else {
			// Replace weight entirely
			gene.Link.ConnectionWeight = rand.Float64()*4 - 2
		}

		// Clamp weights to valid range
		gene.Link.ConnectionWeight = clampWeight(gene.Link.ConnectionWeight)
	}
}

// clampWeight clamps a connection weight to the valid range.
func clampWeight(w float64) float64 {
	if w > maxConnectionWeight {
		return maxConnectionWeight
	}
	if w < -maxConnectionWeight {
		return -maxConnectionWeight
	}
	return w
}

func addNode(genome *genetics.Genome, idGen *GenomeIDGenerator, activators []neatmath.NodeActivationType) bool {
	// Find enabled genes to split
	enabledGenes := make([]*genetics.Gene, 0)
	for _, gene := range genome.Genes {
		if gene.IsEnabled {
			enabledGenes = append(enabledGenes, gene)
		}
	}

	if len(enabledGenes) == 0 {
		return false
	}

	// Pick a random gene to split
	geneToSplit := enabledGenes[rand.Intn(len(enabledGenes))]

	// Disable the original gene
	geneToSplit.IsEnabled = false

	// Find max node ID
	maxNodeID := 0
	for _, node := range genome.Nodes {
		if node.Id > maxNodeID {
			maxNodeID = node.Id
		}
	}

	// Create new hidden node
	newNodeID := maxNodeID + 1
	newNode := network.NewNNode(newNodeID, network.HiddenNeuron)
	newNode.ActivationType = activators[rand.Intn(len(activators))]

	// Create two new genes
	// Gene 1: old_in -> new_node (weight 1.0)
	gene1 := genetics.NewGeneWithTrait(
		nil,
		1.0,
		geneToSplit.Link.InNode,
		newNode,
		false,
		idGen.NextInnovation(),
		0,
	)

	// Gene 2: new_node -> old_out (old weight)
	gene2 := genetics.NewGeneWithTrait(
		nil,
		geneToSplit.Link.ConnectionWeight,
		newNode,
		geneToSplit.Link.OutNode,
		false,
		idGen.NextInnovation(),
		0,
	)

	// Add node and genes to genome
	genome.Nodes = append(genome.Nodes, newNode)
	genome.Genes = append(genome.Genes, gene1, gene2)

	return true
}

func addLink(genome *genetics.Genome, idGen *GenomeIDGenerator) bool {
	// Build list of potential source and target nodes
	inputs := make([]*network.NNode, 0)
	outputs := make([]*network.NNode, 0)
	hidden := make([]*network.NNode, 0)

	for _, node := range genome.Nodes {
		switch node.NeuronType {
		case network.InputNeuron, network.BiasNeuron:
			inputs = append(inputs, node)
		case network.OutputNeuron:
			outputs = append(outputs, node)
		case network.HiddenNeuron:
			hidden = append(hidden, node)
		}
	}

	// Potential sources: inputs, hidden
	sources := append(inputs, hidden...)
	// Potential targets: hidden, outputs
	targets := append(hidden, outputs...)

	if len(sources) == 0 || len(targets) == 0 {
		return false
	}

	// Build existing connections map using integer key for efficiency
	existing := make(map[int64]bool)
	for _, gene := range genome.Genes {
		key := connectionKey(gene.Link.InNode.Id, gene.Link.OutNode.Id)
		existing[key] = true
	}

	// Try to find a new connection
	for attempt := 0; attempt < maxLinkAttempts; attempt++ {
		source := sources[rand.Intn(len(sources))]
		target := targets[rand.Intn(len(targets))]

		// Skip self-connections
		if source.Id == target.Id {
			continue
		}

		// Skip if connection exists
		if existing[connectionKey(source.Id, target.Id)] {
			continue
		}

		// Create new connection
		newGene := genetics.NewGeneWithTrait(
			nil,
			rand.Float64()*4-2,
			source,
			target,
			false,
			idGen.NextInnovation(),
			0,
		)
		genome.Genes = append(genome.Genes, newGene)
		return true
	}

	return false
}

// connectionKey creates a unique key for a connection between two nodes.
func connectionKey(inID, outID int) int64 {
	return int64(inID)<<32 | int64(outID)
}

func toggleEnable(genome *genetics.Genome) {
	if len(genome.Genes) == 0 {
		return
	}

	gene := genome.Genes[rand.Intn(len(genome.Genes))]
	gene.IsEnabled = !gene.IsEnabled

	// If we disabled a gene, make sure at least one gene to each output is enabled
	if !gene.IsEnabled {
		outNode := gene.Link.OutNode
		hasEnabled := false
		for _, g := range genome.Genes {
			if g.Link.OutNode.Id == outNode.Id && g.IsEnabled {
				hasEnabled = true
				break
			}
		}
		if !hasEnabled {
			// Re-enable this gene to keep output connected
			gene.IsEnabled = true
		}
	}
}

// MutateCPPNGenome applies CPPN-appropriate mutations.
func MutateCPPNGenome(genome *genetics.Genome, opts *neat.Options, idGen *GenomeIDGenerator) (bool, error) {
	if genome == nil {
		return false, fmt.Errorf("cannot mutate nil genome")
	}

	mutated := false

	// Weight mutation
	if rand.Float64() < opts.MutateLinkWeightsProb {
		mutateWeights(genome, opts.WeightMutPower)
		mutated = true
	}

	// Add node with CPPN activations
	if rand.Float64() < opts.MutateAddNodeProb {
		if addNode(genome, idGen, getCPPNActivators()) {
			mutated = true
		}
	}

	// Add link
	if rand.Float64() < opts.MutateAddLinkProb {
		if addLink(genome, idGen) {
			mutated = true
		}
	}

	// Toggle enable
	if rand.Float64() < opts.MutateToggleEnableProb {
		toggleEnable(genome)
		mutated = true
	}

	return mutated, nil
}

func getCPPNActivators() []neatmath.NodeActivationType {
	return []neatmath.NodeActivationType{
		neatmath.SigmoidSteepenedActivation,
		neatmath.TanhActivation,
		neatmath.GaussianActivation,
		neatmath.SineActivation,
		neatmath.LinearActivation,
	}
}

// CloneGenome creates a deep copy of a genome with a new ID.
func CloneGenome(genome *genetics.Genome, newID int) (*genetics.Genome, error) {
	if genome == nil {
		return nil, fmt.Errorf("cannot clone nil genome")
	}

	// Copy nodes
	nodeMap := make(map[int]*network.NNode)
	newNodes := make([]*network.NNode, 0, len(genome.Nodes))
	for _, node := range genome.Nodes {
		newNode := copyNode(node)
		nodeMap[node.Id] = newNode
		newNodes = append(newNodes, newNode)
	}

	// Copy genes
	newGenes := make([]*genetics.Gene, 0, len(genome.Genes))
	for _, gene := range genome.Genes {
		inNode := nodeMap[gene.Link.InNode.Id]
		outNode := nodeMap[gene.Link.OutNode.Id]
		if inNode != nil && outNode != nil {
			newGene := genetics.NewGeneWithTrait(
				nil,
				gene.Link.ConnectionWeight,
				inNode,
				outNode,
				gene.Link.IsRecurrent,
				gene.InnovationNum,
				gene.MutationNum,
			)
			newGene.IsEnabled = gene.IsEnabled
			newGenes = append(newGenes, newGene)
		}
	}

	return genetics.NewGenome(newID, nil, newNodes, newGenes), nil
}

// CreateOffspringCPPN creates a child CPPN genome from two parents.
// For HyperNEAT, the brain is derived from the CPPN, so we only need to
// crossover and mutate the CPPN genome.
func CreateOffspringCPPN(
	bodyGenome1, bodyGenome2 *genetics.Genome,
	fitness1, fitness2 float64,
	idGen *GenomeIDGenerator,
	opts *neat.Options,
) (*genetics.Genome, error) {
	// Crossover body genomes (CPPN)
	bodyChild, err := CrossoverGenomes(bodyGenome1, bodyGenome2, fitness1, fitness2, idGen.NextID())
	if err != nil {
		return nil, fmt.Errorf("CPPN crossover failed: %w", err)
	}

	// Mutate CPPN
	_, err = MutateCPPNGenome(bodyChild, opts, idGen)
	if err != nil {
		return nil, fmt.Errorf("CPPN mutation failed: %w", err)
	}

	return bodyChild, nil
}

// CreateCPPNGenome creates a minimal CPPN genome for morphology generation.
func CreateCPPNGenome(id int) *genetics.Genome {
	nodes := make([]*network.NNode, 0, CPPNInputs+CPPNOutputs)

	// Input nodes
	for i := 1; i <= CPPNInputs; i++ {
		node := network.NewNNode(i, network.InputNeuron)
		node.ActivationType = neatmath.LinearActivation
		nodes = append(nodes, node)
	}

	// Output nodes
	for i := 1; i <= CPPNOutputs; i++ {
		node := network.NewNNode(CPPNInputs+i, network.OutputNeuron)
		node.ActivationType = neatmath.TanhActivation
		nodes = append(nodes, node)
	}

	// Create genes with random connections
	genes := make([]*genetics.Gene, 0)
	innovNum := int64(1)
	connectionProb := 0.5

	for i := 0; i < CPPNInputs; i++ {
		for j := 0; j < CPPNOutputs; j++ {
			currentInnov := innovNum
			innovNum++

			if rand.Float64() < connectionProb {
				weight := rand.Float64()*4 - 2
				gene := genetics.NewGeneWithTrait(
					nil, weight,
					nodes[i], nodes[CPPNInputs+j],
					false, currentInnov, 0,
				)
				genes = append(genes, gene)
			}
		}
	}

	// Ensure at least one connection per output
	for j := 0; j < CPPNOutputs; j++ {
		hasConnection := false
		for _, gene := range genes {
			if gene.Link.OutNode.Id == CPPNInputs+j+1 {
				hasConnection = true
				break
			}
		}
		if !hasConnection {
			i := rand.Intn(CPPNInputs)
			gene := genetics.NewGeneWithTrait(
				nil, rand.Float64()*2-1,
				nodes[i], nodes[CPPNInputs+j],
				false, innovNum, 0,
			)
			genes = append(genes, gene)
			innovNum++
		}
	}

	return genetics.NewGenome(id, nil, nodes, genes)
}

// GenomeCompatibility calculates the compatibility distance between two genomes.
func GenomeCompatibility(g1, g2 *genetics.Genome, opts *neat.Options) float64 {
	if g1 == nil || g2 == nil {
		return math.MaxFloat64
	}

	// Build innovation maps
	genes1 := make(map[int64]*genetics.Gene)
	for _, gene := range g1.Genes {
		genes1[gene.InnovationNum] = gene
	}

	genes2 := make(map[int64]*genetics.Gene)
	for _, gene := range g2.Genes {
		genes2[gene.InnovationNum] = gene
	}

	// Find max innovation in each
	maxInnov1 := int64(0)
	for innov := range genes1 {
		if innov > maxInnov1 {
			maxInnov1 = innov
		}
	}

	maxInnov2 := int64(0)
	for innov := range genes2 {
		if innov > maxInnov2 {
			maxInnov2 = innov
		}
	}

	// Count matching, disjoint, excess genes
	matching := 0
	disjoint := 0
	excess := 0
	weightDiff := 0.0

	for innov, gene1 := range genes1 {
		if gene2, exists := genes2[innov]; exists {
			matching++
			weightDiff += math.Abs(gene1.Link.ConnectionWeight - gene2.Link.ConnectionWeight)
		} else if innov > maxInnov2 {
			excess++
		} else {
			disjoint++
		}
	}

	for innov := range genes2 {
		if _, exists := genes1[innov]; !exists {
			if innov > maxInnov1 {
				excess++
			} else {
				disjoint++
			}
		}
	}

	// Normalize by genome size
	n := float64(max(len(g1.Genes), len(g2.Genes)))
	if n < 20 {
		n = 1 // Don't normalize small genomes
	}

	avgWeightDiff := 0.0
	if matching > 0 {
		avgWeightDiff = weightDiff / float64(matching)
	}

	return (opts.ExcessCoeff*float64(excess)+opts.DisjointCoeff*float64(disjoint))/n +
		opts.MutdiffCoeff*avgWeightDiff
}

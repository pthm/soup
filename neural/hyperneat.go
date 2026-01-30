package neural

import (
	"fmt"
	"math"

	"github.com/yaricom/goNEAT/v4/neat/genetics"
	neatmath "github.com/yaricom/goNEAT/v4/neat/math"
	"github.com/yaricom/goNEAT/v4/neat/network"
)

// SubstrateNode represents a node position in the geometric substrate.
type SubstrateNode struct {
	X, Y  float64 // Position in normalized space [-1, 1]
	Type  string  // "sensor", "hidden", "output"
	Index int     // Original index (for sensor/actuator mapping)
}

// Substrate defines the geometric layout of the brain.
// Sensor nodes map to body sensor cells, output nodes to behavior commands,
// and hidden nodes provide intermediate processing.
type Substrate struct {
	SensorNodes []SubstrateNode // Input layer: one per sensor cell
	HiddenNodes []SubstrateNode // Hidden layer: fixed positions
	OutputNodes []SubstrateNode // Output layer: DesireAngle, DesireDistance, Eat, Grow, Breed, Glow
}

// BuildSubstrateFromMorphology creates a substrate where sensor nodes
// are positioned based on actual sensor cell locations.
func BuildSubstrateFromMorphology(morph *MorphologyResult) *Substrate {
	s := &Substrate{}

	// Map cells with sensor capability to input nodes based on their grid positions
	for i, cell := range morph.Cells {
		if cell.HasFunction(CellTypeSensor) {
			s.SensorNodes = append(s.SensorNodes, SubstrateNode{
				X:     float64(cell.GridX) / 4.0, // Normalize to roughly [-1, 1]
				Y:     float64(cell.GridY) / 4.0,
				Type:  "sensor",
				Index: i,
			})
		}
	}

	// Ensure at least one sensor node
	if len(s.SensorNodes) == 0 {
		s.SensorNodes = append(s.SensorNodes, SubstrateNode{
			X: 0, Y: 0, Type: "sensor", Index: 0,
		})
	}

	// Fixed hidden layer in a grid pattern for intermediate processing
	// This provides a fixed-size layer that can learn spatial patterns
	hiddenPositions := []struct{ x, y float64 }{
		{-0.5, 0.5}, {0, 0.5}, {0.5, 0.5},   // Top row
		{-0.5, 0}, {0, 0}, {0.5, 0},          // Middle row
		{-0.5, -0.5}, {0, -0.5}, {0.5, -0.5}, // Bottom row
	}
	for i, pos := range hiddenPositions {
		s.HiddenNodes = append(s.HiddenNodes, SubstrateNode{
			X: pos.x, Y: pos.y, Type: "hidden", Index: i,
		})
	}

	// Output nodes for the 6 behavior outputs (Phase 5), spread along bottom
	// DesireAngle, DesireDistance, Eat, Grow, Breed, Glow
	outputPositions := []struct {
		x, y float64
		name string
	}{
		{-1.0, -1.0, "desire_angle"},
		{-0.6, -1.0, "desire_distance"},
		{-0.2, -1.0, "eat"},
		{0.2, -1.0, "grow"},
		{0.6, -1.0, "breed"},
		{1.0, -1.0, "glow"},
	}
	for i, pos := range outputPositions {
		s.OutputNodes = append(s.OutputNodes, SubstrateNode{
			X: pos.x, Y: pos.y, Type: "output", Index: i,
		})
	}

	return s
}

// QueryConnectionWeight uses the CPPN to determine the weight between
// two substrate nodes based on their geometric positions.
func QueryConnectionWeight(
	cppnNet *network.Network,
	x1, y1, x2, y2 float64,
) (weight float64, expressed bool, err error) {
	// Calculate geometric features for the CPPN query
	dx := x2 - x1
	dy := y2 - y1
	dist := math.Sqrt(dx*dx + dy*dy)
	angle := math.Atan2(dy, dx)

	// Use same input structure as morphology queries, but with connection-specific values
	// Inputs: x1, y1, dist, angle, sin(dist*Pi), cos(dist*Pi), sin(angle*2), bias
	inputs := []float64{
		x1,
		y1,
		dist,
		angle / math.Pi, // Normalize to [-1, 1]
		math.Sin(dist * math.Pi),
		math.Cos(dist * math.Pi),
		math.Sin(angle * 2),
		1.0, // Bias
	}

	if err := cppnNet.LoadSensors(inputs); err != nil {
		return 0, false, fmt.Errorf("failed to load CPPN sensors: %w", err)
	}

	activated, err := cppnNet.Activate()
	if err != nil || !activated {
		cppnNet.Flush()
		return 0, false, fmt.Errorf("CPPN activation failed: %w", err)
	}

	outputs := cppnNet.ReadOutputs()
	cppnNet.Flush()

	if len(outputs) < CPPNOutputs {
		return 0, false, fmt.Errorf("CPPN has insufficient outputs: %d < %d", len(outputs), CPPNOutputs)
	}

	// Brain weight output (tanh-like, in [-1, 1])
	weight = outputs[CPPNOutBrainWeight]

	// Link expression output (LEO) - connection exists if > 0
	expressed = outputs[CPPNOutBrainLEO] > 0

	return weight, expressed, nil
}

// HyperNEATBrain wraps a substrate-based neural network.
type HyperNEATBrain struct {
	substrate *Substrate
	network   *network.Network
	genome    *genetics.Genome // Store genome for reproduction
}

// SimplifiedHyperNEATBrain creates a brain that maps the standard inputs
// to 6 outputs through a hidden layer, using CPPN-queried weights based on
// sensor/actuator geometry. The hidden layer enables more complex behaviors.
func SimplifiedHyperNEATBrain(cppnGenome *genetics.Genome, morph *MorphologyResult) (*BrainController, error) {
	// Build CPPN network
	cppnNet, err := cppnGenome.Genesis(cppnGenome.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to build CPPN network: %w", err)
	}

	substrate := BuildSubstrateFromMorphology(morph)
	numHidden := len(substrate.HiddenNodes)

	// Create brain structure: inputs + hidden + outputs
	// Node IDs: 1..BrainInputs (inputs), BrainInputs+1..+numHidden (hidden),
	//           BrainInputs+numHidden+1..+BrainOutputs (outputs)
	totalNodes := BrainInputs + numHidden + BrainOutputs
	nodes := make([]*network.NNode, 0, totalNodes)

	// Input nodes (IDs 1 to BrainInputs)
	for i := 1; i <= BrainInputs; i++ {
		node := network.NewNNode(i, network.InputNeuron)
		node.ActivationType = neatmath.LinearActivation
		nodes = append(nodes, node)
	}

	// Hidden nodes (IDs BrainInputs+1 to BrainInputs+numHidden)
	hiddenStartID := BrainInputs + 1
	for i := 0; i < numHidden; i++ {
		node := network.NewNNode(hiddenStartID+i, network.HiddenNeuron)
		node.ActivationType = neatmath.TanhActivation
		nodes = append(nodes, node)
	}

	// Output nodes (IDs BrainInputs+numHidden+1 to BrainInputs+numHidden+BrainOutputs)
	outputStartID := BrainInputs + numHidden + 1
	for i := 0; i < BrainOutputs; i++ {
		node := network.NewNNode(outputStartID+i, network.OutputNeuron)
		node.ActivationType = neatmath.SigmoidSteepenedActivation
		nodes = append(nodes, node)
	}

	// Calculate sensor centroid for body-dependent positioning
	var sensorCentroidX, sensorCentroidY float64
	if len(substrate.SensorNodes) > 0 {
		for _, s := range substrate.SensorNodes {
			sensorCentroidX += s.X
			sensorCentroidY += s.Y
		}
		sensorCentroidX /= float64(len(substrate.SensorNodes))
		sensorCentroidY /= float64(len(substrate.SensorNodes))
	}

	genes := make([]*genetics.Gene, 0)
	innovNum := int64(1)

	// Helper to get input node position
	getInputPos := func(i int) (x, y float64) {
		inputX := float64(i)/float64(BrainInputs-1)*2 - 1 // [-1, 1]
		inputY := 1.0                                      // Top of substrate
		// Blend with sensor centroid for body-dependent positioning
		return inputX*0.5 + sensorCentroidX*0.5, inputY*0.5 + sensorCentroidY*0.5
	}

	// Create input -> hidden connections
	for i := 0; i < BrainInputs; i++ {
		inputX, inputY := getInputPos(i)

		for h := 0; h < numHidden; h++ {
			hiddenNode := substrate.HiddenNodes[h]

			weight, expressed, err := QueryConnectionWeight(
				cppnNet, inputX, inputY, hiddenNode.X, hiddenNode.Y)
			if err != nil {
				weight = 0.0
				expressed = false
			}

			if expressed {
				gene := genetics.NewGeneWithTrait(
					nil,
					weight*2, // Scale weight
					nodes[i],
					nodes[BrainInputs+h],
					false,
					innovNum,
					0,
				)
				genes = append(genes, gene)
			}
			innovNum++
		}
	}

	// Create hidden -> output connections
	for h := 0; h < numHidden; h++ {
		hiddenNode := substrate.HiddenNodes[h]

		for j := 0; j < BrainOutputs; j++ {
			outputNode := substrate.OutputNodes[j]

			weight, expressed, err := QueryConnectionWeight(
				cppnNet, hiddenNode.X, hiddenNode.Y, outputNode.X, outputNode.Y)
			if err != nil {
				weight = 0.0
				expressed = false
			}

			if expressed {
				gene := genetics.NewGeneWithTrait(
					nil,
					weight*2, // Scale weight
					nodes[BrainInputs+h],
					nodes[outputStartID-1+j],
					false,
					innovNum,
					0,
				)
				genes = append(genes, gene)
			}
			innovNum++
		}
	}

	// Also create some direct input -> output connections for fast reflexes
	// Query CPPN with longer distance to get potentially different weights
	for i := 0; i < BrainInputs; i++ {
		inputX, inputY := getInputPos(i)

		for j := 0; j < BrainOutputs; j++ {
			outputNode := substrate.OutputNodes[j]

			weight, expressed, err := QueryConnectionWeight(
				cppnNet, inputX, inputY, outputNode.X, outputNode.Y)
			if err != nil {
				weight = 0.0
				expressed = false
			}

			// Direct connections are sparser - use stricter threshold
			if expressed && math.Abs(weight) > 0.3 {
				gene := genetics.NewGeneWithTrait(
					nil,
					weight*1.5, // Slightly weaker than hidden path
					nodes[i],
					nodes[outputStartID-1+j],
					false,
					innovNum,
					0,
				)
				genes = append(genes, gene)
			}
			innovNum++
		}
	}

	// Ensure each output has at least one connection (from hidden or input)
	outputConnections := make([]int, BrainOutputs)
	for _, gene := range genes {
		for j := 0; j < BrainOutputs; j++ {
			if gene.Link.OutNode.Id == outputStartID+j {
				outputConnections[j]++
			}
		}
	}

	// Ensure each hidden node has at least one input connection
	hiddenInputs := make([]int, numHidden)
	for _, gene := range genes {
		for h := 0; h < numHidden; h++ {
			if gene.Link.OutNode.Id == hiddenStartID+h {
				hiddenInputs[h]++
			}
		}
	}

	// Connect unconnected hidden nodes to a relevant input
	for h := 0; h < numHidden; h++ {
		if hiddenInputs[h] == 0 {
			// Connect to input based on hidden node position
			inputIdx := int((substrate.HiddenNodes[h].X + 1) / 2 * float64(BrainInputs-1))
			if inputIdx < 0 {
				inputIdx = 0
			}
			if inputIdx >= BrainInputs {
				inputIdx = BrainInputs - 1
			}

			gene := genetics.NewGeneWithTrait(
				nil,
				1.0,
				nodes[inputIdx],
				nodes[BrainInputs+h],
				false,
				innovNum,
				0,
			)
			genes = append(genes, gene)
			innovNum++
		}
	}

	// Connect unconnected outputs to hidden nodes or inputs
	for j := 0; j < BrainOutputs; j++ {
		if outputConnections[j] == 0 {
			// First try to connect from a hidden node
			hiddenIdx := j % numHidden
			gene := genetics.NewGeneWithTrait(
				nil,
				1.5,
				nodes[BrainInputs+hiddenIdx],
				nodes[outputStartID-1+j],
				false,
				innovNum,
				0,
			)
			genes = append(genes, gene)
			innovNum++

			// Also add a direct input connection for this output
			var inputIdx int
			switch j {
			case 0: // DesireAngle
				inputIdx = 4 // Predator angle sin
			case 1: // DesireDistance
				inputIdx = 0 // Food distance
			case 2: // Eat
				inputIdx = 0 // Food distance
			case 3: // Grow
				inputIdx = 11 // Energy ratio
			case 4: // Breed
				inputIdx = 11 // Energy ratio
			case 5: // Glow
				inputIdx = 8 // Light level
			default:
				inputIdx = 0
			}

			gene = genetics.NewGeneWithTrait(
				nil,
				1.0,
				nodes[inputIdx],
				nodes[outputStartID-1+j],
				false,
				innovNum,
				0,
			)
			genes = append(genes, gene)
			innovNum++
		}
	}

	brainGenome := genetics.NewGenome(cppnGenome.Id+10000, nil, nodes, genes)

	brainNet, err := brainGenome.Genesis(brainGenome.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to build brain network: %w", err)
	}

	return &BrainController{
		Genome:  brainGenome,
		network: brainNet,
	}, nil
}

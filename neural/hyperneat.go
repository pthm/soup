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
// to 6 outputs, but uses CPPN-queried weights based on sensor/actuator geometry.
// This is a simpler approach that maintains compatibility with existing systems.
func SimplifiedHyperNEATBrain(cppnGenome *genetics.Genome, morph *MorphologyResult) (*BrainController, error) {
	// Build CPPN network
	cppnNet, err := cppnGenome.Genesis(cppnGenome.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to build CPPN network: %w", err)
	}

	substrate := BuildSubstrateFromMorphology(morph)

	// Create standard brain structure (BrainInputs inputs, BrainOutputs outputs)
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

	// Query CPPN for connection weights
	// Use sensor centroid for input-side position, output positions for output-side
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

	// Create connections with CPPN-queried weights
	for i := 0; i < BrainInputs; i++ {
		// Map input index to a position (spread inputs across top of substrate)
		inputX := float64(i)/float64(BrainInputs-1)*2 - 1 // [-1, 1]
		inputY := 1.0                                      // Top of substrate

		// Blend with sensor centroid for body-dependent positioning
		blendedX := inputX*0.5 + sensorCentroidX*0.5
		blendedY := inputY*0.5 + sensorCentroidY*0.5

		for j := 0; j < BrainOutputs; j++ {
			outputNode := substrate.OutputNodes[j]

			weight, expressed, err := QueryConnectionWeight(
				cppnNet, blendedX, blendedY, outputNode.X, outputNode.Y)
			if err != nil {
				// Fallback to random weight
				weight = 0.0
				expressed = true
			}

			if expressed {
				gene := genetics.NewGeneWithTrait(
					nil,
					weight*3, // Scale weight for stronger signal
					nodes[i],
					nodes[BrainInputs+j],
					false,
					innovNum,
					0,
				)
				genes = append(genes, gene)
			}
			innovNum++
		}
	}

	// Ensure each output has at least one connection
	// This is critical for behaviors like Eat and Mate to be expressible
	outputConnections := make([]int, BrainOutputs)
	for _, gene := range genes {
		for j := 0; j < BrainOutputs; j++ {
			if gene.Link.OutNode.Id == BrainInputs+j+1 {
				outputConnections[j]++
			}
		}
	}

	for j := 0; j < BrainOutputs; j++ {
		if outputConnections[j] == 0 {
			// Connect relevant inputs to this output
			// Turn (0): connect to predator angle inputs
			// Thrust (1): connect to food distance and energy
			// Eat (2): connect to food distance and energy
			// Mate (3): connect to mate distance and energy
			var inputIdx int
			switch j {
			case 0: // Turn
				inputIdx = 4 // Predator angle sin
			case 1: // Thrust
				inputIdx = 11 // Energy ratio
			case 2: // Eat
				inputIdx = 0 // Food distance
			case 3: // Mate
				inputIdx = 11 // Energy ratio - high energy = want to mate
			}

			gene := genetics.NewGeneWithTrait(
				nil,
				1.5, // Positive weight to bias toward action
				nodes[inputIdx],
				nodes[BrainInputs+j],
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

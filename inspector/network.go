package inspector

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/neural"
)

// Input labels for the neural network visualization.
// Order: Food[8], Threat[8], Kin[8], Energy, Speed, Diet, MetRate
// Sector order: B, BR, R, FR, F, FL, L, BL
var InputLabels = []string{
	// Food (8 sectors)
	"Food B", "Food BR", "Food R", "Food FR", "Food F", "Food FL", "Food L", "Food BL",
	// Threat (8 sectors)
	"Thrt B", "Thrt BR", "Thrt R", "Thrt FR", "Thrt F", "Thrt FL", "Thrt L", "Thrt BL",
	// Kin (8 sectors)
	"Kin B", "Kin BR", "Kin R", "Kin FR", "Kin F", "Kin FL", "Kin L", "Kin BL",
	// Self state
	"Energy", "Speed", "Diet", "MetRate",
}

// Output labels for the neural network visualization.
var OutputLabels = []string{"Turn", "Thrust", "Bite"}

// NetworkColors for activation visualization.
var (
	ColorNodeInactive = rl.Color{R: 60, G: 60, B: 60, A: 255}
	ColorNodePositive = rl.Color{R: 255, G: 100, B: 100, A: 255}
	ColorNodeNegative = rl.Color{R: 100, G: 100, B: 255, A: 255}
	ColorEdgePositive = rl.Color{R: 200, G: 80, B: 80, A: 100}
	ColorEdgeNegative = rl.Color{R: 80, G: 80, B: 200, A: 100}
	ColorLabelDim     = rl.Color{R: 120, G: 120, B: 120, A: 255}
)

// DrawNetworkDiagram renders the neural network with activations.
// Supports multiple hidden layers dynamically based on nn.Layers.
func DrawNetworkDiagram(x, y, width, height int32, nn *neural.FFNN, act *neural.Activations) {
	if nn == nil || act == nil || len(nn.Layers) < 2 {
		rl.DrawText("No network data", x+10, y+10, 14, ColorLabelDim)
		return
	}

	numLayers := len(nn.Layers)
	nodeRadius := float32(6)

	// Calculate column positions (one per layer)
	colWidth := float32(width) / float32(numLayers)
	colX := make([]float32, numLayers)
	for i := 0; i < numLayers; i++ {
		colX[i] = float32(x) + colWidth*float32(i) + colWidth/2
	}

	// Compute node positions for each layer
	allNodes := make([][]rl.Vector2, numLayers)
	for layer := 0; layer < numLayers; layer++ {
		layerSize := nn.Layers[layer]
		spacing := float32(height-20) / float32(layerSize)
		allNodes[layer] = make([]rl.Vector2, layerSize)

		for i := 0; i < layerSize; i++ {
			// Center the layer vertically
			yOffset := (float32(height-20) - float32(layerSize)*spacing) / 2
			allNodes[layer][i] = rl.Vector2{
				X: colX[layer],
				Y: float32(y) + 10 + float32(i)*spacing + yOffset,
			}
		}
	}

	// Draw edges between consecutive layers
	for layer := 0; layer < len(nn.Weights); layer++ {
		fromNodes := allNodes[layer]
		toNodes := allNodes[layer+1]

		for j := 0; j < len(nn.Weights[layer]); j++ {
			for k := 0; k < len(nn.Weights[layer][j]); k++ {
				weight := nn.Weights[layer][j][k]
				if absFloat(weight) < 0.1 {
					continue
				}
				drawEdge(fromNodes[k], toNodes[j], weight)
			}
		}
	}

	// Draw input nodes with labels
	for i := 0; i < nn.Layers[0]; i++ {
		var activation float32
		if i < len(act.Inputs) {
			activation = act.Inputs[i]
		}
		drawNode(allNodes[0][i], nodeRadius, activation)

		// Label on the left
		if i < len(InputLabels) {
			labelWidth := rl.MeasureText(InputLabels[i], 10)
			rl.DrawText(InputLabels[i], int32(allNodes[0][i].X-nodeRadius)-labelWidth-4, int32(allNodes[0][i].Y)-5, 10, ColorLabelDim)
		}
	}

	// Draw hidden layer nodes
	for layer := 1; layer < numLayers-1; layer++ {
		hiddenIdx := layer - 1 // Index into act.Hidden
		for i := 0; i < nn.Layers[layer]; i++ {
			var activation float32
			if hiddenIdx < len(act.Hidden) && i < len(act.Hidden[hiddenIdx]) {
				activation = act.Hidden[hiddenIdx][i]
			}
			drawNode(allNodes[layer][i], nodeRadius, activation)
		}
	}

	// Draw output nodes with labels
	outputLayer := numLayers - 1
	for i := 0; i < nn.Layers[outputLayer]; i++ {
		var activation float32
		if i < len(act.Outputs) {
			activation = act.Outputs[i]
		}
		drawNode(allNodes[outputLayer][i], nodeRadius+2, activation)

		// Label on the right
		if i < len(OutputLabels) {
			rl.DrawText(OutputLabels[i], int32(allNodes[outputLayer][i].X+nodeRadius+6), int32(allNodes[outputLayer][i].Y)-5, 10, ColorLabelDim)
		}
	}
}

// drawNode renders a single neuron node.
func drawNode(pos rl.Vector2, radius, activation float32) {
	color := activationColor(activation)
	rl.DrawCircleV(pos, radius, color)
	rl.DrawCircleLinesV(pos, radius, rl.Color{R: 100, G: 100, B: 100, A: 255})
}

// drawEdge renders a connection between nodes.
func drawEdge(from, to rl.Vector2, weight float32) {
	thickness := absFloat(weight) * 1.5
	if thickness > 3 {
		thickness = 3
	}
	if thickness < 0.5 {
		thickness = 0.5
	}

	color := ColorEdgePositive
	if weight < 0 {
		color = ColorEdgeNegative
	}
	// Adjust alpha based on weight magnitude
	alpha := uint8(40 + int(absFloat(weight)*40))
	if alpha > 150 {
		alpha = 150
	}
	color.A = alpha

	rl.DrawLineEx(from, to, thickness, color)
}

// activationColor returns a color based on activation value.
// Negative = blue, Zero = gray, Positive = red.
func activationColor(activation float32) rl.Color {
	if activation > 0 {
		t := activation
		if t > 1 {
			t = 1
		}
		return rl.Color{
			R: uint8(60 + t*195),
			G: uint8(60 - t*30),
			B: uint8(60 - t*30),
			A: 255,
		}
	} else {
		t := -activation
		if t > 1 {
			t = 1
		}
		return rl.Color{
			R: uint8(60 - t*30),
			G: uint8(60 - t*30),
			B: uint8(60 + t*195),
			A: 255,
		}
	}
}

func absFloat(x float32) float32 {
	return float32(math.Abs(float64(x)))
}

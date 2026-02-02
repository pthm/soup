package inspector

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/neural"
)

// Input labels for the neural network visualization.
// Order: Food[8], Threat[8], Kin[8], Energy, Speed, Diet
// Sector order: B, BR, R, FR, F, FL, L, BL
var InputLabels = []string{
	// Food (8 sectors)
	"Food B", "Food BR", "Food R", "Food FR", "Food F", "Food FL", "Food L", "Food BL",
	// Threat (8 sectors)
	"Thrt B", "Thrt BR", "Thrt R", "Thrt FR", "Thrt F", "Thrt FL", "Thrt L", "Thrt BL",
	// Kin (8 sectors)
	"Kin B", "Kin BR", "Kin R", "Kin FR", "Kin F", "Kin FL", "Kin L", "Kin BL",
	// Self state
	"Energy", "Speed", "Diet",
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
func DrawNetworkDiagram(x, y, width, height int32, nn *neural.FFNN, act *neural.Activations) {
	if nn == nil || act == nil {
		rl.DrawText("No network data", x+10, y+10, 14, ColorLabelDim)
		return
	}

	// Layout: 3 columns
	colWidth := width / 3
	nodeRadius := float32(6)

	// Vertical spacing for each layer
	inputSpacing := float32(height-20) / float32(neural.NumInputs)
	hiddenSpacing := float32(height-20) / float32(neural.NumHidden)
	outputSpacing := float32(height-20) / float32(neural.NumOutputs)

	// Column X positions
	inputX := float32(x) + float32(colWidth)/2
	hiddenX := float32(x) + float32(colWidth) + float32(colWidth)/2
	outputX := float32(x) + float32(2*colWidth) + float32(colWidth)/2

	// Compute node positions
	inputNodes := make([]rl.Vector2, neural.NumInputs)
	for i := 0; i < neural.NumInputs; i++ {
		inputNodes[i] = rl.Vector2{
			X: inputX,
			Y: float32(y) + 10 + float32(i)*inputSpacing,
		}
	}

	hiddenNodes := make([]rl.Vector2, neural.NumHidden)
	for i := 0; i < neural.NumHidden; i++ {
		hiddenNodes[i] = rl.Vector2{
			X: hiddenX,
			Y: float32(y) + 10 + float32(i)*hiddenSpacing + (float32(height-20)-float32(neural.NumHidden)*hiddenSpacing)/2,
		}
	}

	outputNodes := make([]rl.Vector2, neural.NumOutputs)
	for i := 0; i < neural.NumOutputs; i++ {
		outputNodes[i] = rl.Vector2{
			X: outputX,
			Y: float32(y) + 10 + float32(i)*outputSpacing + (float32(height-20)-float32(neural.NumOutputs)*outputSpacing)/2,
		}
	}

	// Draw edges (input -> hidden)
	for h := 0; h < neural.NumHidden; h++ {
		for i := 0; i < neural.NumInputs; i++ {
			weight := nn.W1[h][i]
			if absFloat(weight) < 0.1 {
				continue
			}
			drawEdge(inputNodes[i], hiddenNodes[h], weight)
		}
	}

	// Draw edges (hidden -> output)
	for o := 0; o < neural.NumOutputs; o++ {
		for h := 0; h < neural.NumHidden; h++ {
			weight := nn.W2[o][h]
			if absFloat(weight) < 0.1 {
				continue
			}
			drawEdge(hiddenNodes[h], outputNodes[o], weight)
		}
	}

	// Draw input nodes with labels
	for i := 0; i < neural.NumInputs; i++ {
		var activation float32
		if i < len(act.Inputs) {
			activation = act.Inputs[i]
		}
		drawNode(inputNodes[i], nodeRadius, activation)

		// Label on the left
		if i < len(InputLabels) {
			labelWidth := rl.MeasureText(InputLabels[i], 10)
			rl.DrawText(InputLabels[i], int32(inputNodes[i].X-nodeRadius)-labelWidth-4, int32(inputNodes[i].Y)-5, 10, ColorLabelDim)
		}
	}

	// Draw hidden nodes
	for i := 0; i < neural.NumHidden; i++ {
		var activation float32
		if i < len(act.Hidden) {
			activation = act.Hidden[i]
		}
		drawNode(hiddenNodes[i], nodeRadius, activation)
	}

	// Draw output nodes with labels
	for i := 0; i < neural.NumOutputs; i++ {
		var activation float32
		if i < len(act.Outputs) {
			activation = act.Outputs[i]
		}
		drawNode(outputNodes[i], nodeRadius+2, activation)

		// Label on the right
		if i < len(OutputLabels) {
			rl.DrawText(OutputLabels[i], int32(outputNodes[i].X+nodeRadius+6), int32(outputNodes[i].Y)-5, 10, ColorLabelDim)
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

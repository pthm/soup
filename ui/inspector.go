package ui

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/yaricom/goNEAT/v4/neat/genetics"
	"github.com/yaricom/goNEAT/v4/neat/network"
)

// InspectorData holds all the data needed to render the inspector panel.
type InspectorData struct {
	Organism     *components.Organism
	Cells        *components.CellBuffer
	Capabilities *components.Capabilities
	NeuralGenome *components.NeuralGenome
	HasBrain     bool
}

// Inspector renders the organism inspection panel.
type Inspector struct {
	renderer *Renderer
	x, y     int32
	width    int32
}

// NewInspector creates a new inspector panel.
func NewInspector(x, y, width int32) *Inspector {
	return &Inspector{
		renderer: NewRenderer(),
		x:        x,
		y:        y,
		width:    width,
	}
}

// SetPosition updates the inspector position.
func (ins *Inspector) SetPosition(x, y int32) {
	ins.x = x
	ins.y = y
}

// Draw renders the inspector panel for the given data.
func (ins *Inspector) Draw(data InspectorData) int32 {
	r := ins.renderer
	padding := r.Theme.Padding
	y := ins.y + padding

	// Calculate panel height (will be adjusted as we draw)
	panelHeight := int32(600)

	// Draw panel background
	r.DrawPanel(ins.x, ins.y, ins.width, panelHeight)

	// Content width
	contentWidth := ins.width - padding*2

	// === ORGANISM PREVIEW ===
	previewHeight := int32(100)
	y = ins.drawOrganismPreview(ins.x+padding, y, contentWidth, previewHeight, data)
	y = r.DrawSpacer(y, 8)

	// === HEADER ===
	y = ins.drawHeader(ins.x+padding, y, data, contentWidth)
	y = r.DrawSpacer(y, 6)

	// === STATS SECTION ===
	y = ins.drawStatsSection(ins.x+padding, y, data, contentWidth)

	// === CAPABILITIES SECTION ===
	y = ins.drawCapabilitiesSection(ins.x+padding, y, data, contentWidth)

	// === NEURAL SECTION ===
	if data.HasBrain && data.NeuralGenome != nil {
		y = ins.drawNeuralSection(ins.x+padding, y, data, contentWidth)
	}

	// === SHAPE METRICS SECTION ===
	y = ins.drawShapeSection(ins.x+padding, y, data, contentWidth)

	return y
}

// drawOrganismPreview renders a preview of the organism's cell layout.
func (ins *Inspector) drawOrganismPreview(x, y, width, height int32, data InspectorData) int32 {
	// Draw background
	rl.DrawRectangle(x, y, width, height, rl.Color{R: 25, G: 30, B: 35, A: 255})
	rl.DrawRectangleLinesEx(rl.Rectangle{X: float32(x), Y: float32(y), Width: float32(width), Height: float32(height)}, 1, rl.Color{R: 50, G: 60, B: 70, A: 255})

	if data.Cells == nil || data.Cells.Count == 0 {
		return y + height
	}

	// Find bounding box of all alive cells
	var minX, maxX, minY, maxY int8 = 127, -128, 127, -128
	aliveCount := 0
	for i := uint8(0); i < data.Cells.Count; i++ {
		cell := &data.Cells.Cells[i]
		if !cell.Alive {
			continue
		}
		aliveCount++
		if cell.GridX < minX {
			minX = cell.GridX
		}
		if cell.GridX > maxX {
			maxX = cell.GridX
		}
		if cell.GridY < minY {
			minY = cell.GridY
		}
		if cell.GridY > maxY {
			maxY = cell.GridY
		}
	}

	if aliveCount == 0 {
		return y + height
	}

	// Calculate cell size to fit in preview area with padding
	padding := float32(15)
	availWidth := float32(width) - padding*2
	availHeight := float32(height) - padding*2

	gridWidth := float32(maxX-minX) + 1
	gridHeight := float32(maxY-minY) + 1

	// Scale to fit while maintaining aspect ratio
	cellSize := availWidth / gridWidth
	if availHeight/gridHeight < cellSize {
		cellSize = availHeight / gridHeight
	}
	// Cap cell size for very small organisms
	if cellSize > 20 {
		cellSize = 20
	}

	// Calculate center offset
	actualWidth := gridWidth * cellSize
	actualHeight := gridHeight * cellSize
	offsetX := float32(x) + padding + (availWidth-actualWidth)/2
	offsetY := float32(y) + padding + (availHeight-actualHeight)/2

	// Draw cells
	for i := uint8(0); i < data.Cells.Count; i++ {
		cell := &data.Cells.Cells[i]
		if !cell.Alive {
			continue
		}

		// Calculate position in preview
		cellX := offsetX + float32(cell.GridX-minX)*cellSize
		cellY := offsetY + float32(cell.GridY-minY)*cellSize

		// Get color based on cell's primary type
		r, g, b := cell.PrimaryType.Color()
		cellColor := rl.Color{R: r, G: g, B: b, A: 255}

		// Draw cell as a filled rectangle with slight gap
		gap := cellSize * 0.1
		rl.DrawRectangle(
			int32(cellX+gap),
			int32(cellY+gap),
			int32(cellSize-gap*2),
			int32(cellSize-gap*2),
			cellColor,
		)
	}

	return y + height
}

// drawHeader renders the organism type header.
func (ins *Inspector) drawHeader(x, y int32, data InspectorData, _ int32) int32 {
	r := ins.renderer

	// Get diet-based type name and color
	digestiveSpectrum := data.Capabilities.DigestiveSpectrum()
	red, green, blue := neural.GetCapabilityColor(digestiveSpectrum)
	headerColor := rl.Color{R: red, G: green, B: blue, A: 255}

	typeName := neural.GetDietName(digestiveSpectrum)

	if data.Organism.Dead {
		typeName = "DEAD " + typeName
		headerColor = rl.Gray
	}

	rl.DrawText(typeName, x, y, 18, headerColor)
	return y + r.Theme.LineHeight + 6
}

// drawStatsSection renders organism stats using descriptors.
func (ins *Inspector) drawStatsSection(x, y int32, data InspectorData, width int32) int32 {
	r := ins.renderer

	y = r.DrawSectionHeader(x, y, "Stats")

	// Energy bar (special case - needs two values)
	y = r.DrawEnergyBar(x, y, "Energy", data.Organism.Energy, data.Organism.MaxEnergy, width)

	// Cell count
	cellCount := uint8(0)
	if data.Cells != nil {
		cellCount = data.Cells.Count
	}
	y = r.DrawLabelValue(x, y, "Cells", fmt.Sprintf("%d", cellCount), width)

	// Max speed
	y = r.DrawLabelValue(x, y, "Max Speed", fmt.Sprintf("%.2f", data.Organism.MaxSpeed), width)

	// Allocation mode - use the String() method from components
	y = r.DrawLabelValue(x, y, "Mode", data.Organism.AllocationMode.String(), width)

	return y + 6
}

// drawCapabilitiesSection renders capabilities using descriptors.
func (ins *Inspector) drawCapabilitiesSection(x, y int32, data InspectorData, width int32) int32 {
	r := ins.renderer

	y = r.DrawSectionHeader(x, y, "Capabilities")

	caps := data.Capabilities

	// Diet spectrum (always show)
	y = r.DrawLabelValue(x, y, "Diet", fmt.Sprintf("%.2f (%s)", caps.DigestiveSpectrum(), getDietLabel(caps.DigestiveSpectrum())), width)

	// Show non-zero capabilities using descriptors
	for _, fd := range components.CapabilityFieldDescriptors() {
		value := components.GetCapabilityValue(caps, fd.ID)

		// Skip zero values unless ShowWhenZero is set
		if value == 0 && !fd.ShowWhenZero {
			continue
		}

		// Skip diet (already shown above) and derived fields
		if fd.ID == "diet" || fd.Group == "derived" {
			continue
		}

		if fd.IsBar {
			normalized := value / fd.Max
			if normalized > 1 {
				normalized = 1
			}
			y = r.DrawBar(x, y, fd.Label, normalized, width)
		} else {
			y = r.DrawLabelValue(x, y, fd.Label, fmt.Sprintf(fd.Format, value), width)
		}
	}

	return y + 6
}

// drawNeuralSection renders neural network info using descriptors.
func (ins *Inspector) drawNeuralSection(x, y int32, data InspectorData, width int32) int32 {
	r := ins.renderer
	ng := data.NeuralGenome

	y = r.DrawSectionHeader(x, y, "Neural Network")

	// Species and generation
	rl.DrawText(fmt.Sprintf("Species: %d", ng.SpeciesID), x, y, r.Theme.FontSize, r.Theme.LabelColor)
	rl.DrawText(fmt.Sprintf("Gen: %d", ng.Generation), x+100, y, r.Theme.FontSize, r.Theme.LabelColor)
	y += r.Theme.LineHeight

	// Network complexity
	if ng.BrainGenome != nil {
		nodes := len(ng.BrainGenome.Nodes)
		genes := len(ng.BrainGenome.Genes)
		y = r.DrawLabelValue(x, y, "Nodes", fmt.Sprintf("%d", nodes), width)
		y = r.DrawLabelValue(x, y, "Connections", fmt.Sprintf("%d", genes), width)
	}
	y += 6

	// Brain outputs using descriptors
	y = r.DrawSectionHeader(x, y, "Brain Outputs")

	for _, fd := range components.BrainOutputFieldDescriptors() {
		value := components.GetOrganismValue(data.Organism, fd.ID)

		if fd.IsCentered {
			y = r.DrawCenteredBar(x, y, fd.Label, value, fd.Min, fd.Max, width)
		} else {
			y = r.DrawBar(x, y, fd.Label, value, width)
		}
	}
	y += 6

	// Brain inputs using descriptors from neural package
	y = r.DrawSectionHeader(x, y, "Brain Inputs (Key)")

	// Show a subset of important inputs for debugging
	inputDescs := neural.BrainInputDescriptors()
	keyInputIndices := []int{0, 1, 19, 22, 23} // speed, energy, plant_mag, meat_mag, threat
	for _, idx := range keyInputIndices {
		if idx >= len(inputDescs) {
			continue
		}
		desc := inputDescs[idx]
		value := components.GetBrainInput(data.Organism, idx)
		if desc.IsCentered {
			y = r.DrawCenteredBar(x, y, desc.Label, value, desc.Min, desc.Max, width)
		} else {
			y = r.DrawBar(x, y, desc.Label, value, width)
		}
	}
	y += 6

	// Brain graph
	if ng.BrainGenome != nil {
		y = r.DrawSectionHeader(x, y, "Network Graph")
		y += 2
		ins.drawBrainGraph(x, y, width, 120, ng.BrainGenome)
		y += 120 + 6
	}

	return y
}

// drawShapeSection renders shape metrics.
func (ins *Inspector) drawShapeSection(x, y int32, data InspectorData, width int32) int32 {
	r := ins.renderer

	y = r.DrawSectionHeader(x, y, "Shape Metrics")

	sm := &data.Organism.ShapeMetrics

	for _, fd := range components.ShapeMetricsFieldDescriptors() {
		value := components.GetShapeMetricsValue(sm, fd.ID)

		if fd.IsBar {
			normalized := (value - fd.Min) / (fd.Max - fd.Min)
			if normalized < 0 {
				normalized = 0
			}
			if normalized > 1 {
				normalized = 1
			}
			y = r.DrawBar(x, y, fd.Label, normalized, width)
		} else {
			y = r.DrawLabelValue(x, y, fd.Label, fmt.Sprintf(fd.Format, value), width)
		}
	}

	return y
}

// drawBrainGraph draws the neural network visualization.
func (ins *Inspector) drawBrainGraph(x, y, width, height int32, genome *genetics.Genome) {
	// Draw background
	rl.DrawRectangle(x, y, width, height, rl.Color{R: 30, G: 35, B: 40, A: 255})

	if genome == nil {
		return
	}

	// Categorize nodes
	var inputNodes, outputNodes, hiddenNodes []*network.NNode
	for _, node := range genome.Nodes {
		switch node.NeuronType {
		case network.InputNeuron, network.BiasNeuron:
			inputNodes = append(inputNodes, node)
		case network.OutputNeuron:
			outputNodes = append(outputNodes, node)
		case network.HiddenNeuron:
			hiddenNodes = append(hiddenNodes, node)
		}
	}

	// Calculate node positions
	nodePositions := make(map[int]rl.Vector2)
	padding := float32(15)

	// Input nodes (left column)
	inputSpacing := float32(height-int32(padding*2)) / float32(max(len(inputNodes), 1))
	for i, node := range inputNodes {
		nodePositions[node.Id] = rl.Vector2{
			X: float32(x) + padding,
			Y: float32(y) + padding + float32(i)*inputSpacing + inputSpacing/2,
		}
	}

	// Output nodes (right column)
	outputSpacing := float32(height-int32(padding*2)) / float32(max(len(outputNodes), 1))
	for i, node := range outputNodes {
		nodePositions[node.Id] = rl.Vector2{
			X: float32(x+width) - padding,
			Y: float32(y) + padding + float32(i)*outputSpacing + outputSpacing/2,
		}
	}

	// Hidden nodes (middle columns)
	if len(hiddenNodes) > 0 {
		cols := (len(hiddenNodes) + 7) / 8
		colWidth := (float32(width) - padding*4) / float32(cols+1)
		for i, node := range hiddenNodes {
			col := i / 8
			row := i % 8
			nodePositions[node.Id] = rl.Vector2{
				X: float32(x) + padding*2 + colWidth*float32(col+1),
				Y: float32(y) + padding + float32(row)*float32(height-int32(padding*2))/8 + float32(height-int32(padding*2))/16,
			}
		}
	}

	// Draw connections
	for _, gene := range genome.Genes {
		if !gene.IsEnabled || gene.Link == nil {
			continue
		}
		inPos, ok1 := nodePositions[gene.Link.InNode.Id]
		outPos, ok2 := nodePositions[gene.Link.OutNode.Id]
		if !ok1 || !ok2 {
			continue
		}

		// Color based on weight
		weight := gene.Link.ConnectionWeight
		var lineColor rl.Color
		alpha := uint8(min(255, int(abs(weight)*100)+50))
		if weight > 0 {
			lineColor = rl.Color{R: 100, G: 200, B: 100, A: alpha}
		} else {
			lineColor = rl.Color{R: 200, G: 100, B: 100, A: alpha}
		}
		rl.DrawLine(int32(inPos.X), int32(inPos.Y), int32(outPos.X), int32(outPos.Y), lineColor)
	}

	// Draw nodes
	nodeRadius := float32(4)
	for _, node := range inputNodes {
		pos := nodePositions[node.Id]
		rl.DrawCircle(int32(pos.X), int32(pos.Y), nodeRadius, rl.Color{R: 100, G: 150, B: 255, A: 255})
	}
	for _, node := range outputNodes {
		pos := nodePositions[node.Id]
		rl.DrawCircle(int32(pos.X), int32(pos.Y), nodeRadius, rl.Color{R: 255, G: 180, B: 100, A: 255})
	}
	for _, node := range hiddenNodes {
		pos := nodePositions[node.Id]
		rl.DrawCircle(int32(pos.X), int32(pos.Y), nodeRadius, rl.Color{R: 180, G: 180, B: 180, A: 255})
	}
}

// getDietLabel returns a human-readable label for the diet spectrum.
func getDietLabel(spectrum float32) string {
	if spectrum < 0.3 {
		return "Herbivore"
	} else if spectrum > 0.7 {
		return "Carnivore"
	}
	return "Omnivore"
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

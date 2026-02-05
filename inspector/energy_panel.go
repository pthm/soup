package inspector

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// History buffer size (number of data points to keep)
	energyHistorySize = 120 // ~20 minutes at 10s windows

	// Line series indices
	seriesResource    = 0
	seriesDetritus    = 1
	seriesOrganisms   = 2
	seriesHeat        = 3
	seriesRegenRate   = 4
	seriesHeatRate    = 5
	seriesOrgDelta    = 6
	seriesConservation = 7
	numSeries         = 8
)

// EnergyPanel displays ecosystem-wide energy metrics with line graphs.
type EnergyPanel struct {
	screenWidth  int32
	screenHeight int32

	// Panel dimensions
	panelWidth  int32
	panelHeight int32
	panelX      int32
	panelY      int32

	// Current snapshot
	pools EnergySnapshot
	flows EnergyFlows

	// Previous snapshot for computing deltas
	prevPools    EnergySnapshot
	hasPrevPools bool

	// Historical data for line graphs (ring buffers)
	history      [numSeries][]float64
	historyIndex int
	historyCount int

	// Series visibility (toggled by clicking legend)
	seriesVisible [numSeries]bool

	// Series metadata
	seriesNames  [numSeries]string
	seriesColors [numSeries]rl.Color
}

// EnergySnapshot holds pool totals at a point in time.
type EnergySnapshot struct {
	TotalRes       float64
	TotalDet       float64
	TotalOrganisms float64
	HeatLossAccum  float64
	EnergyInput    float64
	WindowDuration float64
}

// EnergyFlows holds computed flow rates (per second).
type EnergyFlows struct {
	RegenRate     float64
	HeatRate      float64
	OrgDelta      float64
	ResDelta      float64
	DetDelta      float64
	NetSystemFlow float64
	Conservation  float64
}

// Energy panel colors
var (
	colorEnergyTitle   = rl.Color{R: 200, G: 200, B: 220, A: 255}
	colorEnergyPanelBg = rl.Color{R: 20, G: 20, B: 30, A: 230}
	colorGraphBg       = rl.Color{R: 15, G: 15, B: 25, A: 255}
	colorGraphGrid     = rl.Color{R: 40, G: 40, B: 50, A: 255}
	colorGraphBorder   = rl.Color{R: 60, G: 60, B: 70, A: 255}

	// Series colors
	colorSeriesResource    = rl.Color{R: 80, G: 180, B: 80, A: 255}   // Green
	colorSeriesDetritus    = rl.Color{R: 160, G: 120, B: 60, A: 255}  // Brown/tan
	colorSeriesOrganisms   = rl.Color{R: 100, G: 149, B: 237, A: 255} // Cornflower blue
	colorSeriesHeat        = rl.Color{R: 255, G: 100, B: 80, A: 255}  // Red-orange
	colorSeriesRegenRate   = rl.Color{R: 150, G: 255, B: 150, A: 255} // Light green
	colorSeriesHeatRate    = rl.Color{R: 255, G: 150, B: 130, A: 255} // Light red
	colorSeriesOrgDelta    = rl.Color{R: 150, G: 200, B: 255, A: 255} // Light blue
	colorSeriesConservation = rl.Color{R: 255, G: 255, B: 100, A: 255} // Yellow
)

// NewEnergyPanel creates a new energy panel.
func NewEnergyPanel(screenWidth, screenHeight int32) *EnergyPanel {
	// Panel spans bottom of screen, leaving 400px on right for inspector
	panelWidth := screenWidth - 420
	if panelWidth < 400 {
		panelWidth = 400
	}
	panelHeight := int32(200)

	p := &EnergyPanel{
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
		panelWidth:   panelWidth,
		panelHeight:  panelHeight,
		panelX:       10,
		panelY:       screenHeight - panelHeight - 10,
	}

	// Initialize history buffers
	for i := 0; i < numSeries; i++ {
		p.history[i] = make([]float64, energyHistorySize)
	}

	// Default visibility: pools and organism delta visible, others hidden
	p.seriesVisible = [numSeries]bool{
		true,  // Resource
		true,  // Detritus
		true,  // Organisms
		true,  // Heat
		false, // Regen rate
		false, // Heat rate
		true,  // Org delta
		false, // Conservation
	}

	// Series names for legend
	p.seriesNames = [numSeries]string{
		"Resource",
		"Detritus",
		"Organisms",
		"Heat",
		"Regen/s",
		"HeatOut/s",
		"ΔOrg/s",
		"Conserv",
	}

	// Series colors
	p.seriesColors = [numSeries]rl.Color{
		colorSeriesResource,
		colorSeriesDetritus,
		colorSeriesOrganisms,
		colorSeriesHeat,
		colorSeriesRegenRate,
		colorSeriesHeatRate,
		colorSeriesOrgDelta,
		colorSeriesConservation,
	}

	return p
}

// Resize updates panel dimensions when the window is resized.
func (p *EnergyPanel) Resize(screenWidth, screenHeight int32) {
	p.screenWidth = screenWidth
	p.screenHeight = screenHeight

	// Recalculate panel dimensions
	p.panelWidth = screenWidth - 420
	if p.panelWidth < 400 {
		p.panelWidth = 400
	}
	p.panelY = screenHeight - p.panelHeight - 10
}

// Update receives new stats from the telemetry system.
func (p *EnergyPanel) Update(totalRes, totalDet, totalOrganisms, heatLossAccum, energyInput, windowDurationSec float64) {
	// Store previous for delta calculation
	if p.pools.WindowDuration > 0 {
		p.prevPools = p.pools
		p.hasPrevPools = true
	}

	// Update current pools
	p.pools = EnergySnapshot{
		TotalRes:       totalRes,
		TotalDet:       totalDet,
		TotalOrganisms: totalOrganisms,
		HeatLossAccum:  heatLossAccum,
		EnergyInput:    energyInput,
		WindowDuration: windowDurationSec,
	}

	// Compute flows if we have previous data
	if p.hasPrevPools {
		dt := windowDurationSec
		if dt <= 0 {
			dt = 1
		}

		deltaRegen := energyInput - p.prevPools.EnergyInput
		deltaHeat := heatLossAccum - p.prevPools.HeatLossAccum
		deltaRes := totalRes - p.prevPools.TotalRes
		deltaDet := totalDet - p.prevPools.TotalDet
		deltaOrg := totalOrganisms - p.prevPools.TotalOrganisms

		p.flows = EnergyFlows{
			RegenRate:     deltaRegen / dt,
			HeatRate:      deltaHeat / dt,
			OrgDelta:      deltaOrg / dt,
			ResDelta:      deltaRes / dt,
			DetDelta:      deltaDet / dt,
			NetSystemFlow: (deltaRegen - deltaHeat) / dt,
		}
		p.flows.Conservation = p.flows.NetSystemFlow - (p.flows.ResDelta + p.flows.DetDelta + p.flows.OrgDelta)
	}

	// Record to history
	p.recordHistory()
}

// recordHistory adds current values to the ring buffer.
func (p *EnergyPanel) recordHistory() {
	idx := p.historyIndex

	// Record pool values
	p.history[seriesResource][idx] = p.pools.TotalRes
	p.history[seriesDetritus][idx] = p.pools.TotalDet
	p.history[seriesOrganisms][idx] = p.pools.TotalOrganisms
	p.history[seriesHeat][idx] = p.pools.HeatLossAccum

	// Record flow values
	p.history[seriesRegenRate][idx] = p.flows.RegenRate
	p.history[seriesHeatRate][idx] = p.flows.HeatRate
	p.history[seriesOrgDelta][idx] = p.flows.OrgDelta
	p.history[seriesConservation][idx] = p.flows.Conservation

	// Advance ring buffer
	p.historyIndex = (p.historyIndex + 1) % energyHistorySize
	if p.historyCount < energyHistorySize {
		p.historyCount++
	}
}

// HandleInput processes mouse clicks for legend toggling.
func (p *EnergyPanel) HandleInput() {
	if !rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		return
	}

	mx := rl.GetMouseX()
	my := rl.GetMouseY()

	// Check if click is in legend area
	legendY := p.panelY + p.panelHeight - 24
	legendX := p.panelX + 10

	for i := 0; i < numSeries; i++ {
		itemX := legendX + int32(i)*90
		itemW := int32(85)
		itemH := int32(18)

		if mx >= itemX && mx < itemX+itemW && my >= legendY && my < legendY+itemH {
			p.seriesVisible[i] = !p.seriesVisible[i]
			return
		}
	}
}

// Draw renders the energy panel with graphs.
func (p *EnergyPanel) Draw() {
	// Panel background
	rl.DrawRectangle(p.panelX, p.panelY, p.panelWidth, p.panelHeight, colorEnergyPanelBg)
	rl.DrawRectangleLines(p.panelX, p.panelY, p.panelWidth, p.panelHeight, colorGraphBorder)

	// Title
	rl.DrawText("ENERGY", p.panelX+10, p.panelY+6, 14, colorEnergyTitle)

	// Show waiting message if no data yet
	if p.historyCount == 0 {
		rl.DrawText("Waiting for data...", p.panelX+100, p.panelY+80, 14, ColorTextDim)
		return
	}

	// Layout dimensions
	barsWidth := int32(160)
	graphX := p.panelX + barsWidth + 20
	graphY := p.panelY + 24
	graphW := p.panelWidth - barsWidth - 40
	graphH := p.panelHeight - 54 // Leave room for legend

	// Draw pool bars on the left
	p.drawPoolBars(p.panelX+10, p.panelY+28, barsWidth-20)

	// Draw line graph
	p.drawGraph(graphX, graphY, graphW, graphH)

	// Draw legend at bottom
	p.drawLegend(p.panelX+10, p.panelY+p.panelHeight-24)
}

// drawPoolBars draws the pool proportion bars.
func (p *EnergyPanel) drawPoolBars(x, y, width int32) {
	totalEnergy := p.pools.TotalRes + p.pools.TotalDet + p.pools.TotalOrganisms + p.pools.HeatLossAccum
	if totalEnergy <= 0 {
		totalEnergy = 1
	}

	barHeight := int32(14)
	spacing := int32(18)

	// Resource
	p.drawSingleBar(x, y, width, barHeight, "Res", p.pools.TotalRes, totalEnergy, colorSeriesResource)
	y += spacing

	// Detritus
	p.drawSingleBar(x, y, width, barHeight, "Det", p.pools.TotalDet, totalEnergy, colorSeriesDetritus)
	y += spacing

	// Organisms
	p.drawSingleBar(x, y, width, barHeight, "Org", p.pools.TotalOrganisms, totalEnergy, colorSeriesOrganisms)
	y += spacing

	// Heat
	p.drawSingleBar(x, y, width, barHeight, "Heat", p.pools.HeatLossAccum, totalEnergy, colorSeriesHeat)
}

// drawSingleBar draws one horizontal bar.
func (p *EnergyPanel) drawSingleBar(x, y, width, height int32, label string, value, total float64, color rl.Color) {
	labelW := int32(35)
	barW := width - labelW - 45

	// Label
	rl.DrawText(label, x, y, 11, ColorText)

	// Bar background
	barX := x + labelW
	rl.DrawRectangle(barX, y, barW, height, ColorBarBg)

	// Bar fill
	ratio := float32(value / total)
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}
	fillW := int32(float32(barW) * ratio)
	rl.DrawRectangle(barX, y, fillW, height, color)

	// Value
	rl.DrawText(formatEnergy(value), barX+barW+4, y, 10, ColorTextDim)
}

// drawGraph renders the line graph.
func (p *EnergyPanel) drawGraph(x, y, w, h int32) {
	// Graph background
	rl.DrawRectangle(x, y, w, h, colorGraphBg)
	rl.DrawRectangleLines(x, y, w, h, colorGraphBorder)

	// Draw grid lines
	for i := int32(1); i < 4; i++ {
		gridY := y + (h * i / 4)
		rl.DrawLine(x, gridY, x+w, gridY, colorGraphGrid)
	}
	for i := int32(1); i < 6; i++ {
		gridX := x + (w * i / 6)
		rl.DrawLine(gridX, y, gridX, y+h, colorGraphGrid)
	}

	if p.historyCount < 2 {
		return
	}

	// Separate scaling for pools vs flows
	poolMin, poolMax := p.getSeriesRange([]int{seriesResource, seriesDetritus, seriesOrganisms, seriesHeat})
	flowMin, flowMax := p.getSeriesRange([]int{seriesRegenRate, seriesHeatRate, seriesOrgDelta, seriesConservation})

	// Draw pool lines (use left Y axis)
	for _, series := range []int{seriesResource, seriesDetritus, seriesOrganisms, seriesHeat} {
		if p.seriesVisible[series] {
			p.drawSeriesLine(x, y, w, h, series, poolMin, poolMax)
		}
	}

	// Draw flow lines (use right Y axis with different scale)
	for _, series := range []int{seriesRegenRate, seriesHeatRate, seriesOrgDelta, seriesConservation} {
		if p.seriesVisible[series] {
			p.drawSeriesLine(x, y, w, h, series, flowMin, flowMax)
		}
	}

	// Draw Y-axis labels
	p.drawAxisLabels(x, y, w, h, poolMin, poolMax, flowMin, flowMax)
}

// getSeriesRange finds min/max across specified visible series.
func (p *EnergyPanel) getSeriesRange(seriesIndices []int) (min, max float64) {
	min = math.MaxFloat64
	max = -math.MaxFloat64
	hasVisible := false

	for _, s := range seriesIndices {
		if !p.seriesVisible[s] {
			continue
		}
		hasVisible = true

		for i := 0; i < p.historyCount; i++ {
			idx := (p.historyIndex - p.historyCount + i + energyHistorySize) % energyHistorySize
			v := p.history[s][idx]
			if v < min {
				min = v
			}
			if v > max {
				max = v
			}
		}
	}

	if !hasVisible || min >= max {
		return 0, 1
	}

	// Add 10% padding
	padding := (max - min) * 0.1
	if padding < 0.001 {
		padding = 0.001
	}
	return min - padding, max + padding
}

// drawSeriesLine draws one data series as a line.
func (p *EnergyPanel) drawSeriesLine(x, y, w, h int32, series int, minVal, maxVal float64) {
	if p.historyCount < 2 {
		return
	}

	color := p.seriesColors[series]
	valueRange := maxVal - minVal
	if valueRange <= 0 {
		valueRange = 1
	}

	var prevX, prevY int32
	for i := 0; i < p.historyCount; i++ {
		idx := (p.historyIndex - p.historyCount + i + energyHistorySize) % energyHistorySize
		v := p.history[series][idx]

		// Map to screen coordinates
		px := x + int32(float64(i)*float64(w)/float64(p.historyCount-1))
		py := y + h - int32((v-minVal)/valueRange*float64(h))

		// Clamp to graph bounds
		if py < y {
			py = y
		}
		if py > y+h {
			py = y + h
		}

		if i > 0 {
			rl.DrawLine(prevX, prevY, px, py, color)
		}
		prevX, prevY = px, py
	}
}

// drawAxisLabels draws Y-axis scale labels.
func (p *EnergyPanel) drawAxisLabels(x, y, w, h int32, poolMin, poolMax, flowMin, flowMax float64) {
	// Left axis (pools) - show min/max values
	poolMaxLabel := formatEnergy(poolMax)
	poolMinLabel := formatEnergy(poolMin)
	rl.DrawText(poolMaxLabel, x+2, y+2, 9, ColorTextDim)
	rl.DrawText(poolMinLabel, x+2, y+h-10, 9, ColorTextDim)

	// Right axis (flows) - only if any flow series visible
	hasFlowVisible := false
	for _, s := range []int{seriesRegenRate, seriesHeatRate, seriesOrgDelta, seriesConservation} {
		if p.seriesVisible[s] {
			hasFlowVisible = true
			break
		}
	}
	if hasFlowVisible {
		flowMaxLabel := fmt.Sprintf("%.2f", flowMax)
		flowMinLabel := fmt.Sprintf("%.2f", flowMin)
		textW := rl.MeasureText(flowMaxLabel, 9)
		rl.DrawText(flowMaxLabel, x+w-textW-2, y+2, 9, ColorTextDim)
		textW = rl.MeasureText(flowMinLabel, 9)
		rl.DrawText(flowMinLabel, x+w-textW-2, y+h-10, 9, ColorTextDim)
	}
}

// drawLegend draws the interactive legend.
func (p *EnergyPanel) drawLegend(x, y int32) {
	itemWidth := int32(88)

	for i := 0; i < numSeries; i++ {
		itemX := x + int32(i)*itemWidth
		color := p.seriesColors[i]

		// Dim if not visible
		if !p.seriesVisible[i] {
			color.A = 80
		}

		// Color box
		rl.DrawRectangle(itemX, y+2, 10, 10, color)

		// Label
		textColor := ColorText
		if !p.seriesVisible[i] {
			textColor = ColorTextDim
		}
		rl.DrawText(p.seriesNames[i], itemX+14, y, 11, textColor)
	}

	// Hint
	hintX := x + int32(numSeries)*itemWidth + 10
	rl.DrawText("(click to toggle)", hintX, y, 10, ColorTextDim)
}

// formatEnergy formats an energy value for display.
func formatEnergy(v float64) string {
	if v >= 10000 {
		return fmt.Sprintf("%.0fk", v/1000)
	}
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", v/1000)
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	if v >= 10 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

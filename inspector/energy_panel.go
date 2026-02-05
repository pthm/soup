package inspector

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// EnergyPanel displays ecosystem-wide energy metrics in the bottom-left corner.
type EnergyPanel struct {
	screenWidth  int32
	screenHeight int32

	// Panel dimensions
	panelWidth  int32
	panelHeight int32
	panelX      int32
	panelY      int32

	// Current snapshot (pool totals and flows)
	pools EnergySnapshot
	flows EnergyFlows

	// Previous snapshot for computing deltas
	prevPools    EnergySnapshot
	hasPrevPools bool
}

// EnergySnapshot holds pool totals at a point in time.
type EnergySnapshot struct {
	TotalRes       float64
	TotalDet       float64
	TotalOrganisms float64
	HeatLossAccum  float64
	EnergyInput    float64
	WindowDuration float64 // seconds
}

// EnergyFlows holds computed flow rates (per second).
type EnergyFlows struct {
	RegenRate     float64 // Resource regeneration rate
	HeatRate      float64 // Heat dissipation rate
	OrgDelta      float64 // Change in organism energy
	ResDelta      float64 // Change in resource
	DetDelta      float64 // Change in detritus
	NetSystemFlow float64 // Total system energy change (should be ~0 for conservation)
}

// Energy panel colors (prefixed to avoid conflicts with inspector colors)
var (
	colorEnergyTitle    = rl.Color{R: 200, G: 200, B: 220, A: 255}
	colorPoolRes        = rl.Color{R: 80, G: 180, B: 80, A: 255}   // Green for resource
	colorPoolDet        = rl.Color{R: 139, G: 90, B: 43, A: 255}   // Brown for detritus
	colorPoolOrg        = rl.Color{R: 100, G: 149, B: 237, A: 255} // Cornflower blue for organisms
	colorPoolHeat       = rl.Color{R: 255, G: 100, B: 80, A: 255}  // Red-orange for heat
	colorFlowPos        = rl.Color{R: 100, G: 200, B: 100, A: 255} // Green for positive
	colorFlowNeg        = rl.Color{R: 200, G: 100, B: 100, A: 255} // Red for negative
	colorFlowNeutral    = rl.Color{R: 150, G: 150, B: 150, A: 255} // Gray for neutral
	colorEnergyPanelBg  = rl.Color{R: 20, G: 20, B: 30, A: 220}
)

// NewEnergyPanel creates a new energy panel.
func NewEnergyPanel(screenWidth, screenHeight int32) *EnergyPanel {
	panelWidth := int32(260)
	panelHeight := int32(280)

	return &EnergyPanel{
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
		panelWidth:   panelWidth,
		panelHeight:  panelHeight,
		panelX:       10,
		panelY:       screenHeight - panelHeight - 10,
	}
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
	}
}

// Draw renders the energy panel.
func (p *EnergyPanel) Draw() {
	// Panel background
	rl.DrawRectangle(p.panelX, p.panelY, p.panelWidth, p.panelHeight, colorEnergyPanelBg)
	rl.DrawRectangleLines(p.panelX, p.panelY, p.panelWidth, p.panelHeight, ColorTextDim)

	x := p.panelX + 10
	y := p.panelY + 8

	// Title
	rl.DrawText("ENERGY", x, y, 16, colorEnergyTitle)
	y += 24

	// Show waiting message if no data yet
	if p.pools.WindowDuration == 0 {
		rl.DrawText("Waiting for first stats window...", x, y, 12, ColorTextDim)
		return
	}

	// === POOLS SECTION ===
	rl.DrawText("Pools", x, y, 12, ColorTextDim)
	y += 16

	// Calculate total for proportional bars
	totalEnergy := p.pools.TotalRes + p.pools.TotalDet + p.pools.TotalOrganisms + p.pools.HeatLossAccum
	if totalEnergy <= 0 {
		totalEnergy = 1
	}

	y += p.drawPoolBar(x, y, "Resource", p.pools.TotalRes, totalEnergy, colorPoolRes)
	y += p.drawPoolBar(x, y, "Detritus", p.pools.TotalDet, totalEnergy, colorPoolDet)
	y += p.drawPoolBar(x, y, "Organisms", p.pools.TotalOrganisms, totalEnergy, colorPoolOrg)
	y += p.drawPoolBar(x, y, "Heat", p.pools.HeatLossAccum, totalEnergy, colorPoolHeat)

	y += 8

	// === FLOWS SECTION ===
	rl.DrawText("Flows (per sec)", x, y, 12, ColorTextDim)
	y += 16

	y += p.drawFlowRow(x, y, "Regen In", p.flows.RegenRate)
	y += p.drawFlowRow(x, y, "Heat Out", -p.flows.HeatRate)
	y += p.drawFlowRow(x, y, "Δ Resource", p.flows.ResDelta)
	y += p.drawFlowRow(x, y, "Δ Detritus", p.flows.DetDelta)
	y += p.drawFlowRow(x, y, "Δ Organisms", p.flows.OrgDelta)

	y += 8

	// === BALANCE SECTION ===
	rl.DrawText("Balance", x, y, 12, ColorTextDim)
	y += 16

	// Net organism change with trend indicator
	p.drawBalanceRow(x, y, "Organism Trend", p.flows.OrgDelta)
	y += 18

	// Conservation check (should be ~0 if energy is conserved)
	conservation := p.flows.NetSystemFlow - (p.flows.ResDelta + p.flows.DetDelta + p.flows.OrgDelta)
	p.drawConservationRow(x, y, "Conservation", conservation)
}

// drawPoolBar draws a horizontal bar for a pool.
func (p *EnergyPanel) drawPoolBar(x, y int32, name string, value, total float64, color rl.Color) int32 {
	barWidth := int32(140)
	barHeight := int32(12)
	labelWidth := int32(70)

	// Label
	rl.DrawText(name, x, y, 12, ColorText)

	// Bar background
	barX := x + labelWidth
	rl.DrawRectangle(barX, y, barWidth, barHeight, ColorBarBg)

	// Bar fill (proportional to total)
	ratio := float32(value / total)
	if ratio > 1 {
		ratio = 1
	}
	fillWidth := int32(float32(barWidth) * ratio)
	rl.DrawRectangle(barX, y, fillWidth, barHeight, color)

	// Value text
	valueStr := formatEnergy(value)
	rl.DrawText(valueStr, barX+barWidth+5, y, 11, ColorTextDim)

	return 16
}

// drawFlowRow draws a flow rate with directional indicator.
func (p *EnergyPanel) drawFlowRow(x, y int32, name string, rate float64) int32 {
	labelWidth := int32(85)

	// Label
	rl.DrawText(name, x, y, 12, ColorText)

	// Rate value with color and sign
	var color rl.Color
	var prefix string
	if rate > 0.001 {
		color = colorFlowPos
		prefix = "+"
	} else if rate < -0.001 {
		color = colorFlowNeg
		prefix = ""
	} else {
		color = colorFlowNeutral
		prefix = " "
	}

	rateStr := fmt.Sprintf("%s%.3f", prefix, rate)
	rl.DrawText(rateStr, x+labelWidth, y, 12, color)

	return 16
}

// drawBalanceRow draws the organism trend with a visual indicator.
func (p *EnergyPanel) drawBalanceRow(x, y int32, name string, delta float64) {
	labelWidth := int32(100)

	// Label
	rl.DrawText(name, x, y, 12, ColorText)

	// Trend indicator
	var indicator string
	var color rl.Color
	if delta > 0.01 {
		indicator = "▲ Growing"
		color = colorFlowPos
	} else if delta < -0.01 {
		indicator = "▼ Declining"
		color = colorFlowNeg
	} else {
		indicator = "● Stable"
		color = colorFlowNeutral
	}

	rl.DrawText(indicator, x+labelWidth, y, 12, color)
}

// drawConservationRow shows whether energy is being conserved.
func (p *EnergyPanel) drawConservationRow(x, y int32, name string, error float64) {
	labelWidth := int32(100)

	// Label
	rl.DrawText(name, x, y, 12, ColorText)

	// Conservation status
	var status string
	var color rl.Color
	absError := error
	if absError < 0 {
		absError = -absError
	}

	if absError < 0.001 {
		status = "✓ OK"
		color = colorFlowPos
	} else if absError < 0.01 {
		status = fmt.Sprintf("~ %.4f", error)
		color = colorFlowNeutral
	} else {
		status = fmt.Sprintf("! %.4f", error)
		color = colorFlowNeg
	}

	rl.DrawText(status, x+labelWidth, y, 12, color)
}

// formatEnergy formats an energy value for display.
func formatEnergy(v float64) string {
	if v >= 1000 {
		return fmt.Sprintf("%.1fk", v/1000)
	}
	if v >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	return fmt.Sprintf("%.1f", v)
}

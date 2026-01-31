package ui

import (
	"fmt"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/pthm-cable/soup/systems"
)

// HUDData holds all the data needed to render the main HUD.
type HUDData struct {
	Title          string
	FloraCount     int
	FaunaCount     int
	CellCount      int
	SporeCount     int
	Tick           int32
	Speed          int
	FPS            int32
	Paused         bool
	ScreenWidth    int32
	ScreenHeight   int32
}

// HUD renders the main heads-up display.
type HUD struct {
	renderer *Renderer
}

// NewHUD creates a new HUD renderer.
func NewHUD() *HUD {
	return &HUD{
		renderer: NewRenderer(),
	}
}

// Draw renders the HUD.
func (h *HUD) Draw(data HUDData) {
	// Title
	rl.DrawText(data.Title, 10, 10, 20, rl.White)

	// Population counts
	rl.DrawText(
		fmt.Sprintf("Flora: %d | Fauna: %d | Cells: %d", data.FloraCount, data.FaunaCount, data.CellCount),
		10, 35, 16, rl.LightGray,
	)

	// Simulation info
	rl.DrawText(
		fmt.Sprintf("Tick: %d | Speed: %dx | FPS: %d | Spores: %d", data.Tick, data.Speed, data.FPS, data.SporeCount),
		10, 55, 16, rl.LightGray,
	)

	// Status
	statusText := "Running"
	if data.Paused {
		statusText = "PAUSED"
	}
	rl.DrawText(statusText, 10, 75, 16, rl.Yellow)
}

// DrawControls renders the control legend at the bottom of the screen.
func (h *HUD) DrawControls(screenWidth, screenHeight int32, controls string) {
	rl.DrawText(controls, 10, screenHeight-25, 14, rl.Gray)
}

// PerfPanelData holds performance metrics for display.
type PerfPanelData struct {
	SystemTimes map[string]time.Duration
	Total       time.Duration
	Registry    *systems.SystemRegistry
}

// PerfPanel renders the system performance panel.
type PerfPanel struct {
	renderer *Renderer
	x, y     int32
}

// NewPerfPanel creates a new performance panel.
func NewPerfPanel(x, y int32) *PerfPanel {
	return &PerfPanel{
		renderer: NewRenderer(),
		x:        x,
		y:        y,
	}
}

// SetPosition updates the panel position.
func (p *PerfPanel) SetPosition(x, y int32) {
	p.x = x
	p.y = y
}

// Draw renders the performance panel.
func (p *PerfPanel) Draw(data PerfPanelData, sortedNames []string) {
	x := p.x
	y := p.y

	rl.DrawText("System Performance", x, y, 16, rl.White)
	y += 20

	rl.DrawText(fmt.Sprintf("Total: %s", data.Total.Round(time.Microsecond)), x, y, 14, rl.Yellow)
	y += 16

	for i, name := range sortedNames {
		if i >= 12 {
			break
		}

		avg := data.SystemTimes[name]
		pct := float64(0)
		if data.Total > 0 {
			pct = float64(avg) / float64(data.Total) * 100
		}

		color := rl.LightGray
		if pct > 20 {
			color = rl.Red
		} else if pct > 10 {
			color = rl.Orange
		}

		// Use registry to get display name if available
		displayName := name
		if data.Registry != nil {
			displayName = data.Registry.GetName(name)
		}

		rl.DrawText(
			fmt.Sprintf("%-16s %6s %5.1f%%", displayName, avg.Round(time.Microsecond), pct),
			x, y, 12, color,
		)
		y += 14
	}
}

// NeuralStatsData holds data for the neural evolution stats panel.
type NeuralStatsData struct {
	Generation        int
	SpeciesCount      int
	TotalMembers      int
	BestFitness       float64 // Legacy: only shown when fitness tracking enabled
	TotalOffspring    int     // Total offspring across all species
	TopSpecies        []SpeciesInfo
	ShowSpeciesColors bool
	EcologyMode       bool // When true, show offspring instead of fitness
}

// SpeciesInfo holds info about a single species.
type SpeciesInfo struct {
	ID        int
	Size      int
	Age       int
	BestFit   float64 // Legacy: only shown when fitness tracking enabled
	Offspring int     // Total offspring produced
	Color     rl.Color
}

// NeuralStatsPanel renders the neural evolution statistics.
type NeuralStatsPanel struct {
	renderer *Renderer
	x, y     int32
	width    int32
	height   int32
}

// NewNeuralStatsPanel creates a new neural stats panel.
func NewNeuralStatsPanel(x, y, width, height int32) *NeuralStatsPanel {
	return &NeuralStatsPanel{
		renderer: NewRenderer(),
		x:        x,
		y:        y,
		width:    width,
		height:   height,
	}
}

// SetPosition updates the panel position.
func (n *NeuralStatsPanel) SetPosition(x, y int32) {
	n.x = x
	n.y = y
}

// Draw renders the neural stats panel.
func (n *NeuralStatsPanel) Draw(data NeuralStatsData) {
	r := n.renderer
	padding := r.Theme.Padding
	lineHeight := int32(16)

	// Draw panel background
	r.DrawPanel(n.x, n.y, n.width, n.height)

	y := n.y + padding

	// Header
	rl.DrawText("Neural Evolution Stats", n.x+padding, y, 16, rl.White)
	y += lineHeight + 4

	// Mode indicator
	modeText := "Species Colors: OFF"
	modeColor := rl.Gray
	if data.ShowSpeciesColors {
		modeText = "Species Colors: ON"
		modeColor = rl.Green
	}
	rl.DrawText(modeText, n.x+padding, y, 12, modeColor)
	y += lineHeight

	// Overall stats
	rl.DrawText(fmt.Sprintf("Generation: %d", data.Generation), n.x+padding, y, 12, rl.LightGray)
	y += lineHeight
	rl.DrawText(fmt.Sprintf("Species: %d | Members: %d", data.SpeciesCount, data.TotalMembers), n.x+padding, y, 12, rl.LightGray)
	y += lineHeight
	if data.EcologyMode {
		rl.DrawText(fmt.Sprintf("Total Offspring: %d", data.TotalOffspring), n.x+padding, y, 12, rl.LightGray)
	} else {
		rl.DrawText(fmt.Sprintf("Best Fitness: %.1f", data.BestFitness), n.x+padding, y, 12, rl.LightGray)
	}
	y += lineHeight + 4

	// Top species
	if len(data.TopSpecies) > 0 {
		rl.DrawText("Top Species:", n.x+padding, y, 14, rl.Yellow)
		y += lineHeight + 2

		for i, sp := range data.TopSpecies {
			if i >= 5 {
				break
			}

			// Color swatch
			swatchSize := int32(10)
			rl.DrawRectangle(n.x+padding, y+2, swatchSize, swatchSize, sp.Color)

			// Species info - show offspring in ecology mode, fitness otherwise
			var text string
			if data.EcologyMode {
				text = fmt.Sprintf("#%d: %d members (age: %d, offspring: %d)",
					sp.ID, sp.Size, sp.Age, sp.Offspring)
			} else {
				text = fmt.Sprintf("#%d: %d members (age: %d, fit: %.0f)",
					sp.ID, sp.Size, sp.Age, sp.BestFit)
			}
			rl.DrawText(text, n.x+padding+swatchSize+6, y, 12, rl.LightGray)
			y += lineHeight
		}
	}
}

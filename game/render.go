package game

import (
	"fmt"
	"math"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/ui"
)

// Draw renders the game state.
func (g *Game) Draw() {
	rl.BeginDrawing()

	// Render timing (only measure every 120 ticks when perf logging is on)
	measureRender := g.perfLog && g.tick%120 == 0
	var renderStart time.Time
	var waterTime, terrainTime, flowTime, occluderTime, sunTime, floraTime, faunaTime time.Duration

	// Draw animated water background
	if measureRender {
		renderStart = time.Now()
	}
	g.waterBackground.Draw(float32(g.tick) * 0.016) // Convert tick to approximate seconds
	if measureRender {
		waterTime = time.Since(renderStart)
	}

	// Draw terrain (after water, before flow field)
	if measureRender {
		renderStart = time.Now()
	}
	if g.terrainRenderer != nil {
		g.terrainRenderer.Draw(g.terrain, g.tick)
	}
	if measureRender {
		terrainTime = time.Since(renderStart)
	}

	// Draw flow field particles (on top of water and terrain)
	if measureRender {
		renderStart = time.Now()
	}
	g.flowRenderer.Draw(g.flowField.Particles, g.tick)
	if measureRender {
		flowTime = time.Since(renderStart)
	}

	// Collect occluders from organisms for shadow casting
	if measureRender {
		renderStart = time.Now()
	}
	occluders := g.collectOccluders()
	if measureRender {
		occluderTime = time.Since(renderStart)
	}

	// Draw sun with shadows
	if measureRender {
		renderStart = time.Now()
	}
	g.sunRenderer.Draw(g.light, occluders)
	if measureRender {
		sunTime = time.Since(renderStart)
	}

	// Draw lightweight flora from FloraSystem
	if measureRender {
		renderStart = time.Now()
	}
	g.drawLightweightFlora()
	if measureRender {
		floraTime = time.Since(renderStart)
	}

	// Draw all fauna organisms (ECS)
	if measureRender {
		renderStart = time.Now()
	}
	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, org, cells := query.Get()
		g.drawOrganism(entity, pos, org, cells)
	}
	if measureRender {
		faunaTime = time.Since(renderStart)
		Logf("  --- Render Breakdown ---")
		Logf("    water:    %10s", waterTime.Round(time.Microsecond))
		Logf("    terrain:  %10s", terrainTime.Round(time.Microsecond))
		Logf("    flow:     %10s", flowTime.Round(time.Microsecond))
		Logf("    occluder: %10s", occluderTime.Round(time.Microsecond))
		Logf("    sun:      %10s", sunTime.Round(time.Microsecond))
		Logf("    flora:    %10s", floraTime.Round(time.Microsecond))
		Logf("    fauna:    %10s", faunaTime.Round(time.Microsecond))
	}

	// Draw selection indicator (after organisms, before UI)
	g.drawSelectionIndicator()

	// Draw spores
	g.drawSpores()

	// Draw effect particles
	g.particleRenderer.Draw(g.particles.Particles)

	// Draw ambient darkness overlay (based on sun intensity)
	g.sunRenderer.DrawAmbientDarkness(g.light.Intensity)

	// Draw UI
	g.drawUI()

	// Draw neural stats panel if enabled
	if g.showNeuralStats {
		g.drawNeuralStats()
	}

	// Draw overlay controls panel (positioned below neural stats if shown)
	controlsY := int32(100)
	if g.showNeuralStats {
		controlsY = 330 // Below neural stats panel
	}
	g.uiControlsPanel.Draw(g.uiOverlays)

	// Adjust controls panel position dynamically
	if g.uiControlsPanel.IsVisible() {
		// Position already set in NewControlsPanel, but could adjust here if needed
		_ = controlsY // Use this if we want dynamic positioning
	}

	// Draw overlays based on registry state
	g.drawActiveOverlays()

	// Draw info panel when selected, or tooltip when hovering
	if g.hasSelection {
		g.drawInfoPanel()
	} else {
		g.drawTooltip()
	}

	rl.EndDrawing()
}

// drawOrganism draws a single organism with its cells.
func (g *Game) drawOrganism(entity ecs.Entity, pos *components.Position, org *components.Organism, cells *components.CellBuffer) {
	var r, gr, b uint8

	// Compute capabilities for color
	caps := cells.ComputeCapabilities()

	// Use species color if enabled and organism has neural genome
	if g.showSpeciesColors && g.neuralGenomeMap.Has(entity) {
		neuralGenome := g.neuralGenomeMap.Get(entity)
		if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
			speciesColor := g.speciesManager.GetSpeciesColor(neuralGenome.SpeciesID)
			r, gr, b = speciesColor.R, speciesColor.G, speciesColor.B
		} else {
			r, gr, b = neural.GetCapabilityColor(caps.DigestiveSpectrum())
		}
	} else {
		r, gr, b = neural.GetCapabilityColor(caps.DigestiveSpectrum())
	}

	baseColor := rl.Color{R: r, G: gr, B: b, A: 255}

	// Adjust for death/low energy
	if org.Dead {
		baseColor.R = baseColor.R / 2
		baseColor.G = baseColor.G / 2
		baseColor.B = baseColor.B / 2
	} else if org.Energy < 30 {
		// Dim when low energy
		factor := org.Energy / 30
		baseColor.R = uint8(float32(baseColor.R) * (0.5 + 0.5*factor))
		baseColor.G = uint8(float32(baseColor.G) * (0.5 + 0.5*factor))
		baseColor.B = uint8(float32(baseColor.B) * (0.5 + 0.5*factor))
	}

	// Pre-compute rotation for cell positions
	// Add π/2 so Y-axis (forward in grid space) aligns with movement direction
	cosH := float32(math.Cos(float64(org.Heading) + math.Pi/2))
	sinH := float32(math.Sin(float64(org.Heading) + math.Pi/2))

	// Draw each cell
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if !cell.Alive {
			continue
		}

		// Local grid position
		localX := float32(cell.GridX) * org.CellSize
		localY := float32(cell.GridY) * org.CellSize

		// Rotate around center
		rotatedX := localX*cosH - localY*sinH
		rotatedY := localX*sinH + localY*cosH

		// World position
		cellX := pos.X + rotatedX
		cellY := pos.Y + rotatedY

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(cellX, cellY)
		// Apply global sun intensity as additional factor
		light *= (0.3 + g.light.Intensity*0.7) // Min 30% light even at night

		// Adjust alpha for decomposition
		alpha := uint8(255 * (1 - cell.Decomposition))
		cellColor := baseColor
		cellColor.A = alpha

		// Apply lighting to color (darken based on shadow map)
		cellColor.R = uint8(float32(cellColor.R) * light)
		cellColor.G = uint8(float32(cellColor.G) * light)
		cellColor.B = uint8(float32(cellColor.B) * light)

		// Draw cell with rotation matching organism heading
		// Add 90° so cells align with movement direction
		rotationDeg := org.Heading*180/math.Pi + 90
		rl.DrawRectanglePro(
			rl.Rectangle{X: cellX, Y: cellY, Width: org.CellSize, Height: org.CellSize},
			rl.Vector2{X: org.CellSize / 2, Y: org.CellSize / 2}, // rotate around cell center
			rotationDeg,
			cellColor,
		)
	}
}

// drawSpores draws all active spores.
func (g *Game) drawSpores() {
	for i := range g.spores.Spores {
		spore := &g.spores.Spores[i]

		// Calculate alpha based on life/landing state
		alpha := uint8(180)
		if spore.Landed {
			// Fade as germination approaches
			fadeRatio := 1.0 - float32(spore.LandedTimer)/50.0
			alpha = uint8(fadeRatio * 180)
		}

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(spore.X, spore.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		// Green color for spores, adjusted for lighting
		color := rl.Color{
			R: uint8(80 * light),
			G: uint8(180 * light),
			B: uint8(100 * light),
			A: alpha,
		}
		rl.DrawCircle(int32(spore.X), int32(spore.Y), 2, color)
	}
}

// drawLightweightFlora renders all flora from the FloraSystem.
func (g *Game) drawLightweightFlora() {
	// Base flora color (green)
	baseR, baseG, baseB := uint8(50), uint8(180), uint8(80)

	// Draw rooted flora
	for i := range g.floraSystem.Rooted {
		f := &g.floraSystem.Rooted[i]
		if f.Dead {
			continue
		}

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(f.X, f.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		// Energy-based alpha (dimmer when low energy)
		energyRatio := f.Energy / f.MaxEnergy
		if energyRatio < 0.3 {
			light *= 0.5 + energyRatio
		}

		color := rl.Color{
			R: uint8(float32(baseR) * light),
			G: uint8(float32(baseG) * light),
			B: uint8(float32(baseB) * light),
			A: 255,
		}
		rl.DrawCircle(int32(f.X), int32(f.Y), f.Size, color)
	}

	// Draw floating flora (slightly different shade)
	for i := range g.floraSystem.Floating {
		f := &g.floraSystem.Floating[i]
		if f.Dead {
			continue
		}

		light := g.shadowMap.SampleLight(f.X, f.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		energyRatio := f.Energy / f.MaxEnergy
		if energyRatio < 0.3 {
			light *= 0.5 + energyRatio
		}

		// Floating flora is slightly more cyan
		color := rl.Color{
			R: uint8(float32(40) * light),
			G: uint8(float32(170) * light),
			B: uint8(float32(100) * light),
			A: 240,
		}
		rl.DrawCircle(int32(f.X), int32(f.Y), f.Size, color)
	}
}

// drawUI draws the HUD and performance panel.
func (g *Game) drawUI() {
	// Count organisms
	floraCount := g.floraSystem.TotalCount()
	faunaCount := 0
	totalCells := 0

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()
		// All ECS organisms are fauna (flora are in FloraSystem)
		if !org.Dead {
			faunaCount++
			totalCells += int(cells.Count)
		}
	}

	// Draw HUD using descriptor-driven UI
	g.uiHUD.Draw(ui.HUDData{
		Title:        "Primordial Soup",
		FloraCount:   floraCount,
		FaunaCount:   faunaCount,
		CellCount:    totalCells,
		SporeCount:   g.spores.Count(),
		Tick:         g.tick,
		Speed:        g.stepsPerFrame,
		FPS:          rl.GetFPS(),
		Paused:       g.paused,
		ScreenWidth:  int32(g.bounds.Width),
		ScreenHeight: int32(g.bounds.Height),
	})

	// Performance stats (right side) using descriptor-driven UI
	if g.perfLog {
		// Build system times map
		systemTimes := make(map[string]time.Duration)
		for _, name := range g.perf.SortedNames() {
			systemTimes[name] = g.perf.Avg(name)
		}

		g.uiPerfPanel.Draw(ui.PerfPanelData{
			SystemTimes: systemTimes,
			Total:       g.perf.Total(),
			Registry:    g.uiSystemRegistry,
		}, g.perf.SortedNames())
	}

	// Controls
	g.uiHUD.DrawControls(int32(g.bounds.Width), int32(g.bounds.Height),
		"SPACE: Pause | < >: Speed | Click: Select | Shift+Click: Add | F: Flora | C: Carnivore | S: Species | N: Neural | O: Overlays")
}

// drawNeuralStats draws the neural evolution statistics panel.
func (g *Game) drawNeuralStats() {
	// Get stats from species manager
	stats := g.speciesManager.GetStats()
	topSpecies := g.speciesManager.GetTopSpecies(5)

	// Convert to UI data format
	var speciesInfo []ui.SpeciesInfo
	for _, sp := range topSpecies {
		speciesInfo = append(speciesInfo, ui.SpeciesInfo{
			ID:      sp.ID,
			Size:    sp.Size,
			Age:     sp.Age,
			BestFit: sp.BestFit,
			Color:   rl.Color{R: sp.Color.R, G: sp.Color.G, B: sp.Color.B, A: 255},
		})
	}

	// Draw using descriptor-driven UI
	g.uiNeuralStats.Draw(ui.NeuralStatsData{
		Generation:        stats.Generation,
		SpeciesCount:      stats.Count,
		TotalMembers:      stats.TotalMembers,
		BestFitness:       stats.BestFitness,
		TopSpecies:        speciesInfo,
		ShowSpeciesColors: g.showSpeciesColors,
	})
}

// drawInfoPanel draws the detailed info panel for the selected organism.
func (g *Game) drawInfoPanel() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	// Get entity data using maps
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !orgMap.Has(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	// Compute capabilities
	caps := cells.ComputeCapabilities()

	// Check for neural genome
	var neuralGenome *components.NeuralGenome
	hasNeural := g.neuralGenomeMap.Has(g.selectedEntity)
	if hasNeural {
		neuralGenome = g.neuralGenomeMap.Get(g.selectedEntity)
	}

	// Draw using descriptor-driven UI
	g.uiInspector.Draw(ui.InspectorData{
		Organism:     org,
		Cells:        cells,
		Capabilities: &caps,
		NeuralGenome: neuralGenome,
		HasBrain:     hasNeural,
	})
}

// drawTooltip draws a hover tooltip for the organism under the mouse.
func (g *Game) drawTooltip() {
	hovered := g.findOrganismAtMouse()
	if hovered == nil {
		return
	}

	mousePos := rl.GetMousePosition()
	org := hovered.Org
	cells := hovered.Cells

	// Build tooltip content
	var lines []string

	// Type header - determined by cell capabilities
	caps := cells.ComputeCapabilities()
	dietName := neural.GetDietName(caps.DigestiveSpectrum())
	lines = append(lines, dietName)

	lines = append(lines, "")

	// Stats
	lines = append(lines, fmt.Sprintf("Energy: %.0f / %.0f", org.Energy, org.MaxEnergy))
	lines = append(lines, fmt.Sprintf("Cells: %d", cells.Count))
	lines = append(lines, fmt.Sprintf("Speed: %.2f", org.MaxSpeed))

	if org.Dead {
		lines = append(lines, "STATUS: DEAD")
	}

	lines = append(lines, "")

	// Capabilities (derived from cells)
	lines = append(lines, "Capabilities:")
	lines = append(lines, fmt.Sprintf("  Diet: %.2f", caps.DigestiveSpectrum()))
	if caps.StructuralArmor > 0 {
		lines = append(lines, fmt.Sprintf("  Armor: %.2f", caps.StructuralArmor))
	}
	if caps.StorageCapacity > 0 {
		lines = append(lines, fmt.Sprintf("  Storage: %.2f", caps.StorageCapacity))
	}
	if caps.PhotoWeight > 0 {
		lines = append(lines, fmt.Sprintf("  Photo: %.2f", caps.PhotoWeight))
	}

	// Calculate tooltip dimensions
	const fontSize = 14
	const padding = 8
	const lineHeight = 16

	maxWidth := int32(0)
	for _, line := range lines {
		width := rl.MeasureText(line, fontSize)
		if width > maxWidth {
			maxWidth = width
		}
	}

	tooltipWidth := maxWidth + padding*2
	tooltipHeight := int32(len(lines)*lineHeight + padding*2)

	// Position tooltip (offset from cursor, keep on screen)
	tooltipX := int32(mousePos.X) + 15
	tooltipY := int32(mousePos.Y) + 15

	if tooltipX+tooltipWidth > int32(g.bounds.Width)-10 {
		tooltipX = int32(mousePos.X) - tooltipWidth - 10
	}
	if tooltipY+tooltipHeight > int32(g.bounds.Height)-10 {
		tooltipY = int32(mousePos.Y) - tooltipHeight - 10
	}

	// Draw background
	rl.DrawRectangle(tooltipX, tooltipY, tooltipWidth, tooltipHeight, rl.Color{R: 20, G: 25, B: 30, A: 230})
	rl.DrawRectangleLines(tooltipX, tooltipY, tooltipWidth, tooltipHeight, rl.Color{R: 60, G: 70, B: 80, A: 255})

	// Draw text - use capability-based color
	r, gr, b := neural.GetCapabilityColor(caps.DigestiveSpectrum())
	headerColor := rl.Color{R: r, G: gr, B: b, A: 255}

	for i, line := range lines {
		y := tooltipY + padding + int32(i*lineHeight)
		color := rl.LightGray
		if i == 0 {
			color = headerColor
		}
		rl.DrawText(line, tooltipX+padding, y, fontSize, color)
	}
}

// drawSelectionIndicator draws a circle around the selected organism.
func (g *Game) drawSelectionIndicator() {
	if !g.hasSelection {
		return
	}

	// Check if entity still exists
	if !g.world.Alive(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	// Get position and cells for the selected entity
	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	// Calculate organism bounding circle
	var minX, minY, maxX, maxY float32 = pos.X, pos.Y, pos.X, pos.Y

	if cells != nil {
		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			// Rotate cell position by organism heading
			// Add π/2 so Y-axis (forward in grid space) aligns with movement direction
			localX := float32(cell.GridX) * org.CellSize
			localY := float32(cell.GridY) * org.CellSize
			cosH := float32(math.Cos(float64(org.Heading) + math.Pi/2))
			sinH := float32(math.Sin(float64(org.Heading) + math.Pi/2))
			rotatedX := localX*cosH - localY*sinH
			rotatedY := localX*sinH + localY*cosH
			cellX := pos.X + rotatedX
			cellY := pos.Y + rotatedY

			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}
	}

	// Calculate center and radius
	centerX := (minX + maxX) / 2
	centerY := (minY + maxY) / 2
	radius := float32(math.Sqrt(float64((maxX-minX)*(maxX-minX)+(maxY-minY)*(maxY-minY)))) / 2
	radius += org.CellSize + 3 // Padding

	// Pulsing glow effect
	pulse := float32(math.Sin(float64(g.tick)*0.1))*0.3 + 0.7
	alpha := uint8(255 * pulse)

	// Draw selection circle
	rl.DrawCircleLines(int32(centerX), int32(centerY), radius, rl.Color{R: 255, G: 255, B: 255, A: alpha})
	rl.DrawCircleLines(int32(centerX), int32(centerY), radius+1, rl.Color{R: 255, G: 255, B: 255, A: alpha / 2})
}

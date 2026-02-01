package game

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/components"
)

// Draw renders the game.
func (g *Game) Draw() {
	g.perfCollector.RecordFrame()

	rl.BeginDrawing()
	rl.ClearBackground(rl.Color{R: 8, G: 45, B: 60, A: 255}) // Teal sea fallback

	// Layer 1: Background noise texture
	g.drawBackground()

	// Layer 2: Particles
	g.drawParticles()

	// Debug overlays (drawn before entities so entities appear on top)
	if g.debugMode && g.debugShowResource {
		g.drawResourceHeatmap(180)
	}
	if g.debugMode && g.debugShowPotential {
		g.drawPotentialHeatmap(180)
	}
	if g.debugMode && g.debugShowFlow {
		g.drawFlowField()
	}

	// Layer 3: Entities
	g.drawEntities()

	// Layer 4: Shadow (darkens everything underneath including particles and entities)
	g.drawShadow()

	// Layer 5: Caustics (lights up everything underneath)
	g.drawCaustics()

	// Draw selection highlight and vision cone
	g.inspector.DrawSelectionHighlight(g.posMap, g.bodyMap, g.rotMap, g.capsMap, g.orgMap, g.camera)

	// Draw HUD
	rl.DrawText(fmt.Sprintf("Tick: %d", g.tick), 10, 10, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Prey: %d  Pred: %d  Dead: %d", g.numPrey, g.numPred, g.deadCount), 10, 35, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Speed: %dx  [</>]", g.stepsPerUpdate), 10, 60, 20, rl.White)
	cam := g.camera
	rl.DrawText(fmt.Sprintf("Zoom: %.2fx  Pos: (%.0f, %.0f)  [Arrows/+/-/Home]", cam.Zoom, cam.X, cam.Y), 10, 85, 16, rl.LightGray)
	if g.paused {
		rl.DrawText("PAUSED", 10, 110, 20, rl.Yellow)
	}

	// Debug menu
	if g.debugMode {
		g.drawDebugMenu()
	}

	// Draw inspector panel
	g.inspector.Draw(g.posMap, g.velMap, g.rotMap, g.bodyMap, g.energyMap, g.capsMap, g.orgMap, g.brains)

	rl.EndDrawing()
}

// drawEntities renders all entities as oriented triangles.
func (g *Game) drawEntities() {
	cam := g.camera

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, body, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Visibility culling
		if !cam.IsVisible(pos.X, pos.Y, body.Radius*2) {
			continue
		}

		// Color by kind
		color := rl.Green
		if org.Kind == components.KindPredator {
			color = rl.Red
		}

		// Dim based on energy
		alpha := uint8(100 + int(energy.Value*155))
		color.A = alpha

		// Transform to screen coordinates
		sx, sy := cam.WorldToScreen(pos.X, pos.Y)
		scaledRadius := body.Radius * cam.Zoom

		drawOrientedTriangle(sx, sy, rot.Heading, scaledRadius, color)

		// Draw ghost copies for entities near world edges
		ghosts := cam.GhostPositions(pos.X, pos.Y, body.Radius*2)
		for _, ghost := range ghosts {
			drawOrientedTriangle(ghost.X, ghost.Y, rot.Heading, scaledRadius, color)
		}
	}
}

// drawDebugMenu renders the debug overlay menu.
func (g *Game) drawDebugMenu() {
	// Semi-transparent background panel
	panelX := int32(g.screenWidth) - 200
	panelY := int32(10)
	panelW := int32(190)
	panelH := int32(120)

	rl.DrawRectangle(panelX, panelY, panelW, panelH, rl.Color{R: 0, G: 0, B: 0, A: 180})
	rl.DrawRectangleLines(panelX, panelY, panelW, panelH, rl.Yellow)

	// Title
	rl.DrawText("DEBUG [D to close]", panelX+10, panelY+8, 14, rl.Yellow)

	// Resource overlay toggle
	resourceStatus := "OFF"
	resourceColor := rl.Gray
	if g.debugShowResource {
		resourceStatus = "ON"
		resourceColor = rl.Green
	}
	rl.DrawText(fmt.Sprintf("[R] Resource: %s", resourceStatus), panelX+10, panelY+30, 14, resourceColor)

	// Potential overlay toggle
	potentialStatus := "OFF"
	potentialColor := rl.Gray
	if g.debugShowPotential {
		potentialStatus = "ON"
		potentialColor = rl.Green
	}
	rl.DrawText(fmt.Sprintf("[P] Potential: %s", potentialStatus), panelX+10, panelY+48, 14, potentialColor)

	// Flow field overlay toggle
	flowStatus := "OFF"
	flowColor := rl.Gray
	if g.debugShowFlow {
		flowStatus = "ON"
		flowColor = rl.Green
	}
	rl.DrawText(fmt.Sprintf("[F] Flow: %s", flowStatus), panelX+10, panelY+66, 14, flowColor)

	// Performance stats
	stats := g.perfCollector.Stats()
	rl.DrawText(fmt.Sprintf("Tick: %v  TPS: %.0f", stats.AvgTickDuration, stats.TicksPerSecond), panelX+10, panelY+95, 12, rl.White)
}

// drawResourceHeatmap renders the resource field as a colored overlay.
func (g *Game) drawResourceHeatmap(alpha uint8) {
	cam := g.camera
	gridW, gridH := g.resourceField.GridSize()
	cellW := g.worldWidth / float32(gridW)
	cellH := g.worldHeight / float32(gridH)
	res := g.resourceField.ResData()

	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			// Calculate world corners using consistent formula.
			// This ensures cell[x+1]'s left edge == cell[x]'s right edge.
			worldX0 := float32(x) * cellW
			worldY0 := float32(y) * cellH
			worldX1 := float32(x+1) * cellW
			worldY1 := float32(y+1) * cellH

			// Check visibility using cell center
			worldCX := (worldX0 + worldX1) / 2
			worldCY := (worldY0 + worldY1) / 2
			cellRadius := (cellW + cellH) / 2
			if !cam.IsVisible(worldCX, worldCY, cellRadius) {
				continue
			}

			// Transform corners to screen coordinates
			sx0, sy0 := cam.WorldToScreen(worldX0, worldY0)
			sx1, sy1 := cam.WorldToScreen(worldX1, worldY1)

			// Use floor consistently so adjacent cells share exact pixel boundaries
			left := int32(math.Floor(float64(sx0)))
			top := int32(math.Floor(float64(sy0)))
			right := int32(math.Floor(float64(sx1)))
			bottom := int32(math.Floor(float64(sy1)))

			// Ensure minimum size of 1 pixel
			w := right - left
			h := bottom - top
			if w < 1 {
				w = 1
			}
			if h < 1 {
				h = 1
			}

			val := res[y*gridW+x]
			color := resourceToColor(val, alpha)
			rl.DrawRectangle(left, top, w, h, color)
		}
	}
}

// drawPotentialHeatmap renders the potential field as a colored overlay.
func (g *Game) drawPotentialHeatmap(alpha uint8) {
	cam := g.camera
	pf := g.resourceField
	cellW := g.worldWidth / float32(pf.PotW)
	cellH := g.worldHeight / float32(pf.PotH)

	for y := 0; y < pf.PotH; y++ {
		for x := 0; x < pf.PotW; x++ {
			worldX0 := float32(x) * cellW
			worldY0 := float32(y) * cellH
			worldX1 := float32(x+1) * cellW
			worldY1 := float32(y+1) * cellH

			worldCX := (worldX0 + worldX1) / 2
			worldCY := (worldY0 + worldY1) / 2
			cellRadius := (cellW + cellH) / 2
			if !cam.IsVisible(worldCX, worldCY, cellRadius) {
				continue
			}

			sx0, sy0 := cam.WorldToScreen(worldX0, worldY0)
			sx1, sy1 := cam.WorldToScreen(worldX1, worldY1)

			left := int32(math.Floor(float64(sx0)))
			top := int32(math.Floor(float64(sy0)))
			right := int32(math.Floor(float64(sx1)))
			bottom := int32(math.Floor(float64(sy1)))

			w := right - left
			h := bottom - top
			if w < 1 {
				w = 1
			}
			if h < 1 {
				h = 1
			}

			val := pf.Pot[y*pf.PotW+x]
			color := resourceToColor(val, alpha)
			rl.DrawRectangle(left, top, w, h, color)
		}
	}
}

// drawFlowField renders the flow field as short oriented lines ("hairs").
// All hairs have uniform length; color indicates magnitude (viridis scale).
func (g *Game) drawFlowField() {
	cam := g.camera
	pf := g.resourceField
	cellW := g.worldWidth / float32(pf.FlowW)
	cellH := g.worldHeight / float32(pf.FlowH)

	// Fixed hair length in screen pixels
	const hairLen = float32(8)

	for y := 0; y < pf.FlowH; y++ {
		for x := 0; x < pf.FlowW; x++ {
			// Cell center in world coords
			worldCX := (float32(x) + 0.5) * cellW
			worldCY := (float32(y) + 0.5) * cellH

			if !cam.IsVisible(worldCX, worldCY, cellW) {
				continue
			}

			idx := y*pf.FlowW + x
			u := pf.FlowU[idx]
			v := pf.FlowV[idx]

			// Compute magnitude for coloring
			mag := float32(math.Sqrt(float64(u*u + v*v)))
			if mag < 0.001 {
				continue
			}

			// Normalize direction
			nx := u / mag
			ny := v / mag

			// Screen center of this cell
			sx, sy := cam.WorldToScreen(worldCX, worldCY)

			// Hair endpoints in screen space (fixed length, centered)
			halfLen := hairLen / 2
			x0 := sx - nx*halfLen
			y0 := sy - ny*halfLen
			x1 := sx + nx*halfLen
			y1 := sy + ny*halfLen

			// Color by magnitude using viridis (normalize mag to ~[0,1])
			normMag := mag * 2 // Adjust scaling as needed
			if normMag > 1 {
				normMag = 1
			}
			color := resourceToColor(normMag, 220)

			rl.DrawLineEx(
				rl.Vector2{X: x0, Y: y0},
				rl.Vector2{X: x1, Y: y1},
				1.5,
				color,
			)

			// Small arrowhead at the tip
			arrowSize := float32(3)
			angle := float32(math.Atan2(float64(ny), float64(nx)))
			ax1 := x1 - arrowSize*float32(math.Cos(float64(angle-0.5)))
			ay1 := y1 - arrowSize*float32(math.Sin(float64(angle-0.5)))
			ax2 := x1 - arrowSize*float32(math.Cos(float64(angle+0.5)))
			ay2 := y1 - arrowSize*float32(math.Sin(float64(angle+0.5)))
			rl.DrawTriangle(
				rl.Vector2{X: x1, Y: y1},
				rl.Vector2{X: ax2, Y: ay2},
				rl.Vector2{X: ax1, Y: ay1},
				color,
			)
		}
	}
}

// resourceToColor maps a resource value [0,1] to a viridis color scale.
// Viridis: dark purple → blue → teal → green → yellow
func resourceToColor(val float32, alpha uint8) rl.Color {
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}

	// Viridis color stops (5 points)
	type colorStop struct {
		r, g, b uint8
	}
	stops := []colorStop{
		{68, 1, 84},    // 0.00: dark purple
		{59, 82, 139},  // 0.25: blue-purple
		{33, 145, 140}, // 0.50: teal
		{94, 201, 98},  // 0.75: green
		{253, 231, 37}, // 1.00: yellow
	}

	// Find which segment we're in and interpolate
	idx := int(val * 4)
	if idx >= 4 {
		idx = 3
	}
	t := (val - float32(idx)*0.25) / 0.25

	c0 := stops[idx]
	c1 := stops[idx+1]

	r := uint8(float32(c0.r) + t*(float32(c1.r)-float32(c0.r)))
	g := uint8(float32(c0.g) + t*(float32(c1.g)-float32(c0.g)))
	b := uint8(float32(c0.b) + t*(float32(c1.b)-float32(c0.b)))

	return rl.Color{R: r, G: g, B: b, A: alpha}
}

// drawBackground renders the soft noise texture background.
func (g *Game) drawBackground() {
	if g.backgroundRenderer == nil {
		return
	}

	cam := g.camera
	g.backgroundRenderer.Draw(
		float32(g.tick)*0.03,
		cam.X, cam.Y,
		cam.Zoom,
		g.worldWidth, g.worldHeight,
	)
}

// updateLightRenderer updates the light renderer state (potential texture, blend progress).
func (g *Game) updateLightRenderer() {
	if g.lightRenderer == nil {
		return
	}

	// Update potential texture periodically (every ~60 frames / 1 second)
	if g.tick%60 == 0 {
		pf := g.resourceField
		g.lightRenderer.UpdatePotential(pf.Pot, pf.PotW, pf.PotH)
	}

	// Update blend progress
	dt := float32(1.0 / 60.0) // Fixed timestep
	g.lightRenderer.Update(dt)
}

// drawShadow renders the shadow layer (darkens low-potential areas).
func (g *Game) drawShadow() {
	if g.lightRenderer == nil {
		return
	}

	g.updateLightRenderer()

	cam := g.camera
	g.lightRenderer.DrawShadow(
		cam.X, cam.Y,
		cam.Zoom,
		g.worldWidth, g.worldHeight,
	)
}

// drawCaustics renders the caustic light layer (additive glow on top of everything).
func (g *Game) drawCaustics() {
	if g.lightRenderer == nil {
		return
	}

	cam := g.camera
	g.lightRenderer.DrawCaustics(
		float32(g.tick)*0.03,
		cam.X, cam.Y,
		cam.Zoom,
		g.worldWidth, g.worldHeight,
	)
}

// drawParticles renders the floating resource particles with glow effect.
func (g *Game) drawParticles() {
	if g.particleRenderer == nil {
		return
	}

	cam := g.camera
	pf := g.resourceField

	// Begin rendering particles to texture
	g.particleRenderer.BeginParticles()

	// Draw each active particle
	for i := 0; i < pf.MaxCount; i++ {
		if !pf.Active[i] {
			continue
		}

		// Check if particle is visible
		if !cam.IsVisible(pf.X[i], pf.Y[i], 10) {
			continue
		}

		// Transform to screen coordinates
		sx, sy := cam.WorldToScreen(pf.X[i], pf.Y[i])
		g.particleRenderer.DrawParticleScaled(sx, sy, pf.Mass[i], cam.Zoom)

		// Draw ghost copies for particles near world edges
		ghosts := cam.GhostPositions(pf.X[i], pf.Y[i], 10)
		for _, ghost := range ghosts {
			g.particleRenderer.DrawParticleScaled(ghost.X, ghost.Y, pf.Mass[i], cam.Zoom)
		}
	}

	g.particleRenderer.EndParticles()

	// Draw the particle texture with glow shader
	g.particleRenderer.Draw(float32(g.tick) * 0.05)
}

// drawOrientedTriangle draws a triangle pointing in the heading direction.
func drawOrientedTriangle(x, y, heading, radius float32, color rl.Color) {
	cos := float32(math.Cos(float64(heading)))
	sin := float32(math.Sin(float64(heading)))

	// Front point
	frontX := x + cos*radius*1.5
	frontY := y + sin*radius*1.5

	// Back left
	backAngle := heading + math.Pi*0.8
	backLeftX := x + float32(math.Cos(float64(backAngle)))*radius
	backLeftY := y + float32(math.Sin(float64(backAngle)))*radius

	// Back right
	backAngle = heading - math.Pi*0.8
	backRightX := x + float32(math.Cos(float64(backAngle)))*radius
	backRightY := y + float32(math.Sin(float64(backAngle)))*radius

	v1 := rl.Vector2{X: frontX, Y: frontY}
	v2 := rl.Vector2{X: backLeftX, Y: backLeftY}
	v3 := rl.Vector2{X: backRightX, Y: backRightY}

	// DrawTriangle requires counter-clockwise winding (v1, v3, v2)
	rl.DrawTriangle(v1, v3, v2, color)
	rl.DrawTriangleLines(v1, v2, v3, rl.White)
}

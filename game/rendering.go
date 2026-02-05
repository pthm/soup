package game

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/systems"
)

// Draw renders the game.
func (g *Game) Draw() {
	g.perfCollector.RecordFrame()

	rl.BeginDrawing()
	rl.ClearBackground(rl.Color{R: 230, G: 195, B: 150, A: 255}) // Dusty desert background

	// Debug overlays (drawn before entities so entities appear on top)
	if g.debugMode && g.debugShowResource {
		g.drawResourceHeatmap(180)
	}
	if g.debugMode && g.debugShowPotential {
		g.drawPotentialHeatmap(180)
	}

	// Layer 3: Entities
	g.drawEntities()

	// Draw selection highlight and vision cone
	g.inspector.DrawSelectionHighlight(g.posMap, g.bodyMap, g.rotMap, g.capsMap, g.orgMap, g.camera)

	// Draw HUD
	rl.DrawText(fmt.Sprintf("Tick: %d", g.tick), 10, 10, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Herb: %d  Carn: %d  Dead: %d", g.numHerb, g.numCarn, g.deadCount), 10, 35, 20, rl.White)
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

		// Color by diet: lerp purple (diet=0) → orange (diet=1)
		color := dietColor(org.Diet)

		// Dim based on energy ratio (Met / MaxMet)
		metPerBio := systems.GetCachedMetPerBio()
		maxMet := energy.Bio * metPerBio
		var energyRatio float32
		if maxMet > 0 {
			energyRatio = energy.Met / maxMet
		}
		alpha := uint8(100 + int(energyRatio*155))
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

// drawPotentialHeatmap renders the capacity field as a colored overlay.
func (g *Game) drawPotentialHeatmap(alpha uint8) {
	cam := g.camera
	pf := g.resourceField
	cellW := g.worldWidth / float32(pf.W)
	cellH := g.worldHeight / float32(pf.H)

	for y := 0; y < pf.H; y++ {
		for x := 0; x < pf.W; x++ {
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

			val := pf.Cap[y*pf.W+x]
			color := resourceToColor(val, alpha)
			rl.DrawRectangle(left, top, w, h, color)
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

// dietColor returns a color interpolated from purple (diet=0) to burnt orange (diet=1).
func dietColor(diet float32) rl.Color {
	r := uint8(140 + diet*60)   // 140→200
	g := uint8(100 - diet*20)   // 100→80
	b := uint8(200 - diet*150)  // 200→50
	return rl.Color{R: r, G: g, B: b, A: 255}
}

// drawOrientedTriangle draws a smooth rounded triangle pointing in the heading direction.
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

	// Draw filled triangle (counter-clockwise winding)
	rl.DrawTriangle(v1, v3, v2, color)

	// Round the corners with circles
	cornerRadius := radius * 0.25
	rl.DrawCircleV(v1, cornerRadius, color)
	rl.DrawCircleV(v2, cornerRadius, color)
	rl.DrawCircleV(v3, cornerRadius, color)

	// Fill in edges with thick lines for smoother look
	rl.DrawLineEx(v1, v2, cornerRadius*2, color)
	rl.DrawLineEx(v2, v3, cornerRadius*2, color)
	rl.DrawLineEx(v3, v1, cornerRadius*2, color)
}

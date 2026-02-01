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
	rl.ClearBackground(rl.Black)

	// Water background
	g.water.Draw(float32(g.tick) * 0.01)

	// Debug overlays (drawn before entities so entities appear on top)
	if g.debugMode && g.debugShowResource {
		g.drawResourceHeatmap(180)
	}

	// Draw entities
	g.drawEntities()

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
	panelH := int32(80)

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

	// Performance stats
	stats := g.perfCollector.Stats()
	rl.DrawText(fmt.Sprintf("Tick: %v  TPS: %.0f", stats.AvgTickDuration, stats.TicksPerSecond), panelX+10, panelY+55, 12, rl.White)
}

// drawResourceHeatmap renders the CPU resource field as a colored overlay.
func (g *Game) drawResourceHeatmap(alpha uint8) {
	cam := g.camera
	gridW, gridH := g.cpuResourceField.GridSize()
	cellW := g.worldWidth / float32(gridW)
	cellH := g.worldHeight / float32(gridH)
	res := g.cpuResourceField.ResData()

	// Scale cell size by zoom
	screenCellW := cellW * cam.Zoom
	screenCellH := cellH * cam.Zoom

	for y := 0; y < gridH; y++ {
		for x := 0; x < gridW; x++ {
			// Calculate world position of cell center
			worldX := float32(x)*cellW + cellW/2
			worldY := float32(y)*cellH + cellH/2

			// Check visibility (use cell diagonal as radius)
			cellRadius := (cellW + cellH) / 2
			if !cam.IsVisible(worldX, worldY, cellRadius) {
				continue
			}

			// Transform to screen coordinates
			sx, sy := cam.WorldToScreen(worldX, worldY)

			val := res[y*gridW+x]
			color := resourceToColor(val, alpha)
			rl.DrawRectangle(
				int32(sx-screenCellW/2),
				int32(sy-screenCellH/2),
				int32(screenCellW)+1,
				int32(screenCellH)+1,
				color,
			)
		}
	}
}

// resourceToColor maps a resource value [0,1] to a blue-green-yellow-red heatmap color.
func resourceToColor(val float32, alpha uint8) rl.Color {
	if val < 0 {
		val = 0
	}
	if val > 1 {
		val = 1
	}

	var r, g, b uint8
	if val < 0.25 {
		// Blue to cyan
		t := val / 0.25
		r = 0
		g = uint8(t * 255)
		b = 255
	} else if val < 0.5 {
		// Cyan to green
		t := (val - 0.25) / 0.25
		r = 0
		g = 255
		b = uint8((1 - t) * 255)
	} else if val < 0.75 {
		// Green to yellow
		t := (val - 0.5) / 0.25
		r = uint8(t * 255)
		g = 255
		b = 0
	} else {
		// Yellow to red
		t := (val - 0.75) / 0.25
		r = 255
		g = uint8((1 - t) * 255)
		b = 0
	}

	return rl.Color{R: r, G: g, B: b, A: alpha}
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

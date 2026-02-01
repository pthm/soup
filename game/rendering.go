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
		g.gpuResourceField.DrawOverlayHeatmap(180) // Heatmap with good visibility
	}

	// Draw entities
	g.drawEntities()

	// Draw selection highlight and vision cone
	g.inspector.DrawSelectionHighlight(g.posMap, g.bodyMap, g.rotMap, g.capsMap, g.orgMap)

	// Draw HUD
	rl.DrawText(fmt.Sprintf("Tick: %d", g.tick), 10, 10, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Prey: %d  Pred: %d  Dead: %d", g.numPrey, g.numPred, g.deadCount), 10, 35, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Speed: %dx  [</>]", g.stepsPerUpdate), 10, 60, 20, rl.White)
	if g.paused {
		rl.DrawText("PAUSED", 10, 85, 20, rl.Yellow)
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
	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, body, energy, _, org := query.Get()

		if !energy.Alive {
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

		drawOrientedTriangle(pos.X, pos.Y, rot.Heading, body.Radius, color)
	}
}

// drawDebugMenu renders the debug overlay menu.
func (g *Game) drawDebugMenu() {
	// Semi-transparent background panel
	panelX := int32(g.width) - 200
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

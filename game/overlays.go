package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/ui"
)

// handleOverlayKeys checks for overlay toggle key presses.
func (g *Game) handleOverlayKeys() {
	// Check each registered overlay's key
	for _, desc := range g.uiOverlays.All() {
		if desc.Key != 0 && rl.IsKeyPressed(desc.Key) {
			newState := g.uiOverlays.Toggle(desc.ID)

			// Sync legacy fields for backwards compatibility
			if desc.ID == ui.OverlaySpeciesColors {
				g.showSpeciesColors = newState
			}
		}
	}
}

// drawActiveOverlays renders all currently enabled overlays.
func (g *Game) drawActiveOverlays() {
	for _, id := range g.uiOverlays.EnabledOverlays() {
		switch id {
		case ui.OverlayPerceptionCones:
			g.drawPerceptionCones()
		case ui.OverlayLightMap:
			g.drawLightMapOverlay()
		case ui.OverlayPathfinding:
			g.drawPathfindingOverlay()
		case ui.OverlayCollisionBoxes:
			g.drawCollisionBoxes()
		case ui.OverlayFlowField:
			// Flow field is already drawn by default, but could add vector overlay
			g.drawFlowVectors()
		// Species and capability colors are handled in drawOrganism
		}
	}
}

// drawPerceptionCones draws vision cones for the selected organism.
func (g *Game) drawPerceptionCones() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		return
	}

	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)

	// Draw 4 perception cones (front, right, back, left)
	coneAngle := float32(math.Pi / 2) // 90 degree cones
	coneRadius := org.PerceptionRadius

	// Cone colors: Food (green), Threat (red), Friend (blue)
	// Draw as transparent arcs
	// Add π/2 to align with visual orientation (Y-forward in grid space)
	visualHeading := org.Heading + float32(math.Pi/2)
	for i := 0; i < 4; i++ {
		baseAngle := visualHeading + float32(i)*float32(math.Pi/2)

		// Draw cone outline
		startAngle := baseAngle - coneAngle/2
		endAngle := baseAngle + coneAngle/2

		// Draw arc segments
		segments := 8
		for j := 0; j < segments; j++ {
			a1 := startAngle + float32(j)*coneAngle/float32(segments)
			a2 := startAngle + float32(j+1)*coneAngle/float32(segments)

			x1 := pos.X + coneRadius*float32(math.Cos(float64(a1)))
			y1 := pos.Y + coneRadius*float32(math.Sin(float64(a1)))
			x2 := pos.X + coneRadius*float32(math.Cos(float64(a2)))
			y2 := pos.Y + coneRadius*float32(math.Sin(float64(a2)))

			// Color by direction
			var color rl.Color
			switch i {
			case 0: // Front - food seeking
				color = rl.Color{R: 100, G: 200, B: 100, A: 60}
			case 1: // Right
				color = rl.Color{R: 100, G: 150, B: 200, A: 40}
			case 2: // Back - threat detection
				color = rl.Color{R: 200, G: 100, B: 100, A: 60}
			case 3: // Left
				color = rl.Color{R: 100, G: 150, B: 200, A: 40}
			}

			// Draw filled triangle for each segment
			rl.DrawTriangle(
				rl.Vector2{X: pos.X, Y: pos.Y},
				rl.Vector2{X: x1, Y: y1},
				rl.Vector2{X: x2, Y: y2},
				color,
			)
		}

		// Draw cone edge lines
		edgeColor := rl.Color{R: 200, G: 200, B: 200, A: 100}
		x1 := pos.X + coneRadius*float32(math.Cos(float64(startAngle)))
		y1 := pos.Y + coneRadius*float32(math.Sin(float64(startAngle)))
		x2 := pos.X + coneRadius*float32(math.Cos(float64(endAngle)))
		y2 := pos.Y + coneRadius*float32(math.Sin(float64(endAngle)))
		rl.DrawLine(int32(pos.X), int32(pos.Y), int32(x1), int32(y1), edgeColor)
		rl.DrawLine(int32(pos.X), int32(pos.Y), int32(x2), int32(y2), edgeColor)
	}
}

// drawLightMapOverlay visualizes the shadow map / light distribution.
func (g *Game) drawLightMapOverlay() {
	// Sample light at grid points and draw colored squares
	gridSize := int32(40) // Sample every 40 pixels
	squareSize := int32(36)

	for x := int32(0); x < int32(g.bounds.Width); x += gridSize {
		for y := int32(0); y < int32(g.bounds.Height); y += gridSize {
			light := g.shadowMap.SampleLight(float32(x), float32(y))

			// Color: darker = less light, brighter = more light
			brightness := uint8(light * 200)
			color := rl.Color{R: brightness, G: brightness, B: uint8(float32(brightness) * 1.2), A: 40}
			rl.DrawRectangle(x+2, y+2, squareSize, squareSize, color)
		}
	}
}

// drawPathfindingOverlay shows desire vs actual movement vectors.
func (g *Game) drawPathfindingOverlay() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		return
	}

	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)
	velMap := ecs.NewMap[components.Velocity](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)
	vel := velMap.Get(g.selectedEntity)

	// Draw desire vector (where brain wants to go in world space)
	// DesireAngle is relative to physical heading (what brain sees as forward)
	desireLen := float32(50) * org.DesireDistance
	desireAngle := org.Heading + org.DesireAngle
	desireX := pos.X + desireLen*float32(math.Cos(float64(desireAngle)))
	desireY := pos.Y + desireLen*float32(math.Sin(float64(desireAngle)))
	rl.DrawLine(int32(pos.X), int32(pos.Y), int32(desireX), int32(desireY), rl.Color{R: 255, G: 255, B: 255, A: 200})

	// Draw actual velocity vector (true physical movement)
	velLen := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
	if velLen > 0.1 {
		scale := float32(30) / velLen
		actualX := pos.X + vel.X*scale
		actualY := pos.Y + vel.Y*scale
		rl.DrawLine(int32(pos.X), int32(pos.Y), int32(actualX), int32(actualY), rl.Color{R: 100, G: 255, B: 100, A: 200})
	}

	// Draw heading indicator (visual forward direction)
	visualHeading := org.Heading + float32(math.Pi/2)
	headingLen := float32(25)
	headingX := pos.X + headingLen*float32(math.Cos(float64(visualHeading)))
	headingY := pos.Y + headingLen*float32(math.Sin(float64(visualHeading)))
	rl.DrawLine(int32(pos.X), int32(pos.Y), int32(headingX), int32(headingY), rl.Color{R: 255, G: 200, B: 100, A: 150})
}

// drawCollisionBoxes shows organism bounding boxes.
func (g *Game) drawCollisionBoxes() {
	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, _ := query.Get()
		if org.Dead {
			continue
		}

		// Draw OBB as rotated rectangle
		// Add π/2 to match visual cell rotation
		obb := &org.OBB
		cos := float32(math.Cos(float64(org.Heading) + math.Pi/2))
		sin := float32(math.Sin(float64(org.Heading) + math.Pi/2))

		// Calculate corner offsets
		hw, hh := obb.HalfWidth, obb.HalfHeight
		corners := [][2]float32{
			{-hw, -hh},
			{hw, -hh},
			{hw, hh},
			{-hw, hh},
		}

		// Transform and draw
		var transformed [4]rl.Vector2
		for i, c := range corners {
			// Rotate
			rx := c[0]*cos - c[1]*sin
			ry := c[0]*sin + c[1]*cos
			// Translate
			transformed[i] = rl.Vector2{
				X: pos.X + obb.OffsetX + rx,
				Y: pos.Y + obb.OffsetY + ry,
			}
		}

		// Draw lines
		color := rl.Color{R: 200, G: 200, B: 100, A: 100}
		for i := 0; i < 4; i++ {
			j := (i + 1) % 4
			rl.DrawLine(
				int32(transformed[i].X), int32(transformed[i].Y),
				int32(transformed[j].X), int32(transformed[j].Y),
				color,
			)
		}
	}
}

// drawFlowVectors shows flow field particles more prominently.
// The flow field is already visualized via particles, this overlay makes them more visible.
func (g *Game) drawFlowVectors() {
	// Highlight flow particles when overlay is enabled
	// The actual flow field visualization is done by the flow renderer
	// This just adds a subtle indicator that the overlay is active
	rl.DrawText("Flow Field Active", 10, int32(g.bounds.Height)-50, 12, rl.Color{R: 100, G: 180, B: 255, A: 150})
}

package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
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
		case ui.OverlayBehaviorInspect:
			g.drawBehaviorInspectOverlay()
		case ui.OverlayPathfinding:
			g.drawPathfindingOverlay()
		case ui.OverlayCollisionBoxes:
			g.drawCollisionBoxes()
		case ui.OverlayFlowField:
			// Flow field is already drawn by default, but could add vector overlay
			g.drawFlowVectors()
		case ui.OverlayOrientation:
			g.drawOrientationDebug()
			// Species and capability colors are handled in drawOrganism
		}
	}
}

const (
	behaviorInspectMinSensorGain   = float32(0.1)
	behaviorInspectCapabilityScale = 4.0
)

func (g *Game) drawBehaviorInspectOverlay() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		return
	}

	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	centerX, centerY := g.getOrganismCenter(pos, org)

	var caps components.Capabilities
	if cells != nil {
		caps = cells.ComputeCapabilities()
	}
	digestiveSpectrum := caps.DigestiveSpectrum()
	composition := caps.Composition()
	armor := caps.StructuralArmor

	effectiveRadius := g.getEffectivePerceptionRadius(org.PerceptionRadius, cells)
	if effectiveRadius <= 0 {
		return
	}

	selectedSpeciesID := 0
	if g.neuralGenomeMap.Has(g.selectedEntity) {
		if ng := g.neuralGenomeMap.Get(g.selectedEntity); ng != nil {
			selectedSpeciesID = ng.SpeciesID
		}
	}

	foodColor := rl.Color{R: 110, G: 220, B: 120, A: 180}
	mateColor := rl.Color{R: 240, G: 120, B: 200, A: 200}
	threatColor := rl.Color{R: 230, G: 90, B: 90, A: 200}
	radiusColor := rl.Color{R: 200, G: 200, B: 200, A: 40}

	rl.DrawCircleLines(int32(centerX), int32(centerY), effectiveRadius, radiusColor)

	var nearestFoodDist float32 = effectiveRadius + 1
	var nearestFoodX, nearestFoodY float32
	var nearestMateDist float32 = effectiveRadius + 1
	var nearestMateX, nearestMateY float32
	var nearestThreatScore float32
	var nearestThreatX, nearestThreatY float32

	plantPreference := 1.0 - digestiveSpectrum
	meatPreference := digestiveSpectrum

	for i := range g.floraSystem.Flora {
		flora := &g.floraSystem.Flora[i]
		if flora.Dead {
			continue
		}
		if plantPreference < 0.3 {
			continue
		}

		dx, dy := systems.ToroidalDelta(centerX, centerY, flora.X, flora.Y, g.bounds.Width, g.bounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist > effectiveRadius {
			continue
		}

		rl.DrawCircleLines(int32(flora.X), int32(flora.Y), flora.Size+2, foodColor)

		if dist < nearestFoodDist {
			nearestFoodDist = dist
			nearestFoodX = flora.X
			nearestFoodY = flora.Y
		}
	}

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		if entity == g.selectedEntity {
			continue
		}

		pos, _, otherOrg, otherCells := query.Get()

		dx, dy := systems.ToroidalDelta(centerX, centerY, pos.X, pos.Y, g.bounds.Width, g.bounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist > effectiveRadius {
			continue
		}

		isMate := false
		if selectedSpeciesID > 0 && g.neuralGenomeMap.Has(entity) {
			if ng := g.neuralGenomeMap.Get(entity); ng != nil && ng.SpeciesID == selectedSpeciesID {
				isMate = !otherOrg.Dead
			}
		}

		otherDigestive := float32(0.5)
		if otherCells != nil {
			otherCaps := otherCells.ComputeCapabilities()
			otherDigestive = otherCaps.DigestiveSpectrum()
		}

		threatScore := float32(0)
		if !otherOrg.Dead {
			threatLevel := neural.ThreatLevel(otherDigestive, composition, armor)
			proximity := 1.0 - (dist / effectiveRadius)
			if proximity < 0 {
				proximity = 0
			}
			threatScore = threatLevel * proximity
		}

		isFood := meatPreference >= 0.3

		bodyRadius := otherOrg.BodyRadius
		if bodyRadius < 1 {
			bodyRadius = otherOrg.CellSize
		}

		switch {
		case isMate:
			rl.DrawCircleLines(int32(pos.X), int32(pos.Y), bodyRadius+3, mateColor)
			if dist < nearestMateDist {
				nearestMateDist = dist
				nearestMateX = pos.X
				nearestMateY = pos.Y
			}
		case threatScore > 0.1:
			alpha := uint8(120 + clamp01(threatScore)*100)
			rl.DrawCircleLines(int32(pos.X), int32(pos.Y), bodyRadius+3, rl.Color{R: threatColor.R, G: threatColor.G, B: threatColor.B, A: alpha})
			if threatScore > nearestThreatScore {
				nearestThreatScore = threatScore
				nearestThreatX = pos.X
				nearestThreatY = pos.Y
			}
		case isFood:
			rl.DrawCircleLines(int32(pos.X), int32(pos.Y), bodyRadius+2, foodColor)
			if dist < nearestFoodDist {
				nearestFoodDist = dist
				nearestFoodX = pos.X
				nearestFoodY = pos.Y
			}
		}
	}

	if nearestFoodDist <= effectiveRadius {
		g.drawVectorArrow(centerX, centerY, nearestFoodX, nearestFoodY, foodColor, "F")
	}
	if nearestMateDist <= effectiveRadius {
		g.drawVectorArrow(centerX, centerY, nearestMateX, nearestMateY, mateColor, "M")
	}
	if nearestThreatScore > 0.1 {
		g.drawVectorArrow(centerX, centerY, nearestThreatX, nearestThreatY, threatColor, "T")
	}
}

func (g *Game) getOrganismCenter(pos *components.Position, org *components.Organism) (float32, float32) {
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH
	return centerX, centerY
}

func (g *Game) getEffectivePerceptionRadius(baseRadius float32, cells *components.CellBuffer) float32 {
	totalGain := float32(0)
	if cells != nil {
		for i := uint8(0); i < cells.Count; i++ {
			totalGain += cells.Cells[i].GetSensorStrength()
		}
	}
	if totalGain < behaviorInspectMinSensorGain {
		totalGain = behaviorInspectMinSensorGain
	}
	scale := float32(0.5 + math.Min(1.0, float64(totalGain)/behaviorInspectCapabilityScale))
	return baseRadius * scale
}

func (g *Game) drawVectorArrow(startX, startY, endX, endY float32, color rl.Color, label string) {
	rl.DrawLine(int32(startX), int32(startY), int32(endX), int32(endY), color)

	dx := endX - startX
	dy := endY - startY
	length := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if length < 0.1 {
		return
	}

	dirX := dx / length
	dirY := dy / length
	headLen := float32(10)
	headAngle := float32(math.Pi / 6)
	angle := float32(math.Atan2(float64(dirY), float64(dirX)))

	leftX := endX - headLen*float32(math.Cos(float64(angle)-float64(headAngle)))
	leftY := endY - headLen*float32(math.Sin(float64(angle)-float64(headAngle)))
	rightX := endX - headLen*float32(math.Cos(float64(angle)+float64(headAngle)))
	rightY := endY - headLen*float32(math.Sin(float64(angle)+float64(headAngle)))

	rl.DrawLine(int32(endX), int32(endY), int32(leftX), int32(leftY), color)
	rl.DrawLine(int32(endX), int32(endY), int32(rightX), int32(rightY), color)

	if label != "" {
		rl.DrawText(label, int32(endX)+4, int32(endY)-4, 12, color)
	}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	// Draw 4 perception cones (front, right, back, left)
	coneAngle := float32(math.Pi / 2) // 90 degree cones
	coneRadius := org.PerceptionRadius

	// Cone colors: Food (green), Threat (red), Friend (blue)
	// Draw as transparent arcs
	// X+ is forward in local grid space, aligned with heading
	visualHeading := org.Heading
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

			x1 := centerX + coneRadius*float32(math.Cos(float64(a1)))
			y1 := centerY + coneRadius*float32(math.Sin(float64(a1)))
			x2 := centerX + coneRadius*float32(math.Cos(float64(a2)))
			y2 := centerY + coneRadius*float32(math.Sin(float64(a2)))

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
				rl.Vector2{X: centerX, Y: centerY},
				rl.Vector2{X: x1, Y: y1},
				rl.Vector2{X: x2, Y: y2},
				color,
			)
		}

		// Draw cone edge lines
		edgeColor := rl.Color{R: 200, G: 200, B: 200, A: 100}
		x1 := centerX + coneRadius*float32(math.Cos(float64(startAngle)))
		y1 := centerY + coneRadius*float32(math.Sin(float64(startAngle)))
		x2 := centerX + coneRadius*float32(math.Cos(float64(endAngle)))
		y2 := centerY + coneRadius*float32(math.Sin(float64(endAngle)))
		rl.DrawLine(int32(centerX), int32(centerY), int32(x1), int32(y1), edgeColor)
		rl.DrawLine(int32(centerX), int32(centerY), int32(x2), int32(y2), edgeColor)
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

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	// Draw throttle vector (heading direction scaled by throttle)
	// Heading-as-state: desired velocity is along heading, scaled by throttle
	throttleScale := float32(40) * org.UThrottle
	desireX := centerX + cosH*throttleScale
	desireY := centerY + sinH*throttleScale
	rl.DrawLine(int32(centerX), int32(centerY), int32(desireX), int32(desireY), rl.Color{R: 255, G: 255, B: 255, A: 200})

	// Draw actual velocity vector (true physical movement)
	velLen := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
	if velLen > 0.1 {
		scale := float32(30) / velLen
		actualX := centerX + vel.X*scale
		actualY := centerY + vel.Y*scale
		rl.DrawLine(int32(centerX), int32(centerY), int32(actualX), int32(actualY), rl.Color{R: 100, G: 255, B: 100, A: 200})
	}

	// Draw heading indicator (forward direction)
	headingLen := float32(25)
	headingX := centerX + headingLen*cosH
	headingY := centerY + headingLen*sinH
	rl.DrawLine(int32(centerX), int32(centerY), int32(headingX), int32(headingY), rl.Color{R: 255, G: 200, B: 100, A: 150})
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
		// X+ is forward in local grid space, aligned with heading
		obb := &org.OBB
		cos := float32(math.Cos(float64(org.Heading)))
		sin := float32(math.Sin(float64(org.Heading)))

		// OBB center in world coordinates (offset is in local space, must be rotated)
		centerX := pos.X + obb.OffsetX*cos - obb.OffsetY*sin
		centerY := pos.Y + obb.OffsetX*sin + obb.OffsetY*cos

		// Calculate corner offsets from OBB center
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
			// Rotate corner around OBB center
			rx := c[0]*cos - c[1]*sin
			ry := c[0]*sin + c[1]*cos
			// Translate from rotated OBB center
			transformed[i] = rl.Vector2{
				X: centerX + rx,
				Y: centerY + ry,
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

// drawOrientationDebug draws forward/right axes and sensor/actuator cell indicators.
// Forward (local +X) is shown in green, right (local +Y) is shown in blue.
func (g *Game) drawOrientationDebug() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		return
	}

	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	// Pre-compute rotation
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	arrowLen := float32(40)

	// Draw forward arrow (local +X, green)
	forwardX := centerX + arrowLen*cosH
	forwardY := centerY + arrowLen*sinH
	rl.DrawLine(int32(centerX), int32(centerY), int32(forwardX), int32(forwardY), rl.Color{R: 100, G: 255, B: 100, A: 255})
	// Arrowhead
	headLen := float32(8)
	headAngle := float32(math.Pi / 6)
	ax1 := forwardX - headLen*float32(math.Cos(float64(org.Heading)-float64(headAngle)))
	ay1 := forwardY - headLen*float32(math.Sin(float64(org.Heading)-float64(headAngle)))
	ax2 := forwardX - headLen*float32(math.Cos(float64(org.Heading)+float64(headAngle)))
	ay2 := forwardY - headLen*float32(math.Sin(float64(org.Heading)+float64(headAngle)))
	rl.DrawLine(int32(forwardX), int32(forwardY), int32(ax1), int32(ay1), rl.Color{R: 100, G: 255, B: 100, A: 255})
	rl.DrawLine(int32(forwardX), int32(forwardY), int32(ax2), int32(ay2), rl.Color{R: 100, G: 255, B: 100, A: 255})

	// Draw right arrow (local +Y, blue)
	// +Y is 90 degrees clockwise from heading (heading + π/2)
	rightAngle := org.Heading + float32(math.Pi/2)
	rightX := centerX + arrowLen*0.6*float32(math.Cos(float64(rightAngle)))
	rightY := centerY + arrowLen*0.6*float32(math.Sin(float64(rightAngle)))
	rl.DrawLine(int32(centerX), int32(centerY), int32(rightX), int32(rightY), rl.Color{R: 100, G: 150, B: 255, A: 255})

	// Draw sensor and actuator cells
	if cells != nil {
		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}

			// Transform cell position to world
			localX := float32(cell.GridX) * org.CellSize
			localY := float32(cell.GridY) * org.CellSize
			rotatedX := localX*cosH - localY*sinH
			rotatedY := localX*sinH + localY*cosH
			cellX := pos.X + rotatedX
			cellY := pos.Y + rotatedY

			// Draw sensor cells (white with facing line)
			sensorStr := cell.GetSensorStrength()
			if sensorStr > 0 {
				rl.DrawCircle(int32(cellX), int32(cellY), 4, rl.Color{R: 255, G: 255, B: 255, A: 200})
				// Draw facing line (forward from cell)
				faceLen := float32(10) * sensorStr
				faceX := cellX + faceLen*cosH
				faceY := cellY + faceLen*sinH
				rl.DrawLine(int32(cellX), int32(cellY), int32(faceX), int32(faceY), rl.Color{R: 255, G: 255, B: 255, A: 150})
			}

			// Draw actuator cells (orange)
			actuatorStr := cell.GetActuatorStrength()
			if actuatorStr > 0 {
				rl.DrawCircle(int32(cellX), int32(cellY), 3, rl.Color{R: 255, G: 150, B: 50, A: 200})
			}
		}
	}

	// Label
	rl.DrawText("X+ = Forward (green), Y+ = Right (blue)", 10, int32(g.bounds.Height)-30, 12, rl.Color{R: 200, G: 200, B: 200, A: 200})
}

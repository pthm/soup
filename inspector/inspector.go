package inspector

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/camera"
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// Panel dimensions
const (
	PanelWidth   = 380
	PanelPadding = 10
	HeaderHeight = 30
)

// Panel colors
var (
	ColorPanelBg     = rl.Color{R: 30, G: 30, B: 35, A: 240}
	ColorPanelHeader = rl.Color{R: 45, G: 45, B: 55, A: 255}
	ColorPanelBorder = rl.Color{R: 70, G: 70, B: 80, A: 255}
	ColorHeaderText  = rl.Color{R: 255, G: 255, B: 255, A: 255}
	ColorCloseBtn    = rl.Color{R: 180, G: 80, B: 80, A: 255}
	ColorSection     = rl.Color{R: 50, G: 50, B: 60, A: 255}
	ColorSectionText = rl.Color{R: 200, G: 200, B: 220, A: 255}
)

// Inspector manages entity selection and panel rendering.
type Inspector struct {
	selected    ecs.Entity
	hasSelected bool
	panelX      int32
	panelY      int32
	screenWidth int32
	screenHeight int32

	// Cached component data for display
	lastInputs *systems.SensorInputs
	lastAct    *neural.Activations
}

// NewInspector creates a new inspector instance.
func NewInspector(screenWidth, screenHeight int32) *Inspector {
	return &Inspector{
		panelX:       screenWidth - PanelWidth - 10,
		panelY:       10,
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
	}
}

// Resize updates screen dimensions and recalculates panel position.
func (ins *Inspector) Resize(screenWidth, screenHeight int32) {
	if screenWidth == ins.screenWidth && screenHeight == ins.screenHeight {
		return
	}
	ins.screenWidth = screenWidth
	ins.screenHeight = screenHeight
	ins.panelX = screenWidth - PanelWidth - 10
}

// HandleInput processes click detection for entity selection.
func (ins *Inspector) HandleInput(
	mouseX, mouseY float32,
	posMap *ecs.Map1[components.Position],
	bodyMap *ecs.Map1[components.Body],
	orgMap *ecs.Map1[components.Organism],
	filter *ecs.Filter7[
		components.Position,
		components.Velocity,
		components.Rotation,
		components.Body,
		components.Energy,
		components.Capabilities,
		components.Organism,
	],
	cam *camera.Camera,
) {
	// Right click or Escape to deselect
	if rl.IsMouseButtonPressed(rl.MouseButtonRight) || rl.IsKeyPressed(rl.KeyEscape) {
		ins.Deselect()
		return
	}

	// Left click to select
	if !rl.IsMouseButtonPressed(rl.MouseButtonLeft) {
		return
	}

	// Check if clicking the close button
	if ins.hasSelected {
		closeX := ins.panelX + PanelWidth - 25
		closeY := ins.panelY + 5
		if int32(mouseX) >= closeX && int32(mouseX) <= closeX+20 &&
			int32(mouseY) >= closeY && int32(mouseY) <= closeY+20 {
			ins.Deselect()
			return
		}

		// Check if clicking inside panel (ignore)
		if int32(mouseX) >= ins.panelX && int32(mouseX) <= ins.panelX+PanelWidth &&
			int32(mouseY) >= ins.panelY {
			return
		}
	}

	// Convert screen coords to world coords
	worldX, worldY := cam.ScreenToWorld(mouseX, mouseY)

	// Find clicked entity using toroidal distance
	var closest ecs.Entity
	closestDist := float32(1000000)
	found := false

	query := filter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, body, energy, _, _ := query.Get()

		if !energy.Alive {
			continue
		}

		// Use toroidal distance for hit detection
		dx := toroidalDelta(worldX, pos.X, cam.WorldW)
		dy := toroidalDelta(worldY, pos.Y, cam.WorldH)
		dist := dx*dx + dy*dy

		// Check if within body radius (with some tolerance, scaled by zoom)
		hitRadius := body.Radius + 5/cam.Zoom
		if dist < hitRadius*hitRadius && dist < closestDist {
			closest = entity
			closestDist = dist
			found = true
		}
	}

	if found {
		ins.selected = closest
		ins.hasSelected = true
	}
}

// toroidalDelta computes the shortest signed distance from 'from' to 'to'
// in a toroidal space of the given size.
func toroidalDelta(to, from, size float32) float32 {
	d := to - from
	if d > size/2 {
		d -= size
	} else if d < -size/2 {
		d += size
	}
	return d
}

// Deselect clears the current selection.
func (ins *Inspector) Deselect() {
	ins.hasSelected = false
	ins.lastInputs = nil
	ins.lastAct = nil
}

// Selected returns the currently selected entity.
func (ins *Inspector) Selected() (ecs.Entity, bool) {
	return ins.selected, ins.hasSelected
}

// SetSensorData caches sensor inputs for display.
func (ins *Inspector) SetSensorData(inputs *systems.SensorInputs) {
	ins.lastInputs = inputs
}

// SetActivations caches neural activations for display.
func (ins *Inspector) SetActivations(act *neural.Activations) {
	ins.lastAct = act
}

// Draw renders the inspector panel if an entity is selected.
func (ins *Inspector) Draw(
	posMap *ecs.Map1[components.Position],
	velMap *ecs.Map1[components.Velocity],
	rotMap *ecs.Map1[components.Rotation],
	bodyMap *ecs.Map1[components.Body],
	energyMap *ecs.Map1[components.Energy],
	capsMap *ecs.Map1[components.Capabilities],
	orgMap *ecs.Map1[components.Organism],
	brains map[uint32]*neural.FFNN,
) {
	if !ins.hasSelected {
		return
	}

	// Get entity components
	pos := posMap.Get(ins.selected)
	vel := velMap.Get(ins.selected)
	rot := rotMap.Get(ins.selected)
	_ = bodyMap.Get(ins.selected) // body available but not currently displayed
	energy := energyMap.Get(ins.selected)
	_ = capsMap.Get(ins.selected) // capabilities available but not currently displayed
	org := orgMap.Get(ins.selected)

	// Entity may have been deleted
	if pos == nil || org == nil || energy == nil {
		ins.Deselect()
		return
	}

	// Check if entity is still alive
	if !energy.Alive {
		ins.Deselect()
		return
	}

	// Calculate panel height based on content
	panelHeight := ins.calculatePanelHeight()

	// Draw panel background
	rl.DrawRectangle(ins.panelX, ins.panelY, PanelWidth, panelHeight, ColorPanelBg)
	rl.DrawRectangleLinesEx(
		rl.Rectangle{X: float32(ins.panelX), Y: float32(ins.panelY), Width: PanelWidth, Height: float32(panelHeight)},
		1,
		ColorPanelBorder,
	)

	// Draw header
	rl.DrawRectangle(ins.panelX, ins.panelY, PanelWidth, HeaderHeight, ColorPanelHeader)
	rl.DrawText("INSPECTOR", ins.panelX+PanelPadding, ins.panelY+7, 16, ColorHeaderText)

	// Draw close button
	closeX := ins.panelX + PanelWidth - 25
	closeY := ins.panelY + 5
	rl.DrawRectangle(closeX, closeY, 20, 20, ColorCloseBtn)
	rl.DrawText("X", closeX+6, closeY+3, 14, rl.White)

	// Content area
	y := ins.panelY + HeaderHeight + PanelPadding
	x := ins.panelX + PanelPadding

	// Entity info section
	ins.drawSectionHeader(x, y, "ORGANISM")
	y += 20

	// Identity row
	kindStr := "Herbivore"
	if org.Diet >= 0.5 {
		kindStr = "Carnivore"
	}
	rl.DrawText(fmt.Sprintf("ID: %d", org.ID), x, y, 14, ColorHeaderText)
	rl.DrawText(kindStr, x+80, y, 14, ColorHeaderText)
	y += 18

	// Archetype and Clade
	rl.DrawText(fmt.Sprintf("Archetype: %d", org.FounderArchetypeID), x, y, 12, ColorText)
	rl.DrawText(fmt.Sprintf("Clade: %d", org.CladeID), x+120, y, 12, ColorText)
	y += 16

	// Diet bar
	y += DrawBar(x, y, "Diet", org.Diet, nil)

	// Energy bar (normalized to max)
	if energy != nil {
		energyRatio := energy.Value / energy.Max
		y += DrawBar(x, y, "Energy", energyRatio, nil)
	}

	// Age
	if energy != nil {
		y += DrawLabel(x, y, "Age", fmt.Sprintf("%.1fs", energy.Age), nil)
	}

	// Separator
	y += 4
	rl.DrawLine(x, y, ins.panelX+PanelWidth-PanelPadding, y, ColorPanelBorder)
	y += 8

	// State section
	ins.drawSectionHeader(x, y, "STATE")
	y += 20

	// Position
	y += DrawLabel(x, y, "Position", fmt.Sprintf("(%.0f, %.0f)", pos.X, pos.Y), nil)

	// Velocity
	if vel != nil {
		speed := float32(0)
		if vel.X != 0 || vel.Y != 0 {
			speed = float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		}
		y += DrawLabel(x, y, "Speed", fmt.Sprintf("%.1f", speed), nil)
	}

	// Rotation
	if rot != nil {
		y += DrawAngle(x, y, "Heading", rot.Heading, nil)
	}

	// Cooldowns (if any)
	if org.ReproCooldown > 0 {
		y += DrawLabel(x, y, "Repro CD", fmt.Sprintf("%.1fs", org.ReproCooldown), nil)
	}
	if org.DigestCooldown > 0 {
		y += DrawLabel(x, y, "Digest CD", fmt.Sprintf("%.1fs", org.DigestCooldown), nil)
	}

	// Separator
	y += 4
	rl.DrawLine(x, y, ins.panelX+PanelWidth-PanelPadding, y, ColorPanelBorder)
	y += 8

	// Sensors section
	ins.drawSectionHeader(x, y, "SENSORS")
	y += 20

	if ins.lastInputs != nil {
		labelOpts := map[string]string{"labels": "B, BR, R, FR, F, FL, L, BL"}
		y += DrawBarGroup(x, y, "Food", ins.lastInputs.Food[:], labelOpts)
		y += DrawBarGroup(x, y, "Threat", ins.lastInputs.Threat[:], labelOpts)
		y += DrawBarGroup(x, y, "Kin", ins.lastInputs.Kin[:], labelOpts)
	} else {
		rl.DrawText("(no sensor data)", x, y, 12, ColorLabelDim)
		y += 16
	}

	// Separator
	y += 4
	rl.DrawLine(x, y, ins.panelX+PanelWidth-PanelPadding, y, ColorPanelBorder)
	y += 8

	// Neural network section
	ins.drawSectionHeader(x, y, "NEURAL NETWORK")
	y += 20

	var nn *neural.FFNN
	if org != nil {
		nn = brains[org.ID]
	}

	networkHeight := int32(320)
	DrawNetworkDiagram(x, y, PanelWidth-2*PanelPadding, networkHeight, nn, ins.lastAct)
}

// drawSectionHeader renders a section title.
func (ins *Inspector) drawSectionHeader(x, y int32, title string) {
	rl.DrawRectangle(x-2, y-2, PanelWidth-2*PanelPadding+4, 18, ColorSection)
	rl.DrawText(title, x+2, y, 14, ColorSectionText)
}

// calculatePanelHeight computes the dynamic panel height.
func (ins *Inspector) calculatePanelHeight() int32 {
	// Base height
	height := HeaderHeight + PanelPadding // header

	// Organism section
	height += 20  // section header
	height += 18  // ID + kind row
	height += 16  // archetype + clade row
	height += 18  // diet bar
	height += 18  // energy bar
	height += 20  // age
	height += 12  // separator

	// State section
	height += 20 // section header
	height += 20 // position
	height += 20 // speed
	height += 44 // rotation (angle widget)
	height += 40 // estimated cooldowns (may not always show)
	height += 12 // separator

	// Sensors section
	height += 20      // sensors header
	height += 44 * 3  // sensor bars (with labels)
	height += 12      // separator

	// Network section
	height += 20  // network header
	height += 320 // network diagram (increased)
	height += PanelPadding

	return int32(height)
}

// DrawSelectionHighlight draws a highlight around the selected entity.
func (ins *Inspector) DrawSelectionHighlight(
	posMap *ecs.Map1[components.Position],
	bodyMap *ecs.Map1[components.Body],
	rotMap *ecs.Map1[components.Rotation],
	capsMap *ecs.Map1[components.Capabilities],
	orgMap *ecs.Map1[components.Organism],
	cam *camera.Camera,
) {
	if !ins.hasSelected {
		return
	}

	pos := posMap.Get(ins.selected)
	body := bodyMap.Get(ins.selected)
	rot := rotMap.Get(ins.selected)
	caps := capsMap.Get(ins.selected)
	org := orgMap.Get(ins.selected)
	if pos == nil || body == nil {
		return
	}

	// Transform to screen coordinates
	sx, sy := cam.WorldToScreen(pos.X, pos.Y)

	// Selection circle (scaled by zoom)
	radius := body.Radius * 1.8 * cam.Zoom
	rl.DrawCircleLines(int32(sx), int32(sy), radius, rl.Yellow)

	// Draw vision sectors if we have rotation, capabilities, and organism
	if rot != nil && caps != nil && org != nil {
		ins.drawVisionSectors(sx, sy, rot.Heading, caps.VisionRange, cam.Zoom, org.Diet, ins.lastInputs)
	}
}

// drawVisionSectors renders the full 360° vision field with effectiveness-based coloring.
// If sensor inputs are available, overlays nearest-distance wedges with density-based color.
func (ins *Inspector) drawVisionSectors(x, y, heading, visionRange, zoom float32, diet float32, inputs *systems.SensorInputs) {
	const numSectors = systems.NumSectors
	rangePx := visionRange * zoom
	labels := [numSectors]string{"B", "BR", "R", "FR", "F", "FL", "L", "BL"}

	// Draw each sector as a filled arc
	for i := 0; i < numSectors; i++ {
		// Sector angles (relative to heading)
		relStart, relEnd := systems.SectorAngles(i)
		startAngle := heading + relStart
		endAngle := heading + relEnd

		// Calculate effectiveness for this sector
		eff := systems.VisionEffectivenessForSector(i, diet)

		// Base color based on effectiveness: brighter = more effective
		// Lerp between blue-cyan (diet=0) and red-orange (diet=1)
		alpha := uint8(12 + eff*28) // 12-40 alpha range
		r := uint8(100 + diet*155)
		g := uint8(150 + eff*100)
		b := uint8(255 - diet*155)
		color := rl.Color{R: r, G: g, B: b, A: alpha}

		// Draw base sector as triangle fan
		drawSectorFilled(x, y, rangePx, startAngle, endAngle, color)

		if inputs != nil {
			foodLevel := clamp01(inputs.Food[i])
			threatLevel := clamp01(inputs.Threat[i])

			if inputs.NearestFood[i] > 0 {
				foodRadius := inputs.NearestFood[i] * zoom
				foodAlpha := uint8(20 + foodLevel*120)
				foodColor := rl.Color{R: 80, G: 220, B: 80, A: foodAlpha} // Green for food
				drawSectorFilled(x, y, foodRadius, startAngle, endAngle, foodColor)
			}
			if inputs.NearestThreat[i] > 0 {
				threatRadius := inputs.NearestThreat[i] * zoom
				threatAlpha := uint8(20 + threatLevel*120)
				threatColor := rl.Color{R: 255, G: 80, B: 80, A: threatAlpha} // Red for threat
				drawSectorFilled(x, y, threatRadius, startAngle, endAngle, threatColor)
			}
		}

		// Draw sector edge lines
		edgeColor := rl.Color{R: 200, G: 200, B: 200, A: 40}
		drawSectorEdge(x, y, rangePx, startAngle, edgeColor)
	}

	// Sector labels (outer ring)
	labelRadius := rangePx + 10
	for i := 0; i < numSectors; i++ {
		relStart, relEnd := systems.SectorAngles(i)
		midAngle := heading + (relStart+relEnd)*0.5
		lx := x + labelRadius*float32(math.Cos(float64(midAngle)))
		ly := y + labelRadius*float32(math.Sin(float64(midAngle)))
		label := labels[i]
		textW := float32(rl.MeasureText(label, 10))
		rl.DrawText(label, int32(lx-textW*0.5), int32(ly-5), 10, rl.Color{R: 230, G: 230, B: 230, A: 180})
	}

	// Draw outer circle
	drawArc(x, y, rangePx, 0, 2*math.Pi, rl.Color{R: 200, G: 200, B: 200, A: 50})
}

// drawSectorFilled draws a filled pie sector.
func drawSectorFilled(cx, cy, radius, startAngle, endAngle float32, color rl.Color) {
	const segments = 12
	angleStep := (endAngle - startAngle) / float32(segments)

	for i := 0; i < segments; i++ {
		a1 := startAngle + float32(i)*angleStep
		a2 := a1 + angleStep

		x1 := cx + radius*float32(math.Cos(float64(a1)))
		y1 := cy + radius*float32(math.Sin(float64(a1)))
		x2 := cx + radius*float32(math.Cos(float64(a2)))
		y2 := cy + radius*float32(math.Sin(float64(a2)))

		// DrawTriangle requires counter-clockwise winding (screen coords: Y down)
		rl.DrawTriangle(
			rl.Vector2{X: cx, Y: cy},
			rl.Vector2{X: x2, Y: y2},
			rl.Vector2{X: x1, Y: y1},
			color,
		)
	}
}

// drawSectorEdge draws a line from center to edge of vision cone.
func drawSectorEdge(cx, cy, radius, angle float32, color rl.Color) {
	ex := cx + radius*float32(math.Cos(float64(angle)))
	ey := cy + radius*float32(math.Sin(float64(angle)))
	rl.DrawLineV(rl.Vector2{X: cx, Y: cy}, rl.Vector2{X: ex, Y: ey}, color)
}

// drawArc draws an arc between two angles.
func drawArc(cx, cy, radius, startAngle, endAngle float32, color rl.Color) {
	const segments = 20
	angleStep := (endAngle - startAngle) / float32(segments)

	for i := 0; i < segments; i++ {
		a1 := startAngle + float32(i)*angleStep
		a2 := a1 + angleStep

		x1 := cx + radius*float32(math.Cos(float64(a1)))
		y1 := cy + radius*float32(math.Sin(float64(a1)))
		x2 := cx + radius*float32(math.Cos(float64(a2)))
		y2 := cy + radius*float32(math.Sin(float64(a2)))

		rl.DrawLineV(rl.Vector2{X: x1, Y: y1}, rl.Vector2{X: x2, Y: y2}, color)
	}
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

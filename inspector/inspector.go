package inspector

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// Panel dimensions
const (
	PanelWidth   = 320
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

	// Find clicked entity
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

		dx := mouseX - pos.X
		dy := mouseY - pos.Y
		dist := dx*dx + dy*dy

		// Check if within body radius (with some tolerance)
		hitRadius := body.Radius + 5
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
	body := bodyMap.Get(ins.selected)
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
	kindStr := "Prey"
	if org.Kind == components.KindPredator {
		kindStr = "Predator"
	}
	rl.DrawText(fmt.Sprintf("ID: %d  Kind: %s", org.ID, kindStr), x, y, 14, ColorHeaderText)
	y += 22

	// Separator
	rl.DrawLine(x, y, ins.panelX+PanelWidth-PanelPadding, y, ColorPanelBorder)
	y += 8

	// Position
	y += DrawLabel(x, y, "Position", fmt.Sprintf("(%.0f, %.0f)", pos.X, pos.Y), nil)

	// Velocity
	if vel != nil {
		y += DrawLabel(x, y, "Velocity", fmt.Sprintf("(%.1f, %.1f)", vel.X, vel.Y), nil)
	}

	// Rotation
	if rot != nil {
		y += DrawAngle(x, y, "Heading", rot.Heading, nil)
	}

	// Energy
	if energy != nil {
		y += DrawBar(x, y, "Energy", energy.Value, nil)
		y += DrawLabel(x, y, "Age", fmt.Sprintf("%.1fs", energy.Age), nil)
	}

	// Body
	if body != nil {
		y += DrawLabel(x, y, "Radius", fmt.Sprintf("%.1f", body.Radius), nil)
	}

	// Separator
	y += 4
	rl.DrawLine(x, y, ins.panelX+PanelWidth-PanelPadding, y, ColorPanelBorder)
	y += 8

	// Sensors section
	ins.drawSectionHeader(x, y, "SENSORS")
	y += 20

	if ins.lastInputs != nil {
		y += DrawBarGroup(x, y, "Prey", ins.lastInputs.Prey[:], nil)
		y += DrawBarGroup(x, y, "Pred", ins.lastInputs.Pred[:], nil)
		y += DrawBarGroup(x, y, "Food", ins.lastInputs.Resource[:], nil)
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

	networkHeight := int32(200)
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
	height += 22                           // ID line
	height += 8                            // separator
	height += 20                           // position
	height += 20                           // velocity
	height += 44                           // rotation (angle widget)
	height += 18                           // energy bar
	height += 20                           // age
	height += 20                           // radius
	height += 12                           // separator
	height += 20                           // sensors header
	height += 34 * 3                       // sensor bars
	height += 12                           // separator
	height += 20                           // network header
	height += 200                          // network diagram
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

	// Selection circle (fixed size, no pulse)
	radius := body.Radius * 1.8
	rl.DrawCircleLines(int32(pos.X), int32(pos.Y), radius, rl.Yellow)

	// Draw vision sectors if we have rotation, capabilities, and organism
	if rot != nil && caps != nil && org != nil {
		ins.drawVisionSectors(pos.X, pos.Y, rot.Heading, caps.VisionRange, org.Kind)
	}
}

// drawVisionSectors renders the full 360° vision field with effectiveness-based coloring.
// Brighter sectors indicate higher effectiveness for the entity's kind.
func (ins *Inspector) drawVisionSectors(x, y, heading, visionRange float32, kind components.Kind) {
	const numSectors = systems.NumSectors

	sectorWidth := float32(2 * math.Pi / numSectors)
	minEff := float32(config.Cfg().Capabilities.MinEffectiveness)

	// Draw each sector as a filled arc
	for i := 0; i < numSectors; i++ {
		// Sector angles (sector 2 = front at 0° relative to heading)
		relAngle := float32(i)*sectorWidth - math.Pi + sectorWidth/2
		startAngle := heading + relAngle - sectorWidth/2
		endAngle := startAngle + sectorWidth

		// Calculate effectiveness for this sector's center angle
		eff := visionEffectiveness(relAngle, kind, minEff)

		// Color based on effectiveness: brighter = more effective
		alpha := uint8(20 + eff*40) // 20-60 alpha range
		var color rl.Color
		if kind == components.KindPredator {
			// Predator: red-orange tones
			color = rl.Color{R: 255, G: uint8(150 + eff*100), B: 100, A: alpha}
		} else {
			// Prey: blue-cyan tones
			color = rl.Color{R: 100, G: uint8(150 + eff*100), B: 255, A: alpha}
		}

		// Draw sector as triangle fan
		drawSectorFilled(x, y, visionRange, startAngle, endAngle, color)

		// Draw sector edge lines
		edgeColor := rl.Color{R: 200, G: 200, B: 200, A: 40}
		drawSectorEdge(x, y, visionRange, startAngle, edgeColor)
	}

	// Draw outer circle
	drawArc(x, y, visionRange, 0, 2*math.Pi, rl.Color{R: 200, G: 200, B: 200, A: 50})
}

// visionEffectiveness calculates effectiveness for a given angle and kind.
// Uses configurable vision zones from config.
func visionEffectiveness(relAngle float32, kind components.Kind, minEff float32) float32 {
	cfg := config.Cfg().Capabilities

	// Get zones for this kind
	var zones []config.VisionZone
	if kind == components.KindPredator {
		zones = cfg.Predator.VisionZones
	} else {
		zones = cfg.Prey.VisionZones
	}

	// If no zones defined, return minimum effectiveness
	if len(zones) == 0 {
		return minEff
	}

	// Calculate max effectiveness across all zones
	maxEff := float32(0)
	for _, zone := range zones {
		// Angular distance from zone center (handle wraparound)
		angleDist := normalizeAngle(relAngle - float32(zone.Angle))
		absAngleDist := float32(math.Abs(float64(angleDist)))

		// Smooth falloff within zone width using cosine
		if absAngleDist < float32(zone.Width) {
			t := absAngleDist / float32(zone.Width) * (math.Pi / 2)
			zoneEff := float32(zone.Power) * float32(math.Cos(float64(t)))
			if zoneEff > maxEff {
				maxEff = zoneEff
			}
		}
	}

	// Combine with minimum effectiveness
	return minEff + (1-minEff)*maxEff
}

// normalizeAngle wraps angle to [-π, π].
func normalizeAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
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

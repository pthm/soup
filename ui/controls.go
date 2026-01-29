package ui

import (
	"fmt"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// ControlsPanel renders the left-side controls panel with overlay toggles.
type ControlsPanel struct {
	renderer *Renderer
	x, y     int32
	width    int32
	visible  bool
}

// NewControlsPanel creates a new controls panel.
func NewControlsPanel(x, y, width int32) *ControlsPanel {
	return &ControlsPanel{
		renderer: NewRenderer(),
		x:        x,
		y:        y,
		width:    width,
		visible:  false,
	}
}

// SetVisible shows or hides the panel.
func (c *ControlsPanel) SetVisible(visible bool) {
	c.visible = visible
}

// IsVisible returns whether the panel is shown.
func (c *ControlsPanel) IsVisible() bool {
	return c.visible
}

// Toggle switches panel visibility.
func (c *ControlsPanel) Toggle() bool {
	c.visible = !c.visible
	return c.visible
}

// Draw renders the controls panel.
func (c *ControlsPanel) Draw(overlays *OverlayRegistry) int32 {
	if !c.visible {
		return c.y
	}

	r := c.renderer
	padding := r.Theme.Padding
	lineHeight := r.Theme.LineHeight

	// Calculate panel height based on content
	categories := overlays.Categories()
	totalItems := 0
	for _, cat := range categories {
		totalItems += len(overlays.ByCategory(cat)) + 1 // +1 for category header
	}
	panelHeight := int32(totalItems)*lineHeight + padding*3 + lineHeight // Extra for title

	// Draw panel background
	r.DrawPanel(c.x, c.y, c.width, panelHeight)

	y := c.y + padding

	// Title
	rl.DrawText("Overlays", c.x+padding, y, 16, rl.White)
	y += lineHeight + 4

	// Draw overlays by category
	for _, category := range categories {
		// Category header
		catLabel := categoryLabel(category)
		rl.DrawText(catLabel, c.x+padding, y, r.Theme.HeaderFontSize, r.Theme.SectionHeader)
		y += lineHeight

		// Overlays in this category
		for _, desc := range overlays.ByCategory(category) {
			enabled := overlays.IsEnabled(desc.ID)
			c.drawToggle(c.x+padding, y, desc, enabled, c.width-padding*2)
			y += lineHeight
		}

		y += 4 // Gap between categories
	}

	return y
}

// drawToggle draws a single overlay toggle line.
func (c *ControlsPanel) drawToggle(x, y int32, desc OverlayDescriptor, enabled bool, width int32) {
	r := c.renderer

	// Status indicator
	statusColor := rl.Color{R: 80, G: 80, B: 80, A: 255}
	if enabled {
		statusColor = rl.Color{R: 100, G: 200, B: 100, A: 255}
	}
	rl.DrawRectangle(x, y+2, 8, 8, statusColor)

	// Name
	nameColor := r.Theme.LabelColor
	if enabled {
		nameColor = rl.White
	}
	rl.DrawText(desc.Name, x+14, y, r.Theme.FontSize, nameColor)

	// Key binding (right aligned)
	if desc.KeyLabel != "" {
		keyText := fmt.Sprintf("[%s]", desc.KeyLabel)
		keyWidth := rl.MeasureText(keyText, r.Theme.FontSize)
		rl.DrawText(keyText, x+width-keyWidth, y, r.Theme.FontSize, rl.Color{R: 150, G: 150, B: 150, A: 255})
	}
}

// categoryLabel returns a display label for a category.
func categoryLabel(cat string) string {
	switch cat {
	case "visual":
		return "Visual"
	case "perception":
		return "Perception"
	case "debug":
		return "Debug"
	default:
		return cat
	}
}

// QuickStatsData holds data for the quick stats section.
type QuickStatsData struct {
	AvgEnergy   float32
	AvgSize     float32
	DeathRate   float32
	BirthRate   float32
}

// QuickStatsPanel renders quick statistics.
type QuickStatsPanel struct {
	renderer *Renderer
	x, y     int32
	width    int32
}

// NewQuickStatsPanel creates a new quick stats panel.
func NewQuickStatsPanel(x, y, width int32) *QuickStatsPanel {
	return &QuickStatsPanel{
		renderer: NewRenderer(),
		x:        x,
		y:        y,
		width:    width,
	}
}

// Draw renders the quick stats panel.
func (q *QuickStatsPanel) Draw(data QuickStatsData) int32 {
	r := q.renderer
	padding := r.Theme.Padding
	lineHeight := r.Theme.LineHeight

	panelHeight := lineHeight*5 + padding*2

	// Draw panel background
	r.DrawPanel(q.x, q.y, q.width, panelHeight)

	y := q.y + padding

	// Title
	rl.DrawText("Quick Stats", q.x+padding, y, 14, rl.White)
	y += lineHeight + 2

	// Stats
	y = r.DrawLabelValue(q.x+padding, y, "Avg Energy", fmt.Sprintf("%.1f", data.AvgEnergy), q.width-padding*2)
	y = r.DrawLabelValue(q.x+padding, y, "Avg Size", fmt.Sprintf("%.1f", data.AvgSize), q.width-padding*2)
	y = r.DrawLabelValue(q.x+padding, y, "Deaths/s", fmt.Sprintf("%.2f", data.DeathRate), q.width-padding*2)

	return y
}

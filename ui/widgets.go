package ui

import (
	"fmt"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Renderer handles all UI drawing with consistent styling.
type Renderer struct {
	Theme Theme
}

// NewRenderer creates a renderer with the default theme.
func NewRenderer() *Renderer {
	return &Renderer{Theme: DefaultTheme()}
}

// DrawPanel draws a panel background with border.
func (r *Renderer) DrawPanel(x, y, width, height int32) {
	rl.DrawRectangle(x, y, width, height, r.Theme.PanelBg)
	rl.DrawRectangleLines(x, y, width, height, r.Theme.PanelBorder)
}

// DrawSectionHeader draws a section header and returns the new Y position.
func (r *Renderer) DrawSectionHeader(x, y int32, title string) int32 {
	rl.DrawText(title, x, y, r.Theme.HeaderFontSize, r.Theme.SectionHeader)
	return y + r.Theme.LineHeight
}

// DrawLabel draws a text label.
func (r *Renderer) DrawLabel(x, y int32, text string) {
	rl.DrawText(text, x, y, r.Theme.FontSize, r.Theme.LabelColor)
}

// DrawValue draws a value text.
func (r *Renderer) DrawValue(x, y int32, text string) {
	rl.DrawText(text, x, y, r.Theme.FontSize, r.Theme.ValueColor)
}

// DrawLabelValue draws a label and value on the same line.
func (r *Renderer) DrawLabelValue(x, y int32, label, value string, totalWidth int32) int32 {
	rl.DrawText(label+":", x, y, r.Theme.FontSize, r.Theme.LabelColor)
	rl.DrawText(value, x+r.Theme.LabelWidth, y, r.Theme.FontSize, r.Theme.ValueColor)
	return y + r.Theme.LineHeight
}

// DrawBar draws a progress bar for [0, 1] values.
func (r *Renderer) DrawBar(x, y int32, label string, value float32, width int32) int32 {
	// Clamp value
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}

	barX := x + r.Theme.LabelWidth
	barWidth := width - r.Theme.LabelWidth - 50

	// Label
	rl.DrawText(label+":", x, y, r.Theme.FontSize, r.Theme.LabelColor)

	// Background
	rl.DrawRectangle(barX, y+2, barWidth, r.Theme.BarHeight, r.Theme.BarBg)

	// Fill
	fillWidth := int32(float32(barWidth) * value)
	rl.DrawRectangle(barX, y+2, fillWidth, r.Theme.BarHeight, r.Theme.BarFill)

	// Value text
	rl.DrawText(fmt.Sprintf("%.2f", value), barX+barWidth+5, y, r.Theme.FontSize, r.Theme.ValueColor)

	return y + r.Theme.LineHeight + 2
}

// DrawEnergyBar draws an energy bar with color thresholds.
func (r *Renderer) DrawEnergyBar(x, y int32, label string, current, max float32, width int32) int32 {
	ratio := float32(0)
	if max > 0 {
		ratio = current / max
		if ratio > 1 {
			ratio = 1
		}
	}

	barX := x + r.Theme.LabelWidth
	barWidth := width - r.Theme.LabelWidth - 80

	// Label
	rl.DrawText(label+":", x, y, r.Theme.FontSize, r.Theme.LabelColor)

	// Background
	rl.DrawRectangle(barX, y+2, barWidth, r.Theme.BarHeight, r.Theme.BarBg)

	// Choose color based on ratio
	barColor := r.Theme.BarFillHigh
	if ratio < 0.3 {
		barColor = r.Theme.BarFillLow
	} else if ratio < 0.6 {
		barColor = r.Theme.BarFillMedium
	}

	// Fill
	fillWidth := int32(float32(barWidth) * ratio)
	rl.DrawRectangle(barX, y+2, fillWidth, r.Theme.BarHeight, barColor)

	// Value text
	rl.DrawText(fmt.Sprintf("%.0f/%.0f", current, max), barX+barWidth+5, y, r.Theme.FontSize, r.Theme.ValueColor)

	return y + r.Theme.LineHeight + 2
}

// DrawCenteredBar draws a bar centered at 0 for values in a range (e.g., -1 to +1).
func (r *Renderer) DrawCenteredBar(x, y int32, label string, value, minVal, maxVal float32, width int32) int32 {
	barX := x + r.Theme.LabelWidth
	barWidth := width - r.Theme.LabelWidth - 50

	// Label
	rl.DrawText(label+":", x, y, r.Theme.FontSize, r.Theme.LabelColor)

	// Background
	rl.DrawRectangle(barX, y+2, barWidth, r.Theme.BarHeight, r.Theme.BarBg)

	// Center line
	centerX := barX + barWidth/2
	rl.DrawLine(centerX, y+2, centerX, y+2+r.Theme.BarHeight, rl.Color{R: 80, G: 80, B: 80, A: 255})

	// Normalize and clamp
	normalized := (value - minVal) / (maxVal - minVal)
	if normalized < 0 {
		normalized = 0
	}
	if normalized > 1 {
		normalized = 1
	}

	// Draw fill from center
	fillX := centerX
	fillWidth := int32(float32(barWidth/2) * float32(math.Abs(float64(value)/float64(maxVal))))

	barColor := r.Theme.BarFillPositive
	if value < 0 {
		fillX = centerX - fillWidth
		barColor = r.Theme.BarFillNegative
	}
	rl.DrawRectangle(fillX, y+2, fillWidth, r.Theme.BarHeight, barColor)

	// Value text
	rl.DrawText(fmt.Sprintf("%+.2f", value), barX+barWidth+5, y, r.Theme.FontSize, r.Theme.ValueColor)

	return y + r.Theme.LineHeight + 2
}

// DrawColorSwatch draws a color swatch.
func (r *Renderer) DrawColorSwatch(x, y int32, label string, color rl.Color, width int32) int32 {
	swatchSize := int32(12)

	// Label
	rl.DrawText(label+":", x, y, r.Theme.FontSize, r.Theme.LabelColor)

	// Swatch
	rl.DrawRectangle(x+r.Theme.LabelWidth, y+1, swatchSize, swatchSize, color)

	return y + r.Theme.LineHeight
}

// DrawSpacer adds vertical space and returns new Y.
func (r *Renderer) DrawSpacer(y int32, amount int32) int32 {
	return y + amount
}

// DrawField renders a field based on its descriptor.
func (r *Renderer) DrawField(x, y int32, fd FieldDescriptor, data any, width int32) int32 {
	switch fd.Widget {
	case WidgetText:
		var text string
		if fd.TextGetter != nil {
			text = fd.TextGetter(data)
		} else if fd.Getter != nil {
			text = fmt.Sprintf(fd.Format, fd.Getter(data))
		}
		return r.DrawLabelValue(x, y, fd.Label, text, width)

	case WidgetBar:
		value := float32(0)
		if fd.Getter != nil {
			value = fd.Getter(data)
		}
		return r.DrawBar(x, y, fd.Label, value, width)

	case WidgetCenteredBar:
		value := float32(0)
		if fd.Getter != nil {
			value = fd.Getter(data)
		}
		return r.DrawCenteredBar(x, y, fd.Label, value, fd.Range.Min, fd.Range.Max, width)

	case WidgetColorSwatch:
		color := fd.Color
		if fd.ColorGetter != nil {
			color = fd.ColorGetter(data)
		}
		return r.DrawColorSwatch(x, y, fd.Label, color, width)

	case WidgetEnergyBar:
		// Special case: needs current and max from data
		// This would need custom handling
		return y + r.Theme.LineHeight

	case WidgetSection:
		return r.DrawSectionHeader(x, y, fd.Label)

	case WidgetSpacer:
		return r.DrawSpacer(y, 6)
	}

	return y
}

// DrawSection renders a section with header and fields.
func (r *Renderer) DrawSection(x, y int32, sd SectionDescriptor, data any, width int32) int32 {
	// Check section visibility
	if sd.Visible != nil && !sd.Visible(data) {
		return y
	}

	// Header
	if sd.Title != "" {
		y = r.DrawSectionHeader(x, y, sd.Title)
	}

	// Fields
	for _, fd := range sd.Fields {
		// Check field visibility
		if fd.Visible != nil && !fd.Visible(data) {
			continue
		}
		y = r.DrawField(x, y, fd, data, width)
	}

	return y + 4 // Small gap after section
}

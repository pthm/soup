package inspector

import (
	"fmt"
	"math"
	"strings"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Widget colors
var (
	ColorBarBg     = rl.Color{R: 40, G: 40, B: 40, A: 255}
	ColorBarFill   = rl.Color{R: 100, G: 180, B: 100, A: 255}
	ColorBarLow    = rl.Color{R: 180, G: 80, B: 80, A: 255}
	ColorText      = rl.Color{R: 220, G: 220, B: 220, A: 255}
	ColorTextDim   = rl.Color{R: 150, G: 150, B: 150, A: 255}
	ColorAngleBg   = rl.Color{R: 50, G: 50, B: 60, A: 255}
	ColorAngleNeedle = rl.Color{R: 255, G: 200, B: 100, A: 255}
	ColorBoolOn    = rl.Color{R: 100, G: 200, B: 100, A: 255}
	ColorBoolOff   = rl.Color{R: 80, G: 80, B: 80, A: 255}
)

// DrawLabel renders a text value.
func DrawLabel(x, y int32, name string, value interface{}, options map[string]string) int32 {
	fmtStr := options["fmt"]
	text := FormatValue(value, fmtStr)
	rl.DrawText(fmt.Sprintf("%s: %s", name, text), x, y, 16, ColorText)
	return 20
}

// DrawBar renders a horizontal progress bar.
func DrawBar(x, y int32, name string, value float32, options map[string]string) int32 {
	maxVal := GetMax(options)
	ratio := value / maxVal
	if ratio > 1 {
		ratio = 1
	}
	if ratio < 0 {
		ratio = 0
	}

	barWidth := int32(120)
	barHeight := int32(14)

	// Label
	rl.DrawText(name, x, y, 14, ColorTextDim)

	// Bar background
	barX := x + 80
	rl.DrawRectangle(barX, y, barWidth, barHeight, ColorBarBg)

	// Bar fill
	fillWidth := int32(float32(barWidth) * ratio)
	fillColor := ColorBarFill
	if ratio < 0.3 {
		fillColor = ColorBarLow
	}
	rl.DrawRectangle(barX, y, fillWidth, barHeight, fillColor)

	// Value text
	valueStr := fmt.Sprintf("%.2f", value)
	rl.DrawText(valueStr, barX+barWidth+5, y, 14, ColorTextDim)

	return 18
}

// DrawBarGroup renders multiple mini-bars for array data (like sensor sectors).
func DrawBarGroup(x, y int32, name string, values []float32, options map[string]string) int32 {
	maxVal := GetMax(options)
	barWidth := int32(20)
	barHeight := int32(30)
	gap := int32(2)
	labelHeight := int32(0)
	labels := parseLabels(options, len(values))
	if labels != nil {
		labelHeight = 10
	}

	// Label
	rl.DrawText(name, x, y, 14, ColorTextDim)

	// Bars
	barX := x + 60
	for i, v := range values {
		ratio := v / maxVal
		if ratio > 1 {
			ratio = 1
		}
		if ratio < 0 {
			ratio = 0
		}

		// Background
		rl.DrawRectangle(barX+int32(i)*(barWidth+gap), y, barWidth, barHeight, ColorBarBg)

		// Fill from bottom
		fillHeight := int32(float32(barHeight) * ratio)
		fillY := y + barHeight - fillHeight
		fillColor := lerpColor(ColorBarLow, ColorBarFill, ratio)
		rl.DrawRectangle(barX+int32(i)*(barWidth+gap), fillY, barWidth, fillHeight, fillColor)
	}

	if labels != nil {
		labelY := y + barHeight + 2
		for i, label := range labels {
			if label == "" {
				continue
			}
			lx := barX + int32(i)*(barWidth+gap) + barWidth/2
			textW := int32(rl.MeasureText(label, 8))
			rl.DrawText(label, lx-textW/2, labelY, 8, ColorTextDim)
		}
	}

	return barHeight + labelHeight + 4
}

// DrawAngle renders a compass-style angle indicator.
func DrawAngle(x, y int32, name string, radians float32, options map[string]string) int32 {
	size := int32(40)
	centerX := x + 60 + size/2
	centerY := y + size/2

	// Label
	rl.DrawText(name, x, y+size/2-7, 14, ColorTextDim)

	// Circle background
	rl.DrawCircle(centerX, centerY, float32(size/2), ColorAngleBg)
	rl.DrawCircleLines(centerX, centerY, float32(size/2), ColorTextDim)

	// Needle
	needleLen := float32(size/2 - 4)
	endX := float32(centerX) + needleLen*float32(math.Cos(float64(radians)))
	endY := float32(centerY) + needleLen*float32(math.Sin(float64(radians)))
	rl.DrawLineEx(
		rl.Vector2{X: float32(centerX), Y: float32(centerY)},
		rl.Vector2{X: endX, Y: endY},
		2,
		ColorAngleNeedle,
	)

	// Degree text
	degrees := radians * 180 / math.Pi
	rl.DrawText(fmt.Sprintf("%.0f°", degrees), x+60+size+5, y+size/2-7, 14, ColorTextDim)

	return size + 4
}

// DrawBool renders an on/off indicator.
func DrawBool(x, y int32, name string, value bool) int32 {
	// Label
	rl.DrawText(name, x, y, 14, ColorTextDim)

	// Indicator
	indicatorX := x + 80
	indicatorSize := int32(14)

	color := ColorBoolOff
	text := "OFF"
	if value {
		color = ColorBoolOn
		text = "ON"
	}

	rl.DrawRectangle(indicatorX, y, indicatorSize, indicatorSize, color)
	rl.DrawText(text, indicatorX+indicatorSize+5, y, 14, color)

	return 18
}

// DrawField renders a field using its widget type.
func DrawField(x, y int32, field Field) int32 {
	switch field.Widget {
	case WidgetBar:
		// Check if it's an array
		if values, ok := GetFloatSlice(field.Value); ok {
			return DrawBarGroup(x, y, field.Name, values, field.Options)
		}
		// Single value bar
		if v, ok := GetFloatValue(field.Value); ok {
			return DrawBar(x, y, field.Name, v, field.Options)
		}
		return DrawLabel(x, y, field.Name, field.Value, field.Options)

	case WidgetAngle:
		if v, ok := GetFloatValue(field.Value); ok {
			return DrawAngle(x, y, field.Name, v, field.Options)
		}
		return DrawLabel(x, y, field.Name, field.Value, field.Options)

	case WidgetBool:
		if v, ok := field.Value.(bool); ok {
			return DrawBool(x, y, field.Name, v)
		}
		return DrawLabel(x, y, field.Name, field.Value, field.Options)

	default:
		return DrawLabel(x, y, field.Name, field.Value, field.Options)
	}
}

func parseLabels(options map[string]string, count int) []string {
	if options == nil {
		return nil
	}
	raw, ok := options["labels"]
	if !ok || raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	if len(parts) != count {
		return nil
	}
	for i := 0; i < len(parts); i++ {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// lerpColor interpolates between two colors.
func lerpColor(a, b rl.Color, t float32) rl.Color {
	return rl.Color{
		R: uint8(float32(a.R) + (float32(b.R)-float32(a.R))*t),
		G: uint8(float32(a.G) + (float32(b.G)-float32(a.G))*t),
		B: uint8(float32(a.B) + (float32(b.B)-float32(a.B))*t),
		A: 255,
	}
}

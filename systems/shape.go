package systems

import "github.com/pthm-cable/soup/components"

// CalculateShapeMetrics computes physical shape characteristics from an organism's cells.
// These metrics influence drag, flow resistance, and movement efficiency.
func CalculateShapeMetrics(cells *components.CellBuffer) components.ShapeMetrics {
	if cells.Count == 0 {
		return components.ShapeMetrics{
			AspectRatio:     1.0,
			CrossSection:    1.0,
			Streamlining:    0.0,
			DragCoefficient: 1.0,
		}
	}

	// Get bounding box from cells
	minX, minY := int8(127), int8(127)
	maxX, maxY := int8(-128), int8(-128)

	for i := uint8(0); i < cells.Count; i++ {
		c := &cells.Cells[i]
		if c.GridX < minX {
			minX = c.GridX
		}
		if c.GridX > maxX {
			maxX = c.GridX
		}
		if c.GridY < minY {
			minY = c.GridY
		}
		if c.GridY > maxY {
			maxY = c.GridY
		}
	}

	width := float32(maxX - minX + 1)
	height := float32(maxY - minY + 1)

	// Aspect ratio (Y is forward direction)
	aspectRatio := height / width
	if width > height {
		aspectRatio = 1.0 / aspectRatio // Invert if wider than tall
	}

	// Density (how filled the bounding box is)
	area := width * height
	density := float32(cells.Count) / area

	// Streamlining: high aspect + moderate density = streamlined
	streamlining := clampFloat((aspectRatio-1.0)/3.0, 0, 1) * (1 - density*0.3)

	// Drag coefficient: 0.3 (fish) to 1.0 (flat plate)
	dragCoeff := 1.0 - streamlining*0.7

	return components.ShapeMetrics{
		AspectRatio:     aspectRatio,
		CrossSection:    width,
		Streamlining:    streamlining,
		DragCoefficient: dragCoeff,
	}
}

// clampFloat clamps a value between min and max.
func clampFloat(v, minVal, maxVal float32) float32 {
	if v < minVal {
		return minVal
	}
	if v > maxVal {
		return maxVal
	}
	return v
}

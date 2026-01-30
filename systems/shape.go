package systems

import "github.com/pthm-cable/soup/components"

// cellBounds represents the bounding box of cells in grid coordinates.
type cellBounds struct {
	minX, minY, maxX, maxY int8
	valid                  bool
}

// getCellBounds computes the bounding box of all alive cells.
// If onlyAlive is true, only alive cells are considered.
func getCellBounds(cells *components.CellBuffer, onlyAlive bool) cellBounds {
	if cells.Count == 0 {
		return cellBounds{valid: false}
	}

	minX, minY := int8(127), int8(127)
	maxX, maxY := int8(-128), int8(-128)
	found := false

	for i := uint8(0); i < cells.Count; i++ {
		c := &cells.Cells[i]
		if onlyAlive && !c.Alive {
			continue
		}
		found = true
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

	return cellBounds{minX: minX, minY: minY, maxX: maxX, maxY: maxY, valid: found}
}

// CalculateShapeMetrics computes physical shape characteristics from an organism's cells.
// These metrics influence drag, flow resistance, and movement efficiency.
func CalculateShapeMetrics(cells *components.CellBuffer) components.ShapeMetrics {
	bounds := getCellBounds(cells, false)
	if !bounds.valid {
		return components.ShapeMetrics{
			AspectRatio:     1.0,
			CrossSection:    1.0,
			Streamlining:    0.0,
			DragCoefficient: 1.0,
		}
	}

	width := float32(bounds.maxX - bounds.minX + 1)
	height := float32(bounds.maxY - bounds.minY + 1)

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

// ComputeCollisionOBB computes an oriented bounding box from an organism's cells.
// The OBB is aligned to the organism's local coordinate system and rotates with heading.
func ComputeCollisionOBB(cells *components.CellBuffer, cellSize float32) components.CollisionOBB {
	defaultOBB := components.CollisionOBB{
		HalfWidth:  cellSize,
		HalfHeight: cellSize,
		OffsetX:    0,
		OffsetY:    0,
	}

	bounds := getCellBounds(cells, true) // Only alive cells for collision
	if !bounds.valid {
		return defaultOBB
	}

	// Convert grid bounds to world coordinates
	// Grid coordinates are centered at (0,0), so a cell at (0,0) is at world origin
	// Each cell occupies cellSize world units

	// Width spans from minX to maxX, plus cellSize for the cell itself, plus padding
	width := float32(bounds.maxX-bounds.minX+1)*cellSize + cellSize // +1 cell padding
	height := float32(bounds.maxY-bounds.minY+1)*cellSize + cellSize

	// Center offset: average of min/max in each dimension
	centerX := float32(bounds.minX+bounds.maxX) / 2.0 * cellSize
	centerY := float32(bounds.minY+bounds.maxY) / 2.0 * cellSize

	return components.CollisionOBB{
		HalfWidth:  width / 2,
		HalfHeight: height / 2,
		OffsetX:    centerX,
		OffsetY:    centerY,
	}
}

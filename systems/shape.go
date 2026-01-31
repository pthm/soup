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

// CalculateShapeMetrics computes drag from the organism's actual frontal profile.
// Y+ is the forward direction (heading). Drag is based on:
// - Frontal area: cells at the leading edge (highest Y)
// - Body length: extent in Y direction
// - Taper: whether front is narrower than the widest point
//
// A fish shape (narrow front, long body) has low drag ~0.2-0.4
// A flat plate (wide front, short body) has high drag ~1.0+
func CalculateShapeMetrics(cells *components.CellBuffer) components.ShapeMetrics {
	if cells.Count == 0 {
		return components.ShapeMetrics{Drag: 1.0}
	}

	bounds := getCellBounds(cells, false)
	if !bounds.valid {
		return components.ShapeMetrics{Drag: 1.0}
	}

	// Count cells at each Y level to analyze frontal profile
	// Y+ is forward, so maxY is the "front" (leading edge)
	yProfile := make(map[int8]int)
	for i := uint8(0); i < cells.Count; i++ {
		c := &cells.Cells[i]
		yProfile[c.GridY]++
	}

	// Find frontal width (cells at leading edge) and max width
	frontWidth := yProfile[bounds.maxY]
	maxWidth := 0
	for _, count := range yProfile {
		if count > maxWidth {
			maxWidth = count
		}
	}

	// Body length in Y direction
	length := float32(bounds.maxY - bounds.minY + 1)

	// Base drag: frontal area / length
	// Fish: front=1, length=6 -> 1/6 = 0.17
	// Plate: front=10, length=2 -> 10/2 = 5.0
	baseDrag := float32(frontWidth) / length

	// Taper bonus: if front is narrower than widest point, reduce drag
	// taper=1.0 means no taper (front is as wide as body)
	// taper=0.5 means front is half as wide as widest point
	taper := float32(1.0)
	if maxWidth > 0 {
		taper = float32(frontWidth) / float32(maxWidth)
	}

	// Final drag: base * taper factor
	// Good taper (0.3) reduces drag significantly
	// No taper (1.0) keeps full drag
	drag := baseDrag * (0.3 + taper*0.7)

	// Clamp to reasonable range
	// 0.2 = very streamlined (long thin fish)
	// 1.0 = neutral (square-ish)
	// 2.0+ = very draggy (wide flat plate)
	return components.ShapeMetrics{
		Drag: clampFloat(drag, 0.2, 3.0),
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

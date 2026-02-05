// Package systems provides ECS systems for the simulation.
package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Neighbor holds a nearby entity with precomputed spatial data.
// This avoids recomputing toroidal delta and distance in sensors.
type Neighbor struct {
	E      ecs.Entity
	DX, DY float32 // Toroidal delta from query origin
	DistSq float32 // Squared distance (avoid sqrt in hot path)
}

// SpatialGrid provides O(1) neighbor lookups using a cell-based grid.
type SpatialGrid struct {
	cellSize float32
	cols     int
	rows     int
	width    float32
	height   float32
	cells    [][]ecs.Entity // flat grid of entity lists
}

// NewSpatialGrid creates a spatial grid covering the given world size.
func NewSpatialGrid(width, height, cellSize float32) *SpatialGrid {
	cols := int(width/cellSize) + 1
	rows := int(height/cellSize) + 1

	cells := make([][]ecs.Entity, cols*rows)
	for i := range cells {
		cells[i] = make([]ecs.Entity, 0, 8) // pre-allocate small capacity
	}

	return &SpatialGrid{
		cellSize: cellSize,
		cols:     cols,
		rows:     rows,
		width:    width,
		height:   height,
		cells:    cells,
	}
}

// Clear removes all entities from the grid.
func (g *SpatialGrid) Clear() {
	for i := range g.cells {
		g.cells[i] = g.cells[i][:0]
	}
}

// Insert adds an entity to the grid at the given position.
func (g *SpatialGrid) Insert(e ecs.Entity, x, y float32) {
	idx := g.cellIndex(x, y)
	if idx >= 0 && idx < len(g.cells) {
		g.cells[idx] = append(g.cells[idx], e)
	}
}

// MaxQueryResults caps the number of neighbors returned by spatial queries.
// This prevents density spikes from causing unbounded work.
const MaxQueryResults = 128

// QueryRadiusInto finds entities within radius and appends to dst (up to MaxQueryResults).
// Returns the updated slice. Reuse dst across calls to avoid allocations.
// Each Neighbor includes precomputed DX, DY, DistSq for efficient sensor processing.
func (g *SpatialGrid) QueryRadiusInto(dst []Neighbor, x, y, radius float32, exclude ecs.Entity, posMap *ecs.Map1[components.Position]) []Neighbor {
	// Determine cell range to check
	cellRadius := int(radius/g.cellSize) + 1

	centerCol := int(x / g.cellSize)
	centerRow := int(y / g.cellSize)

	radiusSq := radius * radius

	for dc := -cellRadius; dc <= cellRadius; dc++ {
		for dr := -cellRadius; dr <= cellRadius; dr++ {
			// Toroidal wrap
			col := (centerCol + dc + g.cols) % g.cols
			row := (centerRow + dr + g.rows) % g.rows
			idx := row*g.cols + col

			for _, e := range g.cells[idx] {
				if e == exclude {
					continue
				}

				// Get position and compute delta with toroidal wrapping
				pos := posMap.Get(e)
				if pos == nil {
					continue
				}

				dx, dy := ToroidalDelta(x, y, pos.X, pos.Y, g.width, g.height)
				distSq := dx*dx + dy*dy

				if distSq <= radiusSq {
					dst = append(dst, Neighbor{E: e, DX: dx, DY: dy, DistSq: distSq})
					// Early exit if we hit the cap
					if len(dst) >= MaxQueryResults {
						return dst
					}
				}
			}
		}
	}

	return dst
}

// QueryRadius returns all entities within radius of the given position.
// Deprecated: Use QueryRadiusInto to avoid allocations.
func (g *SpatialGrid) QueryRadius(x, y, radius float32, exclude ecs.Entity, posMap *ecs.Map1[components.Position]) []ecs.Entity {
	neighbors := g.QueryRadiusInto(nil, x, y, radius, exclude, posMap)
	result := make([]ecs.Entity, len(neighbors))
	for i, n := range neighbors {
		result[i] = n.E
	}
	return result
}

// cellIndex returns the flat index for a world position.
func (g *SpatialGrid) cellIndex(x, y float32) int {
	col := int(x / g.cellSize)
	row := int(y / g.cellSize)

	// Clamp to valid range
	if col < 0 {
		col = 0
	} else if col >= g.cols {
		col = g.cols - 1
	}
	if row < 0 {
		row = 0
	} else if row >= g.rows {
		row = g.rows - 1
	}

	return row*g.cols + col
}

// ToroidalDelta returns the shortest path delta from (x1,y1) to (x2,y2).
func ToroidalDelta(x1, y1, x2, y2, w, h float32) (dx, dy float32) {
	dx = x2 - x1
	dy = y2 - y1

	if dx > w/2 {
		dx -= w
	} else if dx < -w/2 {
		dx += w
	}
	if dy > h/2 {
		dy -= h
	} else if dy < -h/2 {
		dy += h
	}

	return dx, dy
}

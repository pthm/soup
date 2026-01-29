package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// CellSystem handles cell aging and decomposition.
type CellSystem struct {
	filter ecs.Filter2[components.Organism, components.CellBuffer]
}

// NewCellSystem creates a new cell system.
func NewCellSystem(w *ecs.World) *CellSystem {
	return &CellSystem{
		filter: *ecs.NewFilter2[components.Organism, components.CellBuffer](w),
	}
}

// Update runs the cell system.
func (s *CellSystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		org, cells := query.Get()

		// Process cells in reverse to allow safe removal
		cellsRemoved := false
		for i := int(cells.Count) - 1; i >= 0; i-- {
			cell := &cells.Cells[i]
			cell.Age++

			// Start decomposing when old
			if cell.Age > cell.MaxAge {
				cell.Decomposition += 0.002
				if cell.Decomposition >= 1 {
					cell.Alive = false
				}
			}

			// Remove dead cells
			if !cell.Alive {
				cells.RemoveCell(uint8(i))
				cellsRemoved = true
				continue
			}
		}

		// Recalculate shape metrics if cells were removed
		if cellsRemoved {
			org.ShapeMetrics = CalculateShapeMetrics(cells)
		}
	}
}

package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
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

		// Track traits that need updating
		var activeTraits traits.Trait

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

			// Disease speeds up decomposition
			if cell.Mutation == traits.Disease {
				cell.Decomposition += 0.0005
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

			// Accumulate active traits
			if cell.Trait != 0 {
				activeTraits = activeTraits.Add(cell.Trait)
			}
		}

		// Recalculate shape metrics if cells were removed
		if cellsRemoved {
			org.ShapeMetrics = CalculateShapeMetrics(cells)
		}

		// Preserve core traits that aren't cell-based
		if org.Traits.Has(traits.Flora) {
			activeTraits = activeTraits.Add(traits.Flora)
		}
		if org.Traits.Has(traits.Herbivore) {
			activeTraits = activeTraits.Add(traits.Herbivore)
		}
		if org.Traits.Has(traits.Carnivore) {
			activeTraits = activeTraits.Add(traits.Carnivore)
		}
		if org.Traits.Has(traits.Carrion) {
			activeTraits = activeTraits.Add(traits.Carrion)
		}
		if org.Traits.Has(traits.Rooted) {
			activeTraits = activeTraits.Add(traits.Rooted)
		}
		if org.Traits.Has(traits.Floating) {
			activeTraits = activeTraits.Add(traits.Floating)
		}
		if org.Traits.Has(traits.Male) {
			activeTraits = activeTraits.Add(traits.Male)
		}
		if org.Traits.Has(traits.Female) {
			activeTraits = activeTraits.Add(traits.Female)
		}

		org.Traits = activeTraits
	}
}

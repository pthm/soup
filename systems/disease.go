package systems

import (
	"math"
	"math/rand"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

const (
	diseaseSpreadRadius      = 8.0    // Distance for disease transmission
	diseaseSpreadProb        = 0.001  // Base probability per tick per nearby infected
	sameSpeciesMultiplier    = 5.0    // Spread rate multiplier within same species
	differentSpeciesMultiplier = 0.1  // Cross-species spread is rare
	spontaneousDiseaseProb   = 0.00005 // Rare spontaneous disease
)

// DiseaseSystem handles disease spread between organisms.
type DiseaseSystem struct {
	// Reusable buffers to avoid allocations
	infectedIndices []int
	healthyIndices  []int
}

// NewDiseaseSystem creates a new disease system.
func NewDiseaseSystem() *DiseaseSystem {
	return &DiseaseSystem{
		infectedIndices: make([]int, 0, 64),
		healthyIndices:  make([]int, 0, 256),
	}
}

// Update processes disease spread for all organisms.
func (s *DiseaseSystem) Update(
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	faunaCells []*components.CellBuffer,
	faunaGenomes []*components.NeuralGenome,
	spatialGrid *SpatialGrid,
) {
	if len(faunaPos) == 0 {
		return
	}

	// Reset buffers
	s.infectedIndices = s.infectedIndices[:0]
	s.healthyIndices = s.healthyIndices[:0]

	// Categorize organisms by infection status
	for i := range faunaPos {
		if faunaOrgs[i].Dead {
			continue
		}

		if hasDisease(faunaCells[i]) {
			s.infectedIndices = append(s.infectedIndices, i)
		} else {
			s.healthyIndices = append(s.healthyIndices, i)
		}
	}

	// Process disease spread from infected to healthy
	for _, healthyIdx := range s.healthyIndices {
		pos := faunaPos[healthyIdx]

		// Get nearby fauna using spatial grid
		nearby := spatialGrid.GetNearbyFauna(pos.X, pos.Y, diseaseSpreadRadius)

		for _, nearIdx := range nearby {
			// Skip self
			if nearIdx == healthyIdx {
				continue
			}

			// Skip if not infected
			if !hasDisease(faunaCells[nearIdx]) {
				continue
			}

			// Skip dead
			if faunaOrgs[nearIdx].Dead {
				continue
			}

			// Calculate actual distance
			dx := pos.X - faunaPos[nearIdx].X
			dy := pos.Y - faunaPos[nearIdx].Y
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

			if dist > diseaseSpreadRadius {
				continue
			}

			// Calculate spread probability based on species relationship
			spreadProb := diseaseSpreadProb

			// Same species = much higher transmission (genetic similarity)
			// Different species = very low transmission (genetic barrier)
			if faunaGenomes != nil &&
			   faunaGenomes[healthyIdx] != nil &&
			   faunaGenomes[nearIdx] != nil {
				if faunaGenomes[healthyIdx].SpeciesID == faunaGenomes[nearIdx].SpeciesID {
					spreadProb *= sameSpeciesMultiplier
				} else {
					spreadProb *= differentSpeciesMultiplier
				}
			}

			// Distance falloff - closer = more likely
			distFactor := 1.0 - (float64(dist) / diseaseSpreadRadius)
			spreadProb *= distFactor

			// Roll for infection
			if rand.Float64() < spreadProb {
				infectOrganism(faunaCells[healthyIdx])
				break // Only get infected once per tick
			}
		}
	}

	// Spontaneous disease (rare, provides initial infections)
	for _, healthyIdx := range s.healthyIndices {
		if rand.Float64() < spontaneousDiseaseProb {
			infectOrganism(faunaCells[healthyIdx])
		}
	}
}

// hasDisease checks if any cell in the organism has disease.
func hasDisease(cells *components.CellBuffer) bool {
	for i := uint8(0); i < cells.Count; i++ {
		if cells.Cells[i].Mutation == traits.Disease && cells.Cells[i].Alive {
			return true
		}
	}
	return false
}

// infectOrganism gives disease to a random healthy cell.
func infectOrganism(cells *components.CellBuffer) {
	// Find healthy cells without disease
	healthyCells := make([]uint8, 0, cells.Count)
	for i := uint8(0); i < cells.Count; i++ {
		if cells.Cells[i].Alive && cells.Cells[i].Mutation == traits.NoMutation {
			healthyCells = append(healthyCells, i)
		}
	}

	if len(healthyCells) == 0 {
		return
	}

	// Infect a random healthy cell
	targetIdx := healthyCells[rand.Intn(len(healthyCells))]
	cells.Cells[targetIdx].Mutation = traits.Disease
}

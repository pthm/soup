package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// PhotosynthesisSystem handles flora energy based on light intensity.
type PhotosynthesisSystem struct {
	filter    ecs.Filter4[components.Position, components.Organism, components.CellBuffer, components.Flora]
	shadowMap *ShadowMap
}

// NewPhotosynthesisSystem creates a new photosynthesis system.
func NewPhotosynthesisSystem(w *ecs.World, shadowMap *ShadowMap) *PhotosynthesisSystem {
	return &PhotosynthesisSystem{
		filter:    *ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Flora](w),
		shadowMap: shadowMap,
	}
}

// Update processes photosynthesis for all flora organisms.
func (s *PhotosynthesisSystem) Update() {
	query := s.filter.Query()
	for query.Next() {
		pos, org, cells, _ := query.Get()

		if org.Dead {
			continue
		}

		// Sample ambient light at organism position
		// Phase 5b note: Only uses ambient light from shadowMap, not bioluminescence.
		// This ensures self-emitted light doesn't contribute to photosynthesis (no energy loops).
		light := s.shadowMap.SampleLight(pos.X, pos.Y)

		// Rooted flora is slightly adapted to lower light but not immune to shade
		if org.Traits.Has(traits.Rooted) && light < 0.3 {
			light = 0.3 // Minimum 30% light for rooted flora
		}

		// Check for disease - reduces effective max energy
		hasDisease := false
		for i := uint8(0); i < cells.Count; i++ {
			if cells.Cells[i].Mutation == traits.Disease {
				hasDisease = true
				break
			}
		}

		// Energy gain based on light (direct gain, not target-seeking)
		// Base rate: 0.3/tick in full light, scaled by light level
		baseGain := float32(0.3) * light

		// Disease reduces photosynthesis efficiency
		if hasDisease {
			baseGain *= 0.5
		}

		// More cells = more photosynthesis, but with diminishing returns (sqrt scaling)
		// 1 cell = 1.0x, 4 cells = 1.5x, 9 cells = 2.0x, 16 cells = 2.5x
		cellMultiplier := float32(1.0) + float32(cells.Count-1)*0.05
		if cellMultiplier > 2.5 {
			cellMultiplier = 2.5 // Cap at 2.5x
		}
		baseGain *= cellMultiplier

		org.Energy += baseGain

		// Death if energy drops below 10% of max
		if org.Energy < org.MaxEnergy*0.10 {
			org.Dead = true
		}

		// Clamp to max
		if org.Energy > org.MaxEnergy {
			org.Energy = org.MaxEnergy
		}
	}
}

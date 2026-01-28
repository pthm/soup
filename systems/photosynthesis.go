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

		// Sample light at organism position
		light := s.shadowMap.SampleLight(pos.X, pos.Y)

		// Rooted flora is adapted to lower light (anchored, can't move to light)
		// They get a minimum light floor and bonus in shadows
		if org.Traits.Has(traits.Rooted) {
			// Rooted flora has adapted to partial shade
			if light < 0.5 {
				light = 0.5 + (light * 0.5) // Minimum 0.5, scaled bonus up to 0.75
			}
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
		// Base rate: 0.5/tick in full light, scaled by light level
		baseGain := float32(0.5) * light

		// Disease reduces photosynthesis efficiency
		if hasDisease {
			baseGain *= 0.5
		}

		// More cells = more photosynthesis (stronger scaling)
		baseGain *= (1 + float32(cells.Count)*0.15)

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

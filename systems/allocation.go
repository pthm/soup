package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// AllocationSystem evaluates each organism's state and sets energy allocation mode.
type AllocationSystem struct {
	filter      ecs.Filter4[components.Position, components.Organism, components.CellBuffer, components.Velocity]
	floraSystem *FloraSystem
}

// NewAllocationSystem creates a new allocation system.
func NewAllocationSystem(w *ecs.World) *AllocationSystem {
	return &AllocationSystem{
		filter: *ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Velocity](w),
	}
}

// SetFloraSystem sets the flora system reference for food queries.
func (s *AllocationSystem) SetFloraSystem(fs *FloraSystem) {
	s.floraSystem = fs
}

// Update evaluates allocation mode for all organisms.
func (s *AllocationSystem) Update(
	floraPositions, faunaPositions []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
) {
	query := s.filter.Query()
	for query.Next() {
		pos, org, cells, _ := query.Get()

		if org.Dead {
			continue
		}

		// Calculate energy ratio
		energyRatio := org.Energy / org.MaxEnergy

		// Determine context
		isFlora := traits.IsFlora(org.Traits)
		cellCount := int(cells.Count)

		// Find nearby food and threats
		var foodNearby, threatNearby bool
		if !isFlora {
			foodNearby = s.hasFoodNearby(pos, org, floraPositions, faunaPositions, floraOrgs, faunaOrgs)
			threatNearby = s.hasThreatNearby(pos, org, faunaPositions, faunaOrgs)
		}

		// Determine target cell count based on conditions
		org.TargetCells = s.calculateTargetCells(org, cellCount, energyRatio, foodNearby, isFlora)

		// Determine allocation mode
		org.AllocationMode = s.determineMode(org, cells, energyRatio, foodNearby, threatNearby, isFlora)
	}
}

func (s *AllocationSystem) calculateTargetCells(org *components.Organism, currentCells int, energyRatio float32, foodNearby, isFlora bool) uint8 {
	// Base target depends on organism type
	var baseTarget int

	if isFlora {
		// Flora: grow larger when healthy, cap based on energy
		if energyRatio > 0.7 {
			baseTarget = 8
		} else if energyRatio > 0.5 {
			baseTarget = 6
		} else if energyRatio > 0.3 {
			baseTarget = 4
		} else {
			baseTarget = 2 // Minimum viable size
		}
	} else {
		// Fauna: size vs speed tradeoff
		// Larger = more energy capacity but slower
		// Smaller = faster but less reserves

		if org.Traits.Has(traits.Carnivore) {
			// Predators: medium size for balance of speed and power
			if energyRatio > 0.6 && foodNearby {
				baseTarget = 5
			} else if energyRatio > 0.4 {
				baseTarget = 4
			} else {
				baseTarget = 3 // Stay lean when hungry
			}
		} else if org.Traits.Has(traits.Herbivore) {
			// Herbivores: can be larger since food is plentiful
			if energyRatio > 0.6 {
				baseTarget = 6
			} else if energyRatio > 0.4 {
				baseTarget = 4
			} else {
				baseTarget = 3
			}
		} else {
			// Carrion eaters: stay medium
			baseTarget = 4
		}

	}

	return uint8(min(baseTarget, 10))
}

func (s *AllocationSystem) determineMode(
	org *components.Organism,
	cells *components.CellBuffer,
	energyRatio float32,
	foodNearby, threatNearby, isFlora bool,
) components.AllocationMode {
	cellCount := int(cells.Count)
	targetCells := int(org.TargetCells)

	// SURVIVE mode: critical energy or under threat
	if energyRatio < 0.2 {
		return components.ModeSurvive
	}
	if threatNearby && energyRatio < 0.4 {
		return components.ModeSurvive
	}

	// For fauna with breeding capability (all fauna can breed, just need cooldown)
	canBreed := !isFlora && org.BreedingCooldown == 0

	// BREED mode: healthy enough and wants to breed
	// Lower threshold (35%) to allow more breeding opportunities
	if !isFlora && canBreed && cellCount >= 1 && energyRatio > 0.35 {
		return components.ModeBreed
	}

	// GROW mode: below target size and have energy to spare
	if cellCount < targetCells && energyRatio > 0.35 {
		return components.ModeGrow
	}

	// STORE mode: at target size, building reserves
	if cellCount >= targetCells {
		// If breeding available and energy high, switch to breed
		if !isFlora && canBreed && energyRatio > 0.6 {
			return components.ModeBreed
		}
		return components.ModeStore
	}

	// Default to store
	return components.ModeStore
}

func (s *AllocationSystem) hasFoodNearby(
	pos *components.Position,
	org *components.Organism,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
) bool {
	searchRadius := org.PerceptionRadius * 1.5

	// Herbivores look for flora
	if org.Traits.Has(traits.Herbivore) {
		// Use FloraSystem if available
		if s.floraSystem != nil {
			nearbyFlora := s.floraSystem.GetNearbyFlora(pos.X, pos.Y, searchRadius)
			if len(nearbyFlora) > 0 {
				return true
			}
		} else if floraOrgs != nil {
			// Fallback to old method
			for i := range floraPos {
				if floraOrgs[i].Dead {
					continue
				}
				dx := pos.X - floraPos[i].X
				dy := pos.Y - floraPos[i].Y
				if dx*dx+dy*dy < searchRadius*searchRadius {
					return true
				}
			}
		}
	}

	// Carnivores look for fauna
	if org.Traits.Has(traits.Carnivore) && faunaOrgs != nil {
		for i := range faunaPos {
			if faunaOrgs[i] == org || faunaOrgs[i].Dead {
				continue
			}
			// Don't count other carnivores as food
			if faunaOrgs[i].Traits.Has(traits.Carnivore) {
				continue
			}
			dx := pos.X - faunaPos[i].X
			dy := pos.Y - faunaPos[i].Y
			if dx*dx+dy*dy < searchRadius*searchRadius {
				return true
			}
		}
	}

	// Carrion eaters look for dead fauna
	// Note: Dead flora are removed immediately in FloraSystem, so we only check fauna
	if org.Traits.Has(traits.Carrion) {
		for i := range faunaPos {
			if faunaOrgs == nil || !faunaOrgs[i].Dead {
				continue
			}
			dx := pos.X - faunaPos[i].X
			dy := pos.Y - faunaPos[i].Y
			if dx*dx+dy*dy < searchRadius*searchRadius {
				return true
			}
		}
	}

	return false
}

func (s *AllocationSystem) hasThreatNearby(
	pos *components.Position,
	org *components.Organism,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
) bool {
	// Only herbivores perceive threats
	if !org.Traits.Has(traits.Herbivore) || org.Traits.Has(traits.Carnivore) {
		return false
	}

	if faunaOrgs == nil {
		return false
	}

	threatRadius := org.PerceptionRadius * 2.0

	for i := range faunaPos {
		if faunaOrgs[i] == org || faunaOrgs[i].Dead {
			continue
		}
		if !faunaOrgs[i].Traits.Has(traits.Carnivore) {
			continue
		}
		dx := pos.X - faunaPos[i].X
		dy := pos.Y - faunaPos[i].Y
		if dx*dx+dy*dy < threatRadius*threatRadius {
			return true
		}
	}

	return false
}

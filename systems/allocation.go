package systems

import (
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// AllocationSystem evaluates each organism's state and sets energy allocation mode.
// All ECS organisms are fauna - flora are managed separately by FloraSystem.
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

// getDigestiveSpectrum computes the digestive spectrum from cells.
// Returns 0.0 for herbivore, 0.5 for omnivore, 1.0 for carnivore.
func (s *AllocationSystem) getDigestiveSpectrum(cells *components.CellBuffer) float32 {
	if cells == nil {
		return 0.5 // neutral
	}
	caps := cells.ComputeCapabilities()
	return caps.DigestiveSpectrum()
}

// Update evaluates allocation mode for all organisms (all are fauna).
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

		energyRatio := org.Energy / org.MaxEnergy
		digestiveSpectrum := s.getDigestiveSpectrum(cells)

		foodNearby := s.hasFoodNearby(pos, org, digestiveSpectrum, floraPositions, faunaPositions, floraOrgs, faunaOrgs)
		threatNearby := s.hasThreatNearby(pos, org, digestiveSpectrum, faunaPositions, faunaOrgs)

		org.TargetCells = s.calculateTargetCells(cells, energyRatio, foodNearby)
		org.AllocationMode = s.determineMode(org, cells, energyRatio, threatNearby)
	}
}

func (s *AllocationSystem) calculateTargetCells(cells *components.CellBuffer, energyRatio float32, foodNearby bool) uint8 {
	// Fauna: size vs speed tradeoff
	// Larger = more energy capacity but slower
	// Smaller = faster but less reserves

	// Use capability-based digestive spectrum
	digestiveSpectrum := s.getDigestiveSpectrum(cells)

	var baseTarget int
	switch {
	case digestiveSpectrum > 0.6:
		// Carnivores: medium size for balance of speed and power
		switch {
		case energyRatio > 0.6 && foodNearby:
			baseTarget = 5
		case energyRatio > 0.4:
			baseTarget = 4
		default:
			baseTarget = 3 // Stay lean when hungry
		}
	case digestiveSpectrum < 0.4:
		// Herbivores: can be larger since food is plentiful
		switch {
		case energyRatio > 0.6:
			baseTarget = 6
		case energyRatio > 0.4:
			baseTarget = 4
		default:
			baseTarget = 3
		}
	default:
		// Omnivores (0.4-0.6): stay medium
		baseTarget = 4
	}

	return uint8(min(baseTarget, 10))
}

func (s *AllocationSystem) determineMode(
	org *components.Organism,
	cells *components.CellBuffer,
	energyRatio float32,
	threatNearby bool,
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

	// All ECS organisms are fauna and can breed
	canBreed := org.BreedingCooldown == 0

	// BREED mode: healthy enough and wants to breed
	// Lower threshold (35%) to allow more breeding opportunities
	if canBreed && cellCount >= 1 && energyRatio > 0.35 {
		return components.ModeBreed
	}

	// GROW mode: below target size and have energy to spare
	if cellCount < targetCells && energyRatio > 0.35 {
		return components.ModeGrow
	}

	// STORE mode: at target size, building reserves
	if cellCount >= targetCells {
		// If breeding available and energy high, switch to breed
		if canBreed && energyRatio > 0.6 {
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
	digestiveSpectrum float32,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
) bool {
	searchRadius := org.PerceptionRadius * 1.5
	searchRadiusSq := searchRadius * searchRadius

	// Low digestive spectrum (< 0.5) can eat flora
	if digestiveSpectrum < 0.5 {
		if s.floraSystem != nil {
			if len(s.floraSystem.GetNearbyFlora(pos.X, pos.Y, searchRadius)) > 0 {
				return true
			}
		} else if floraOrgs != nil {
			for i := range floraPos {
				if !floraOrgs[i].Dead && distanceSq(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y) < searchRadiusSq {
					return true
				}
			}
		}
	}

	// High digestive spectrum (> 0.5) can eat fauna
	if digestiveSpectrum > 0.5 && faunaOrgs != nil {
		for i := range faunaPos {
			if faunaOrgs[i] == org || faunaOrgs[i].Dead {
				continue
			}
			if distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) < searchRadiusSq {
				return true
			}
		}
	}

	// Carrion eating: organisms with digestive spectrum > 0.3 can eat dead fauna
	if digestiveSpectrum > 0.3 && faunaOrgs != nil {
		for i := range faunaPos {
			if faunaOrgs[i].Dead && distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) < searchRadiusSq {
				return true
			}
		}
	}

	return false
}

func (s *AllocationSystem) hasThreatNearby(
	pos *components.Position,
	org *components.Organism,
	digestiveSpectrum float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
) bool {
	// Carnivores don't perceive threats in allocation
	if digestiveSpectrum > 0.5 || faunaOrgs == nil {
		return false
	}

	threatRadius := org.PerceptionRadius * 2.0
	threatRadiusSq := threatRadius * threatRadius

	for i := range faunaPos {
		if faunaOrgs[i] == org || faunaOrgs[i].Dead {
			continue
		}
		if distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) < threatRadiusSq {
			return true
		}
	}

	return false
}

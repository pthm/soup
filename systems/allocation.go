package systems

import (
	"math"

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
// Simplified: no growth mode since fauna don't grow - evolution via breeding only.
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
		threatNearby := s.hasThreatNearby(pos, org, digestiveSpectrum, faunaPositions, faunaOrgs)

		org.AllocationMode = s.determineMode(org, energyRatio, threatNearby)
	}
}

func (s *AllocationSystem) determineMode(
	org *components.Organism,
	energyRatio float32,
	threatNearby bool,
) components.AllocationMode {
	// SURVIVE mode: critical energy or under threat
	if energyRatio < 0.2 {
		return components.ModeSurvive
	}
	if threatNearby && energyRatio < 0.4 {
		return components.ModeSurvive
	}

	// BREED mode: healthy enough and not on cooldown
	canBreed := org.BreedingCooldown == 0
	if canBreed && energyRatio > 0.35 {
		return components.ModeBreed
	}

	// STORE mode: building reserves
	return components.ModeStore
}

func (s *AllocationSystem) hasFoodNearby(
	pos *components.Position,
	org *components.Organism,
	digestiveSpectrum float32,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
) bool {
	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	searchRadius := org.PerceptionRadius * 1.5
	searchRadiusSq := searchRadius * searchRadius

	// Low digestive spectrum (< 0.5) can eat flora
	if digestiveSpectrum < 0.5 {
		if s.floraSystem != nil {
			if len(s.floraSystem.GetNearbyFlora(centerX, centerY, searchRadius)) > 0 {
				return true
			}
		} else if floraOrgs != nil {
			for i := range floraPos {
				if !floraOrgs[i].Dead && distanceSq(centerX, centerY, floraPos[i].X, floraPos[i].Y) < searchRadiusSq {
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
			if distanceSq(centerX, centerY, faunaPos[i].X, faunaPos[i].Y) < searchRadiusSq {
				return true
			}
		}
	}

	// Carrion eating: organisms with digestive spectrum > 0.3 can eat dead fauna
	if digestiveSpectrum > 0.3 && faunaOrgs != nil {
		for i := range faunaPos {
			if faunaOrgs[i].Dead && distanceSq(centerX, centerY, faunaPos[i].X, faunaPos[i].Y) < searchRadiusSq {
				return true
			}
		}
	}

	return false
}

func (s *AllocationSystem) hasThreatNearby(
	pos *components.Position,
	org *components.Organism,
	_ float32, // digestiveSpectrum - unused now that all organisms can perceive threats
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
) bool {
	if faunaOrgs == nil {
		return false
	}
	// All organisms can perceive threats from larger/hungrier neighbors

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	threatRadius := org.PerceptionRadius * 2.0
	threatRadiusSq := threatRadius * threatRadius

	myEnergy := org.Energy
	myMaxEnergy := org.MaxEnergy

	for i := range faunaPos {
		other := faunaOrgs[i]
		if other == org || other.Dead {
			continue
		}
		if distanceSq(centerX, centerY, faunaPos[i].X, faunaPos[i].Y) > threatRadiusSq {
			continue
		}

		// Threat assessment: consider if other is significantly larger or much hungrier
		// Be conservative - only trigger for clear threats to avoid constant fleeing
		otherEnergyRatio := other.Energy / other.MaxEnergy

		isSignificantlyLarger := other.MaxEnergy > myMaxEnergy*1.5 // 50% larger
		isDesperatelyHungry := otherEnergyRatio < 0.3 && other.MaxEnergy > myMaxEnergy
		isMuchStronger := other.Energy > myEnergy*2.0 // Has 2x more absolute energy

		if isSignificantlyLarger || isDesperatelyHungry || isMuchStronger {
			return true
		}
	}

	return false
}

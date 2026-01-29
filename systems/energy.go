package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// EnergySystem handles organism energy updates.
type EnergySystem struct {
	filter ecs.Filter2[components.Organism, components.CellBuffer]
}

// NewEnergySystem creates a new energy system.
func NewEnergySystem(w *ecs.World) *EnergySystem {
	return &EnergySystem{
		filter: *ecs.NewFilter2[components.Organism, components.CellBuffer](w),
	}
}

// Update runs the energy system.
func (s *EnergySystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		org, cells := query.Get()

		// Base energy drain (reduced for better balance)
		energyDrain := float32(0.005) + 0.001*float32(cells.Count)

		// Check for mutations
		hasDisease := false
		hasRage := false
		for i := uint8(0); i < cells.Count; i++ {
			if cells.Cells[i].Mutation == traits.Disease {
				hasDisease = true
			}
			if cells.Cells[i].Mutation == traits.Rage {
				hasRage = true
			}
		}

		// Disease drains extra energy
		if hasDisease {
			energyDrain += 0.02
		}

		// Class-based speed and energy drain based on cell count
		// Key insight: Base drain is LOW (resting is efficient for all sizes)
		// Activity cost scales with mass (moving is expensive for large organisms)
		cellCount := int(cells.Count)
		var baseSpeed, baseDrain float32

		switch {
		case cellCount <= 3: // Drifters: small, efficient, fast for size
			baseDrain = 0.002 + 0.0005*float32(cellCount)
			baseSpeed = 0.5 + org.ShapeMetrics.Streamlining*0.2

		case cellCount <= 10: // Generalists: balanced metabolism
			penalty := float32(cellCount-1) * 0.08
			baseDrain = 0.003 + 0.0008*float32(cellCount)
			baseSpeed = float32(max(0.8, 2.0-float64(penalty))) * (0.8 + org.ShapeMetrics.Streamlining*0.4)

		default: // Apex: low resting metabolism, high activity cost (applied below)
			penalty := float32(cellCount-1) * 0.04
			baseDrain = 0.004 + 0.0006*float32(cellCount) // Lower base than before
			baseSpeed = float32(max(0.6, 1.4-float64(penalty))) * (0.7 + org.ShapeMetrics.Streamlining*0.5)
		}

		energyDrain = baseDrain

		// Rage mutation gives speed boost but drains energy
		if hasRage {
			energyDrain += 0.03
			baseSpeed *= 1.4
		}

		// Floating flora special case
		if traits.IsFlora(org.Traits) && org.Traits.Has(traits.Floating) {
			baseSpeed = 0.3
		}

		org.MaxSpeed = baseSpeed

		// Flora has reduced energy drain (photosynthesis handled separately)
		if traits.IsFlora(org.Traits) {
			energyDrain *= 0.2
		}

		// Movement cost: active thrust × drag coefficient × mass factor
		// Larger organisms pay MORE to move - hunting is expensive for apex predators
		// Using cells^0.7 instead of sqrt (cells^0.5) for steeper scaling
		// 3-cell: 2.2x, 10-cell: 5.0x, 25-cell: 9.5x (vs sqrt: 1.7x, 3.2x, 5.0x)
		massFactor := float32(math.Pow(float64(cells.Count), 0.7))
		thrustCost := org.ActiveThrust * org.ShapeMetrics.DragCoefficient * 1.5 * massFactor
		energyDrain += thrustCost
		org.ActiveThrust = 0 // Reset for next tick

		org.Energy -= energyDrain
		org.Energy = float32(math.Min(float64(org.Energy), float64(org.MaxEnergy)))

		// Death from starvation
		if org.Energy <= 0 {
			org.Dead = true
		}

		// Breeding cooldown
		if org.BreedingCooldown > 0 {
			org.BreedingCooldown--
		}

		// Update max energy based on cell count
		org.MaxEnergy = 100 + float32(cells.Count)*50
	}
}

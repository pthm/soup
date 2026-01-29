package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// EnergySystem handles organism energy updates.
// All ECS organisms are fauna - flora are managed separately by FloraSystem.
type EnergySystem struct {
	filter    ecs.Filter3[components.Position, components.Organism, components.CellBuffer]
	shadowMap *ShadowMap
}

// NewEnergySystem creates a new energy system.
func NewEnergySystem(w *ecs.World, shadowMap *ShadowMap) *EnergySystem {
	return &EnergySystem{
		filter:    *ecs.NewFilter3[components.Position, components.Organism, components.CellBuffer](w),
		shadowMap: shadowMap,
	}
}

// Update runs the energy system.
func (s *EnergySystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		pos, org, cells := query.Get()

		// Compute capabilities once for this organism
		caps := cells.ComputeCapabilities()

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

		energyDrain := baseDrain
		org.MaxSpeed = baseSpeed

		// Fauna photosynthesis: organisms with photosynthetic cells can offset energy drain
		// This allows evolution of mixed strategies (photosynthetic fauna)
		if caps.PhotoWeight > 0 && s.shadowMap != nil {
			light := s.shadowMap.SampleLight(pos.X, pos.Y)
			photoEnergy := float32(0.1) * light * caps.PhotoWeight
			// Cap photosynthesis at 80% of base drain (can't fully sustain on light alone)
			maxOffset := energyDrain * 0.8
			if photoEnergy > maxOffset {
				photoEnergy = maxOffset
			}
			energyDrain -= photoEnergy
		}

		// Movement cost: active thrust × drag coefficient × mass factor × armor penalty
		// Larger organisms pay MORE to move - hunting is expensive for apex predators
		// Using cells^0.7 instead of sqrt (cells^0.5) for steeper scaling
		// 3-cell: 2.2x, 10-cell: 5.0x, 25-cell: 9.5x (vs sqrt: 1.7x, 3.2x, 5.0x)
		// Armor adds drag penalty: 40% more movement cost at full armor
		massFactor := float32(math.Pow(float64(cells.Count), 0.7))
		armorPenalty := float32(1.0) + caps.StructuralArmor*0.4
		thrustCost := org.ActiveThrust * org.ShapeMetrics.DragCoefficient * 1.5 * massFactor * armorPenalty
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

		// Update max energy based on cell count and storage capacity
		// Storage cells provide bonus energy capacity (30 per cell at full storage)
		baseMax := float32(100) + float32(cells.Count)*50
		storageBonus := caps.StorageCapacity * float32(cells.Count) * 30
		org.MaxEnergy = baseMax + storageBonus
	}
}

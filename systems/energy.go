package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Energy system constants
const (
	photoMaxOffset    = 0.80  // Photosynthesis can offset max 80% of base drain
	photoRate         = 0.10  // Energy per light per photosynthetic weight
	massScaleExponent = 0.70  // Mass factor for movement cost (cells^0.7)
	movementCostBase  = 1.50  // Base multiplier for thrust cost
	armorDragPenalty  = 0.40  // 40% more movement cost at full armor
	baseEnergyPerCell = 50.0  // Base energy capacity per cell
	storageBonus      = 30.0  // Bonus energy per cell at full storage
	baseEnergy        = 100.0 // Minimum energy capacity
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
		if caps.PhotoWeight > 0 && s.shadowMap != nil {
			light := s.shadowMap.SampleLight(pos.X, pos.Y)
			photoEnergy := photoRate * light * caps.PhotoWeight
			// Cap photosynthesis (can't fully sustain on light alone)
			maxOffset := energyDrain * photoMaxOffset
			if photoEnergy > maxOffset {
				photoEnergy = maxOffset
			}
			energyDrain -= photoEnergy
		}

		// Movement cost: active thrust x drag x mass factor x armor penalty
		// Larger organisms pay more to move - hunting is expensive for apex predators
		massFactor := float32(math.Pow(float64(cells.Count), massScaleExponent))
		armorPen := 1.0 + caps.StructuralArmor*armorDragPenalty
		thrustCost := org.ActiveThrust * org.ShapeMetrics.DragCoefficient * movementCostBase * massFactor * armorPen
		energyDrain += thrustCost
		org.ActiveThrust = 0

		org.Energy -= energyDrain
		if org.Energy > org.MaxEnergy {
			org.Energy = org.MaxEnergy
		}

		if org.Energy <= 0 {
			org.Dead = true
		}

		if org.BreedingCooldown > 0 {
			org.BreedingCooldown--
		}

		// Update max energy based on cell count and storage capacity
		cellCountF := float32(cellCount)
		org.MaxEnergy = baseEnergy + cellCountF*baseEnergyPerCell + caps.StorageCapacity*cellCountF*storageBonus
	}
}

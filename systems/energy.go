package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Energy system constants - REDESIGNED for brain-output-driven costs
const (
	// Base metabolism (low - resting is efficient)
	baseMetabolismPerCell = 0.0005 // Low drain when doing nothing

	// Brain output costs - MINIMAL pressure
	// Intent costs are very light - the main pressure is movement and feeding success
	eatIntentCost   = 0.00005 // Minimal - eating is core survival
	breedIntentCost = 0.00010 // Minimal - breeding is core to evolution

	// Movement costs - PRIMARY selective pressure (NON-LINEAR)
	// Quadratic cost curve: efficient cruising, expensive bursting
	movementCostBase  = 0.003  // Base cost coefficient (quadratic: speed² * base)
	thrustCostBase    = 0.003  // Cost for actual acceleration/thrust
	massScaleExponent = 0.5    // Larger organisms pay more to move (was 0.4)

	// Jitter penalty - penalizes rapid direction changes
	// Encourages smooth movement, discourages oscillation
	jitterCostBase = 0.002 // Cost per unit of direction change

	// Energy capacity
	baseEnergy        = 100.0 // Minimum energy capacity
	baseEnergyPerCell = 50.0  // Base energy capacity per cell
	storageBonus      = 30.0  // Bonus energy per cell at full storage

	// Armor penalty
	armorDragPenalty = 0.30 // 30% more movement cost at full armor
)

// EnergySystem handles organism energy updates with brain-output-driven costs.
// This creates evolutionary pressure for efficient, context-sensitive behavior.
type EnergySystem struct {
	filter ecs.Filter3[components.Position, components.Organism, components.CellBuffer]
}

// NewEnergySystem creates a new energy system.
func NewEnergySystem(w *ecs.World) *EnergySystem {
	return &EnergySystem{
		filter: *ecs.NewFilter3[components.Position, components.Organism, components.CellBuffer](w),
	}
}

// Update runs the energy system with brain-output-driven costs.
func (s *EnergySystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		_, org, cells := query.Get()

		if org.Dead {
			continue
		}

		// Compute capabilities once
		caps := cells.ComputeCapabilities()
		cellCount := float32(cells.Count)

		// === BASE METABOLISM ===
		// Very low - resting is efficient for all organisms
		baseDrain := baseMetabolismPerCell * cellCount

		// === BRAIN OUTPUT COSTS ===
		// These create selective pressure - maxing outputs is expensive
		intentCost := float32(0.0)

		// Eat intent cost - "hunting mode" uses energy for active sensing, digestion prep
		// Scaled by intensity: low intent = low cost, high intent = high cost
		intentCost += org.EatIntent * eatIntentCost * cellCount

		// Breed intent cost - reproductive readiness requires energy
		intentCost += org.BreedIntent * breedIntentCost * cellCount

		// === MOVEMENT COSTS (NON-LINEAR) ===
		// Larger organisms pay exponentially more to move
		massFactor := float32(math.Pow(float64(cellCount), massScaleExponent))
		armorPen := 1.0 + caps.StructuralArmor*armorDragPenalty

		// QUADRATIC movement cost: cost = throttle² * base
		// This makes full throttle (1.0) 4x more expensive than half throttle (0.5)
		// Encourages cruising at moderate speeds, bursting only when needed
		throttleSquared := org.UThrottle * org.UThrottle
		movementCost := throttleSquared * movementCostBase * massFactor * armorPen

		// JITTER PENALTY: penalize rapid turn/throttle changes
		// Discourages oscillating, rewards smooth trajectories
		deltaTurn := org.UTurn - org.PrevUTurn
		deltaThrottle := org.UThrottle - org.PrevUThrottle
		controlChange := float32(math.Sqrt(float64(deltaTurn*deltaTurn + deltaThrottle*deltaThrottle)))
		jitterCost := controlChange * jitterCostBase * massFactor

		// Store current outputs for next tick's jitter calculation
		org.PrevUTurn = org.UTurn
		org.PrevUThrottle = org.UThrottle

		// Actual acceleration cost (ActiveThrust set by behavior system)
		// High drag = more energy to move
		thrustCost := org.ActiveThrust * thrustCostBase * massFactor * armorPen * org.ShapeMetrics.Drag
		org.ActiveThrust = 0 // Reset for next tick

		// === TOTAL ENERGY DRAIN ===
		totalDrain := baseDrain + intentCost + movementCost + jitterCost + thrustCost

		// Minimum drain (can't gain energy from just photosynthesis without feeding)
		if totalDrain < 0.0001 {
			totalDrain = 0.0001
		}

		org.Energy -= totalDrain

		// Cap energy at max
		if org.Energy > org.MaxEnergy {
			org.Energy = org.MaxEnergy
		}

		// Death check
		if org.Energy <= 0 {
			org.Dead = true
		}

		// Update breeding cooldown
		if org.BreedingCooldown > 0 {
			org.BreedingCooldown--
		}

		// Decay "being eaten" awareness (fades over ~10 ticks)
		if org.BeingEaten > 0 {
			org.BeingEaten *= 0.85 // Decay factor
			if org.BeingEaten < 0.01 {
				org.BeingEaten = 0
			}
		}

		// Update max energy based on cell count and storage
		org.MaxEnergy = baseEnergy + cellCount*baseEnergyPerCell + caps.StorageCapacity*cellCount*storageBonus

		// Update max speed based on size and drag
		// Smaller organisms are faster, low drag = faster
		speedPenalty := float32(math.Pow(float64(cellCount), 0.3)) * 0.1
		dragFactor := 1.2 - org.ShapeMetrics.Drag*0.5 // Low drag (0.2) = 1.1x, High drag (1.0) = 0.7x
		org.MaxSpeed = float32(math.Max(0.5, 1.5-float64(speedPenalty))) * dragFactor
	}
}

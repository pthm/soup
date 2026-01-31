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
	glowIntentCost  = 0.00050 // Slightly higher - glow is optional luxury
	// Note: GrowIntent removed - fauna no longer grow, evolution via breeding only

	// Movement costs - PRIMARY selective pressure
	// These should be the main energy sink for active organisms
	movementCostBase  = 0.0015 // Cost per unit of desired velocity (urgency)
	thrustCostBase    = 0.0020 // Cost for actual acceleration/thrust
	massScaleExponent = 0.40   // Larger organisms pay more to move

	// Photosynthesis - helps organisms with photo cells survive
	photoMaxOffset = 0.95 // Photosynthesis can offset up to 95% of base drain
	photoRate      = 0.12 // Energy per light per photosynthetic weight (increased)

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

// Update runs the energy system with brain-output-driven costs.
func (s *EnergySystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		pos, org, cells := query.Get()

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

		// Glow intent cost - bioluminescence is expensive
		// Note: GrowIntent removed - fauna born with their cells, evolution via breeding
		intentCost += org.GlowIntent * glowIntentCost * cellCount

		// === MOVEMENT COSTS ===
		// Larger organisms pay exponentially more to move
		massFactor := float32(math.Pow(float64(cellCount), massScaleExponent))
		armorPen := 1.0 + caps.StructuralArmor*armorDragPenalty

		// Movement intent cost (UFwd/UUp magnitude) - wanting to move costs energy
		desiredSpeed := float32(math.Sqrt(float64(org.UFwd*org.UFwd + org.UUp*org.UUp)))
		movementCost := desiredSpeed * movementCostBase * massFactor * armorPen

		// Actual acceleration cost (ActiveThrust set by behavior system)
		// High drag = more energy to move
		thrustCost := org.ActiveThrust * thrustCostBase * massFactor * armorPen * org.ShapeMetrics.Drag
		org.ActiveThrust = 0 // Reset for next tick

		// === PHOTOSYNTHESIS OFFSET ===
		// Can offset most of base drain but not activity costs
		photoOffset := float32(0.0)
		if caps.PhotoWeight > 0 && s.shadowMap != nil {
			// Calculate organism center using OBB offset (offset is in local space, must be rotated)
			cosH := float32(math.Cos(float64(org.Heading)))
			sinH := float32(math.Sin(float64(org.Heading)))
			centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
			centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH
			light := s.shadowMap.SampleLight(centerX, centerY)
			photoEnergy := photoRate * light * caps.PhotoWeight
			// Cap at fraction of base drain only
			maxOffset := baseDrain * photoMaxOffset
			if photoEnergy > maxOffset {
				photoEnergy = maxOffset
			}
			photoOffset = photoEnergy
		}

		// === TOTAL ENERGY DRAIN ===
		totalDrain := baseDrain + intentCost + movementCost + thrustCost - photoOffset

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

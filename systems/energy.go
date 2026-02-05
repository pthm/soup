package systems

import (
	"github.com/pthm-cable/soup/components"
)

// UpdateEnergy applies metabolic costs and checks for death.
// Costs are: base + bio_cost*Bio + movement + acceleration, all scaled by metabolic rate.
// Returns total metabolic cost for heat tracking (conservation accounting).
func UpdateEnergy(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	metabolicRate float32,
	dt float32,
) float32 {
	if !energy.Alive {
		return 0
	}

	// Age
	energy.Age += dt

	// All costs scaled by metabolic rate
	baseCost := cachedBaseCost * metabolicRate
	bioCost := cachedBioCost * metabolicRate
	moveCost := cachedMoveCost * metabolicRate
	accelCost := cachedAccelCost * metabolicRate

	var cost float32

	// Base metabolism + biomass maintenance cost
	baseDrain := (baseCost + bioCost*energy.Bio) * dt
	cost += baseDrain
	energy.Met -= baseDrain

	// Movement cost: proportional to (speed/maxSpeed)^2
	speedSq := vel.X*vel.X + vel.Y*vel.Y
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	if maxSpeedSq > 0 {
		speedRatio := speedSq / maxSpeedSq
		moveDrain := moveCost * speedRatio * dt
		cost += moveDrain
		energy.Met -= moveDrain
	}

	// Acceleration cost: proportional to thrust^2
	accelDrain := accelCost * energy.LastThrust * energy.LastThrust * dt
	cost += accelDrain
	energy.Met -= accelDrain

	// Death check (when metabolic energy is depleted)
	if energy.Met <= 0 {
		energy.Met = 0
		energy.Alive = false
	}

	return cost
}

// GrowBiomass converts surplus metabolic energy to biomass.
// Called after feeding, before metabolism costs.
// Returns amount of Met converted to Bio.
func GrowBiomass(energy *components.Energy, dt float32) float32 {
	if !energy.Alive {
		return 0
	}

	metPerBio := cachedMetPerBio
	growthRate := cachedGrowthRate
	growthThreshold := cachedGrowthThreshold

	maxMet := energy.Bio * metPerBio
	threshold := maxMet * growthThreshold

	// Only grow if above threshold and below BioCap
	if energy.Met <= threshold || energy.Bio >= energy.BioCap {
		return 0
	}

	// Available surplus above threshold
	surplus := energy.Met - threshold

	// Grow limited by surplus, growth rate, and remaining cap
	grow := surplus
	if grow > growthRate*dt {
		grow = growthRate * dt
	}
	if grow > energy.BioCap-energy.Bio {
		grow = energy.BioCap - energy.Bio
	}

	energy.Met -= grow
	energy.Bio += grow

	return grow
}

// lerp32 performs linear interpolation between a and b by t.
func lerp32(a, b, t float32) float32 {
	return a + (b-a)*t
}

// EnergyTransfer holds the full accounting for an energy transfer event.
// The caller is responsible for depositing ToDet and tracking ToHeat.
type EnergyTransfer struct {
	Removed  float32 // total energy taken from prey (Bio + Met on kill)
	ToGainer float32 // energy given to predator (after efficiency)
	ToDet    float32 // energy to deposit as detritus at prey position
	ToHeat   float32 // energy lost to heat sink (transfer inefficiency)
	Overflow float32 // predator overflow above MaxMet (deposit as detritus)
	Killed   bool    // whether prey was killed
}

// TransferEnergy handles feeding with the two-pool biomass model.
// On kill: transfer is based on prey's total (Bio + Met).
// On wound: transfer is based on damage dealt (from prey's Met only).
// Returns EnergyTransfer with all energy flows. The caller must:
//   - Deposit ToDet at prey position
//   - Deposit Overflow at predator position
//   - Add ToHeat to heatLossAccum
//   - Set digest cooldown based on ToGainer
func TransferEnergy(
	predatorEnergy *components.Energy,
	preyEnergy *components.Energy,
	damage float32,
) EnergyTransfer {
	if !predatorEnergy.Alive || !preyEnergy.Alive {
		return EnergyTransfer{}
	}

	// Wounds only affect Met (metabolic energy)
	actualDamage := damage
	if preyEnergy.Met < actualDamage {
		actualDamage = preyEnergy.Met
	}
	preyEnergy.Met -= actualDamage

	// Check if prey dies (Met depleted)
	killed := preyEnergy.Met <= 0
	if killed {
		preyEnergy.Met = 0
		preyEnergy.Alive = false
	}

	// Calculate transfer amount
	var transferBase float32
	if killed {
		// On kill: predator gets prey's total (Bio + Met)
		// Prey's Bio is consumed (body eaten)
		transferBase = preyEnergy.Bio + actualDamage // Bio + what was in Met before kill
		preyEnergy.Bio = 0                           // Body consumed
	} else {
		// On wound: transfer is damage dealt
		transferBase = actualDamage
	}

	// Apply feeding efficiency
	eta := cachedFeedingEfficiency
	toGainer := transferBase * eta
	toHeat := transferBase - toGainer

	predatorEnergy.Met += toGainer

	// Compute overflow (predator can't hold more than MaxMet)
	metPerBio := cachedMetPerBio
	predMaxMet := predatorEnergy.Bio * metPerBio
	var overflow float32
	if predatorEnergy.Met > predMaxMet {
		overflow = predatorEnergy.Met - predMaxMet
		predatorEnergy.Met = predMaxMet
	}

	return EnergyTransfer{
		Removed:  transferBase,
		ToGainer: toGainer,
		ToDet:    0, // Kills transfer all to predator; starvation deaths handle carcass
		ToHeat:   toHeat,
		Overflow: overflow,
		Killed:   killed,
	}
}

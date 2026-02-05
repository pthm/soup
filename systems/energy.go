package systems

import (
	"github.com/pthm-cable/soup/components"
)

// UpdateEnergy applies metabolic costs and checks for death.
// Costs are interpolated based on diet (0=herbivore, 1=carnivore).
// Returns total metabolic cost for heat tracking (conservation accounting).
func UpdateEnergy(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	diet float32,
	dt float32,
) float32 {
	if !energy.Alive {
		return 0
	}

	// Age
	energy.Age += dt

	// Interpolate costs based on diet (0=prey, 1=predator)
	baseCost := lerp32(cachedPreyBaseCost, cachedPredBaseCost, diet)
	moveCost := lerp32(cachedPreyMoveCost, cachedPredMoveCost, diet)
	accelCost := lerp32(cachedPreyAccelCost, cachedPredAccelCost, diet)

	var cost float32

	// Base metabolism
	baseDrain := baseCost * dt
	cost += baseDrain
	energy.Value -= baseDrain

	// Movement cost: proportional to (speed/maxSpeed)^2
	speedSq := vel.X*vel.X + vel.Y*vel.Y
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	if maxSpeedSq > 0 {
		speedRatio := speedSq / maxSpeedSq
		moveDrain := moveCost * speedRatio * dt
		cost += moveDrain
		energy.Value -= moveDrain
	}

	// Acceleration cost: proportional to thrust^2
	accelDrain := accelCost * energy.LastThrust * energy.LastThrust * dt
	cost += accelDrain
	energy.Value -= accelDrain

	// Bite cost: charged every tick the brain signals bite
	// Interpolated from 0 (prey) to cachedBiteCost (predator) by diet
	if energy.LastBite > cachedThrustDeadzone {
		biteCost := lerp32(0, cachedBiteCost, diet)
		biteDrain := biteCost * dt
		cost += biteDrain
		energy.Value -= biteDrain
	}

	// Death check
	if energy.Value <= 0 {
		energy.Value = 0
		energy.Alive = false
	}

	return cost
}

// lerp32 performs linear interpolation between a and b by t.
func lerp32(a, b, t float32) float32 {
	return a + (b-a)*t
}

// GrazingEfficiency returns the grazing efficiency for a given diet.
// Returns 1.0 at diet=0, falls to 0 at diet=grazing_diet_cap.
func GrazingEfficiency(diet float32) float32 {
	if cachedGrazingDietCap <= 0 {
		return 0.0
	}
	eff := 1.0 - diet/cachedGrazingDietCap
	if eff < 0 {
		return 0
	}
	return eff
}

// HuntingEfficiency returns the hunting efficiency for a given diet.
// Returns 0 below hunting_diet_floor, ramps to 1.0 at diet=1.0.
func HuntingEfficiency(diet float32) float32 {
	if diet < cachedHuntingDietFloor {
		return 0.0
	}
	range_ := 1.0 - cachedHuntingDietFloor
	if range_ <= 0 {
		return 1.0
	}
	return (diet - cachedHuntingDietFloor) / range_
}

// UpdatePreyForage adds energy gain from the resource field.
// Call this before UpdateEnergy for prey entities.
func UpdatePreyForage(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	resourceHere float32,
	dt float32,
) {
	if !energy.Alive {
		return
	}

	// Grazing efficiency is constant - organisms can graze while moving
	// (required for static depleting resources)
	_ = vel  // unused, kept for API stability
	_ = caps // unused, kept for API stability
	forageRate := cachedForageRate
	gain := resourceHere * forageRate * dt
	energy.Value += gain

	// Clamp to max
	if energy.Value > energy.Max {
		energy.Value = energy.Max
	}
}

// EnergyTransfer holds the full accounting for an energy transfer event.
// The caller is responsible for depositing ToDet and tracking ToHeat.
type EnergyTransfer struct {
	Removed  float32 // energy taken from prey
	ToGainer float32 // energy given to predator (after efficiency and detritus)
	ToDet    float32 // energy to deposit as detritus at prey position
	ToHeat   float32 // energy lost to heat sink (transfer inefficiency)
	Overflow float32 // predator overflow above Max (deposit as detritus at predator position)
}

// TransferEnergy handles predator feeding on prey with full conservation accounting.
// Returns EnergyTransfer with all energy flows. The caller must:
//   - Deposit ToDet at prey position
//   - Deposit Overflow at predator position
//   - Add ToHeat to heatLossAccum
func TransferEnergy(
	predatorEnergy *components.Energy,
	preyEnergy *components.Energy,
	amount float32,
) EnergyTransfer {
	if !predatorEnergy.Alive || !preyEnergy.Alive {
		return EnergyTransfer{}
	}

	// Take from prey
	removed := amount
	if preyEnergy.Value < removed {
		removed = preyEnergy.Value
	}

	// Accounting: split removed into predator gain, detritus, and heat
	detFrac := cachedDetritusFraction
	eta := cachedTransferEfficiency
	toGainer := eta * removed * (1 - detFrac)
	toDet := detFrac * removed
	toHeat := removed - toGainer - toDet

	preyEnergy.Value -= removed
	predatorEnergy.Value += toGainer

	// Compute overflow (caller routes to detritus at predator position)
	var overflow float32
	if predatorEnergy.Value > predatorEnergy.Max {
		overflow = predatorEnergy.Value - predatorEnergy.Max
		predatorEnergy.Value = predatorEnergy.Max
	}

	// Check prey death
	if preyEnergy.Value <= 0 {
		preyEnergy.Value = 0
		preyEnergy.Alive = false
	}

	return EnergyTransfer{
		Removed:  removed,
		ToGainer: toGainer,
		ToDet:    toDet,
		ToHeat:   toHeat,
		Overflow: overflow,
	}
}

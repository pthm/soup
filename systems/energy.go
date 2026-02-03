package systems

import (
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

// UpdateEnergy applies metabolic costs and checks for death.
// Costs are interpolated based on diet (0=herbivore, 1=carnivore).
func UpdateEnergy(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	diet float32,
	biteActive bool,
	dt float32,
) {
	if !energy.Alive {
		return
	}

	cfg := config.Cfg()

	// Age
	energy.Age += dt

	// Interpolate costs based on diet (0=prey, 1=predator)
	baseCost := lerp32(cachedPreyBaseCost, cachedPredBaseCost, diet)
	moveCost := lerp32(cachedPreyMoveCost, cachedPredMoveCost, diet)
	accelCost := lerp32(cachedPreyAccelCost, cachedPredAccelCost, diet)

	// Base metabolism
	energy.Value -= baseCost * dt

	// Movement cost: proportional to (speed/maxSpeed)^2
	speedSq := vel.X*vel.X + vel.Y*vel.Y
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	if maxSpeedSq > 0 {
		speedRatio := speedSq / maxSpeedSq
		energy.Value -= moveCost * speedRatio * dt
	}

	// Acceleration cost: proportional to thrust^2
	energy.Value -= accelCost * energy.LastThrust * energy.LastThrust * dt

	// Bite cost (predators attacking)
	if biteActive {
		energy.Value -= float32(cfg.Energy.Predator.BiteCost)
	}

	// Death check
	if energy.Value <= 0 {
		energy.Value = 0
		energy.Alive = false
	}
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

	// Compute speed ratio [0, 1] (use fast sqrt)
	speed := fastSqrt(vel.X*vel.X + vel.Y*vel.Y)
	speedRatio := speed / caps.MaxSpeed
	if speedRatio > 1 {
		speedRatio = 1
	}

	// Peak efficiency at grazing speed, drops toward 0 and max speed
	// eff = 1 - 2*|speedRatio - grazingPeak|, clamped to [0, 1]
	eff := 1.0 - 2.0*absf(speedRatio-cachedGrazingPeak)
	if eff < 0 {
		eff = 0
	}
	forageRate := cachedForageRate
	gain := resourceHere * forageRate * eff * dt
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

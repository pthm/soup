package systems

import (
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

// UpdateEnergy applies metabolic costs and checks for death.
// Uses per-kind costs for prey vs predator.
func UpdateEnergy(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	kind components.Kind,
	biteActive bool,
	dt float32,
) {
	if !energy.Alive {
		return
	}

	cfg := config.Cfg()

	// Age
	energy.Age += dt

	// Select costs by kind
	var baseCost, moveCost float32
	if kind == components.KindPredator {
		baseCost = float32(cfg.Energy.Predator.BaseCost)
		moveCost = float32(cfg.Energy.Predator.MoveCost)
	} else {
		baseCost = float32(cfg.Energy.Prey.BaseCost)
		moveCost = float32(cfg.Energy.Prey.MoveCost)
	}

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
	var accelCost float32
	if kind == components.KindPredator {
		accelCost = cachedPredAccelCost
	} else {
		accelCost = cachedPreyAccelCost
	}
	energy.Value -= accelCost * energy.LastThrust * energy.LastThrust * dt

	// Bite cost (predators attacking)
	if biteActive {
		energy.Value -= float32(cfg.Energy.Predator.BiteCost)
	}

	// Clamp energy
	if energy.Value > 1.0 {
		energy.Value = 1.0
	}

	// Death check
	if energy.Value <= 0 {
		energy.Value = 0
		energy.Alive = false
	}
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
	if energy.Value > 1.0 {
		energy.Value = 1.0
	}
}

// TransferEnergy handles predator feeding on prey.
// Returns the amount of energy transferred.
func TransferEnergy(
	predatorEnergy *components.Energy,
	preyEnergy *components.Energy,
	amount float32,
) float32 {
	if !predatorEnergy.Alive || !preyEnergy.Alive {
		return 0
	}

	// Take from prey
	actual := amount
	if preyEnergy.Value < actual {
		actual = preyEnergy.Value
	}

	preyEnergy.Value -= actual
	predatorEnergy.Value += actual * cachedTransferEfficiency

	// Check prey death
	if preyEnergy.Value <= 0 {
		preyEnergy.Value = 0
		preyEnergy.Alive = false
	}

	return actual
}

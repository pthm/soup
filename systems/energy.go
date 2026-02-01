package systems

import (
	"math"

	"github.com/pthm-cable/soup/components"
)

// Prey energy economics.
const (
	PreyBaseCost  = 0.010 // base metabolism per second
	PreyMoveCost  = 0.030 // movement cost scaling
	PreyForageRate = 0.060 // energy/sec at resource=1.0 when stationary
)

// Predator energy economics.
const (
	PredBaseCost  = 0.016 // higher base metabolism
	PredMoveCost  = 0.045 // higher movement cost
	PredBiteCost  = 0.010 // cost to attempt bite
	PredBiteReward = 0.35  // energy gained on successful bite
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

	// Age
	energy.Age += dt

	// Select costs by kind
	var baseCost, moveCost float32
	if kind == components.KindPredator {
		baseCost = PredBaseCost
		moveCost = PredMoveCost
	} else {
		baseCost = PreyBaseCost
		moveCost = PreyMoveCost
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

	// Bite cost (predators attacking)
	if biteActive {
		energy.Value -= PredBiteCost
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

	// Compute speed ratio [0, 1]
	speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
	speedRatio := speed / caps.MaxSpeed
	if speedRatio > 1 {
		speedRatio = 1
	}

	// Gain is higher when slow (grazing), lower when running
	gain := resourceHere * PreyForageRate * (1 - speedRatio) * dt
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
	predatorEnergy.Value += actual * 0.8 // 80% efficiency

	// Check prey death
	if preyEnergy.Value <= 0 {
		preyEnergy.Value = 0
		preyEnergy.Alive = false
	}

	return actual
}

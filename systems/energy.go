package systems

import (
	"github.com/pthm-cable/soup/components"
)

// Energy cost constants.
const (
	BaseCost = 0.01 // base metabolism per second
	MoveCost = 0.03 // movement cost scaling
)

// UpdateEnergy applies metabolic costs and checks for death.
func UpdateEnergy(
	energy *components.Energy,
	vel components.Velocity,
	caps components.Capabilities,
	biteActive bool,
	dt float32,
) {
	if !energy.Alive {
		return
	}

	// Age
	energy.Age += dt

	// Base metabolism
	energy.Value -= BaseCost * dt

	// Movement cost: proportional to (speed/maxSpeed)^2
	speed := vel.X*vel.X + vel.Y*vel.Y // squared
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	if maxSpeedSq > 0 {
		speedRatio := speed / maxSpeedSq
		energy.Value -= MoveCost * speedRatio * dt
	}

	// Bite cost (predators attacking)
	if biteActive {
		energy.Value -= caps.BiteCost
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

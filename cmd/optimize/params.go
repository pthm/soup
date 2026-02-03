// Package main provides CMA-ES optimization for soup simulation parameters.
package main

import (
	"github.com/pthm-cable/soup/config"
)

// ParamSpec defines a single optimizable parameter.
type ParamSpec struct {
	Name    string  // Human-readable name
	Path    string  // Config path for logging
	Min     float64 // Lower bound
	Max     float64 // Upper bound
	Default float64 // Default value
}

// ParamVector holds the set of all optimizable parameters.
type ParamVector struct {
	Specs []ParamSpec
}

// NewParamVector creates the standard set of optimizable parameters.
func NewParamVector() *ParamVector {
	return &ParamVector{
		Specs: []ParamSpec{
			// Energy - Prey (prey_base_cost locked at 0.005)
			{Name: "prey_move_cost", Path: "energy.prey.move_cost", Min: 0.05, Max: 0.25, Default: 0.12},
			{Name: "prey_forage_rate", Path: "energy.prey.forage_rate", Min: 0.02, Max: 0.10, Default: 0.045},
			// Energy - Predator
			{Name: "pred_base_cost", Path: "energy.predator.base_cost", Min: 0.002, Max: 0.02, Default: 0.008},
			{Name: "pred_move_cost", Path: "energy.predator.move_cost", Min: 0.01, Max: 0.08, Default: 0.025},
			{Name: "pred_bite_reward", Path: "energy.predator.bite_reward", Min: 0.4, Max: 0.8, Default: 0.5},
			{Name: "pred_transfer_eff", Path: "energy.predator.transfer_efficiency", Min: 0.6, Max: 1.0, Default: 0.85},
			{Name: "pred_digest_time", Path: "energy.predator.digest_time", Min: 0.2, Max: 3.0, Default: 0.8},
			// Reproduction
			// prey_repro_thresh locked at 0.50
			{Name: "pred_repro_thresh", Path: "reproduction.pred_threshold", Min: 0.5, Max: 0.95, Default: 0.85},
			{Name: "maturity_age", Path: "reproduction.maturity_age", Min: 2.0, Max: 15.0, Default: 8.0},
			{Name: "prey_cooldown", Path: "reproduction.prey_cooldown", Min: 4.0, Max: 20.0, Default: 8.0},
			{Name: "pred_cooldown", Path: "reproduction.pred_cooldown", Min: 6.0, Max: 20.0, Default: 12.0},
			// cooldown_jitter locked at 3.0, child_energy locked at 0.50
			{Name: "parent_energy_split", Path: "reproduction.parent_energy_split", Min: 0.4, Max: 0.7, Default: 0.55},
			{Name: "spawn_offset", Path: "reproduction.spawn_offset", Min: 5.0, Max: 30.0, Default: 15.0},
			{Name: "heading_jitter", Path: "reproduction.heading_jitter", Min: 0.0, Max: 1.0, Default: 0.25},
			{Name: "pred_density_k", Path: "reproduction.pred_density_k", Min: 0, Max: 300, Default: 0},
			{Name: "newborn_hunt_cooldown", Path: "reproduction.newborn_hunt_cooldown", Min: 0.5, Max: 5.0, Default: 2.0},
			// Population
			{Name: "max_prey", Path: "population.max_prey", Min: 200, Max: 2000, Default: 400},
			{Name: "max_pred", Path: "population.max_pred", Min: 40, Max: 1000, Default: 120},
			// Refugia (prey protection in resource-rich areas)
			{Name: "refugia_strength", Path: "refugia.strength", Min: 0.5, Max: 1.5, Default: 1.0},
			// Particles (resource transport system)
			{Name: "part_spawn_rate", Path: "particles.spawn_rate", Min: 20, Max: 300, Default: 100},
			{Name: "part_initial_mass", Path: "particles.initial_mass", Min: 0.002, Max: 0.05, Default: 0.01},
			{Name: "part_deposit_rate", Path: "particles.deposit_rate", Min: 0.5, Max: 5.0, Default: 2.0},
			{Name: "part_pickup_rate", Path: "particles.pickup_rate", Min: 0.1, Max: 3.0, Default: 0.5},
			{Name: "part_cell_capacity", Path: "particles.cell_capacity", Min: 0.3, Max: 2.0, Default: 1.0},
		},
	}
}

// Dim returns the number of parameters.
func (pv *ParamVector) Dim() int {
	return len(pv.Specs)
}

// DefaultVector returns the default parameter values as a slice.
func (pv *ParamVector) DefaultVector() []float64 {
	v := make([]float64, len(pv.Specs))
	for i, spec := range pv.Specs {
		v[i] = spec.Default
	}
	return v
}

// Normalize converts raw parameter values to [0,1] range.
func (pv *ParamVector) Normalize(raw []float64) []float64 {
	normalized := make([]float64, len(pv.Specs))
	for i, spec := range pv.Specs {
		normalized[i] = (raw[i] - spec.Min) / (spec.Max - spec.Min)
	}
	return normalized
}

// Denormalize converts [0,1] values back to raw parameter values.
func (pv *ParamVector) Denormalize(normalized []float64) []float64 {
	raw := make([]float64, len(pv.Specs))
	for i, spec := range pv.Specs {
		raw[i] = spec.Min + normalized[i]*(spec.Max-spec.Min)
	}
	return raw
}

// Clamp ensures all values are within bounds.
func (pv *ParamVector) Clamp(v []float64) []float64 {
	clamped := make([]float64, len(pv.Specs))
	for i, spec := range pv.Specs {
		val := v[i]
		if val < spec.Min {
			val = spec.Min
		}
		if val > spec.Max {
			val = spec.Max
		}
		clamped[i] = val
	}
	return clamped
}

// ApplyToConfig applies parameter values to a Config struct.
func (pv *ParamVector) ApplyToConfig(cfg *config.Config, values []float64) {
	// Clamp values to ensure they're within bounds
	clamped := pv.Clamp(values)

	// Apply each parameter to the config
	// Order must match Specs order
	i := 0

	// Energy - Prey (base_cost locked)
	cfg.Energy.Prey.BaseCost = 0.005
	cfg.Energy.Prey.MoveCost = clamped[i]; i++
	cfg.Energy.Prey.ForageRate = clamped[i]; i++

	// Energy - Predator
	cfg.Energy.Predator.BaseCost = clamped[i]; i++
	cfg.Energy.Predator.MoveCost = clamped[i]; i++
	cfg.Energy.Predator.BiteReward = clamped[i]; i++
	cfg.Energy.Predator.TransferEfficiency = clamped[i]; i++
	cfg.Energy.Predator.DigestTime = clamped[i]; i++

	// Reproduction (prey_threshold locked)
	cfg.Reproduction.PreyThreshold = 0.50
	cfg.Reproduction.PredThreshold = clamped[i]; i++
	cfg.Reproduction.MaturityAge = clamped[i]; i++
	cfg.Reproduction.PreyCooldown = clamped[i]; i++
	cfg.Reproduction.PredCooldown = clamped[i]; i++
	// cooldown_jitter and child_energy locked
	cfg.Reproduction.CooldownJitter = 3.0
	cfg.Reproduction.ParentEnergySplit = clamped[i]; i++
	cfg.Reproduction.ChildEnergy = 0.50
	cfg.Reproduction.SpawnOffset = clamped[i]; i++
	cfg.Reproduction.HeadingJitter = clamped[i]; i++
	cfg.Reproduction.PredDensityK = clamped[i]; i++
	cfg.Reproduction.NewbornHuntCooldown = clamped[i]; i++

	// Population
	cfg.Population.MaxPrey = int(clamped[i]); i++
	cfg.Population.MaxPred = int(clamped[i]); i++

	// Refugia
	cfg.Refugia.Strength = clamped[i]; i++

	// Particles
	cfg.Particles.SpawnRate = clamped[i]; i++
	cfg.Particles.InitialMass = clamped[i]; i++
	cfg.Particles.DepositRate = clamped[i]; i++
	cfg.Particles.PickupRate = clamped[i]; i++
	cfg.Particles.CellCapacity = clamped[i]
}

// ExtractFromConfig extracts current parameter values from a Config struct.
func (pv *ParamVector) ExtractFromConfig(cfg *config.Config) []float64 {
	return []float64{
		// Energy - Prey (base_cost locked)
		cfg.Energy.Prey.MoveCost,
		cfg.Energy.Prey.ForageRate,
		// Energy - Predator
		cfg.Energy.Predator.BaseCost,
		cfg.Energy.Predator.MoveCost,
		cfg.Energy.Predator.BiteReward,
		cfg.Energy.Predator.TransferEfficiency,
		cfg.Energy.Predator.DigestTime,
		// Reproduction
		// prey_threshold locked
		cfg.Reproduction.PredThreshold,
		cfg.Reproduction.MaturityAge,
		cfg.Reproduction.PreyCooldown,
		cfg.Reproduction.PredCooldown,
		// cooldown_jitter and child_energy locked
		cfg.Reproduction.ParentEnergySplit,
		cfg.Reproduction.SpawnOffset,
		cfg.Reproduction.HeadingJitter,
		cfg.Reproduction.PredDensityK,
		cfg.Reproduction.NewbornHuntCooldown,
		// Population
		float64(cfg.Population.MaxPrey),
		float64(cfg.Population.MaxPred),
		// Refugia
		cfg.Refugia.Strength,
		// Particles
		cfg.Particles.SpawnRate,
		cfg.Particles.InitialMass,
		cfg.Particles.DepositRate,
		cfg.Particles.PickupRate,
		cfg.Particles.CellCapacity,
	}
}

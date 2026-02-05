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
// exp10: locks converged params, adds detritus/diet/density params.
func NewParamVector() *ParamVector {
	return &ParamVector{
		Specs: []ParamSpec{
			// --- Kept from exp9 (energy) ---
			{Name: "prey_move_cost", Path: "energy.prey.move_cost", Min: 0.05, Max: 0.25, Default: 0.12},
			{Name: "pred_move_cost", Path: "energy.predator.move_cost", Min: 0.01, Max: 0.08, Default: 0.025},
			{Name: "pred_bite_reward", Path: "energy.predator.bite_reward", Min: 0.4, Max: 0.8, Default: 0.5},
			{Name: "pred_digest_time", Path: "energy.predator.digest_time", Min: 0.2, Max: 3.0, Default: 0.8},

			// --- Kept from exp9 (reproduction) ---
			{Name: "pred_repro_thresh", Path: "reproduction.pred_threshold", Min: 0.5, Max: 0.95, Default: 0.85},
			{Name: "prey_cooldown", Path: "reproduction.prey_cooldown", Min: 4.0, Max: 20.0, Default: 8.0},
			{Name: "pred_cooldown", Path: "reproduction.pred_cooldown", Min: 6.0, Max: 20.0, Default: 12.0},
			{Name: "parent_energy_split", Path: "reproduction.parent_energy_split", Min: 0.4, Max: 0.7, Default: 0.55},
			{Name: "spawn_offset", Path: "reproduction.spawn_offset", Min: 5.0, Max: 30.0, Default: 15.0},
			{Name: "heading_jitter", Path: "reproduction.heading_jitter", Min: 0.0, Max: 1.0, Default: 0.25},
			{Name: "pred_density_k", Path: "reproduction.pred_density_k", Min: 10, Max: 600, Default: 0},
			{Name: "newborn_hunt_cooldown", Path: "reproduction.newborn_hunt_cooldown", Min: 0.5, Max: 5.0, Default: 2.0},

			// --- Kept from exp9 (refugia) ---
			{Name: "refugia_strength", Path: "refugia.strength", Min: 0.5, Max: 1.5, Default: 1.0},

			// --- Detritus ---
			{Name: "detritus_fraction", Path: "energy.predator.detritus_fraction", Min: 0.0, Max: 0.30, Default: 0.10},
			{Name: "carcass_fraction", Path: "detritus.carcass_fraction", Min: 0.30, Max: 0.90, Default: 0.70},
			{Name: "detritus_decay_rate", Path: "detritus.decay_rate", Min: 0.01, Max: 0.20, Default: 0.05},
			{Name: "detritus_decay_eff", Path: "detritus.decay_efficiency", Min: 0.30, Max: 0.80, Default: 0.50},

			// --- New for exp10 (diet thresholds) ---
			{Name: "grazing_diet_cap", Path: "energy.interpolation.grazing_diet_cap", Min: 0.15, Max: 0.50, Default: 0.30},
			{Name: "hunting_diet_floor", Path: "energy.interpolation.hunting_diet_floor", Min: 0.50, Max: 0.85, Default: 0.70},

			// --- New for exp10 (prey soft cap) ---
			{Name: "prey_density_k", Path: "reproduction.prey_density_k", Min: 50, Max: 500, Default: 200},
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
// Converged params from exp9 are locked to their best values.
func (pv *ParamVector) ApplyToConfig(cfg *config.Config, values []float64) {
	clamped := pv.Clamp(values)
	i := 0

	// --- Locked from exp9 ---
	cfg.Energy.Prey.BaseCost = 0.005
	cfg.Energy.Prey.ForageRate = 0.10
	cfg.Energy.Predator.BaseCost = 0.002
	cfg.Energy.Predator.TransferEfficiency = 1.0
	cfg.Reproduction.PreyThreshold = 0.50
	cfg.Reproduction.MaturityAge = 2.0
	cfg.Reproduction.CooldownJitter = 3.0
	cfg.Reproduction.ChildEnergy = 0.50
	cfg.Population.MaxPrey = 2000
	cfg.Population.MaxPred = 500

	// --- Energy (optimizable) ---
	cfg.Energy.Prey.MoveCost = clamped[i]; i++
	cfg.Energy.Predator.MoveCost = clamped[i]; i++
	cfg.Energy.Predator.BiteReward = clamped[i]; i++
	cfg.Energy.Predator.DigestTime = clamped[i]; i++

	// --- Reproduction (optimizable) ---
	cfg.Reproduction.PredThreshold = clamped[i]; i++
	cfg.Reproduction.PreyCooldown = clamped[i]; i++
	cfg.Reproduction.PredCooldown = clamped[i]; i++
	cfg.Reproduction.ParentEnergySplit = clamped[i]; i++
	cfg.Reproduction.SpawnOffset = clamped[i]; i++
	cfg.Reproduction.HeadingJitter = clamped[i]; i++
	cfg.Reproduction.PredDensityK = clamped[i]; i++
	cfg.Reproduction.NewbornHuntCooldown = clamped[i]; i++

	// --- Refugia ---
	cfg.Refugia.Strength = clamped[i]; i++

	// --- Detritus ---
	cfg.Energy.Predator.DetritusFraction = clamped[i]; i++
	cfg.Detritus.CarcassFraction = clamped[i]; i++
	cfg.Detritus.DecayRate = clamped[i]; i++
	cfg.Detritus.DecayEfficiency = clamped[i]; i++

	// --- New: Diet thresholds ---
	cfg.Energy.Interpolation.GrazingDietCap = clamped[i]; i++
	cfg.Energy.Interpolation.HuntingDietFloor = clamped[i]; i++

	// --- New: Prey soft cap ---
	cfg.Reproduction.PreyDensityK = clamped[i]
}


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
			{Name: "prey_base_cost", Path: "energy.prey.base_cost", Min: 0.005, Max: 0.05, Default: 0.015},
			{Name: "prey_move_cost", Path: "energy.prey.move_cost", Min: 0.05, Max: 0.25, Default: 0.12},
			{Name: "prey_forage_rate", Path: "energy.prey.forage_rate", Min: 0.02, Max: 0.10, Default: 0.045},
			{Name: "pred_base_cost", Path: "energy.predator.base_cost", Min: 0.002, Max: 0.02, Default: 0.008},
			{Name: "pred_move_cost", Path: "energy.predator.move_cost", Min: 0.01, Max: 0.08, Default: 0.025},
			{Name: "pred_bite_reward", Path: "energy.predator.bite_reward", Min: 0.2, Max: 0.8, Default: 0.5},
			{Name: "pred_transfer_eff", Path: "energy.predator.transfer_efficiency", Min: 0.6, Max: 1.0, Default: 0.85},
			{Name: "prey_repro_thresh", Path: "reproduction.prey_threshold", Min: 0.7, Max: 0.95, Default: 0.85},
			{Name: "pred_repro_thresh", Path: "reproduction.pred_threshold", Min: 0.7, Max: 0.95, Default: 0.85},
			{Name: "prey_cooldown", Path: "reproduction.prey_cooldown", Min: 4.0, Max: 15.0, Default: 8.0},
			{Name: "pred_cooldown", Path: "reproduction.pred_cooldown", Min: 6.0, Max: 20.0, Default: 12.0},
			{Name: "max_prey", Path: "population.max_prey", Min: 200, Max: 600, Default: 400},
			{Name: "max_pred", Path: "population.max_pred", Min: 40, Max: 200, Default: 120},
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
	cfg.Energy.Prey.BaseCost = clamped[0]
	cfg.Energy.Prey.MoveCost = clamped[1]
	cfg.Energy.Prey.ForageRate = clamped[2]
	cfg.Energy.Predator.BaseCost = clamped[3]
	cfg.Energy.Predator.MoveCost = clamped[4]
	cfg.Energy.Predator.BiteReward = clamped[5]
	cfg.Energy.Predator.TransferEfficiency = clamped[6]
	cfg.Reproduction.PreyThreshold = clamped[7]
	cfg.Reproduction.PredThreshold = clamped[8]
	cfg.Reproduction.PreyCooldown = clamped[9]
	cfg.Reproduction.PredCooldown = clamped[10]
	cfg.Population.MaxPrey = int(clamped[11])
	cfg.Population.MaxPred = int(clamped[12])
}

// ExtractFromConfig extracts current parameter values from a Config struct.
func (pv *ParamVector) ExtractFromConfig(cfg *config.Config) []float64 {
	return []float64{
		cfg.Energy.Prey.BaseCost,
		cfg.Energy.Prey.MoveCost,
		cfg.Energy.Prey.ForageRate,
		cfg.Energy.Predator.BaseCost,
		cfg.Energy.Predator.MoveCost,
		cfg.Energy.Predator.BiteReward,
		cfg.Energy.Predator.TransferEfficiency,
		cfg.Reproduction.PreyThreshold,
		cfg.Reproduction.PredThreshold,
		cfg.Reproduction.PreyCooldown,
		cfg.Reproduction.PredCooldown,
		float64(cfg.Population.MaxPrey),
		float64(cfg.Population.MaxPred),
	}
}

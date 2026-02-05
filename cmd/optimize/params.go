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
// Updated for unified organism model (no separate prey/predator costs).
func NewParamVector() *ParamVector {
	return &ParamVector{
		Specs: []ParamSpec{
			// --- Unified energy costs ---
			{Name: "base_cost", Path: "energy.base_cost", Min: 0.003, Max: 0.015, Default: 0.007},
			{Name: "move_cost", Path: "energy.move_cost", Min: 0.02, Max: 0.08, Default: 0.035},
			{Name: "accel_cost", Path: "energy.accel_cost", Min: 0.005, Max: 0.025, Default: 0.012},

			// --- Unified feeding ---
			{Name: "feeding_rate", Path: "energy.feeding_rate", Min: 0.03, Max: 0.12, Default: 0.06},
			{Name: "feeding_efficiency", Path: "energy.feeding_efficiency", Min: 0.6, Max: 0.95, Default: 0.80},
			{Name: "cooldown_factor", Path: "energy.cooldown_factor", Min: 0.5, Max: 3.0, Default: 1.5},

			// --- Reproduction ---
			{Name: "pred_repro_thresh", Path: "reproduction.pred_threshold", Min: 0.5, Max: 0.95, Default: 0.85},
			{Name: "prey_cooldown", Path: "reproduction.prey_cooldown", Min: 4.0, Max: 20.0, Default: 8.0},
			{Name: "pred_cooldown", Path: "reproduction.pred_cooldown", Min: 6.0, Max: 20.0, Default: 12.0},
			{Name: "parent_energy_split", Path: "reproduction.parent_energy_split", Min: 0.4, Max: 0.7, Default: 0.55},
			{Name: "spawn_offset", Path: "reproduction.spawn_offset", Min: 5.0, Max: 30.0, Default: 15.0},
			{Name: "heading_jitter", Path: "reproduction.heading_jitter", Min: 0.0, Max: 1.0, Default: 0.25},
			{Name: "pred_density_k", Path: "reproduction.pred_density_k", Min: 10, Max: 600, Default: 0},
			{Name: "newborn_hunt_cooldown", Path: "reproduction.newborn_hunt_cooldown", Min: 0.5, Max: 5.0, Default: 2.0},

			// --- Refugia ---
			{Name: "refugia_strength", Path: "refugia.strength", Min: 0.3, Max: 0.8, Default: 0.5},

			// --- Detritus ---
			{Name: "carcass_fraction", Path: "detritus.carcass_fraction", Min: 0.30, Max: 0.90, Default: 0.70},
			{Name: "detritus_decay_rate", Path: "detritus.decay_rate", Min: 0.01, Max: 0.20, Default: 0.05},
			{Name: "detritus_decay_eff", Path: "detritus.decay_efficiency", Min: 0.30, Max: 0.80, Default: 0.50},

			// --- Archetype metabolic rates ---
			{Name: "grazer_metabolic_rate", Path: "archetypes[0].metabolic_rate", Min: 0.6, Max: 1.5, Default: 1.0},
			{Name: "hunter_metabolic_rate", Path: "archetypes[1].metabolic_rate", Min: 0.4, Max: 1.2, Default: 0.75},

			// --- Prey soft cap ---
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
func (pv *ParamVector) ApplyToConfig(cfg *config.Config, values []float64) {
	clamped := pv.Clamp(values)
	i := 0

	// --- Locked reproduction params ---
	cfg.Reproduction.PreyThreshold = 0.50
	cfg.Reproduction.MaturityAge = 2.0
	cfg.Reproduction.CooldownJitter = 3.0
	cfg.Population.MaxPrey = 2000
	cfg.Population.MaxPred = 500

	// --- Unified energy costs ---
	cfg.Energy.BaseCost = clamped[i]; i++
	cfg.Energy.MoveCost = clamped[i]; i++
	cfg.Energy.AccelCost = clamped[i]; i++

	// --- Unified feeding ---
	cfg.Energy.FeedingRate = clamped[i]; i++
	cfg.Energy.FeedingEfficiency = clamped[i]; i++
	cfg.Energy.CooldownFactor = clamped[i]; i++

	// --- Reproduction ---
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
	cfg.Detritus.CarcassFraction = clamped[i]; i++
	cfg.Detritus.DecayRate = clamped[i]; i++
	cfg.Detritus.DecayEfficiency = clamped[i]; i++

	// --- Archetype metabolic rates ---
	if len(cfg.Archetypes) > 0 {
		cfg.Archetypes[0].MetabolicRate = clamped[i]
	}
	i++
	if len(cfg.Archetypes) > 1 {
		cfg.Archetypes[1].MetabolicRate = clamped[i]
	}
	i++

	// --- Prey soft cap ---
	cfg.Reproduction.PreyDensityK = clamped[i]
}

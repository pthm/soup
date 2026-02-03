package main

import (
	"math"
	"sync"

	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/game"
	"github.com/pthm-cable/soup/telemetry"
)

// FitnessEvaluator runs headless simulations and computes fitness.
type FitnessEvaluator struct {
	params      *ParamVector
	maxTicks    int32
	seeds       []int64
	baseConfig  *config.Config
	statsWindow float64

	// Best run tracking
	mu             sync.Mutex
	bestFitness    float64
	bestHallOfFame *telemetry.HallOfFame
}

// NewFitnessEvaluator creates a new evaluator.
func NewFitnessEvaluator(params *ParamVector, maxTicks int32, seeds []int64, baseCfg *config.Config) *FitnessEvaluator {
	return &FitnessEvaluator{
		params:      params,
		maxTicks:    maxTicks,
		seeds:       seeds,
		baseConfig:  baseCfg,
		statsWindow: 10.0, // 10 seconds per window
		bestFitness: math.Inf(1),
	}
}

// BestHallOfFame returns the hall of fame from the best evaluation.
func (fe *FitnessEvaluator) BestHallOfFame() *telemetry.HallOfFame {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	return fe.bestHallOfFame
}

// Minimum viable population: if either species stays below this for
// extinctionGraceTicks consecutive ticks, it counts as functionally extinct.
const (
	minViablePop        = 3
	extinctionGraceSec  = 30.0  // seconds of grace below minViablePop
)

// runResult holds the results from a single simulation run.
type runResult struct {
	survivalTicks int32 // ticks before functional extinction (or maxTicks if survived)
	hallOfFame    *telemetry.HallOfFame
}

// seedResult holds the result from one seed evaluation.
type seedResult struct {
	fitness    float64
	hallOfFame *telemetry.HallOfFame
}

// Evaluate computes fitness for a parameter vector (lower = better).
// Fitness is negative survival ticks: longer survival = lower (better) fitness.
func (fe *FitnessEvaluator) Evaluate(x []float64) float64 {
	// Run all seeds in parallel
	results := make([]seedResult, len(fe.seeds))
	var wg sync.WaitGroup

	for i, seed := range fe.seeds {
		wg.Add(1)
		go func(idx int, s int64) {
			defer wg.Done()
			result := fe.runSimulation(x, s)
			results[idx] = seedResult{
				fitness:    fe.computeFitness(result),
				hallOfFame: result.hallOfFame,
			}
		}(i, seed)
	}
	wg.Wait()

	// Aggregate results
	var totalFitness float64
	var bestSeedFitness float64 = math.Inf(1)
	var bestSeedHallOfFame *telemetry.HallOfFame

	for _, r := range results {
		totalFitness += r.fitness
		if r.fitness < bestSeedFitness {
			bestSeedFitness = r.fitness
			bestSeedHallOfFame = r.hallOfFame
		}
	}

	avgFitness := totalFitness / float64(len(fe.seeds))

	// Update best tracking
	fe.mu.Lock()
	if avgFitness < fe.bestFitness {
		fe.bestFitness = avgFitness
		fe.bestHallOfFame = bestSeedHallOfFame
	}
	fe.mu.Unlock()

	return avgFitness
}

// runSimulation executes a single headless simulation run.
// Runs until functional extinction or maxTicks, whichever comes first.
func (fe *FitnessEvaluator) runSimulation(x []float64, seed int64) *runResult {
	// Create a fresh config copy and apply parameters
	cfg := fe.copyConfig()
	fe.params.ApplyToConfig(cfg, x)

	// Disable hall of fame reseeding — ecosystems must self-sustain
	cfg.HallOfFame.Enabled = false

	result := &runResult{}

	// Create and run game
	g := game.NewGameWithOptions(game.Options{
		Seed:           seed,
		Headless:       true,
		StatsWindowSec: fe.statsWindow,
		StepsPerUpdate: 1,
		Config:         cfg,
	})

	// Track how long each species has been below minimum viable population
	dt := cfg.Physics.DT
	var preyBelowSec float64
	var predBelowSec float64
	graceTicks := int32(extinctionGraceSec / dt)

	// Let population establish before checking (skip first 5 sim-seconds)
	warmupTicks := int32(5.0 / dt)

	// Run simulation until extinction or max ticks
	for g.Tick() < fe.maxTicks {
		g.UpdateHeadless()

		tick := g.Tick()
		if tick < warmupTicks {
			continue
		}

		prey := g.PreyCount()
		pred := g.PredCount()

		// Hard extinction: either species completely gone
		if prey == 0 || pred == 0 {
			result.survivalTicks = tick
			result.hallOfFame = g.HallOfFame()
			g.Unload()
			return result
		}

		// Functional extinction: species below minimum viable population too long
		if prey < minViablePop {
			preyBelowSec += dt
		} else {
			preyBelowSec = 0
		}

		if pred < minViablePop {
			predBelowSec += dt
		} else {
			predBelowSec = 0
		}

		if preyBelowSec > 0 && int32(preyBelowSec/dt) >= graceTicks {
			result.survivalTicks = tick
			result.hallOfFame = g.HallOfFame()
			g.Unload()
			return result
		}
		if predBelowSec > 0 && int32(predBelowSec/dt) >= graceTicks {
			result.survivalTicks = tick
			result.hallOfFame = g.HallOfFame()
			g.Unload()
			return result
		}
	}

	// Survived the full run
	result.survivalTicks = fe.maxTicks
	result.hallOfFame = g.HallOfFame()
	g.Unload()
	return result
}

// copyConfig creates a deep copy of the base config.
func (fe *FitnessEvaluator) copyConfig() *config.Config {
	// Load fresh defaults and copy base values
	cfg, _ := config.Load("")

	// Copy optimizable fields from base
	cfg.Energy = fe.baseConfig.Energy
	cfg.Reproduction = fe.baseConfig.Reproduction
	cfg.Population = fe.baseConfig.Population

	// Copy other important fields
	cfg.Screen = fe.baseConfig.Screen
	cfg.World = fe.baseConfig.World
	cfg.Physics = fe.baseConfig.Physics
	cfg.Entity = fe.baseConfig.Entity
	cfg.Capabilities = fe.baseConfig.Capabilities
	cfg.Mutation = fe.baseConfig.Mutation
	cfg.Resource = fe.baseConfig.Resource
	cfg.Potential = fe.baseConfig.Potential
	cfg.Neural = fe.baseConfig.Neural
	cfg.Sensors = fe.baseConfig.Sensors
	cfg.GPU = fe.baseConfig.GPU
	cfg.Telemetry = fe.baseConfig.Telemetry
	cfg.Bookmarks = fe.baseConfig.Bookmarks
	cfg.Refugia = fe.baseConfig.Refugia
	cfg.HallOfFame = fe.baseConfig.HallOfFame
	cfg.Particles = fe.baseConfig.Particles
	cfg.Archetypes = fe.baseConfig.Archetypes
	cfg.Clades = fe.baseConfig.Clades

	return cfg
}

// computeFitness calculates the scalar fitness (lower = better).
// Fitness = negative survival ticks. Longer survival = lower fitness = better.
func (fe *FitnessEvaluator) computeFitness(r *runResult) float64 {
	return -float64(r.survivalTicks)
}

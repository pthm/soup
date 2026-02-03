package main

import (
	"math"
	"sync"

	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/game"
	"github.com/pthm-cable/soup/systems"
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
	lastQuality    float64 // quality from most recent Evaluate call
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

// LastQuality returns the quality score from the most recent evaluation.
func (fe *FitnessEvaluator) LastQuality() float64 {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	return fe.lastQuality
}

// Minimum viable population: if either species stays below this for
// extinctionGraceTicks consecutive ticks, it counts as functionally extinct.
const (
	minViablePop        = 3
	extinctionGraceSec  = 30.0  // seconds of grace below minViablePop
)

// runResult holds the results from a single simulation run.
type runResult struct {
	survivalTicks int32                   // ticks before functional extinction (or maxTicks if survived)
	windowStats   []telemetry.WindowStats // collected via StatsCallback each window
	hallOfFame    *telemetry.HallOfFame
}

// seedResult holds the result from one seed evaluation.
type seedResult struct {
	fitness    float64
	quality    float64
	hallOfFame *telemetry.HallOfFame
}

// Evaluate computes fitness for a parameter vector (lower = better).
// Fitness is negative survival ticks: longer survival = lower (better) fitness.
func (fe *FitnessEvaluator) Evaluate(x []float64) float64 {
	// Initialize sensor cache with this eval's config.
	// All seeds share the same parameters, so init once before parallel launch.
	cfg := fe.copyConfig()
	fe.params.ApplyToConfig(cfg, x)
	systems.InitSensorCacheFrom(cfg)

	// Run all seeds in parallel
	results := make([]seedResult, len(fe.seeds))
	var wg sync.WaitGroup

	for i, seed := range fe.seeds {
		wg.Add(1)
		go func(idx int, s int64) {
			defer wg.Done()
			result := fe.runSimulation(x, s)
			quality := fe.computeQuality(result.windowStats)
			results[idx] = seedResult{
				fitness:    fe.computeFitness(result),
				quality:    quality,
				hallOfFame: result.hallOfFame,
			}
		}(i, seed)
	}
	wg.Wait()

	// Aggregate results
	var totalFitness, totalQuality float64
	var bestSeedFitness float64 = math.Inf(1)
	var bestSeedHallOfFame *telemetry.HallOfFame

	for _, r := range results {
		totalFitness += r.fitness
		totalQuality += r.quality
		if r.fitness < bestSeedFitness {
			bestSeedFitness = r.fitness
			bestSeedHallOfFame = r.hallOfFame
		}
	}

	n := float64(len(fe.seeds))
	avgFitness := totalFitness / n

	// Update best tracking
	fe.mu.Lock()
	if avgFitness < fe.bestFitness {
		fe.bestFitness = avgFitness
		fe.bestHallOfFame = bestSeedHallOfFame
	}
	fe.lastQuality = totalQuality / n
	fe.mu.Unlock()

	return avgFitness
}

// runSimulation executes a single headless simulation run.
// Runs until functional extinction or maxTicks, whichever comes first.
func (fe *FitnessEvaluator) runSimulation(x []float64, seed int64) *runResult {
	// Create a fresh config copy and apply parameters
	cfg := fe.copyConfig()
	fe.params.ApplyToConfig(cfg, x)

	result := &runResult{}

	// Create and run game, collecting window stats via callback
	g := game.NewGameWithOptions(game.Options{
		Seed:           seed,
		Headless:       true,
		StatsWindowSec: fe.statsWindow,
		StepsPerUpdate: 1,
		Config:         cfg,
		StatsCallback: func(stats telemetry.WindowStats) {
			result.windowStats = append(result.windowStats, stats)
		},
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
// Formula: -(survivalTicks × (1.0 + 0.2 × quality))
// Survival dominates; quality adds up to 20% bonus to differentiate
// configs with similar survival.
func (fe *FitnessEvaluator) computeFitness(r *runResult) float64 {
	survival := float64(r.survivalTicks)
	quality := fe.computeQuality(r.windowStats)
	return -(survival * (1.0 + 0.2*quality))
}

// Quality component weights.
const (
	qualityWeightRatio     = 0.30
	qualityWeightStability = 0.25
	qualityWeightEnergy    = 0.25
	qualityWeightHunting   = 0.20

	qualityWarmupWindows = 3  // skip first N windows (warmup)
	qualityMinPop        = 3  // exclude windows where either species < this
)

// computeQuality computes ecosystem quality ∈ [0, 1] from window stats.
func (fe *FitnessEvaluator) computeQuality(windows []telemetry.WindowStats) float64 {
	if len(windows) <= qualityWarmupWindows {
		return 0
	}

	// Collect valid windows (past warmup, both species present)
	valid := windows[qualityWarmupWindows:]

	// --- Per-window accumulators ---
	var ratioSum, energySum float64
	var huntSum float64
	var ratioCount, energyCount, huntCount int

	// --- Full time series for stability ---
	preyCounts := make([]float64, 0, len(valid))
	predCounts := make([]float64, 0, len(valid))

	for _, w := range valid {
		if w.PreyCount < qualityMinPop || w.PredCount < qualityMinPop {
			continue
		}

		preyCounts = append(preyCounts, float64(w.PreyCount))
		predCounts = append(predCounts, float64(w.PredCount))

		// 1. Population ratio score
		ratio := float64(w.PreyCount) / float64(w.PredCount)
		logErr := math.Log(ratio / 10.0)
		ratioSum += math.Exp(-logErr * logErr / 1.0)
		ratioCount++

		// 3. Energy health score
		preyH := math.Exp(-math.Pow((w.PreyEnergyP50-0.40)/0.20, 2))
		predH := math.Exp(-math.Pow((w.PredEnergyP50-0.40)/0.20, 2))
		energySum += (preyH + predH) / 2.0
		energyCount++

		// 4. Hunting activity score (only when predators present and bites attempted)
		if w.PredCount > 0 && w.BitesAttempted > 0 {
			hrScore := math.Exp(-math.Pow((w.HitRate-0.15)/0.12, 2))
			bitesPerPred := float64(w.BitesAttempted) / float64(w.PredCount)
			activityScore := 1.0 - math.Exp(-bitesPerPred/3.0)
			huntSum += 0.6*hrScore + 0.4*activityScore
			huntCount++
		}
	}

	// No valid windows → zero quality
	if ratioCount == 0 {
		return 0
	}

	// 1. Population ratio (averaged per valid window)
	ratioScore := ratioSum / float64(ratioCount)

	// 2. Population stability (CV across all valid windows)
	stabilityScore := 0.0
	if len(preyCounts) >= 2 {
		cvPrey := cv(preyCounts)
		cvPred := cv(predCounts)
		stabilityScore = math.Exp(-(cvPrey*cvPrey + cvPred*cvPred))
	}

	// 3. Energy health (averaged per valid window)
	energyScore := 0.0
	if energyCount > 0 {
		energyScore = energySum / float64(energyCount)
	}

	// 4. Hunting activity (averaged per valid window with hunting)
	huntScore := 0.0
	if huntCount > 0 {
		huntScore = huntSum / float64(huntCount)
	}

	quality := qualityWeightRatio*ratioScore +
		qualityWeightStability*stabilityScore +
		qualityWeightEnergy*energyScore +
		qualityWeightHunting*huntScore

	return clamp01(quality)
}

// cv computes the coefficient of variation (std/mean) for a slice of values.
func cv(values []float64) float64 {
	n := float64(len(values))
	if n == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n
	if mean == 0 {
		return 0
	}
	var sqDiff float64
	for _, v := range values {
		d := v - mean
		sqDiff += d * d
	}
	return math.Sqrt(sqDiff/n) / mean
}

// clamp01 clamps x to [0, 1].
func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

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
	mu              sync.Mutex
	bestFitness     float64
	bestHallOfFame  *telemetry.HallOfFame
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

// runResult holds the results from a single simulation run.
type runResult struct {
	extinct       bool
	coexistFrac   float64
	totalPop      []int
	preyPops      []int
	predPops      []int
	killRates     []float64
	birthRates    []float64
	deathRates    []float64
	activeClades  []int
	windowCount   int
	hallOfFame    *telemetry.HallOfFame
}

// Evaluate computes fitness for a parameter vector (lower = better).
func (fe *FitnessEvaluator) Evaluate(x []float64) float64 {
	// Average fitness across multiple seeds
	var totalFitness float64
	var bestSeedFitness float64 = math.Inf(1)
	var bestSeedHallOfFame *telemetry.HallOfFame

	for _, seed := range fe.seeds {
		result := fe.runSimulation(x, seed)
		fitness := fe.computeFitness(result)
		totalFitness += fitness

		if fitness < bestSeedFitness {
			bestSeedFitness = fitness
			bestSeedHallOfFame = result.hallOfFame
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
func (fe *FitnessEvaluator) runSimulation(x []float64, seed int64) *runResult {
	// Create a fresh config copy and apply parameters
	cfg := fe.copyConfig()
	fe.params.ApplyToConfig(cfg, x)

	// Initialize config globally (required by game)
	config.MustInit("")
	*config.Cfg() = *cfg

	result := &runResult{}

	// Callback to collect stats
	var stats []telemetry.WindowStats
	callback := func(s telemetry.WindowStats) {
		stats = append(stats, s)
	}

	// Create and run game
	g := game.NewGameWithOptions(game.Options{
		Seed:           seed,
		Headless:       true,
		StatsWindowSec: fe.statsWindow,
		StepsPerUpdate: 1,
		StatsCallback:  callback,
	})

	// Run simulation
	for g.Tick() < fe.maxTicks {
		g.UpdateHeadless()

		// Check for extinction
		// We need to check via the stats callback
	}

	// Capture hall of fame before unloading
	result.hallOfFame = g.HallOfFame()

	g.Unload()

	// Process collected stats
	result.windowCount = len(stats)
	if result.windowCount == 0 {
		result.extinct = true
		return result
	}

	for _, s := range stats {
		// Check for extinction
		if s.PreyCount == 0 || s.PredCount == 0 {
			result.extinct = true
		}

		result.totalPop = append(result.totalPop, s.PreyCount+s.PredCount)
		result.preyPops = append(result.preyPops, s.PreyCount)
		result.predPops = append(result.predPops, s.PredCount)
		result.killRates = append(result.killRates, s.KillRate)
		result.birthRates = append(result.birthRates, float64(s.PreyBirths+s.PredBirths))
		result.deathRates = append(result.deathRates, float64(s.PreyDeaths+s.PredDeaths))
		result.activeClades = append(result.activeClades, s.ActiveClades)
	}

	// Calculate coexistence fraction
	coexistCount := 0
	for i := range stats {
		if stats[i].PreyCount > 0 && stats[i].PredCount > 0 {
			coexistCount++
		}
	}
	result.coexistFrac = float64(coexistCount) / float64(result.windowCount)

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
func (fe *FitnessEvaluator) computeFitness(r *runResult) float64 {
	// 1. Extinction penalty
	if r.extinct {
		return 1e6
	}

	fitness := 0.0

	// 2. Coexistence penalty: penalize if <90% of windows have both species
	if r.coexistFrac < 0.9 {
		fitness += 100.0 * (0.9 - r.coexistFrac)
	}

	// 3. Population band penalty: target 200-500 total
	meanPop := mean(r.totalPop)
	if meanPop < 200 {
		fitness += 0.1 * (200 - meanPop)
	} else if meanPop > 500 {
		fitness += 0.1 * (meanPop - 500)
	}

	// 4. CV stability: target CV 0.1-0.35 (not stagnant, not chaotic)
	cv := coefficientOfVariation(r.totalPop)
	if cv < 0.1 {
		// Too stagnant
		fitness += 50.0 * (0.1 - cv)
	} else if cv > 0.35 {
		// Too chaotic
		fitness += 50.0 * (cv - 0.35)
	}

	// 5. Activity penalty: penalize zero birth/death rates
	meanBirths := meanFloat(r.birthRates)
	meanDeaths := meanFloat(r.deathRates)
	if meanBirths < 1 {
		fitness += 20.0
	}
	if meanDeaths < 1 {
		fitness += 20.0
	}

	// 6. Hunting success: penalize kill rate < 0.1
	meanKillRate := meanFloat(r.killRates)
	if meanKillRate < 0.1 {
		fitness += 30.0 * (0.1 - meanKillRate)
	}

	// 7. Diversity: penalize if ActiveClades < 3
	meanClades := mean(r.activeClades)
	if meanClades < 3 {
		fitness += 10.0 * (3 - meanClades)
	}

	return fitness
}

// mean calculates the mean of an int slice.
func mean(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0
	for _, v := range vals {
		sum += v
	}
	return float64(sum) / float64(len(vals))
}

// meanFloat calculates the mean of a float64 slice.
func meanFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

// coefficientOfVariation calculates CV = stddev / mean.
func coefficientOfVariation(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := mean(vals)
	if m == 0 {
		return 0
	}

	// Calculate variance
	var sumSq float64
	for _, v := range vals {
		diff := float64(v) - m
		sumSq += diff * diff
	}
	variance := sumSq / float64(len(vals))
	stddev := math.Sqrt(variance)

	return stddev / m
}

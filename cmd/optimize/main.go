// Package main provides CMA-ES optimization for finding simulation parameters
// that produce stable predator-prey ecosystems.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gonum.org/v1/gonum/optimize"

	"github.com/pthm-cable/soup/config"
)

// formatDuration formats a duration as HH:MM:SS or MM:SS for shorter durations.
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	return fmt.Sprintf("%dm%02ds", m, s)
}

func main() {
	// CLI flags
	configPath := flag.String("config", "", "Base config YAML file (empty = use defaults)")
	maxTicks := flag.Int("max-ticks", 3000000, "Maximum simulation duration in ticks (cap)")
	seeds := flag.Int("seeds", 3, "Number of seeds per evaluation")
	maxEvals := flag.Int("max-evals", 200, "Maximum number of evaluations")
	population := flag.Int("population", 0, "CMA-ES population size (0 = auto)")
	outputDir := flag.String("output", "", "Output directory for results")
	flag.Parse()

	if *outputDir == "" {
		log.Fatal("--output is required")
	}

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("failed to create output directory: %v", err)
	}

	// Load base config
	if err := config.Init(*configPath); err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	baseCfg := config.Cfg()

	// Create parameter vector
	params := NewParamVector()

	// Generate seeds for evaluation
	evalSeeds := make([]int64, *seeds)
	for i := range evalSeeds {
		evalSeeds[i] = int64(i*1000 + 42)
	}

	// Create fitness evaluator
	evaluator := NewFitnessEvaluator(params, int32(*maxTicks), evalSeeds, baseCfg)

	// Set up CMA-ES
	dim := params.Dim()
	initX := params.Normalize(params.DefaultVector())

	// Create optimization problem
	problem := optimize.Problem{
		Func: func(x []float64) float64 {
			// Denormalize to get raw parameter values
			raw := params.Denormalize(x)
			return evaluator.Evaluate(raw)
		},
	}

	// CMA-ES settings
	settings := &optimize.Settings{
		FuncEvaluations: *maxEvals,
		Concurrent:      0, // Sequential evaluation
	}

	// Population size
	popSize := *population
	if popSize == 0 {
		// Auto-size: 4 + floor(3*ln(n))
		popSize = 4 + int(3.0*float64(dim)/2.0)
	}

	method := &optimize.CmaEsChol{
		InitStepSize: 0.3,
		Population:   popSize,
	}

	// Open log file
	logPath := filepath.Join(*outputDir, "optimize_log.csv")
	logFile, err := os.Create(logPath)
	if err != nil {
		log.Fatalf("failed to create log file: %v", err)
	}
	defer logFile.Close()

	logWriter := csv.NewWriter(logFile)
	defer logWriter.Flush()

	// Write header
	header := []string{"eval", "fitness"}
	for _, spec := range params.Specs {
		header = append(header, spec.Name)
	}
	logWriter.Write(header)

	// Track evaluations and timing
	evalCount := 0
	var bestFitness float64 = 1e9
	var bestParams []float64
	startTime := time.Now()

	// Wrap the function to log evaluations
	originalFunc := problem.Func
	problem.Func = func(x []float64) float64 {
		fitness := originalFunc(x)
		evalCount++

		// Denormalize and clamp to get actual parameter values
		raw := params.Denormalize(x)
		clamped := params.Clamp(raw)
		if fitness < bestFitness {
			bestFitness = fitness
			bestParams = make([]float64, len(clamped))
			copy(bestParams, clamped)
		}

		// Log clamped values to CSV (these are the values actually used)
		row := []string{strconv.Itoa(evalCount), fmt.Sprintf("%.6f", fitness)}
		for _, v := range clamped {
			row = append(row, fmt.Sprintf("%.6f", v))
		}
		logWriter.Write(row)
		logWriter.Flush()

		// Calculate timing
		elapsed := time.Since(startTime)
		avgPerEval := elapsed / time.Duration(evalCount)
		remaining := time.Duration(*maxEvals-evalCount) * avgPerEval

		// Print progress with timing
		// Fitness = -(survivalTicks × (1 + 0.2×quality)), so extract survival estimate
		quality := evaluator.LastQuality()
		survivalSec := -fitness / (1.0 + 0.2*quality) / 60.0 // approximate ticks to sim-seconds
		fmt.Printf("Eval %d/%d: survived=%.0fs quality=%.2f (best=%.0f) | elapsed: %s, ETA: %s\n",
			evalCount, *maxEvals, survivalSec, quality, bestFitness,
			formatDuration(elapsed), formatDuration(remaining))

		return fitness
	}

	// Run optimization
	fmt.Printf("Starting CMA-ES optimization with %d parameters, population=%d, max_evals=%d\n",
		dim, popSize, *maxEvals)
	fmt.Printf("Seeds per evaluation: %d, ticks per run: %d\n", *seeds, *maxTicks)

	result, err := optimize.Minimize(problem, initX, settings, method)
	if err != nil {
		log.Printf("optimization ended: %v", err)
	}

	// Use best params found (may be from any evaluation, not just final)
	if bestParams == nil {
		bestParams = params.Denormalize(result.X)
	}

	totalTime := time.Since(startTime)
	fmt.Printf("\nOptimization complete after %d evaluations in %s\n", evalCount, formatDuration(totalTime))
	fmt.Printf("Best fitness: %.0f\n", bestFitness)

	// Print best parameters
	fmt.Println("\nBest parameters:")
	for i, spec := range params.Specs {
		fmt.Printf("  %s: %.6f\n", spec.Name, bestParams[i])
	}

	// Save best config
	bestCfg, _ := config.Load(*configPath)
	params.ApplyToConfig(bestCfg, bestParams)

	configOutPath := filepath.Join(*outputDir, "best_config.yaml")
	if err := bestCfg.WriteYAML(configOutPath); err != nil {
		log.Printf("failed to write best config: %v", err)
	} else {
		fmt.Printf("\nBest config saved to: %s\n", configOutPath)
	}

	// Save hall of fame from best run
	if hof := evaluator.BestHallOfFame(); hof != nil {
		hofPath := filepath.Join(*outputDir, "hall_of_fame.json")
		hofData, err := json.MarshalIndent(hof, "", "  ")
		if err != nil {
			log.Printf("failed to marshal hall of fame: %v", err)
		} else if err := os.WriteFile(hofPath, hofData, 0644); err != nil {
			log.Printf("failed to write hall of fame: %v", err)
		} else {
			fmt.Printf("Hall of fame saved to: %s\n", hofPath)
		}
	}
}

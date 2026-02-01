package main

import (
	"flag"
	"log/slog"
	"os"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/game"
	"github.com/pthm-cable/soup/systems"
)

func main() {
	// CLI flags
	configPath := flag.String("config", "", "Path to config.yaml (empty = use defaults)")
	headless := flag.Bool("headless", false, "Run without graphics")
	logStats := flag.Bool("log-stats", false, "Output stats via slog")
	statsWindow := flag.Float64("stats-window", 0, "Stats window size in seconds (0 = use config)")
	snapshotDir := flag.String("snapshot-dir", "", "Directory for snapshot files")
	outputDir := flag.String("output-dir", "", "Output directory for CSV logs and config snapshot")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	maxTicks := flag.Int("max-ticks", 0, "Stop after N ticks (0 = unlimited)")
	stepsPerUpdate := flag.Int("steps-per-update", 1, "Simulation ticks per update call (higher = faster headless runs)")

	flag.Parse()

	// Initialize config before anything else
	if err := config.Init(*configPath); err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	cfg := config.Cfg()

	// Initialize cached config values for hot paths
	systems.InitSensorCache()

	// Set up seed
	rngSeed := *seed
	if rngSeed == 0 {
		rngSeed = time.Now().UnixNano()
	}

	// Set up slog (JSON to stdout for structured logging)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	// Use config stats window if not overridden by CLI
	statsWindowSec := cfg.Telemetry.StatsWindow
	if *statsWindow > 0 {
		statsWindowSec = *statsWindow
	}

	// Build game options
	opts := game.Options{
		Seed:           rngSeed,
		LogStats:       *logStats,
		StatsWindowSec: statsWindowSec,
		SnapshotDir:    *snapshotDir,
		OutputDir:      *outputDir,
		Headless:       *headless,
		StepsPerUpdate: *stepsPerUpdate,
	}

	if *headless {
		// Headless mode - pure CPU simulation, no raylib needed
		g := game.NewGameWithOptions(opts)
		defer g.Unload()

		slog.Info("starting headless simulation",
			"seed", rngSeed,
			"stats_window", *statsWindow,
			"max_ticks", *maxTicks,
			"steps_per_update", *stepsPerUpdate,
		)

		for {
			g.UpdateHeadless()

			if *maxTicks > 0 && int(g.Tick()) >= *maxTicks {
				slog.Info("max ticks reached", "tick", g.Tick())
				return
			}
		}
	} else {
		// Graphical mode
		rl.InitWindow(int32(cfg.Screen.Width), int32(cfg.Screen.Height), "Primordial Soup")
		defer rl.CloseWindow()

		rl.SetTargetFPS(int32(cfg.Screen.TargetFPS))

		g := game.NewGameWithOptions(opts)
		defer g.Unload()

		for !rl.WindowShouldClose() {
			g.Update()
			g.Draw()

			if *maxTicks > 0 && int(g.Tick()) >= *maxTicks {
				break
			}
		}
	}
}

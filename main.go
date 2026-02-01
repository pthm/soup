package main

import (
	"flag"
	"io"
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
	logFile := flag.String("log-file", "", "Write logs to file (empty = stdout)")
	statsWindow := flag.Float64("stats-window", 0, "Stats window size in seconds (0 = use config)")
	snapshotDir := flag.String("snapshot-dir", "", "Directory for snapshot files")
	seed := flag.Int64("seed", 0, "RNG seed (0 = time-based)")
	maxTicks := flag.Int("max-ticks", 0, "Stop after N ticks (0 = unlimited)")

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

	// Set up slog
	var logWriter io.Writer = os.Stdout
	if *logFile != "" {
		f, err := os.Create(*logFile)
		if err != nil {
			slog.Error("failed to create log file", "error", err)
			os.Exit(1)
		}
		defer f.Close()
		logWriter = f
	}

	logger := slog.New(slog.NewJSONHandler(logWriter, nil))
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
		Headless:       *headless,
	}

	if *headless {
		// Headless mode - hidden window for GPU access
		rl.SetConfigFlags(rl.FlagWindowHidden)
		rl.InitWindow(int32(cfg.Screen.Width), int32(cfg.Screen.Height), "Primordial Soup (headless)")
		defer rl.CloseWindow()

		g := game.NewGameWithOptions(opts)
		defer g.Unload()

		slog.Info("starting headless simulation",
			"seed", rngSeed,
			"stats_window", *statsWindow,
			"max_ticks", *maxTicks,
		)

		for {
			g.UpdateHeadless()

			if *maxTicks > 0 && int(g.Tick()) >= *maxTicks {
				slog.Info("max ticks reached", "tick", g.Tick())
				break
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

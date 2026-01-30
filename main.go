package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/game"
)

var (
	initialSpeed    = flag.Int("speed", 1, "Initial simulation speed (1-10, or 0 for full/uncapped in headless mode)")
	logInterval     = flag.Int("log", 0, "Log world state every N ticks (0 = disabled)")
	logFile         = flag.String("logfile", "", "Write logs to file instead of stdout")
	perfLog         = flag.Bool("perf", false, "Enable performance logging")
	neuralLog       = flag.Bool("neural", false, "Enable neural evolution logging")
	neuralLogDetail = flag.Bool("neural-detail", false, "Enable detailed neural logging (genomes, individual organisms)")
	headless        = flag.Bool("headless", false, "Run without graphics (for logging/benchmarking)")
	maxTicks        = flag.Int("max-ticks", 0, "Stop after N ticks (0 = run forever, useful with -headless)")
)

func main() {
	flag.Parse()

	// Setup log file if specified
	if *logFile != "" {
		f, err := os.Create(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		game.SetLogWriter(f)
	}

	if *headless {
		runHeadless()
		return
	}

	rl.InitWindow(int32(game.ScreenWidth), int32(game.ScreenHeight), "Primordial Soup")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	cfg := game.DefaultConfig()
	g := game.NewGame(cfg)
	g.SetLogging(*logInterval, *perfLog, *neuralLog, *neuralLogDetail)

	// Apply initial speed
	if *initialSpeed > 0 && *initialSpeed <= 10 {
		g.SetSpeed(*initialSpeed)
	}

	defer g.Unload()

	for !rl.WindowShouldClose() {
		g.Update()
		g.Draw()
	}
}

// runHeadless runs the simulation without graphics for logging/benchmarking.
func runHeadless() {
	game.Logf("Starting headless simulation...")
	if *initialSpeed == 0 {
		game.Logf("  Speed: FULL (uncapped), Max ticks: %d", *maxTicks)
	} else {
		game.Logf("  Speed: %dx, Max ticks: %d", *initialSpeed, *maxTicks)
	}
	if *neuralLog {
		game.Logf("  Neural logging: enabled (detail=%v)", *neuralLogDetail)
	}
	game.Logf("")

	cfg := game.GameConfig{
		Headless: true,
		Width:    game.ScreenWidth,
		Height:   game.ScreenHeight,
	}
	g := game.NewGame(cfg)
	g.SetLogging(*logInterval, *perfLog, *neuralLog, *neuralLogDetail)

	// Apply initial speed (in headless, this is steps per "frame")
	// Speed 0 = full/uncapped (run as many ticks as possible per iteration)
	if *initialSpeed == 0 {
		g.SetSpeed(10000) // Uncapped - run many ticks per iteration
	} else if *initialSpeed > 0 && *initialSpeed <= 10 {
		g.SetSpeed(*initialSpeed)
	}

	startTime := time.Now()
	lastReport := startTime
	reportInterval := 10 * time.Second

	for {
		// Check max ticks
		if *maxTicks > 0 && int(g.Tick()) >= *maxTicks {
			game.Logf("Reached max ticks (%d), stopping.", *maxTicks)
			break
		}

		// Run simulation step
		g.UpdateHeadless()

		// Periodic progress report
		if time.Since(lastReport) >= reportInterval {
			elapsed := time.Since(startTime)
			ticksPerSec := float64(g.Tick()) / elapsed.Seconds()
			game.Logf("[PROGRESS] Tick %d | %.0f ticks/sec | Elapsed: %s",
				g.Tick(), ticksPerSec, elapsed.Round(time.Second))
			lastReport = time.Now()
		}
	}

	elapsed := time.Since(startTime)
	game.Logf("")
	game.Logf("Simulation complete.")
	game.Logf("  Total ticks: %d", g.Tick())
	game.Logf("  Elapsed time: %s", elapsed.Round(time.Millisecond))
	game.Logf("  Average: %.0f ticks/sec", float64(g.Tick())/elapsed.Seconds())
}

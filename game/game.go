package game

import (
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/camera"
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/inspector"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/telemetry"
)

// Game holds the complete game state.
type Game struct {
	world *ecs.World
	rng   *rand.Rand

	// Entity mappers - using the 7 components we need
	entityMapper *ecs.Map7[
		components.Position,
		components.Velocity,
		components.Rotation,
		components.Body,
		components.Energy,
		components.Capabilities,
		components.Organism,
	]
	entityFilter *ecs.Filter7[
		components.Position,
		components.Velocity,
		components.Rotation,
		components.Body,
		components.Energy,
		components.Capabilities,
		components.Organism,
	]

	// Individual component mappers for lookups
	posMap    *ecs.Map1[components.Position]
	velMap    *ecs.Map1[components.Velocity]
	rotMap    *ecs.Map1[components.Rotation]
	bodyMap   *ecs.Map1[components.Body]
	energyMap *ecs.Map1[components.Energy]
	capsMap   *ecs.Map1[components.Capabilities]
	orgMap    *ecs.Map1[components.Organism]

	// Brain storage (per entity by ID)
	brains map[uint32]*neural.FFNN

	// Spatial index
	spatialGrid *systems.SpatialGrid

	// Resource field (static with regeneration)
	resourceField *systems.ResourceField

	// Reusable buffers to avoid per-entity allocations
	neighborBuf []systems.Neighbor         // reused each entity in behavior loop
	inputBuf    [systems.NumInputs]float32 // reused for neural net inputs

	// Parallel processing state
	parallel *parallelState

	// Rendering
	inspector *inspector.Inspector

	// State
	tick           int32
	paused         bool
	nextID         uint32
	nextCladeID    uint64 // Counter for generating unique clade IDs
	aliveCount     int
	deadCount      int
	numHerb        int
	numCarn        int
	stepsPerUpdate int // simulation ticks per update call

	// Energy accounting
	heatLossAccum    float32 // cumulative energy lost to heat (metabolism, inefficiency, decay)
	energyInputAccum float32 // cumulative energy input from resource regeneration

	// Debug state
	debugMode          bool
	debugShowResource  bool
	debugShowPotential bool

	// Dimensions
	screenWidth, screenHeight float32 // Viewport (window) size
	worldWidth, worldHeight   float32 // Simulation world size

	// Camera (nil in headless mode)
	camera *camera.Camera

	// Telemetry
	collector        *telemetry.Collector
	bookmarkDetector *telemetry.BookmarkDetector
	lifetimeTracker  *telemetry.LifetimeTracker
	perfCollector    *telemetry.PerfCollector
	hallOfFame       *telemetry.HallOfFame
	outputManager    *telemetry.OutputManager
	logStats         bool
	snapshotDir      string
	rngSeed          int64
	statsCallback    func(telemetry.WindowStats)

	// Per-game config (nil = use global config.Cfg())
	cfg *config.Config

	// Seed hall of fame for initial brain seeding (nil = random brains)
	seedHallOfFame *telemetry.HallOfFame
}

// config returns the game's config, falling back to global if not set.
func (g *Game) config() *config.Config {
	if g.cfg != nil {
		return g.cfg
	}
	return config.Cfg()
}

// Options configures game behavior.
type Options struct {
	Seed               int64
	LogStats           bool
	StatsWindowSec     float64
	SnapshotDir        string
	Headless           bool
	StepsPerUpdate     int                            // simulation ticks per update call (1+), 0 = use default (1)
	OutputDir          string                         // output directory for CSV logs and config snapshot
	StatsCallback      func(telemetry.WindowStats)    // called after each window flush
	Config             *config.Config                 // optional per-game config (nil = use global)
	SeedHallOfFamePath string                         // path to hall_of_fame.json for seeding initial brains
}

// NewGameWithOptions creates a new game instance with the given options.
func NewGameWithOptions(opts Options) *Game {
	world := ecs.NewWorld()

	// Use per-game config if provided, otherwise fall back to global
	cfg := opts.Config
	if cfg == nil {
		cfg = config.Cfg()
	}

	stepsPerUpdate := opts.StepsPerUpdate
	if stepsPerUpdate < 1 {
		stepsPerUpdate = 1
	}

	g := &Game{
		world:          world,
		rng:            rand.New(rand.NewSource(opts.Seed)),
		screenWidth:    cfg.Derived.ScreenW32,
		screenHeight:   cfg.Derived.ScreenH32,
		worldWidth:     cfg.Derived.WorldW32,
		worldHeight:    cfg.Derived.WorldH32,
		stepsPerUpdate: stepsPerUpdate,
		brains: make(map[uint32]*neural.FFNN),
		entityMapper: ecs.NewMap7[
			components.Position,
			components.Velocity,
			components.Rotation,
			components.Body,
			components.Energy,
			components.Capabilities,
			components.Organism,
		](world),
		entityFilter: ecs.NewFilter7[
			components.Position,
			components.Velocity,
			components.Rotation,
			components.Body,
			components.Energy,
			components.Capabilities,
			components.Organism,
		](world),
		posMap:    ecs.NewMap1[components.Position](world),
		velMap:    ecs.NewMap1[components.Velocity](world),
		rotMap:    ecs.NewMap1[components.Rotation](world),
		bodyMap:   ecs.NewMap1[components.Body](world),
		energyMap: ecs.NewMap1[components.Energy](world),
		capsMap:   ecs.NewMap1[components.Capabilities](world),
		orgMap:    ecs.NewMap1[components.Organism](world),

		// Telemetry
		logStats:      opts.LogStats,
		snapshotDir:   opts.SnapshotDir,
		rngSeed:       opts.Seed,
		statsCallback: opts.StatsCallback,

		// Per-game config
		cfg: cfg,
	}

	// Initialize telemetry
	g.collector = telemetry.NewCollector(opts.StatsWindowSec, cfg.Derived.DT32)
	g.bookmarkDetector = telemetry.NewBookmarkDetector(cfg.Telemetry.BookmarkHistorySize)
	g.lifetimeTracker = telemetry.NewLifetimeTracker()
	g.perfCollector = telemetry.NewPerfCollector(cfg.Telemetry.PerfCollectorWindow)
	if cfg.HallOfFame.Enabled {
		g.hallOfFame = telemetry.NewHallOfFame(cfg.HallOfFame.Size, len(cfg.Archetypes), g.rng)
	}

	// Initialize output manager for CSV logging
	if opts.OutputDir != "" {
		om, err := telemetry.NewOutputManager(opts.OutputDir)
		if err != nil {
			panic("failed to create output manager: " + err.Error())
		}
		g.outputManager = om

		// Write config snapshot
		if err := om.WriteConfig(cfg); err != nil {
			panic("failed to write config: " + err.Error())
		}
	}

	// Spatial grid (uses world dimensions)
	g.spatialGrid = systems.NewSpatialGrid(g.worldWidth, g.worldHeight, float32(cfg.Physics.GridCellSize))

	// Parallel processing
	g.parallel = newParallelState()

	// Resource field (particle-based with mass transport)
	// Compute grid dimensions based on cell size (constant resolution regardless of world size)
	cellSize := float32(cfg.Resource.CellSize)
	gridW := int(g.worldWidth / cellSize)
	gridH := int(g.worldHeight / cellSize)
	if gridW < 1 {
		gridW = 1
	}
	if gridH < 1 {
		gridH = 1
	}
	g.resourceField = systems.NewResourceField(gridW, gridH, g.worldWidth, g.worldHeight, opts.Seed, cfg)

	// Only initialize visual rendering if not headless
	if !opts.Headless {
		// Inspector
		g.inspector = inspector.NewInspector(int32(g.screenWidth), int32(g.screenHeight))

		// Camera (centered on world with 1:1 zoom)
		g.camera = camera.New(g.screenWidth, g.screenHeight, g.worldWidth, g.worldHeight)
	}

	// Load seed hall of fame if provided (for seeding initial brains from prior runs)
	if opts.SeedHallOfFamePath != "" {
		seedHoF, err := telemetry.LoadHallOfFameFromFile(
			opts.SeedHallOfFamePath,
			len(cfg.Archetypes),
			cfg.Derived.ArchetypeIndex,
			g.rng,
		)
		if err != nil {
			panic("failed to load seed hall of fame: " + err.Error())
		}
		g.seedHallOfFame = seedHoF
	}

	// Spawn initial population
	g.spawnInitialPopulation()

	return g
}

// Update runs one or more simulation steps based on speed setting.
func (g *Game) Update() {
	// Handle input
	g.handleInput()

	if g.paused {
		return
	}

	// Run multiple simulation steps based on stepsPerUpdate
	for i := 0; i < g.stepsPerUpdate; i++ {
		g.simulationStep()
	}
}

// simulationStep runs a single tick of the simulation.
func (g *Game) simulationStep() {
	g.perfCollector.StartTick()
	cfg := g.config()

	// 0. Update resource field (regeneration, detritus decay)
	g.perfCollector.StartPhase(telemetry.PhaseResourceField)
	stepResult := g.resourceField.Step(cfg.Derived.DT32, true)
	g.heatLossAccum += stepResult.HeatLoss
	g.energyInputAccum += stepResult.EnergyInput

	// 1. Update spatial grid
	g.perfCollector.StartPhase(telemetry.PhaseSpatialGrid)
	g.updateSpatialGrid()

	// 2. Run behavior (sensors + brains) and physics
	g.perfCollector.StartPhase(telemetry.PhaseBehaviorPhysics)
	g.updateBehaviorAndPhysicsParallel()

	// 3. Handle feeding (predator bites)
	g.perfCollector.StartPhase(telemetry.PhaseFeeding)
	g.updateFeeding()

	// 4. Update energy and check deaths
	g.perfCollector.StartPhase(telemetry.PhaseEnergy)
	g.updateEnergy()

	// 5. Update cooldowns
	g.perfCollector.StartPhase(telemetry.PhaseCooldowns)
	g.updateCooldowns()

	// 6. Handle reproduction
	g.perfCollector.StartPhase(telemetry.PhaseReproduction)
	g.updateReproduction()

	// 7. Cleanup dead entities
	g.perfCollector.StartPhase(telemetry.PhaseCleanup)
	g.cleanupDead()

	// 7b. Reseed from hall of fame if population dropped below threshold
	g.reseedFromHallOfFame()

	// 8. Flush telemetry window if needed
	g.perfCollector.StartPhase(telemetry.PhaseTelemetry)
	g.flushTelemetry()

	g.perfCollector.EndTick()
	g.tick++
}

// Unload releases all resources.
func (g *Game) Unload() {
	// Stop parallel workers first
	g.stopParallelWorkers()

	// Write hall of fame and close output manager
	if g.outputManager != nil {
		if g.hallOfFame != nil {
			g.outputManager.WriteHallOfFame(g.hallOfFame)
		}
		g.outputManager.Close()
	}
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
}

// PreyCount returns the current herbivore population (diet < 0.5).
func (g *Game) PreyCount() int {
	return g.numHerb
}

// PredCount returns the current carnivore population (diet >= 0.5).
func (g *Game) PredCount() int {
	return g.numCarn
}

// UpdateHeadless runs simulation steps without rendering, respecting stepsPerUpdate setting.
func (g *Game) UpdateHeadless() {
	for i := 0; i < g.stepsPerUpdate; i++ {
		g.simulationStep()
	}
}

// ResourceField returns the resource sampler.
func (g *Game) ResourceField() systems.ResourceSampler {
	return g.resourceField
}

// PerfStats returns the current performance statistics.
func (g *Game) PerfStats() telemetry.PerfStats {
	return g.perfCollector.Stats()
}

// Camera returns the camera (nil in headless mode).
func (g *Game) Camera() *camera.Camera {
	return g.camera
}

// WorldSize returns the world dimensions.
func (g *Game) WorldSize() (width, height float32) {
	return g.worldWidth, g.worldHeight
}

// HallOfFame returns the hall of fame for optimization output.
func (g *Game) HallOfFame() *telemetry.HallOfFame {
	return g.hallOfFame
}

// HeatLossAccum returns the cumulative energy lost to heat.
func (g *Game) HeatLossAccum() float32 {
	return g.heatLossAccum
}


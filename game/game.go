package game

import (
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/camera"
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/inspector"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/telemetry"
)

// ScreenWidth returns the configured screen width.
func ScreenWidth() int { return config.Cfg().Screen.Width }

// ScreenHeight returns the configured screen height.
func ScreenHeight() int { return config.Cfg().Screen.Height }

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

	// Resource field (particle-based with mass transport)
	resourceField *systems.ParticleResourceField

	// Reusable buffers to avoid per-entity allocations
	neighborBuf []systems.Neighbor         // reused each entity in behavior loop
	inputBuf    [systems.NumInputs]float32 // reused for neural net inputs

	// Parallel processing state
	parallel *parallelState

	// Rendering
	backgroundRenderer *renderer.BackgroundRenderer
	lightRenderer      *renderer.LightRenderer
	particleRenderer   *renderer.ParticleRenderer
	inspector          *inspector.Inspector

	// State
	tick       int32
	paused     bool
	nextID     uint32
	aliveCount int
	deadCount  int
	numPrey    int
	numPred    int
	stepsPerUpdate int // simulation ticks per update call

	// Debug state
	debugMode         bool
	debugShowResource bool

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
	logStats         bool
	snapshotDir      string
	rngSeed          int64
}

// Options configures game behavior.
type Options struct {
	Seed           int64
	LogStats       bool
	StatsWindowSec float64
	SnapshotDir    string
	Headless       bool
	StepsPerUpdate int // simulation ticks per update call (1+), 0 = use default (1)
}

// NewGame creates a new game instance with default options.
func NewGame() *Game {
	return NewGameWithOptions(Options{
		Seed:           42,
		LogStats:       false,
		StatsWindowSec: 10.0,
		SnapshotDir:    "",
	})
}

// NewGameWithOptions creates a new game instance with the given options.
func NewGameWithOptions(opts Options) *Game {
	world := ecs.NewWorld()

	cfg := config.Cfg()

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
		logStats:    opts.LogStats,
		snapshotDir: opts.SnapshotDir,
		rngSeed:     opts.Seed,
	}

	// Initialize telemetry
	g.collector = telemetry.NewCollector(opts.StatsWindowSec, cfg.Derived.DT32)
	g.bookmarkDetector = telemetry.NewBookmarkDetector(cfg.Telemetry.BookmarkHistorySize)
	g.lifetimeTracker = telemetry.NewLifetimeTracker()
	g.perfCollector = telemetry.NewPerfCollector(cfg.Telemetry.PerfCollectorWindow)
	if cfg.HallOfFame.Enabled {
		g.hallOfFame = telemetry.NewHallOfFame(cfg.HallOfFame.Size, g.rng)
	}

	// Spatial grid (uses world dimensions)
	g.spatialGrid = systems.NewSpatialGrid(g.worldWidth, g.worldHeight, float32(cfg.Physics.GridCellSize))

	// Parallel processing
	g.parallel = newParallelState()

	// Resource field (particle-based with mass transport)
	// Compute grid dimensions to maintain square cells matching world aspect ratio
	baseGridSize := cfg.GPU.ResourceTextureSize
	var gridW, gridH int
	if g.worldWidth >= g.worldHeight {
		// Landscape: scale width up
		gridH = baseGridSize
		gridW = int(float32(baseGridSize) * g.worldWidth / g.worldHeight)
	} else {
		// Portrait: scale height up
		gridW = baseGridSize
		gridH = int(float32(baseGridSize) * g.worldHeight / g.worldWidth)
	}
	g.resourceField = systems.NewParticleResourceField(gridW, gridH, g.worldWidth, g.worldHeight, opts.Seed)

	// Only initialize visual rendering if not headless
	if !opts.Headless {
		// Background noise texture (teal sea color with soft noise)
		g.backgroundRenderer = renderer.NewBackgroundRenderer(int32(g.screenWidth), int32(g.screenHeight), 8, 45, 60)

		// Light background (renders potential field as sunlight)
		g.lightRenderer = renderer.NewLightRenderer(int32(g.screenWidth), int32(g.screenHeight))

		// Particle renderer for floating resource particles
		g.particleRenderer = renderer.NewParticleRenderer(int32(g.screenWidth), int32(g.screenHeight))

		// Inspector
		g.inspector = inspector.NewInspector(int32(g.screenWidth), int32(g.screenHeight))

		// Camera (centered on world with 1:1 zoom)
		g.camera = camera.New(g.screenWidth, g.screenHeight, g.worldWidth, g.worldHeight)
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
	cfg := config.Cfg()

	// 0. Update resource field (regrowth, diffusion, capacity evolution / particle dynamics)
	g.resourceField.Step(cfg.Derived.DT32, true)

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

	// 8. Flush telemetry window if needed
	g.perfCollector.StartPhase(telemetry.PhaseTelemetry)
	g.flushTelemetry()

	g.perfCollector.EndTick()
	g.tick++
}

// Unload releases all resources.
func (g *Game) Unload() {
	if g.backgroundRenderer != nil {
		g.backgroundRenderer.Unload()
	}
	if g.lightRenderer != nil {
		g.lightRenderer.Unload()
	}
	if g.particleRenderer != nil {
		g.particleRenderer.Unload()
	}
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
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

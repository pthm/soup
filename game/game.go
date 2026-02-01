package game

import (
	"math/rand"

	"github.com/mlange-42/ark/ecs"

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

	// Resource field (GPU-accelerated, O(1) sampling)
	resourceField    systems.ResourceSampler
	gpuResourceField *renderer.GPUResourceField

	// Reusable buffers to avoid per-entity allocations
	neighborBuf []systems.Neighbor         // reused each entity in behavior loop
	inputBuf    [systems.NumInputs]float32 // reused for neural net inputs

	// Rendering
	water     *renderer.WaterBackground
	flow      *renderer.GPUFlowField
	inspector *inspector.Inspector

	// State
	tick       int32
	paused     bool
	nextID     uint32
	aliveCount int
	deadCount  int
	numPrey    int
	numPred    int
	speed      int // simulation speed multiplier (1-10)

	// Debug state
	debugMode         bool
	debugShowResource bool

	// Window dimensions
	width, height float32

	// Telemetry
	collector        *telemetry.Collector
	bookmarkDetector *telemetry.BookmarkDetector
	lifetimeTracker  *telemetry.LifetimeTracker
	perfCollector    *telemetry.PerfCollector
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

	g := &Game{
		world:  world,
		rng:    rand.New(rand.NewSource(opts.Seed)),
		width:  float32(cfg.Screen.Width),
		height: float32(cfg.Screen.Height),
		speed:  1,
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

	// Spatial grid
	g.spatialGrid = systems.NewSpatialGrid(g.width, g.height, float32(cfg.Physics.GridCellSize))

	// GPU resource field (O(1) sampling via precomputed texture)
	g.gpuResourceField = renderer.NewGPUResourceField(g.width, g.height)
	g.gpuResourceField.Initialize(0) // Static field at time=0
	g.resourceField = g.gpuResourceField

	// Only initialize visual rendering if not headless
	if !opts.Headless {
		// GPU flow field (visual only, not used in simulation)
		g.flow = renderer.NewGPUFlowField(g.width, g.height)

		// Water background
		g.water = renderer.NewWaterBackground(int32(g.width), int32(g.height))

		// Inspector
		g.inspector = inspector.NewInspector(int32(g.width), int32(g.height))
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

	// Run multiple simulation steps based on speed
	for i := 0; i < g.speed; i++ {
		g.simulationStep()
	}
}

// simulationStep runs a single tick of the simulation.
func (g *Game) simulationStep() {
	g.perfCollector.StartTick()

	// Update flow field (only if available - nil in headless mode initially)
	g.perfCollector.StartPhase(telemetry.PhaseFlowField)
	if g.flow != nil {
		g.flow.Update(g.tick, float32(g.tick)*0.01)
	}

	// 1. Update spatial grid
	g.perfCollector.StartPhase(telemetry.PhaseSpatialGrid)
	g.updateSpatialGrid()

	// 2. Run behavior (sensors + brains) and physics
	g.perfCollector.StartPhase(telemetry.PhaseBehaviorPhysics)
	g.updateBehaviorAndPhysics()

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
	if g.water != nil {
		g.water.Unload()
	}
	if g.flow != nil {
		g.flow.Unload()
	}
	if g.gpuResourceField != nil {
		g.gpuResourceField.Unload()
	}
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
}

// UpdateHeadless runs a single simulation step without rendering.
func (g *Game) UpdateHeadless() {
	g.simulationStep()
}

// ResourceField returns the resource sampler.
func (g *Game) ResourceField() systems.ResourceSampler {
	return g.resourceField
}

// PerfStats returns the current performance statistics.
func (g *Game) PerfStats() telemetry.PerfStats {
	return g.perfCollector.Stats()
}

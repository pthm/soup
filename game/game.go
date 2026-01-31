package game

import (
	"math/rand"
	"time"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/ui"
)

// Game holds the complete game state.
type Game struct {
	world           *ecs.World
	bounds          systems.Bounds
	physics         *systems.PhysicsSystem
	energy          *systems.EnergySystem
	cells           *systems.CellSystem
	behavior        *systems.BehaviorSystem
	flowField       *systems.FlowFieldSystem
	flowRenderer    *renderer.FlowRenderer
	gpuFlowField    *renderer.GPUFlowField // GPU-accelerated flow field
	waterBackground *renderer.WaterBackground
	sunRenderer     *renderer.SunRenderer
	light           renderer.LightState
	tick            int32
	paused          bool
	stepsPerFrame   int
	perf            *PerfStats

	// Terrain
	terrain         *systems.TerrainSystem
	terrainRenderer *renderer.TerrainRenderer

	// New systems
	shadowMap        *systems.ShadowMap
	floraSystem      *systems.FloraSystem
	feeding          *systems.FeedingSystem
	spores           *systems.SporeSystem
	breeding         *systems.BreedingSystem
	particles        *systems.ParticleSystem
	particleRenderer *renderer.ParticleRenderer
	allocation       *systems.AllocationSystem
	spatialGrid      *systems.SpatialGrid

	// Neural evolution
	neuralConfig   *neural.Config
	genomeIDGen    *neural.GenomeIDGenerator
	speciesManager *neural.SpeciesManager

	// Display settings
	showSpeciesColors bool // Toggle species coloring with 'S' key
	showNeuralStats   bool // Toggle neural stats panel with 'N' key

	// Selection state
	selectedEntity ecs.Entity
	hasSelection   bool

	// Mappers for creating entities with components (fauna only - flora uses FloraSystem)
	faunaMapper *ecs.Map5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Fauna]

	// Neural component mappers (for adding neural components to fauna entities)
	neuralGenomeMap *ecs.Map[components.NeuralGenome]
	brainMap        *ecs.Map[components.Brain]

	// Filters for querying (fauna only - flora uses FloraSystem)
	faunaFilter     *ecs.Filter3[components.Position, components.Organism, components.Fauna]
	faunaCellFilter *ecs.Filter4[components.Position, components.Organism, components.CellBuffer, components.Fauna]
	allOrgFilter    *ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]

	// UI components (descriptor-driven)
	uiHUD            *ui.HUD
	uiInspector      *ui.Inspector
	uiNeuralStats    *ui.NeuralStatsPanel
	uiPerfPanel      *ui.PerfPanel
	uiSystemRegistry *systems.SystemRegistry
	uiOverlays       *ui.OverlayRegistry
	uiControlsPanel  *ui.ControlsPanel

	// Logging configuration
	logInterval     int
	perfLog         bool
	neuralLog       bool
	neuralLogDetail bool
}

// NewGame creates a new game instance with the given configuration.
func NewGame(cfg GameConfig) *Game {
	world := ecs.NewWorld()

	bounds := systems.Bounds{
		Width:  float32(cfg.Width),
		Height: float32(cfg.Height),
	}

	// Create shadow map first as other systems depend on it
	shadowMap := systems.NewShadowMap(float32(cfg.Width), float32(cfg.Height))

	// Create terrain
	terrain := systems.NewTerrainSystem(float32(cfg.Width), float32(cfg.Height), time.Now().UnixNano())

	// Neural evolution config
	neuralConfig := neural.DefaultConfig()
	genomeIDGen := neural.NewGenomeIDGenerator()

	g := &Game{
		world:         world,
		bounds:        bounds,
		physics:       systems.NewPhysicsSystemWithTerrain(world, bounds, terrain),
		energy:        systems.NewEnergySystem(world, shadowMap),
		cells:         systems.NewCellSystem(world),
		behavior:      systems.NewBehaviorSystem(world, shadowMap, terrain),
		flowField:     systems.NewFlowFieldSystemWithTerrain(bounds, 3000, terrain),
		light:         renderer.LightState{PosX: 0.5, PosY: -0.15, Intensity: 1.0},
		stepsPerFrame: 1,
		perf:          NewPerfStats(),

		// Terrain
		terrain: terrain,

		// Systems
		shadowMap:   shadowMap,
		feeding:     systems.NewFeedingSystem(world),
		spores:      systems.NewSporeSystemWithTerrain(bounds, terrain),
		breeding:    systems.NewBreedingSystem(world, neuralConfig.NEAT, genomeIDGen, neuralConfig.CPPN),
		particles:   systems.NewParticleSystem(),
		allocation:  systems.NewAllocationSystem(world),
		spatialGrid: systems.NewSpatialGrid(float32(cfg.Width), float32(cfg.Height)),

		// Neural evolution
		neuralConfig:   neuralConfig,
		genomeIDGen:    genomeIDGen,
		speciesManager: neural.NewSpeciesManager(neuralConfig.NEAT),

		// Display settings
		showSpeciesColors: false,
		showNeuralStats:   false,

		faunaMapper:     ecs.NewMap5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Fauna](world),
		faunaFilter:     ecs.NewFilter3[components.Position, components.Organism, components.Fauna](world),
		faunaCellFilter: ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Fauna](world),
		allOrgFilter:    ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](world),

		// Neural component mappers
		neuralGenomeMap: ecs.NewMap[components.NeuralGenome](world),
		brainMap:        ecs.NewMap[components.Brain](world),

		// System registry (for consistent naming in logs)
		uiSystemRegistry: systems.NewSystemRegistry(),
	}

	// GPU compute resources (always created - headless uses hidden window)
	g.gpuFlowField = renderer.NewGPUFlowField(
		float32(cfg.Width), float32(cfg.Height),
		terrain.DistanceToSolid, // Pass terrain distance function
	)
	g.flowField.SetGPUSampler(g.gpuFlowField)

	// Visual renderers and UI - only for graphics mode (not headless)
	if !cfg.Headless {
		g.flowRenderer = renderer.NewFlowRenderer(int32(cfg.Width), int32(cfg.Height), 0.08)
		g.waterBackground = renderer.NewWaterBackground(int32(cfg.Width), int32(cfg.Height))
		g.sunRenderer = renderer.NewSunRenderer(int32(cfg.Width), int32(cfg.Height))
		g.terrainRenderer = renderer.NewTerrainRenderer(int32(cfg.Width), int32(cfg.Height))
		g.particleRenderer = renderer.NewParticleRenderer()

		// UI components
		g.uiHUD = ui.NewHUD()
		g.uiInspector = ui.NewInspector(int32(cfg.Width-330), 10, 320)
		g.uiNeuralStats = ui.NewNeuralStatsPanel(10, 100, 280, 220)
		g.uiPerfPanel = ui.NewPerfPanel(int32(cfg.Width-200), 10)
		g.uiOverlays = ui.NewOverlayRegistry()
		g.uiControlsPanel = ui.NewControlsPanel(10, 100, 200)
	}

	// Create FloraSystem after other systems are initialized (needs shadowMap, terrain, flowField)
	g.floraSystem = systems.NewFloraSystem(bounds, terrain, shadowMap, g.flowField)

	// Wire up FloraSystem to systems that need it
	g.feeding.SetFloraSystem(g.floraSystem)
	g.feeding.SetSpatialGrid(g.spatialGrid)
	g.behavior.SetFloraSystem(g.floraSystem)
	g.allocation.SetFloraSystem(g.floraSystem)

	g.seedUniverse()

	return g
}

// SetLogging configures logging options.
func (g *Game) SetLogging(logInterval int, perfLog, neuralLog, neuralLogDetail bool) {
	g.logInterval = logInterval
	g.perfLog = perfLog
	g.neuralLog = neuralLog
	g.neuralLogDetail = neuralLogDetail

	// Enable behavior subsystem profiling when perf logging is on
	g.behavior.SetPerfEnabled(perfLog)
}

// SetSpeed sets the simulation speed (steps per frame).
func (g *Game) SetSpeed(speed int) {
	if speed > 0 {
		g.stepsPerFrame = speed
	}
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
}

// Unload releases all renderer resources.
func (g *Game) Unload() {
	if g.flowRenderer != nil {
		g.flowRenderer.Unload()
	}
	if g.waterBackground != nil {
		g.waterBackground.Unload()
	}
	if g.gpuFlowField != nil {
		g.gpuFlowField.Unload()
	}
}

// seedUniverse populates the world with initial organisms.
func (g *Game) seedUniverse() {
	// Seed initial flora directly using FloraSystem
	// Rooted flora on terrain and seafloor
	for i := 0; i < 60; i++ {
		x := rand.Float32() * g.bounds.Width
		// Place on seafloor or terrain
		y := g.bounds.Height - 4 - rand.Float32()*20
		g.floraSystem.AddRooted(x, y, 80+rand.Float32()*40)
	}

	// Floating flora in water column
	for i := 0; i < 40; i++ {
		x := rand.Float32() * g.bounds.Width
		y := rand.Float32()*g.bounds.Height*0.7 + 50
		g.floraSystem.AddFloating(x, y, 60+rand.Float32()*40)
	}

	// Also spawn some spores to demonstrate the spore system
	for i := 0; i < 20; i++ {
		g.spores.SpawnSpore(
			rand.Float32()*g.bounds.Width,
			rand.Float32()*g.bounds.Height*0.5,
			true, // parentRooted
		)
	}

	// Create fauna with neural brains (CPPN morphology + evolved behavior)
	// Diet is now derived from cell DigestiveSpectrum, not from organism traits
	// Higher initial energy gives untrained brains more time to find food
	for i := 0; i < 85; i++ {
		g.createInitialNeuralOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			// CPPN determines cell capabilities
			200,
		)
	}
}

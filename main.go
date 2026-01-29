package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/ui"
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
	logWriter       *os.File
)

// PerfStats tracks execution time for each system
type PerfStats struct {
	samples    map[string][]time.Duration
	maxSamples int
}

func NewPerfStats() *PerfStats {
	return &PerfStats{
		samples:    make(map[string][]time.Duration),
		maxSamples: 120, // ~2 seconds of samples at 60fps
	}
}

func (p *PerfStats) Record(name string, d time.Duration) {
	p.samples[name] = append(p.samples[name], d)
	if len(p.samples[name]) > p.maxSamples {
		p.samples[name] = p.samples[name][1:]
	}
}

func (p *PerfStats) Avg(name string) time.Duration {
	s := p.samples[name]
	if len(s) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range s {
		total += d
	}
	return total / time.Duration(len(s))
}

func (p *PerfStats) Total() time.Duration {
	var total time.Duration
	for name := range p.samples {
		total += p.Avg(name)
	}
	return total
}

func (p *PerfStats) SortedNames() []string {
	names := make([]string, 0, len(p.samples))
	for name := range p.samples {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return p.Avg(names[i]) > p.Avg(names[j])
	})
	return names
}

const (
	screenWidth  = 1280
	screenHeight = 800
)

// GrowIntent constants for neural-controlled growth
const (
	GrowIntentThreshold = 0.4  // Minimum intent to trigger growth
	MinGrowthInterval   = 60   // Fastest growth (high intent)
	MaxGrowthInterval   = 300  // Slowest growth (low intent)
)

type Game struct {
	world           *ecs.World
	bounds          systems.Bounds
	physics         *systems.PhysicsSystem
	energy          *systems.EnergySystem
	cells           *systems.CellSystem
	behavior        *systems.BehaviorSystem
	flowField       *systems.FlowFieldSystem
	flowRenderer    *renderer.FlowRenderer
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
	shadowMap   *systems.ShadowMap
	floraSystem *systems.FloraSystem
	feeding     *systems.FeedingSystem
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
	uiHUD             *ui.HUD
	uiInspector       *ui.Inspector
	uiNeuralStats     *ui.NeuralStatsPanel
	uiPerfPanel       *ui.PerfPanel
	uiSystemRegistry  *systems.SystemRegistry
}

func NewGame() *Game {
	world := ecs.NewWorld()

	bounds := systems.Bounds{
		Width:  screenWidth,
		Height: screenHeight,
	}

	// Create shadow map first as other systems depend on it
	shadowMap := systems.NewShadowMap(screenWidth, screenHeight)

	// Create terrain
	terrain := systems.NewTerrainSystem(screenWidth, screenHeight, time.Now().UnixNano())

	// Neural evolution config
	neuralConfig := neural.DefaultConfig()
	genomeIDGen := neural.NewGenomeIDGenerator()

	g := &Game{
		world:           world,
		bounds:          bounds,
		physics:         systems.NewPhysicsSystemWithTerrain(world, bounds, terrain),
		energy:          systems.NewEnergySystem(world, shadowMap),
		cells:           systems.NewCellSystem(world),
		behavior:        systems.NewBehaviorSystem(world, shadowMap, terrain),
		flowField:       systems.NewFlowFieldSystemWithTerrain(bounds, 8000, terrain),
		flowRenderer:    renderer.NewFlowRenderer(screenWidth, screenHeight, 0.08),
		waterBackground: renderer.NewWaterBackground(screenWidth, screenHeight),
		sunRenderer:     renderer.NewSunRenderer(screenWidth, screenHeight),
		light:           renderer.LightState{PosX: 0.5, PosY: -0.15, Intensity: 1.0}, // Static sun at top center
		stepsPerFrame:   1,
		perf:            NewPerfStats(),

		// Terrain
		terrain:         terrain,
		terrainRenderer: renderer.NewTerrainRenderer(screenWidth, screenHeight),

		// New systems
		shadowMap:        shadowMap,
		feeding:          systems.NewFeedingSystem(world),
		spores:           systems.NewSporeSystemWithTerrain(bounds, terrain),
		breeding:         systems.NewBreedingSystem(world, neuralConfig.NEAT, genomeIDGen, neuralConfig.CPPN),
		particles:        systems.NewParticleSystem(),
		allocation:       systems.NewAllocationSystem(world),
		spatialGrid:      systems.NewSpatialGrid(screenWidth, screenHeight),
		particleRenderer: renderer.NewParticleRenderer(),

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
	}

	// Create FloraSystem after other systems are initialized (needs shadowMap, terrain, flowField)
	g.floraSystem = systems.NewFloraSystem(bounds, terrain, shadowMap, g.flowField)

	// Wire up FloraSystem to systems that need it
	g.feeding.SetFloraSystem(g.floraSystem)
	g.behavior.SetFloraSystem(g.floraSystem)
	g.allocation.SetFloraSystem(g.floraSystem)

	// Initialize UI components (descriptor-driven)
	g.uiSystemRegistry = systems.NewSystemRegistry()
	g.uiHUD = ui.NewHUD()
	g.uiInspector = ui.NewInspector(screenWidth-330, 10, 320)
	g.uiNeuralStats = ui.NewNeuralStatsPanel(10, 100, 280, 220)
	g.uiPerfPanel = ui.NewPerfPanel(screenWidth-200, 10)

	g.seedUniverse()

	return g
}

// NewGameHeadless creates a game without graphics for headless simulation
func NewGameHeadless() *Game {
	world := ecs.NewWorld()

	bounds := systems.Bounds{
		Width:  screenWidth,
		Height: screenHeight,
	}

	// Create shadow map (still needed for photosynthesis calculations)
	shadowMap := systems.NewShadowMap(screenWidth, screenHeight)

	// Create terrain
	terrain := systems.NewTerrainSystem(screenWidth, screenHeight, time.Now().UnixNano())

	// Neural evolution config
	neuralConfig := neural.DefaultConfig()
	genomeIDGen := neural.NewGenomeIDGenerator()

	g := &Game{
		world:     world,
		bounds:    bounds,
		physics:   systems.NewPhysicsSystemWithTerrain(world, bounds, terrain),
		energy:    systems.NewEnergySystem(world, shadowMap),
		cells:     systems.NewCellSystem(world),
		behavior:  systems.NewBehaviorSystem(world, shadowMap, terrain),
		flowField: systems.NewFlowFieldSystemWithTerrain(bounds, 8000, terrain),
		// Skip renderers - they require raylib
		flowRenderer:     nil,
		waterBackground:  nil,
		sunRenderer:      nil,
		terrainRenderer:  nil,
		particleRenderer: nil,
		light:            renderer.LightState{PosX: 0.5, PosY: -0.15, Intensity: 1.0},
		stepsPerFrame:    1,
		perf:             NewPerfStats(),

		// Terrain
		terrain: terrain,

		// Systems
		shadowMap:   shadowMap,
		feeding:     systems.NewFeedingSystem(world),
		spores:      systems.NewSporeSystemWithTerrain(bounds, terrain),
		breeding:    systems.NewBreedingSystem(world, neuralConfig.NEAT, genomeIDGen, neuralConfig.CPPN),
		particles:   systems.NewParticleSystem(),
		allocation:  systems.NewAllocationSystem(world),
		spatialGrid: systems.NewSpatialGrid(screenWidth, screenHeight),

		// Neural evolution
		neuralConfig:   neuralConfig,
		genomeIDGen:    genomeIDGen,
		speciesManager: neural.NewSpeciesManager(neuralConfig.NEAT),

		// Display settings (unused in headless)
		showSpeciesColors: false,
		showNeuralStats:   false,

		faunaMapper:     ecs.NewMap5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Fauna](world),
		faunaFilter:     ecs.NewFilter3[components.Position, components.Organism, components.Fauna](world),
		faunaCellFilter: ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Fauna](world),
		allOrgFilter:    ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](world),

		// Neural component mappers
		neuralGenomeMap: ecs.NewMap[components.NeuralGenome](world),
		brainMap:        ecs.NewMap[components.Brain](world),
	}

	// Create FloraSystem after other systems are initialized
	g.floraSystem = systems.NewFloraSystem(bounds, terrain, shadowMap, g.flowField)

	// Wire up FloraSystem to systems that need it
	g.feeding.SetFloraSystem(g.floraSystem)
	g.behavior.SetFloraSystem(g.floraSystem)
	g.allocation.SetFloraSystem(g.floraSystem)

	// System registry (for consistent naming in logs)
	g.uiSystemRegistry = systems.NewSystemRegistry()

	g.seedUniverse()

	return g
}

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

// createFloraLightweight creates a lightweight flora using FloraSystem.
// Returns true if flora was created successfully.
func (g *Game) createFloraLightweight(x, y float32, isRooted bool, energy float32) bool {
	if isRooted {
		return g.floraSystem.AddRooted(x, y, energy)
	}
	return g.floraSystem.AddFloating(x, y, energy)
}

// createNeuralOrganism creates an organism with neural network brain and CPPN-generated morphology.
func (g *Game) createNeuralOrganism(x, y float32, energy float32, neuralGenome *components.NeuralGenome, brain *components.Brain) ecs.Entity {
	// Reproduction mode is determined by ReproductiveMode spectrum from CPPN (0=asexual, 0.5=mixed, 1=sexual)

	// Generate morphology from CPPN
	var morph neural.MorphologyResult
	if neuralGenome != nil && neuralGenome.BodyGenome != nil {
		var err error
		morph, err = neural.GenerateMorphologyWithConfig(neuralGenome.BodyGenome, g.neuralConfig.CPPN)
		if err != nil {
			// Fallback to minimal viable morphology
			morph = neural.MorphologyResult{
				Cells: []neural.CellSpec{{
					GridX: 0, GridY: 0,
					PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
					PrimaryStrength: 0.5, SecondaryStrength: 0.3,
					EffectivePrimary: 0.5 * neural.MixedPrimaryPenalty,
					EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
				}},
			}
		}
	} else {
		// No CPPN genome, minimal viable fallback
		morph = neural.MorphologyResult{
			Cells: []neural.CellSpec{{
				GridX: 0, GridY: 0,
				PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
				PrimaryStrength: 0.5, SecondaryStrength: 0.3,
				EffectivePrimary: 0.5 * neural.MixedPrimaryPenalty,
				EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
			}},
		}
	}

	// Diet is now derived from cell DigestiveSpectrum, not from organism traits

	// Calculate max energy based on cell count
	cellCount := morph.CellCount()
	maxEnergy := float32(100 + cellCount*50)

	pos := &components.Position{X: x, Y: y}
	vel := &components.Velocity{X: 0, Y: 0}
	org := &components.Organism{
		// Note: Traits field has been removed. Diet is derived from cell DigestiveSpectrum.
		Energy:           energy,
		MaxEnergy:        maxEnergy,
		CellSize:         5,
		MaxSpeed:         1.5,
		MaxForce:         0.03,
		PerceptionRadius: 100,
		Dead:             false,
		Heading:          rand.Float32() * 6.28318,
		GrowthTimer:      0,
		GrowthInterval:   120,
		SporeTimer:       0,
		SporeInterval:    400,
		BreedingCooldown: 0,
	}

	// Create cell buffer from morphology
	cells := &components.CellBuffer{}
	for _, cellSpec := range morph.Cells {
		cells.AddCell(components.Cell{
			GridX:             cellSpec.GridX,
			GridY:             cellSpec.GridY,
			Age:               0,
			MaxAge:            3000 + rand.Int31n(2000),
			Alive:             true,
			PrimaryType:       cellSpec.PrimaryType,
			SecondaryType:     cellSpec.SecondaryType,
			PrimaryStrength:   cellSpec.EffectivePrimary,
			SecondaryStrength: cellSpec.EffectiveSecondary,
			DigestiveSpectrum: cellSpec.DigestiveSpectrum,
			StructuralArmor:   cellSpec.StructuralArmor,
			StorageCapacity:   cellSpec.StorageCapacity,
			ReproductiveMode:  cellSpec.ReproductiveMode,
		})
	}

	// Calculate shape metrics and collision OBB
	org.ShapeMetrics = systems.CalculateShapeMetrics(cells)
	org.OBB = systems.ComputeCollisionOBB(cells, org.CellSize)

	// Create fauna entity (neural organisms are always fauna)
	entity := g.faunaMapper.NewEntity(pos, vel, org, cells, &components.Fauna{})

	// Add neural components and assign species
	if neuralGenome != nil {
		// Always evaluate species based on brain genome for proper NEAT speciation
		// Even offspring should be re-evaluated as they may have mutated into a new species
		if neuralGenome.BrainGenome != nil {
			parentSpeciesID := neuralGenome.SpeciesID
			speciesID := g.speciesManager.AssignSpecies(neuralGenome.BrainGenome)
			neuralGenome.SpeciesID = speciesID
			g.speciesManager.AddMember(speciesID, int(entity.ID()))

			// If this is offspring (had a parent species), record it and log
			if parentSpeciesID > 0 {
				g.speciesManager.RecordOffspring(parentSpeciesID)

				// Log breeding event
				if *neuralLog && *neuralLogDetail {
					nodes := len(neuralGenome.BrainGenome.Nodes)
					genes := len(neuralGenome.BrainGenome.Genes)
					speciated := ""
					if speciesID != parentSpeciesID {
						speciated = fmt.Sprintf(" [SPECIATED: %d->%d]", parentSpeciesID, speciesID)
					}
					logf("[BIRTH] Entity %d @ (%.0f,%.0f): gen=%d, nodes=%d, genes=%d, species=%d%s",
						entity.ID(), x, y, neuralGenome.Generation, nodes, genes, speciesID, speciated)
				}
			}
		}
		// Use Add to add new component to entity (not Set which requires existing component)
		g.neuralGenomeMap.Add(entity, neuralGenome)
	}
	if brain != nil {
		g.brainMap.Add(entity, brain)
	}

	return entity
}

// createInitialNeuralOrganism creates a new neural organism with fresh genomes.
// This is used during seeding to create the initial neural population.
// Initial organisms are constrained to 1-4 cells for easier evolution.
// Uses HyperNEAT: CPPN generates both morphology and brain weights.
func (g *Game) createInitialNeuralOrganism(x, y float32, energy float32) ecs.Entity {
	// Create CPPN genome (generates both body morphology and brain weights)
	bodyGenome := neural.CreateCPPNGenome(g.genomeIDGen.NextID())

	// Generate morphology from CPPN first (needed for brain building)
	morph, err := neural.GenerateMorphology(bodyGenome, neural.InitialMaxCells, g.neuralConfig.CPPN.CellThreshold)
	if err != nil {
		// Retry with a new genome
		bodyGenome = neural.CreateCPPNGenome(g.genomeIDGen.NextID())
		morph, err = neural.GenerateMorphology(bodyGenome, neural.InitialMaxCells, g.neuralConfig.CPPN.CellThreshold)
		if err != nil {
			// Use minimal morphology as last resort
			morph = neural.MorphologyResult{
				Cells: []neural.CellSpec{{
					GridX: 0, GridY: 0,
					PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
					PrimaryStrength: 0.5, SecondaryStrength: 0.3,
					EffectivePrimary: 0.5 * neural.MixedPrimaryPenalty,
					EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
				}},
			}
		}
	}

	// Build brain from CPPN using HyperNEAT (CPPN determines connection weights)
	brainController, err := neural.SimplifiedHyperNEATBrain(bodyGenome, &morph)
	if err != nil {
		// Fallback to traditional brain if HyperNEAT fails
		brainGenome := neural.CreateBrainGenome(g.genomeIDGen.NextID(), g.neuralConfig.Brain.InitialConnectionProb)
		brainController, err = neural.NewBrainController(brainGenome)
		if err != nil {
			// Last resort - minimal brain
			brainGenome = neural.CreateMinimalBrainGenome(g.genomeIDGen.NextID())
			brainController, _ = neural.NewBrainController(brainGenome)
		}
	}

	// Build neural components (BrainGenome is now derived from CPPN, stored for compatibility)
	neuralGenome := &components.NeuralGenome{
		BodyGenome:  bodyGenome,
		BrainGenome: brainController.Genome, // Store the derived brain genome
		SpeciesID:   0,                      // Will be assigned by species manager
		Generation:  0,
	}

	brain := &components.Brain{
		Controller: brainController,
	}

	return g.createNeuralOrganismConstrained(x, y, energy, neuralGenome, brain, neural.InitialMaxCells)
}

// createNeuralOrganismConstrained creates a neural organism with cell count constraint.
func (g *Game) createNeuralOrganismConstrained(x, y float32, energy float32, neuralGenome *components.NeuralGenome, brain *components.Brain, maxCells int) ecs.Entity {
	// Reproduction mode is determined by ReproductiveMode spectrum from CPPN (0=asexual, 0.5=mixed, 1=sexual)

	// Generate morphology from CPPN with constrained cell count
	var morph neural.MorphologyResult
	if neuralGenome != nil && neuralGenome.BodyGenome != nil {
		var err error
		morph, err = neural.GenerateMorphology(neuralGenome.BodyGenome, maxCells, g.neuralConfig.CPPN.CellThreshold)
		if err != nil {
			// Fallback to minimal viable morphology
			morph = neural.MorphologyResult{
				Cells: []neural.CellSpec{{
					GridX: 0, GridY: 0,
					PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
					PrimaryStrength: 0.5, SecondaryStrength: 0.3,
					EffectivePrimary: 0.5 * neural.MixedPrimaryPenalty,
					EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
				}},
			}
		}
	} else {
		// No CPPN genome, minimal viable fallback
		morph = neural.MorphologyResult{
			Cells: []neural.CellSpec{{
				GridX: 0, GridY: 0,
				PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
				PrimaryStrength: 0.5, SecondaryStrength: 0.3,
				EffectivePrimary: 0.5 * neural.MixedPrimaryPenalty,
				EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
			}},
		}
	}

	// Diet is now derived from cell DigestiveSpectrum, not from organism traits

	// Calculate max energy based on cell count
	cellCount := morph.CellCount()
	maxEnergy := float32(100 + cellCount*50)

	pos := &components.Position{X: x, Y: y}
	vel := &components.Velocity{X: 0, Y: 0}
	org := &components.Organism{
		// Note: Traits field has been removed. Diet is derived from cell DigestiveSpectrum.
		Energy:           energy,
		MaxEnergy:        maxEnergy,
		CellSize:         5,
		MaxSpeed:         1.5,
		MaxForce:         0.1,
		PerceptionRadius: 60,
		Heading:          rand.Float32() * 3.14159 * 2,
		GrowthInterval:   200,
		SporeInterval:    400,
		BreedingCooldown: 300,
		TargetCells:      uint8(cellCount),
		EatIntent:        0.5,
		GrowIntent:       0.3,
		BreedIntent:      0.3,
	}

	// Create cells from morphology
	cells := &components.CellBuffer{Count: 0}
	for _, cellSpec := range morph.Cells {
		cell := components.Cell{
			GridX:             cellSpec.GridX,
			GridY:             cellSpec.GridY,
			Age:               0,
			MaxAge:            3000 + rand.Int31n(1000),
			Alive:             true,
			PrimaryType:       cellSpec.PrimaryType,
			SecondaryType:     cellSpec.SecondaryType,
			PrimaryStrength:   cellSpec.EffectivePrimary,
			SecondaryStrength: cellSpec.EffectiveSecondary,
			DigestiveSpectrum: cellSpec.DigestiveSpectrum,
			StructuralArmor:   cellSpec.StructuralArmor,
			StorageCapacity:   cellSpec.StorageCapacity,
			ReproductiveMode:  cellSpec.ReproductiveMode,
		}
		cells.AddCell(cell)
	}

	// Calculate shape metrics and collision OBB from morphology
	org.ShapeMetrics = systems.CalculateShapeMetrics(cells)
	org.OBB = systems.ComputeCollisionOBB(cells, org.CellSize)

	// Create entity - neural organisms are always fauna
	// (Flora are now managed by FloraSystem and don't have neural brains)
	entity := g.faunaMapper.NewEntity(pos, vel, org, cells, &components.Fauna{})

	// Add neural components
	g.neuralGenomeMap.Add(entity, neuralGenome)
	g.brainMap.Add(entity, brain)

	// Register with species manager
	if neuralGenome.BrainGenome != nil && g.speciesManager != nil {
		speciesID := g.speciesManager.AssignSpecies(neuralGenome.BrainGenome)
		neuralGenome.SpeciesID = speciesID
		g.speciesManager.AddMember(speciesID, int(entity.ID()))
	}

	return entity
}

func (g *Game) Update() {
	// Handle input
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}
	if rl.IsKeyPressed(rl.KeyPeriod) {
		if g.stepsPerFrame < 10 {
			g.stepsPerFrame++
		}
	}
	if rl.IsKeyPressed(rl.KeyComma) {
		if g.stepsPerFrame > 1 {
			g.stepsPerFrame--
		}
	}

	// Left-click: select organism (or Shift+click to spawn fauna)
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift) {
			// Shift+click: spawn neural fauna (diet derived from cells)
			pos := rl.GetMousePosition()
			g.createInitialNeuralOrganism(pos.X, pos.Y, 100)
		} else {
			// Regular click: select organism
			entity, found := g.findOrganismAtClick()
			if found {
				g.selectedEntity = entity
				g.hasSelection = true
			} else {
				g.hasSelection = false
			}
		}
	}
	if rl.IsKeyPressed(rl.KeyF) {
		g.createFloraLightweight(
			rand.Float32()*(g.bounds.Width-100)+50,
			g.bounds.Height-4,
			true, // isRooted
			80,
		)
	}
	if rl.IsKeyPressed(rl.KeyC) {
		// Add new fauna - diet derived from CPPN-generated cells
		g.createInitialNeuralOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			120,
		)
	}

	// Toggle species coloring
	if rl.IsKeyPressed(rl.KeyS) {
		g.showSpeciesColors = !g.showSpeciesColors
	}

	// Toggle neural stats panel
	if rl.IsKeyPressed(rl.KeyN) {
		g.showNeuralStats = !g.showNeuralStats
	}

	if g.paused {
		return
	}

	// Run simulation steps
	for step := 0; step < g.stepsPerFrame; step++ {
		g.tick++

		// Helper to time a function
		measure := func(name string, fn func()) {
			if *perfLog {
				start := time.Now()
				fn()
				g.perf.Record(name, time.Since(start))
			} else {
				fn()
			}
		}

		// Update day/night cycle
		measure("dayNight", func() { g.updateDayNightCycle() })

		// Update flow field particles (visual, independent)
		measure("flowField", func() { g.flowField.Update(g.tick) })

		// Update shadow map (foundation for light-based systems)
		var occluders []systems.Occluder
		measure("collectOccluders", func() {
			occluders = g.collectOccluders()
		})
		measure("shadowMap", func() {
			sunX := g.light.PosX * g.bounds.Width
			sunY := g.light.PosY * g.bounds.Height
			g.shadowMap.Update(g.tick, sunX, sunY, occluders)
		})

		// Collect position data for behavior system
		var floraPos, faunaPos []components.Position
		var floraOrgs, faunaOrgs []*components.Organism
		measure("collectPositions", func() {
			floraPos, faunaPos = g.collectPositions()
			floraOrgs, faunaOrgs = g.collectOrganisms()
		})

		// Update spatial grid for O(1) neighbor lookups
		measure("spatialGrid", func() { g.spatialGrid.Update(floraPos, faunaPos) })

		// Update allocation modes (determines how organisms spend energy)
		measure("allocation", func() { g.allocation.Update(floraPos, faunaPos, floraOrgs, faunaOrgs) })

		// Run systems
		measure("behavior", func() { g.behavior.Update(g.world, g.bounds, floraPos, faunaPos, floraOrgs, faunaOrgs, g.spatialGrid) })
		measure("physics", func() { g.physics.Update(g.world) })
		measure("feeding", func() { g.feeding.Update() })
		measure("floraSystem", func() {
			g.floraSystem.Update(g.tick, func(x, y float32, isRooted bool) {
				g.spores.SpawnSpore(x, y, isRooted)
			})
		})
		measure("energy", func() { g.energy.Update(g.world) })
		measure("cells", func() { g.cells.Update(g.world) })

		// Breeding (fauna reproduction - flora don't use breeding system anymore)
		measure("breeding", func() { g.breeding.Update(g.world, nil, g.createNeuralOrganism) })

		// Spores (germinates into new flora via FloraSystem)
		measure("spores", func() {
			g.spores.Update(g.tick, func(x, y float32, isRooted bool, energy float32) ecs.Entity {
				g.createFloraLightweight(x, y, isRooted, energy)
				return ecs.Entity{} // Return zero entity, not used
			})
		})

		// Growth, spore spawning, and splitting
		measure("growth", func() { g.updateGrowth() })

		// Effect particles
		measure("particles", func() { g.particles.Update() })

		// Cleanup
		measure("cleanup", func() { g.cleanupDead() })

		// Periodic logging
		if *logInterval > 0 && g.tick%int32(*logInterval) == 0 {
			g.logWorldState()
		}

		// Performance logging (every 120 ticks = ~2 seconds at 1x speed)
		if *perfLog && g.tick%120 == 0 {
			g.logPerfStats()
		}

		// Neural evolution logging (every 500 ticks = ~8 seconds at 1x speed)
		if *neuralLog && g.tick%500 == 0 {
			g.logNeuralStats()
		}
	}
}

// UpdateHeadless runs simulation without any input handling or graphics
func (g *Game) UpdateHeadless() {
	// Run simulation steps
	for step := 0; step < g.stepsPerFrame; step++ {
		g.tick++

		// Helper to time a function
		measure := func(name string, fn func()) {
			if *perfLog {
				start := time.Now()
				fn()
				g.perf.Record(name, time.Since(start))
			} else {
				fn()
			}
		}

		// Update day/night cycle
		measure("dayNight", func() { g.updateDayNightCycle() })

		// Update flow field (still affects behavior even without rendering)
		measure("flowField", func() { g.flowField.Update(g.tick) })

		// Update shadow map
		var occluders []systems.Occluder
		measure("collectOccluders", func() {
			occluders = g.collectOccluders()
		})
		measure("shadowMap", func() {
			sunX := g.light.PosX * g.bounds.Width
			sunY := g.light.PosY * g.bounds.Height
			g.shadowMap.Update(g.tick, sunX, sunY, occluders)
		})

		// Collect position data for behavior system
		var floraPos, faunaPos []components.Position
		var floraOrgs, faunaOrgs []*components.Organism
		measure("collectPositions", func() {
			floraPos, faunaPos = g.collectPositions()
			floraOrgs, faunaOrgs = g.collectOrganisms()
		})

		// Update spatial grid
		measure("spatialGrid", func() { g.spatialGrid.Update(floraPos, faunaPos) })

		// Update allocation modes
		measure("allocation", func() { g.allocation.Update(floraPos, faunaPos, floraOrgs, faunaOrgs) })

		// Run systems
		measure("behavior", func() { g.behavior.Update(g.world, g.bounds, floraPos, faunaPos, floraOrgs, faunaOrgs, g.spatialGrid) })
		measure("physics", func() { g.physics.Update(g.world) })
		measure("feeding", func() { g.feeding.Update() })
		measure("floraSystem", func() {
			g.floraSystem.Update(g.tick, func(x, y float32, isRooted bool) {
				g.spores.SpawnSpore(x, y, isRooted)
			})
		})
		measure("energy", func() { g.energy.Update(g.world) })
		measure("cells", func() { g.cells.Update(g.world) })

		// Breeding (fauna only)
		measure("breeding", func() { g.breeding.Update(g.world, nil, g.createNeuralOrganism) })

		// Spores (germinates into new flora via FloraSystem)
		measure("spores", func() {
			g.spores.Update(g.tick, func(x, y float32, isRooted bool, energy float32) ecs.Entity {
				g.createFloraLightweight(x, y, isRooted, energy)
				return ecs.Entity{}
			})
		})

		// Growth
		measure("growth", func() { g.updateGrowth() })

		// Particles (visual but system still runs)
		measure("particles", func() { g.particles.Update() })

		// Cleanup
		measure("cleanup", func() { g.cleanupDead() })

		// Periodic logging
		if *logInterval > 0 && g.tick%int32(*logInterval) == 0 {
			g.logWorldState()
		}

		// Performance logging
		if *perfLog && g.tick%120 == 0 {
			g.logPerfStats()
		}

		// Neural evolution logging
		if *neuralLog && g.tick%500 == 0 {
			g.logNeuralStats()
		}
	}
}

func (g *Game) logPerfStats() {
	total := g.perf.Total()
	logf("=== Perf @ Tick %d (speed %dx) ===", g.tick, g.stepsPerFrame)
	logf("Total step time: %s", total.Round(time.Microsecond))

	for _, name := range g.perf.SortedNames() {
		avg := g.perf.Avg(name)
		pct := float64(0)
		if total > 0 {
			pct = float64(avg) / float64(total) * 100
		}
		logf("  %-18s %10s  %5.1f%%", name, avg.Round(time.Microsecond), pct)
	}
	logf("")
}

func (g *Game) logWorldState() {
	var faunaCount, deadCount int
	var faunaEnergy float32
	var herbivoreCount, carnivoreCount, carrionCount int
	var minFaunaEnergy, maxFaunaEnergy float32 = 9999, 0
	var totalFaunaCells int

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()

		if org.Dead {
			deadCount++
			continue
		}

		// All ECS organisms are fauna (flora are in FloraSystem)
		faunaCount++
		faunaEnergy += org.Energy
		totalFaunaCells += int(cells.Count)
		if org.Energy < minFaunaEnergy {
			minFaunaEnergy = org.Energy
		}
		if org.Energy > maxFaunaEnergy {
			maxFaunaEnergy = org.Energy
		}

		// Count by diet based on cell capabilities
		caps := cells.ComputeCapabilities()
		digestiveSpectrum := caps.DigestiveSpectrum()
		if digestiveSpectrum < 0.35 {
			herbivoreCount++
		} else if digestiveSpectrum > 0.65 {
			carnivoreCount++
		} else {
			carrionCount++ // Using carrion slot for omnivores
		}
	}

	// Get flora stats from FloraSystem
	floraCount := g.floraSystem.TotalCount()
	floraEnergy := g.floraSystem.TotalEnergy()
	avgFloraEnergy := float32(0)
	if floraCount > 0 {
		avgFloraEnergy = floraEnergy / float32(floraCount)
	}

	avgFaunaEnergy := float32(0)
	if faunaCount > 0 {
		avgFaunaEnergy = faunaEnergy / float32(faunaCount)
		if avgFaunaEnergy != avgFaunaEnergy { // NaN check
			avgFaunaEnergy = 0
		}
	}
	if minFaunaEnergy == 9999 {
		minFaunaEnergy = 0
	}

	logf("=== Tick %d ===", g.tick)
	logf("Flora: %d (rooted: %d, floating: %d, energy: %.1f avg)",
		floraCount, g.floraSystem.RootedCount(), g.floraSystem.FloatingCount(), avgFloraEnergy)
	logf("Fauna: %d (cells: %d, energy: %.1f avg, %.1f-%.1f range)",
		faunaCount, totalFaunaCells, avgFaunaEnergy, minFaunaEnergy, maxFaunaEnergy)
	logf("  Herbivores: %d, Carnivores: %d, Carrion: %d",
		herbivoreCount, carnivoreCount, carrionCount)
	logf("Dead: %d, Spores: %d, Particles: %d",
		deadCount, g.spores.Count(), g.particles.Count())

	// Count breeding-eligible fauna
	breedingEligible := 0
	var modeGrow, modeBreed, modeSurvive, modeStore int
	var omnivores int
	var drifters, generalists, apex int
	var totalStreamlining, totalDrag float32
	var shapeCount int
	query2 := g.allOrgFilter.Query()
	for query2.Next() {
		_, _, org, cells := query2.Get()
		// All ECS organisms are fauna (flora are in FloraSystem)
		if org.Dead {
			continue
		}
		// Count allocation modes
		switch org.AllocationMode {
		case components.ModeGrow:
			modeGrow++
		case components.ModeBreed:
			modeBreed++
		case components.ModeSurvive:
			modeSurvive++
		case components.ModeStore:
			modeStore++
		}
		// Count omnivores based on cell digestive spectrum
		caps := cells.ComputeCapabilities()
		digestiveSpectrum := caps.DigestiveSpectrum()
		if digestiveSpectrum >= 0.35 && digestiveSpectrum <= 0.65 {
			omnivores++
		}
		if org.AllocationMode == components.ModeBreed && org.Energy >= org.MaxEnergy*0.35 && cells.Count >= 1 && org.BreedingCooldown == 0 {
			breedingEligible++
		}
		// Count organism classes by size
		cellCount := int(cells.Count)
		switch {
		case cellCount <= 3:
			drifters++
		case cellCount <= 10:
			generalists++
		default:
			apex++
		}
		// Accumulate shape metrics
		totalStreamlining += org.ShapeMetrics.Streamlining
		totalDrag += org.ShapeMetrics.DragCoefficient
		shapeCount++
	}

	avgStreamlining := float32(0)
	avgDrag := float32(0)
	if shapeCount > 0 {
		avgStreamlining = totalStreamlining / float32(shapeCount)
		avgDrag = totalDrag / float32(shapeCount)
	}

	logf("Breeding eligible: %d", breedingEligible)
	logf("Modes: Grow=%d, Breed=%d, Survive=%d, Store=%d", modeGrow, modeBreed, modeSurvive, modeStore)
	logf("Omnivores: %d", omnivores)
	logf("Classes: Drifters=%d, Generalists=%d, Apex=%d | Shape: Streamlining=%.2f, Drag=%.2f",
		drifters, generalists, apex, avgStreamlining, avgDrag)
	logf("")
}

func (g *Game) logNeuralStats() {
	stats := g.speciesManager.GetStats()
	topSpecies := g.speciesManager.GetTopSpecies(10)

	logf("╔══════════════════════════════════════════════════════════════════╗")
	logf("║ NEURAL EVOLUTION @ Tick %d (Gen %d)                              ", g.tick, stats.Generation)
	logf("╠══════════════════════════════════════════════════════════════════╣")
	logf("║ Species: %d | Total Members: %d | Best Fitness: %.2f",
		stats.Count, stats.TotalMembers, stats.BestFitness)
	logf("║ Total Offspring: %d | Avg Staleness: %.1f",
		stats.TotalOffspring, stats.AverageStaleness)

	// Count neural organisms
	neuralCount := 0
	var totalNodes, totalGenes int
	var minNodes, maxNodes int = 9999, 0
	var minGenes, maxGenes int = 9999, 0

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, org, _ := query.Get()

		if org.Dead {
			continue
		}

		if g.neuralGenomeMap.Has(entity) {
			neuralCount++
			neuralGenome := g.neuralGenomeMap.Get(entity)
			if neuralGenome != nil && neuralGenome.BrainGenome != nil {
				nodes := len(neuralGenome.BrainGenome.Nodes)
				genes := len(neuralGenome.BrainGenome.Genes)
				totalNodes += nodes
				totalGenes += genes

				if nodes < minNodes {
					minNodes = nodes
				}
				if nodes > maxNodes {
					maxNodes = nodes
				}
				if genes < minGenes {
					minGenes = genes
				}
				if genes > maxGenes {
					maxGenes = genes
				}
			}
		}
	}

	if minNodes == 9999 {
		minNodes = 0
	}
	if minGenes == 9999 {
		minGenes = 0
	}

	avgNodes := 0.0
	avgGenes := 0.0
	if neuralCount > 0 {
		avgNodes = float64(totalNodes) / float64(neuralCount)
		avgGenes = float64(totalGenes) / float64(neuralCount)
	}

	logf("╠══════════════════════════════════════════════════════════════════╣")
	logf("║ Neural Organisms: %d", neuralCount)
	logf("║ Brain Complexity:")
	logf("║   Nodes: avg=%.1f, min=%d, max=%d", avgNodes, minNodes, maxNodes)
	logf("║   Genes: avg=%.1f, min=%d, max=%d", avgGenes, minGenes, maxGenes)

	if len(topSpecies) > 0 {
		logf("╠══════════════════════════════════════════════════════════════════╣")
		logf("║ TOP SPECIES:")
		for i, sp := range topSpecies {
			logf("║   #%d: Species %d - %d members, age=%d, stale=%d, fit=%.1f, offspring=%d",
				i+1, sp.ID, sp.Size, sp.Age, sp.Staleness, sp.BestFit, sp.Offspring)
		}
	}

	// Detailed per-organism logging if enabled
	if *neuralLogDetail {
		logf("╠══════════════════════════════════════════════════════════════════╣")
		logf("║ DETAILED ORGANISM DATA (sample of 10):")

		count := 0
		query2 := g.allOrgFilter.Query()
		for query2.Next() {
			if count >= 10 {
				continue // Must consume entire query to release world lock
			}

			entity := query2.Entity()
			pos, _, org, cells := query2.Get()

			if org.Dead || !g.neuralGenomeMap.Has(entity) {
				continue
			}

			neuralGenome := g.neuralGenomeMap.Get(entity)
			if neuralGenome == nil || neuralGenome.BrainGenome == nil {
				continue
			}

			nodes := len(neuralGenome.BrainGenome.Nodes)
			genes := len(neuralGenome.BrainGenome.Genes)

			logf("║   Entity %d @ (%.0f,%.0f): species=%d, gen=%d, cells=%d, energy=%.0f/%.0f, nodes=%d, genes=%d",
				entity.ID(), pos.X, pos.Y, neuralGenome.SpeciesID, neuralGenome.Generation,
				cells.Count, org.Energy, org.MaxEnergy, nodes, genes)
			count++
		}
	}

	logf("╚══════════════════════════════════════════════════════════════════╝")
	logf("")
}

func (g *Game) collectPositions() ([]components.Position, []components.Position) {
	var floraPos, faunaPos []components.Position

	// Collect flora positions from FloraSystem
	allFlora := g.floraSystem.GetAllFlora()
	for _, ref := range allFlora {
		floraPos = append(floraPos, components.Position{X: ref.X, Y: ref.Y})
	}

	// Collect fauna positions from ECS
	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		pos, _, _ := faunaQuery.Get()
		faunaPos = append(faunaPos, *pos)
	}

	return floraPos, faunaPos
}

func (g *Game) collectOccluders() []systems.Occluder {
	// Start with terrain occluders (static, cached, full density)
	occluders := append([]systems.Occluder{}, g.terrain.GetOccluders()...)

	// Add floating flora occluders only (fauna don't cast shadows)
	// Rooted flora don't cast shadows - they're attached to terrain which already casts shadows
	for i := range g.floraSystem.Floating {
		f := &g.floraSystem.Floating[i]
		if f.Dead {
			continue
		}

		// Simple bounding box based on flora size
		size := f.Size * 3
		occluders = append(occluders, systems.Occluder{
			X:       f.X - size/2,
			Y:       f.Y - size/2,
			Width:   size,
			Height:  size,
			Density: 0.08, // Very sparse foliage - minimal shadow
		})
	}

	return occluders
}

func (g *Game) collectOrganisms() ([]*components.Organism, []*components.Organism) {
	// Note: This function is legacy - flora are no longer ECS organisms.
	// We return empty flora slice since FloraSystem handles flora now.
	// Behavior/allocation systems that need flora data should use FloraSystem directly.
	var faunaOrgs []*components.Organism

	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		_, org, _ := faunaQuery.Get()
		faunaOrgs = append(faunaOrgs, org)
	}

	// Return nil for floraOrgs since flora are now managed by FloraSystem
	return nil, faunaOrgs
}

func (g *Game) updateGrowth() {
	// Note: Flora are now managed by FloraSystem, not ECS
	// Spore spawning for flora is handled in FloraSystem.Update()

	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, cells := query.Get()

		if org.Dead {
			// Emit death particles
			g.particles.EmitDeath(pos.X, pos.Y)
			continue
		}

		org.GrowthTimer++

		// Scale growth interval by GrowIntent: high intent = faster growth
		// Maps GrowIntent (0-1) to interval range (MaxGrowthInterval to MinGrowthInterval)
		effectiveInterval := int32(MaxGrowthInterval) - int32(float32(MaxGrowthInterval-MinGrowthInterval)*org.GrowIntent)

		// Determine if mode allows growth
		// ModeGrow always allows, ModeStore allows if intent is very high (>= 0.8)
		modeAllows := org.AllocationMode == components.ModeGrow
		if org.AllocationMode == components.ModeStore && org.GrowIntent >= 0.8 {
			modeAllows = true
		}

		// Gate conditions for growth
		intentStrong := org.GrowIntent >= GrowIntentThreshold
		belowTarget := cells.Count < org.TargetCells
		hasEnergy := org.Energy > 40

		canGrow := modeAllows && intentStrong && belowTarget && hasEnergy

		if org.GrowthTimer >= effectiveInterval && canGrow {
			org.GrowthTimer = 0
			g.tryGrow(pos, org, cells)
		}
	}
}

type gridPos struct{ x, y int8 }

func (g *Game) tryGrow(orgPos *components.Position, org *components.Organism, cells *components.CellBuffer) {
	if cells.Count >= 32 || org.Energy < 30 {
		return
	}

	// Find valid growth positions
	occupied := make(map[gridPos]bool)
	for i := uint8(0); i < cells.Count; i++ {
		occupied[gridPos{cells.Cells[i].GridX, cells.Cells[i].GridY}] = true
	}

	var candidates []gridPos
	directions := []gridPos{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	for i := uint8(0); i < cells.Count; i++ {
		for _, d := range directions {
			np := gridPos{cells.Cells[i].GridX + d.x, cells.Cells[i].GridY + d.y}
			if !occupied[np] {
				candidates = append(candidates, np)
			}
		}
	}

	if len(candidates) == 0 {
		return
	}

	// All ECS organisms are fauna (flora are in FloraSystem)
	// Pick random growth position
	newPos := candidates[rand.Intn(len(candidates))]

	// New cells inherit properties from a random existing cell
	// This allows the organism's morphology to grow organically
	sourceCell := &cells.Cells[rand.Intn(int(cells.Count))]

	// Add cell with inherited properties
	cells.AddCell(components.Cell{
		GridX:             newPos.x,
		GridY:             newPos.y,
		Age:               0,
		MaxAge:            3000 + rand.Int31n(2000),
		Alive:             true,
		Decomposition:     0,
		PrimaryType:       sourceCell.PrimaryType,
		SecondaryType:     sourceCell.SecondaryType,
		PrimaryStrength:   sourceCell.PrimaryStrength * (0.8 + rand.Float32()*0.4), // Slight variation
		SecondaryStrength: sourceCell.SecondaryStrength * (0.8 + rand.Float32()*0.4),
		DigestiveSpectrum: sourceCell.DigestiveSpectrum,
		StructuralArmor:   sourceCell.StructuralArmor,
		StorageCapacity:   sourceCell.StorageCapacity,
		ReproductiveMode:  sourceCell.ReproductiveMode,
	})

	// Recalculate shape metrics and collision OBB after growth
	org.ShapeMetrics = systems.CalculateShapeMetrics(cells)
	org.OBB = systems.ComputeCollisionOBB(cells, org.CellSize)

	org.Energy -= 30
}

// updateDayNightCycle keeps light at constant intensity (day/night cycle disabled).
func (g *Game) updateDayNightCycle() {
	// Static sun at top center with constant full intensity
	g.light.PosX = 0.5
	g.light.Intensity = 1.0
}

func (g *Game) cleanupDead() {
	const maxDeadTime = 600 // Remove after ~10 seconds at normal speed

	// Collect entities to remove (can't modify during query)
	var toRemove []ecs.Entity

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, org, cells := query.Get()

		if org.Dead {
			// On first death tick, remove from species and record fitness
			if org.DeadTime == 0 && g.neuralGenomeMap.Has(entity) {
				neuralGenome := g.neuralGenomeMap.Get(entity)
				if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
					// Calculate final fitness before removal
					fitness := neural.CalculateFitness(org.Energy, org.MaxEnergy, g.tick, 0)
					g.speciesManager.AccumulateFitness(neuralGenome.SpeciesID, fitness)
					g.speciesManager.RemoveMember(neuralGenome.SpeciesID, int(entity.ID()))

					// Log death event
					if *neuralLog && *neuralLogDetail {
						logf("[DEATH] Entity %d: gen=%d, species=%d, fitness=%.2f, survived=%d ticks",
							entity.ID(), neuralGenome.Generation, neuralGenome.SpeciesID, fitness, g.tick)
					}
				}
			}

			org.DeadTime++
			if org.DeadTime > maxDeadTime {
				toRemove = append(toRemove, entity)
			}
		} else if g.neuralGenomeMap.Has(entity) {
			// Periodically update fitness for living organisms (every 100 ticks)
			if g.tick%100 == 0 {
				neuralGenome := g.neuralGenomeMap.Get(entity)
				if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
					fitness := neural.CalculateFitness(org.Energy, org.MaxEnergy, g.tick, 0)
					g.speciesManager.AccumulateFitness(neuralGenome.SpeciesID, fitness)
				}
			}
		}

		// Suppress unused variable warning
		_ = cells
	}

	// Remove dead entities
	for _, e := range toRemove {
		g.world.RemoveEntity(e)
	}

	// Update generations periodically (every 3000 ticks ≈ 50 seconds at normal speed)
	if g.tick%3000 == 0 && g.tick > 0 {
		g.speciesManager.EndGeneration()
	}
}

func (g *Game) Draw() {
	rl.BeginDrawing()

	// Draw animated water background
	g.waterBackground.Draw(float32(g.tick) * 0.016) // Convert tick to approximate seconds

	// Draw terrain (after water, before flow field)
	if g.terrainRenderer != nil {
		g.terrainRenderer.Draw(g.terrain, g.tick)
	}

	// Draw flow field particles (on top of water and terrain)
	g.flowRenderer.Draw(g.flowField.Particles, g.tick)

	// Collect occluders from organisms for shadow casting
	occluders := g.collectOccluders()

	// Draw sun with shadows
	g.sunRenderer.Draw(g.light, occluders)

	// Draw lightweight flora from FloraSystem
	g.drawLightweightFlora()

	// Draw all fauna organisms (ECS)
	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, org, cells := query.Get()
		g.drawOrganism(entity, pos, org, cells)
	}

	// Draw selection indicator (after organisms, before UI)
	g.drawSelectionIndicator()

	// Draw spores
	g.drawSpores()

	// Draw effect particles
	g.particleRenderer.Draw(g.particles.Particles)

	// Draw ambient darkness overlay (based on sun intensity)
	g.sunRenderer.DrawAmbientDarkness(g.light.Intensity)

	// Draw UI
	g.drawUI()

	// Draw neural stats panel if enabled
	if g.showNeuralStats {
		g.drawNeuralStats()
	}

	// Draw info panel when selected, or tooltip when hovering
	if g.hasSelection {
		g.drawInfoPanel()
	} else {
		g.drawTooltip()
	}

	rl.EndDrawing()
}

// HoveredOrganism holds data about the organism under the cursor.
type HoveredOrganism struct {
	Pos   *components.Position
	Org   *components.Organism
	Cells *components.CellBuffer
}

// findOrganismAtMouse returns the organism under the mouse cursor, if any.
func (g *Game) findOrganismAtMouse() *HoveredOrganism {
	mousePos := rl.GetMousePosition()
	mouseX, mouseY := mousePos.X, mousePos.Y

	var closest *HoveredOrganism
	closestDist := float32(20.0) // Max hover distance

	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, cells := query.Get()

		// Calculate organism bounds
		minX, minY := pos.X, pos.Y
		maxX, maxY := pos.X, pos.Y

		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			cellX := pos.X + float32(cell.GridX)*org.CellSize
			cellY := pos.Y + float32(cell.GridY)*org.CellSize
			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}

		// Expand bounds by cell size
		minX -= org.CellSize
		minY -= org.CellSize
		maxX += org.CellSize
		maxY += org.CellSize

		// Check if mouse is within bounds
		if mouseX >= minX && mouseX <= maxX && mouseY >= minY && mouseY <= maxY {
			// Calculate distance to center
			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2
			dist := float32(math.Sqrt(float64((mouseX-centerX)*(mouseX-centerX) + (mouseY-centerY)*(mouseY-centerY))))

			if dist < closestDist {
				closestDist = dist
				closest = &HoveredOrganism{Pos: pos, Org: org, Cells: cells}
			}
		}
	}

	return closest
}

// findOrganismAtClick returns the entity under the mouse cursor, if any.
func (g *Game) findOrganismAtClick() (ecs.Entity, bool) {
	mousePos := rl.GetMousePosition()
	mouseX, mouseY := mousePos.X, mousePos.Y

	var closestEntity ecs.Entity
	closestDist := float32(20.0) // Max click distance
	found := false

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, org, cells := query.Get()

		// Calculate organism bounds
		minX, minY := pos.X, pos.Y
		maxX, maxY := pos.X, pos.Y

		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			cellX := pos.X + float32(cell.GridX)*org.CellSize
			cellY := pos.Y + float32(cell.GridY)*org.CellSize
			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}

		// Expand bounds by cell size
		minX -= org.CellSize
		minY -= org.CellSize
		maxX += org.CellSize
		maxY += org.CellSize

		// Check if mouse is within bounds
		if mouseX >= minX && mouseX <= maxX && mouseY >= minY && mouseY <= maxY {
			// Calculate distance to center
			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2
			dist := float32(math.Sqrt(float64((mouseX-centerX)*(mouseX-centerX) + (mouseY-centerY)*(mouseY-centerY))))

			if dist < closestDist {
				closestDist = dist
				closestEntity = entity
				found = true
			}
		}
	}

	return closestEntity, found
}

// drawSelectionIndicator draws a circle around the selected organism.
func (g *Game) drawSelectionIndicator() {
	if !g.hasSelection {
		return
	}

	// Check if entity still exists
	if !g.world.Alive(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	// Get position and cells for the selected entity
	posMap := ecs.NewMap[components.Position](g.world)
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !posMap.Has(g.selectedEntity) || !orgMap.Has(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	pos := posMap.Get(g.selectedEntity)
	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	// Calculate organism bounding circle
	var minX, minY, maxX, maxY float32 = pos.X, pos.Y, pos.X, pos.Y

	if cells != nil {
		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			// Rotate cell position by organism heading
			localX := float32(cell.GridX) * org.CellSize
			localY := float32(cell.GridY) * org.CellSize
			cosH := float32(math.Cos(float64(org.Heading)))
			sinH := float32(math.Sin(float64(org.Heading)))
			rotatedX := localX*cosH - localY*sinH
			rotatedY := localX*sinH + localY*cosH
			cellX := pos.X + rotatedX
			cellY := pos.Y + rotatedY

			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}
	}

	// Calculate center and radius
	centerX := (minX + maxX) / 2
	centerY := (minY + maxY) / 2
	radius := float32(math.Sqrt(float64((maxX-minX)*(maxX-minX)+(maxY-minY)*(maxY-minY)))) / 2
	radius += org.CellSize + 3 // Padding

	// Pulsing glow effect
	pulse := float32(math.Sin(float64(g.tick)*0.1))*0.3 + 0.7
	alpha := uint8(255 * pulse)

	// Draw selection circle
	rl.DrawCircleLines(int32(centerX), int32(centerY), radius, rl.Color{R: 255, G: 255, B: 255, A: alpha})
	rl.DrawCircleLines(int32(centerX), int32(centerY), radius+1, rl.Color{R: 255, G: 255, B: 255, A: alpha / 2})
}

func (g *Game) drawTooltip() {
	hovered := g.findOrganismAtMouse()
	if hovered == nil {
		return
	}

	mousePos := rl.GetMousePosition()
	org := hovered.Org
	cells := hovered.Cells

	// Build tooltip content
	var lines []string

	// Type header - determined by cell capabilities
	caps := cells.ComputeCapabilities()
	dietName := neural.GetDietName(caps.DigestiveSpectrum())
	lines = append(lines, dietName)

	lines = append(lines, "")

	// Stats
	lines = append(lines, fmt.Sprintf("Energy: %.0f / %.0f", org.Energy, org.MaxEnergy))
	lines = append(lines, fmt.Sprintf("Cells: %d", cells.Count))
	lines = append(lines, fmt.Sprintf("Speed: %.2f", org.MaxSpeed))

	if org.Dead {
		lines = append(lines, "STATUS: DEAD")
	}

	lines = append(lines, "")

	// Capabilities (derived from cells)
	lines = append(lines, "Capabilities:")
	lines = append(lines, fmt.Sprintf("  Diet: %.2f", caps.DigestiveSpectrum()))
	if caps.StructuralArmor > 0 {
		lines = append(lines, fmt.Sprintf("  Armor: %.2f", caps.StructuralArmor))
	}
	if caps.StorageCapacity > 0 {
		lines = append(lines, fmt.Sprintf("  Storage: %.2f", caps.StorageCapacity))
	}
	if caps.PhotoWeight > 0 {
		lines = append(lines, fmt.Sprintf("  Photo: %.2f", caps.PhotoWeight))
	}

	// Calculate tooltip dimensions
	const fontSize = 14
	const padding = 8
	const lineHeight = 16

	maxWidth := int32(0)
	for _, line := range lines {
		width := rl.MeasureText(line, fontSize)
		if width > maxWidth {
			maxWidth = width
		}
	}

	tooltipWidth := maxWidth + padding*2
	tooltipHeight := int32(len(lines)*lineHeight + padding*2)

	// Position tooltip (offset from cursor, keep on screen)
	tooltipX := int32(mousePos.X) + 15
	tooltipY := int32(mousePos.Y) + 15

	if tooltipX+tooltipWidth > screenWidth-10 {
		tooltipX = int32(mousePos.X) - tooltipWidth - 10
	}
	if tooltipY+tooltipHeight > screenHeight-10 {
		tooltipY = int32(mousePos.Y) - tooltipHeight - 10
	}

	// Draw background
	rl.DrawRectangle(tooltipX, tooltipY, tooltipWidth, tooltipHeight, rl.Color{R: 20, G: 25, B: 30, A: 230})
	rl.DrawRectangleLines(tooltipX, tooltipY, tooltipWidth, tooltipHeight, rl.Color{R: 60, G: 70, B: 80, A: 255})

	// Draw text - use capability-based color
	r, gr, b := neural.GetCapabilityColor(caps.DigestiveSpectrum())
	headerColor := rl.Color{R: r, G: gr, B: b, A: 255}

	for i, line := range lines {
		y := tooltipY + padding + int32(i*lineHeight)
		color := rl.LightGray
		if i == 0 {
			color = headerColor
		}
		rl.DrawText(line, tooltipX+padding, y, fontSize, color)
	}
}

func (g *Game) drawOrganism(entity ecs.Entity, pos *components.Position, org *components.Organism, cells *components.CellBuffer) {
	var r, gr, b uint8

	// Compute capabilities for color
	caps := cells.ComputeCapabilities()

	// Use species color if enabled and organism has neural genome
	if g.showSpeciesColors && g.neuralGenomeMap.Has(entity) {
		neuralGenome := g.neuralGenomeMap.Get(entity)
		if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
			speciesColor := g.speciesManager.GetSpeciesColor(neuralGenome.SpeciesID)
			r, gr, b = speciesColor.R, speciesColor.G, speciesColor.B
		} else {
			r, gr, b = neural.GetCapabilityColor(caps.DigestiveSpectrum())
		}
	} else {
		r, gr, b = neural.GetCapabilityColor(caps.DigestiveSpectrum())
	}

	baseColor := rl.Color{R: r, G: gr, B: b, A: 255}

	// Adjust for death/low energy
	if org.Dead {
		baseColor.R = baseColor.R / 2
		baseColor.G = baseColor.G / 2
		baseColor.B = baseColor.B / 2
	} else if org.Energy < 30 {
		// Dim when low energy
		factor := org.Energy / 30
		baseColor.R = uint8(float32(baseColor.R) * (0.5 + 0.5*factor))
		baseColor.G = uint8(float32(baseColor.G) * (0.5 + 0.5*factor))
		baseColor.B = uint8(float32(baseColor.B) * (0.5 + 0.5*factor))
	}

	// Pre-compute rotation for cell positions
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))

	// Draw each cell
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if !cell.Alive {
			continue
		}

		// Local grid position
		localX := float32(cell.GridX) * org.CellSize
		localY := float32(cell.GridY) * org.CellSize

		// Rotate around center
		rotatedX := localX*cosH - localY*sinH
		rotatedY := localX*sinH + localY*cosH

		// World position
		cellX := pos.X + rotatedX
		cellY := pos.Y + rotatedY

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(cellX, cellY)
		// Apply global sun intensity as additional factor
		light *= (0.3 + g.light.Intensity*0.7) // Min 30% light even at night

		// Adjust alpha for decomposition
		alpha := uint8(255 * (1 - cell.Decomposition))
		cellColor := baseColor
		cellColor.A = alpha

		// Apply lighting to color (darken based on shadow map)
		cellColor.R = uint8(float32(cellColor.R) * light)
		cellColor.G = uint8(float32(cellColor.G) * light)
		cellColor.B = uint8(float32(cellColor.B) * light)

		// Draw cell with rotation matching organism heading
		rotationDeg := org.Heading * 180 / math.Pi
		rl.DrawRectanglePro(
			rl.Rectangle{X: cellX, Y: cellY, Width: org.CellSize, Height: org.CellSize},
			rl.Vector2{X: org.CellSize / 2, Y: org.CellSize / 2}, // rotate around cell center
			rotationDeg,
			cellColor,
		)
	}
}

func (g *Game) drawSpores() {
	for i := range g.spores.Spores {
		spore := &g.spores.Spores[i]

		// Calculate alpha based on life/landing state
		alpha := uint8(180)
		if spore.Landed {
			// Fade as germination approaches
			fadeRatio := 1.0 - float32(spore.LandedTimer)/50.0
			alpha = uint8(fadeRatio * 180)
		}

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(spore.X, spore.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		// Green color for spores, adjusted for lighting
		color := rl.Color{
			R: uint8(80 * light),
			G: uint8(180 * light),
			B: uint8(100 * light),
			A: alpha,
		}
		rl.DrawCircle(int32(spore.X), int32(spore.Y), 2, color)
	}
}

// drawLightweightFlora renders all flora from the FloraSystem.
func (g *Game) drawLightweightFlora() {
	// Base flora color (green)
	baseR, baseG, baseB := uint8(50), uint8(180), uint8(80)

	// Draw rooted flora
	for i := range g.floraSystem.Rooted {
		f := &g.floraSystem.Rooted[i]
		if f.Dead {
			continue
		}

		// Sample shadow map for local lighting
		light := g.shadowMap.SampleLight(f.X, f.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		// Energy-based alpha (dimmer when low energy)
		energyRatio := f.Energy / f.MaxEnergy
		if energyRatio < 0.3 {
			light *= 0.5 + energyRatio
		}

		color := rl.Color{
			R: uint8(float32(baseR) * light),
			G: uint8(float32(baseG) * light),
			B: uint8(float32(baseB) * light),
			A: 255,
		}
		rl.DrawCircle(int32(f.X), int32(f.Y), f.Size, color)
	}

	// Draw floating flora (slightly different shade)
	for i := range g.floraSystem.Floating {
		f := &g.floraSystem.Floating[i]
		if f.Dead {
			continue
		}

		light := g.shadowMap.SampleLight(f.X, f.Y)
		light *= (0.3 + g.light.Intensity*0.7)

		energyRatio := f.Energy / f.MaxEnergy
		if energyRatio < 0.3 {
			light *= 0.5 + energyRatio
		}

		// Floating flora is slightly more cyan
		color := rl.Color{
			R: uint8(float32(40) * light),
			G: uint8(float32(170) * light),
			B: uint8(float32(100) * light),
			A: 240,
		}
		rl.DrawCircle(int32(f.X), int32(f.Y), f.Size, color)
	}
}

func (g *Game) drawUI() {
	// Count organisms
	floraCount := g.floraSystem.TotalCount()
	faunaCount := 0
	totalCells := 0

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()
		// All ECS organisms are fauna (flora are in FloraSystem)
		if !org.Dead {
			faunaCount++
			totalCells += int(cells.Count)
		}
	}

	// Draw HUD using descriptor-driven UI
	g.uiHUD.Draw(ui.HUDData{
		Title:        "Primordial Soup",
		FloraCount:   floraCount,
		FaunaCount:   faunaCount,
		CellCount:    totalCells,
		SporeCount:   g.spores.Count(),
		Tick:         g.tick,
		Speed:        g.stepsPerFrame,
		FPS:          rl.GetFPS(),
		Paused:       g.paused,
		ScreenWidth:  screenWidth,
		ScreenHeight: screenHeight,
	})

	// Performance stats (right side) using descriptor-driven UI
	if *perfLog {
		// Build system times map
		systemTimes := make(map[string]time.Duration)
		for _, name := range g.perf.SortedNames() {
			systemTimes[name] = g.perf.Avg(name)
		}

		g.uiPerfPanel.Draw(ui.PerfPanelData{
			SystemTimes: systemTimes,
			Total:       g.perf.Total(),
			Registry:    g.uiSystemRegistry,
		}, g.perf.SortedNames())
	}

	// Controls
	g.uiHUD.DrawControls(screenWidth, screenHeight,
		"SPACE: Pause | < >: Speed | Click: Select | Shift+Click: Add | F: Flora | C: Carnivore | S: Species | N: Neural")
}

func (g *Game) drawNeuralStats() {
	// Get stats from species manager
	stats := g.speciesManager.GetStats()
	topSpecies := g.speciesManager.GetTopSpecies(5)

	// Convert to UI data format
	var speciesInfo []ui.SpeciesInfo
	for _, sp := range topSpecies {
		speciesInfo = append(speciesInfo, ui.SpeciesInfo{
			ID:      sp.ID,
			Size:    sp.Size,
			Age:     sp.Age,
			BestFit: sp.BestFit,
			Color:   rl.Color{R: sp.Color.R, G: sp.Color.G, B: sp.Color.B, A: 255},
		})
	}

	// Draw using descriptor-driven UI
	g.uiNeuralStats.Draw(ui.NeuralStatsData{
		Generation:        stats.Generation,
		SpeciesCount:      stats.Count,
		TotalMembers:      stats.TotalMembers,
		BestFitness:       stats.BestFitness,
		TopSpecies:        speciesInfo,
		ShowSpeciesColors: g.showSpeciesColors,
	})
}

// drawInfoPanel draws the detailed info panel for the selected organism.
func (g *Game) drawInfoPanel() {
	if !g.hasSelection || !g.world.Alive(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	// Get entity data using maps
	orgMap := ecs.NewMap[components.Organism](g.world)
	cellMap := ecs.NewMap[components.CellBuffer](g.world)

	if !orgMap.Has(g.selectedEntity) {
		g.hasSelection = false
		return
	}

	org := orgMap.Get(g.selectedEntity)
	cells := cellMap.Get(g.selectedEntity)

	// Compute capabilities
	caps := cells.ComputeCapabilities()

	// Check for neural genome
	var neuralGenome *components.NeuralGenome
	hasNeural := g.neuralGenomeMap.Has(g.selectedEntity)
	if hasNeural {
		neuralGenome = g.neuralGenomeMap.Get(g.selectedEntity)
	}

	// Draw using descriptor-driven UI
	g.uiInspector.Draw(ui.InspectorData{
		Organism:     org,
		Cells:        cells,
		Capabilities: &caps,
		NeuralGenome: neuralGenome,
		HasBrain:     hasNeural,
	})
}

func main() {
	flag.Parse()

	// Setup log file if specified
	if *logFile != "" {
		var err error
		logWriter, err = os.Create(*logFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create log file: %v\n", err)
			os.Exit(1)
		}
		defer logWriter.Close()
	}

	if *headless {
		// Run simulation without graphics
		runHeadless()
		return
	}

	rl.InitWindow(screenWidth, screenHeight, "Primordial Soup")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	game := NewGame()

	// Apply initial speed
	if *initialSpeed > 0 && *initialSpeed <= 10 {
		game.stepsPerFrame = *initialSpeed
	}

	defer game.flowRenderer.Unload()
	defer game.waterBackground.Unload()

	for !rl.WindowShouldClose() {
		game.Update()
		game.Draw()
	}
}

// runHeadless runs the simulation without graphics for logging/benchmarking
func runHeadless() {
	logf("Starting headless simulation...")
	if *initialSpeed == 0 {
		logf("  Speed: FULL (uncapped), Max ticks: %d", *maxTicks)
	} else {
		logf("  Speed: %dx, Max ticks: %d", *initialSpeed, *maxTicks)
	}
	if *neuralLog {
		logf("  Neural logging: enabled (detail=%v)", *neuralLogDetail)
	}
	logf("")

	game := NewGameHeadless()

	// Apply initial speed (in headless, this is steps per "frame")
	// Speed 0 = full/uncapped (run as many ticks as possible per iteration)
	if *initialSpeed == 0 {
		game.stepsPerFrame = 10000 // Uncapped - run many ticks per iteration
	} else if *initialSpeed > 0 && *initialSpeed <= 10 {
		game.stepsPerFrame = *initialSpeed
	}

	startTime := time.Now()
	lastReport := startTime
	reportInterval := 10 * time.Second

	for {
		// Check max ticks
		if *maxTicks > 0 && int(game.tick) >= *maxTicks {
			logf("Reached max ticks (%d), stopping.", *maxTicks)
			break
		}

		// Run simulation step
		game.UpdateHeadless()

		// Periodic progress report
		if time.Since(lastReport) >= reportInterval {
			elapsed := time.Since(startTime)
			ticksPerSec := float64(game.tick) / elapsed.Seconds()
			logf("[PROGRESS] Tick %d | %.0f ticks/sec | Elapsed: %s",
				game.tick, ticksPerSec, elapsed.Round(time.Second))
			lastReport = time.Now()
		}
	}

	elapsed := time.Since(startTime)
	logf("")
	logf("Simulation complete.")
	logf("  Total ticks: %d", game.tick)
	logf("  Elapsed time: %s", elapsed.Round(time.Millisecond))
	logf("  Average: %.0f ticks/sec", float64(game.tick)/elapsed.Seconds())
}

func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if logWriter != nil {
		fmt.Fprintln(logWriter, msg)
	} else {
		fmt.Println(msg)
	}
}

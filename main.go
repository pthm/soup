package main

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sort"
	"strings"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/traits"
)

var (
	initialSpeed = flag.Int("speed", 1, "Initial simulation speed (1-10)")
	logInterval  = flag.Int("log", 0, "Log world state every N ticks (0 = disabled)")
	logFile      = flag.String("logfile", "", "Write logs to file instead of stdout")
	perfLog      = flag.Bool("perf", false, "Enable performance logging")
	logWriter    *os.File
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

type Game struct {
	world         *ecs.World
	bounds        systems.Bounds
	physics       *systems.PhysicsSystem
	energy        *systems.EnergySystem
	cells         *systems.CellSystem
	behavior      *systems.BehaviorSystem
	flowField       *systems.FlowFieldSystem
	flowRenderer    *renderer.FlowRenderer
	waterBackground *renderer.WaterBackground
	sunRenderer     *renderer.SunRenderer
	light           renderer.LightState
	nightTicks      int32 // Counter for darkness period
	tick          int32
	paused        bool
	stepsPerFrame int
	perf          *PerfStats

	// New systems
	shadowMap      *systems.ShadowMap
	photosynthesis *systems.PhotosynthesisSystem
	feeding        *systems.FeedingSystem
	spores         *systems.SporeSystem
	breeding       *systems.BreedingSystem
	splitting      *systems.SplittingSystem
	particles      *systems.ParticleSystem
	particleRenderer *renderer.ParticleRenderer
	allocation     *systems.AllocationSystem
	spatialGrid    *systems.SpatialGrid

	// Mappers for creating entities with components
	floraMapper *ecs.Map5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Flora]
	faunaMapper *ecs.Map5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Fauna]

	// Filters for querying
	floraFilter  *ecs.Filter3[components.Position, components.Organism, components.Flora]
	faunaFilter  *ecs.Filter3[components.Position, components.Organism, components.Fauna]
	allOrgFilter *ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
}

func NewGame() *Game {
	world := ecs.NewWorld()

	bounds := systems.Bounds{
		Width:  screenWidth,
		Height: screenHeight,
	}

	// Create shadow map first as other systems depend on it
	shadowMap := systems.NewShadowMap(screenWidth, screenHeight)

	g := &Game{
		world:         world,
		bounds:        bounds,
		physics:       systems.NewPhysicsSystem(world, bounds),
		energy:        systems.NewEnergySystem(world),
		cells:         systems.NewCellSystem(world),
		behavior:      systems.NewBehaviorSystem(world, shadowMap),
		flowField:       systems.NewFlowFieldSystem(bounds, 8000),
		flowRenderer:    renderer.NewFlowRenderer(screenWidth, screenHeight, 0.08),
		waterBackground: renderer.NewWaterBackground(screenWidth, screenHeight),
		sunRenderer:     renderer.NewSunRenderer(screenWidth, screenHeight),
		light:           renderer.LightState{PosX: 1.2, PosY: -0.15, Intensity: 1.0}, // Start off-screen right
		stepsPerFrame: 1,
		perf:          NewPerfStats(),

		// New systems
		shadowMap:      shadowMap,
		photosynthesis: systems.NewPhotosynthesisSystem(world, shadowMap),
		feeding:        systems.NewFeedingSystem(world),
		spores:         systems.NewSporeSystem(bounds),
		breeding:       systems.NewBreedingSystem(world),
		splitting:      systems.NewSplittingSystem(),
		particles:      systems.NewParticleSystem(),
		allocation:     systems.NewAllocationSystem(world),
		spatialGrid:   systems.NewSpatialGrid(screenWidth, screenHeight),
		particleRenderer: renderer.NewParticleRenderer(),

		floraMapper:   ecs.NewMap5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Flora](world),
		faunaMapper:   ecs.NewMap5[components.Position, components.Velocity, components.Organism, components.CellBuffer, components.Fauna](world),
		floraFilter:   ecs.NewFilter3[components.Position, components.Organism, components.Flora](world),
		faunaFilter:   ecs.NewFilter3[components.Position, components.Organism, components.Fauna](world),
		allOrgFilter:  ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](world),
	}

	g.seedUniverse()

	return g
}

func (g *Game) seedUniverse() {
	// Create rooted flora along the bottom
	for i := 0; i < 60; i++ {
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			g.bounds.Height-3,
			traits.Flora|traits.Rooted,
			80,
		)
	}

	// Create floating flora
	for i := 0; i < 40; i++ {
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-250)+100,
			traits.Flora|traits.Floating,
			80,
		)
	}

	// Create herbivores (all get Breeding)
	for i := 0; i < 50; i++ {
		t := traits.Herbivore | traits.Breeding
		if rand.Float32() > 0.5 {
			t = t.Add(traits.Herding)
		}
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			t,
			100,
		)
	}

	// Create carnivores (all get Breeding)
	for i := 0; i < 20; i++ {
		t := traits.Carnivore | traits.Breeding
		if rand.Float32() > 0.3 {
			t = t.Add(traits.Herding)
		}
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			t,
			120,
		)
	}

	// Create carrion eaters (with Breeding)
	for i := 0; i < 15; i++ {
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			traits.Carrion|traits.Herding|traits.Breeding,
			80,
		)
	}
}

func (g *Game) createOrganism(x, y float32, t traits.Trait, energy float32) ecs.Entity {
	// Assign gender if breeding
	if t.Has(traits.Breeding) {
		if rand.Float32() > 0.5 {
			t = t.Add(traits.Male)
		} else {
			t = t.Add(traits.Female)
		}
	}

	pos := &components.Position{X: x, Y: y}
	vel := &components.Velocity{X: 0, Y: 0}
	org := &components.Organism{
		Traits:           t,
		Energy:           energy,
		MaxEnergy:        150,
		CellSize:         2.5,
		MaxSpeed:         1.5,
		MaxForce:         0.03,
		PerceptionRadius: 100,
		Dead:             false,
		Heading:          rand.Float32() * 6.28318,
		GrowthTimer:      0,
		GrowthInterval:   120,
		SporeTimer:       0,
		SporeInterval:    300, // Reduced from 600
		BreedingCooldown: 0,
	}

	// Create initial cell buffer
	cells := &components.CellBuffer{}
	cells.AddCell(components.Cell{
		GridX:    0,
		GridY:    0,
		Age:      0,
		MaxAge:   3000 + rand.Int31n(2000),
		Trait:    t & (traits.Flora | traits.Herbivore | traits.Carnivore | traits.Carrion),
		Mutation: traits.NoMutation,
		Alive:    true,
	})

	// Create entity with appropriate tag
	if traits.IsFlora(t) {
		return g.floraMapper.NewEntity(pos, vel, org, cells, &components.Flora{})
	}
	return g.faunaMapper.NewEntity(pos, vel, org, cells, &components.Fauna{})
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

	// Add organisms on click
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		pos := rl.GetMousePosition()
		g.createOrganism(pos.X, pos.Y, traits.Herbivore|traits.Breeding, 100)
	}
	if rl.IsKeyPressed(rl.KeyF) {
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			g.bounds.Height-4,
			traits.Flora|traits.Rooted,
			80,
		)
	}
	if rl.IsKeyPressed(rl.KeyC) {
		g.createOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			traits.Carnivore|traits.Breeding,
			120,
		)
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
		measure("photosynthesis", func() { g.photosynthesis.Update() })
		measure("energy", func() { g.energy.Update(g.world) })
		measure("cells", func() { g.cells.Update(g.world) })

		// Breeding (fauna reproduction)
		measure("breeding", func() { g.breeding.Update(g.world, g.createOrganism) })

		// Spores (flora reproduction)
		measure("spores", func() { g.spores.Update(g.tick, g.createOrganism) })

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
	var floraCount, faunaCount, deadCount int
	var floraEnergy, faunaEnergy float32
	var herbivoreCount, carnivoreCount, carrionCount int
	var minFaunaEnergy, maxFaunaEnergy float32 = 9999, 0
	var minFloraEnergy, maxFloraEnergy float32 = 9999, 0
	var totalFloraCells, totalFaunaCells int

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()

		if org.Dead {
			deadCount++
			continue
		}

		if traits.IsFlora(org.Traits) {
			floraCount++
			floraEnergy += org.Energy
			totalFloraCells += int(cells.Count)
			if org.Energy < minFloraEnergy {
				minFloraEnergy = org.Energy
			}
			if org.Energy > maxFloraEnergy {
				maxFloraEnergy = org.Energy
			}
		} else {
			faunaCount++
			faunaEnergy += org.Energy
			totalFaunaCells += int(cells.Count)
			if org.Energy < minFaunaEnergy {
				minFaunaEnergy = org.Energy
			}
			if org.Energy > maxFaunaEnergy {
				maxFaunaEnergy = org.Energy
			}

			if org.Traits.Has(traits.Herbivore) {
				herbivoreCount++
			}
			if org.Traits.Has(traits.Carnivore) {
				carnivoreCount++
			}
			if org.Traits.Has(traits.Carrion) {
				carrionCount++
			}
		}
	}

	avgFloraEnergy := float32(0)
	avgFaunaEnergy := float32(0)
	if floraCount > 0 {
		avgFloraEnergy = floraEnergy / float32(floraCount)
		// Handle NaN from potential edge cases
		if avgFloraEnergy != avgFloraEnergy { // NaN check
			avgFloraEnergy = 0
		}
	}
	if faunaCount > 0 {
		avgFaunaEnergy = faunaEnergy / float32(faunaCount)
		if avgFaunaEnergy != avgFaunaEnergy {
			avgFaunaEnergy = 0
		}
	}
	if minFaunaEnergy == 9999 {
		minFaunaEnergy = 0
	}
	if minFloraEnergy == 9999 {
		minFloraEnergy = 0
	}

	logf("=== Tick %d ===", g.tick)
	logf("Flora: %d (cells: %d, energy: %.1f avg, %.1f-%.1f range)",
		floraCount, totalFloraCells, avgFloraEnergy, minFloraEnergy, maxFloraEnergy)
	logf("Fauna: %d (cells: %d, energy: %.1f avg, %.1f-%.1f range)",
		faunaCount, totalFaunaCells, avgFaunaEnergy, minFaunaEnergy, maxFaunaEnergy)
	logf("  Herbivores: %d, Carnivores: %d, Carrion: %d",
		herbivoreCount, carnivoreCount, carrionCount)
	logf("Dead: %d, Spores: %d, Particles: %d",
		deadCount, g.spores.Count(), g.particles.Count())

	// Count breeding-eligible fauna and trait diversity
	breedingEligible := 0
	var modeGrow, modeBreed, modeSurvive, modeStore int
	var withSpeed, withHerding, withFarSight, omnivores int
	var withPredatorEyes, withPreyEyes int
	query2 := g.allOrgFilter.Query()
	for query2.Next() {
		_, _, org, cells := query2.Get()
		if org.Dead || traits.IsFlora(org.Traits) {
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
		// Count trait diversity
		if org.Traits.Has(traits.Speed) {
			withSpeed++
		}
		if org.Traits.Has(traits.Herding) {
			withHerding++
		}
		if org.Traits.Has(traits.FarSight) {
			withFarSight++
		}
		if org.Traits.Has(traits.PredatorEyes) {
			withPredatorEyes++
		}
		if org.Traits.Has(traits.PreyEyes) {
			withPreyEyes++
		}
		if traits.IsOmnivore(org.Traits) {
			omnivores++
		}
		if org.Traits.Has(traits.Breeding) && org.AllocationMode == components.ModeBreed && org.Energy >= org.MaxEnergy*0.35 && cells.Count >= 1 && org.BreedingCooldown == 0 {
			breedingEligible++
		}
	}

	// Count light sensitivity traits
	var photophilic, photophobic int
	query3 := g.allOrgFilter.Query()
	for query3.Next() {
		_, _, org, _ := query3.Get()
		if org.Dead || traits.IsFlora(org.Traits) {
			continue
		}
		if org.Traits.Has(traits.Photophilic) {
			photophilic++
		}
		if org.Traits.Has(traits.Photophobic) {
			photophobic++
		}
	}

	logf("Breeding eligible: %d", breedingEligible)
	logf("Modes: Grow=%d, Breed=%d, Survive=%d, Store=%d", modeGrow, modeBreed, modeSurvive, modeStore)
	logf("Traits: Speed=%d, Herding=%d, FarSight=%d, PredEyes=%d, PreyEyes=%d, Omnivore=%d",
		withSpeed, withHerding, withFarSight, withPredatorEyes, withPreyEyes, omnivores)
	logf("Light: Photophilic=%d, Photophobic=%d (Sun: %.2f)", photophilic, photophobic, g.light.PosX)
	logf("")
}

func (g *Game) collectPositions() ([]components.Position, []components.Position) {
	var floraPos, faunaPos []components.Position

	floraQuery := g.floraFilter.Query()
	for floraQuery.Next() {
		pos, _, _ := floraQuery.Get()
		floraPos = append(floraPos, *pos)
	}

	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		pos, _, _ := faunaQuery.Get()
		faunaPos = append(faunaPos, *pos)
	}

	return floraPos, faunaPos
}

func (g *Game) collectOccluders() []systems.Occluder {
	var occluders []systems.Occluder

	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, cells := query.Get()

		if org.Dead || cells.Count == 0 {
			continue
		}

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

		// Add cell size padding
		minX -= org.CellSize / 2
		minY -= org.CellSize / 2
		maxX += org.CellSize / 2
		maxY += org.CellSize / 2

		occluders = append(occluders, systems.Occluder{
			X:      minX,
			Y:      minY,
			Width:  maxX - minX,
			Height: maxY - minY,
		})
	}

	return occluders
}

func (g *Game) collectOrganisms() ([]*components.Organism, []*components.Organism) {
	var floraOrgs, faunaOrgs []*components.Organism

	floraQuery := g.floraFilter.Query()
	for floraQuery.Next() {
		_, org, _ := floraQuery.Get()
		floraOrgs = append(floraOrgs, org)
	}

	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		_, org, _ := faunaQuery.Get()
		faunaOrgs = append(faunaOrgs, org)
	}

	return floraOrgs, faunaOrgs
}

func (g *Game) updateGrowth() {
	// Collect deferred actions to avoid modifying world during query
	type splitRequest struct {
		pos       *components.Position
		vel       *components.Velocity
		org       *components.Organism
		cells     *components.CellBuffer
	}
	var pendingSplits []splitRequest

	type sporeRequest struct {
		x, y   float32
		traits traits.Trait
	}
	var pendingSpores []sporeRequest

	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, vel, org, cells := query.Get()

		if org.Dead {
			// Emit death particles
			g.particles.EmitDeath(pos.X, pos.Y)
			continue
		}

		// Check for disease and emit particles
		for i := uint8(0); i < cells.Count; i++ {
			if cells.Cells[i].Mutation == traits.Disease {
				cellX := pos.X + float32(cells.Cells[i].GridX)*org.CellSize
				cellY := pos.Y + float32(cells.Cells[i].GridY)*org.CellSize
				g.particles.EmitDisease(cellX, cellY)
				break // Only emit once per organism per tick
			}
		}

		org.GrowthTimer++
		// Only grow if in Grow mode and below target cell count
		canGrow := org.AllocationMode == components.ModeGrow && cells.Count < org.TargetCells
		if org.GrowthTimer >= org.GrowthInterval && org.Energy > 40 && canGrow {
			org.GrowthTimer = 0
			g.tryGrow(pos, org, cells)

			// Queue splitting for after query completes
			if g.shouldTrySplit(cells) {
				pendingSplits = append(pendingSplits, splitRequest{pos, vel, org, cells})
			}
		}

		// Spore timer (flora only)
		if traits.IsFlora(org.Traits) {
			org.SporeTimer++
			// Queue spore spawn for after query completes
			if org.SporeTimer >= org.SporeInterval && org.Energy > 30 {
				org.SporeTimer = 0
				org.Energy -= 10 // Reduced cost from 20
				pendingSpores = append(pendingSpores, sporeRequest{pos.X, pos.Y - org.CellSize, org.Traits})
			}
		}
	}

	// Process deferred splits (after query iteration completes)
	for _, req := range pendingSplits {
		g.splitting.TrySplit(req.pos, req.vel, req.org, req.cells, g.createOrganism, g.particles)
	}

	// Process deferred spore spawns
	for _, req := range pendingSpores {
		g.spores.SpawnSpore(req.x, req.y, req.traits)
	}
}

// shouldTrySplit checks if an organism should attempt to split.
func (g *Game) shouldTrySplit(cells *components.CellBuffer) bool {
	if cells.Count < 4 {
		return false
	}
	for i := uint8(0); i < cells.Count; i++ {
		if cells.Cells[i].Mutation == traits.Splitting {
			return true
		}
	}
	return false
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

	// Pick position - weighted for flora (phototropism), random for fauna
	var newPos gridPos
	if traits.IsFlora(org.Traits) {
		newPos = g.selectFloraGrowthPosition(orgPos, org, candidates)
	} else {
		newPos = candidates[rand.Intn(len(candidates))]
	}

	// Pick random trait
	var newTrait traits.Trait
	if traits.IsFlora(org.Traits) {
		newTrait = pickFloraTraitWeighted()
	} else {
		newTrait = pickFaunaTraitWeighted()
	}

	// Add cell
	cells.AddCell(components.Cell{
		GridX:         newPos.x,
		GridY:         newPos.y,
		Age:           0,
		MaxAge:        3000 + rand.Int31n(2000),
		Trait:         newTrait,
		Mutation:      pickMutation(),
		Alive:         true,
		Decomposition: 0,
	})

	// Add trait to organism
	if newTrait != 0 {
		org.Traits = org.Traits.Add(newTrait)
	}

	org.Energy -= 30
}

// selectFloraGrowthPosition uses phototropism to weight growth toward light.
func (g *Game) selectFloraGrowthPosition(orgPos *components.Position, org *components.Organism, candidates []gridPos) gridPos {
	if len(candidates) == 1 {
		return candidates[0]
	}

	sunX := g.light.PosX * g.bounds.Width
	sunY := g.light.PosY * g.bounds.Height

	// Calculate weights for each candidate
	weights := make([]float32, len(candidates))
	totalWeight := float32(0)

	for i, c := range candidates {
		// World position of candidate
		worldX := orgPos.X + float32(c.x)*org.CellSize
		worldY := orgPos.Y + float32(c.y)*org.CellSize

		// Base weight from light intensity
		light := g.shadowMap.SampleLight(worldX, worldY)
		weight := light

		// Direction to sun
		toSunX, toSunY := g.shadowMap.SunDirection(worldX, worldY, sunX, sunY)

		// Growth direction (normalized)
		growthDX := float32(c.x)
		growthDY := float32(c.y)
		growthMag := float32(math.Sqrt(float64(growthDX*growthDX + growthDY*growthDY)))
		if growthMag > 0 {
			growthDX /= growthMag
			growthDY /= growthMag
		}

		// Dot product: direction bonus for growing toward sun
		dot := growthDX*toSunX + growthDY*toSunY
		weight += dot * 0.3

		// Rooted flora prefer growing upward, penalize downward
		if org.Traits.Has(traits.Rooted) {
			if c.y < 0 { // Growing up
				weight *= 1.5
			} else if c.y > 0 { // Growing down
				weight -= 0.3
			}
		}

		// Minimum weight
		if weight < 0.01 {
			weight = 0.01
		}

		weights[i] = weight
		totalWeight += weight
	}

	// Weighted random selection
	r := rand.Float32() * totalWeight
	cumulative := float32(0)
	for i, w := range weights {
		cumulative += w
		if r <= cumulative {
			return candidates[i]
		}
	}

	// Fallback
	return candidates[len(candidates)-1]
}

func pickFloraTraitWeighted() traits.Trait {
	r := rand.Float32()
	// Flora can gain Floating trait through growth
	if r < 0.03 {
		return traits.Floating
	}
	return 0 // Most flora cells are simple
}

func pickFaunaTraitWeighted() traits.Trait {
	// Use weighted selection from traits package + diet evolution
	// This allows organisms to evolve new capabilities through growth
	r := rand.Float32()
	total := float32(0)

	// Fauna-compatible traits with weights (allows diet evolution for diversity)
	weights := []struct {
		t traits.Trait
		w float32
	}{
		// Behavior traits (common)
		{traits.Herding, 0.10},
		{traits.Breeding, 0.06},

		// Vision traits (moderate)
		{traits.PreyEyes, 0.05},
		{traits.PredatorEyes, 0.04},
		{traits.FarSight, 0.04},

		// Physical traits
		{traits.Speed, 0.05},

		// Light sensitivity (creates habitat preferences)
		{traits.Photophilic, 0.06}, // Prefers bright areas
		{traits.Photophobic, 0.06}, // Prefers shadows

		// Diet evolution (rare but important for diversity)
		{traits.Herbivore, 0.03}, // Can become omnivore
		{traits.Carnivore, 0.02}, // Can become predator
		{traits.Carrion, 0.04},   // Can become scavenger
	}

	for _, w := range weights {
		total += w.w
		if r < total {
			return w.t
		}
	}

	// ~52% chance of no trait (simple growth)
	return 0
}

func pickMutation() traits.Mutation {
	// Use weights from traits package for consistency
	r := rand.Float32()
	total := float32(0)

	for mutation, weight := range traits.MutationWeights {
		total += weight
		if r < total {
			return mutation
		}
	}

	return traits.NoMutation
}

// updateDayNightCycle moves the sun across the sky and adjusts light intensity.
func (g *Game) updateDayNightCycle() {
	// Day cycle duration in ticks (about 60 seconds at normal speed)
	const cycleDuration = 3600
	const sunSpeed = 1.4 / float32(cycleDuration) // Travel 1.4 units (from 1.2 to -0.2)
	const darknessDuration = 900                  // ~15 seconds of darkness

	// If in darkness period, count down
	if g.nightTicks > 0 {
		g.nightTicks--
		g.light.Intensity = 0.0
		// When darkness ends, reset sun to right side
		if g.nightTicks == 0 {
			g.light.PosX = 1.2
		}
		return
	}

	// Move sun from right to left
	g.light.PosX -= sunSpeed

	// When sun goes off-screen left, enter darkness period then reset
	if g.light.PosX < -0.2 {
		g.nightTicks = darknessDuration
		g.light.Intensity = 0.0
		return
	}

	// Calculate intensity based on sun position
	// Sun starts at 0 intensity on right edge, peaks at center, falls to 0 at left edge
	if g.light.PosX >= 0 && g.light.PosX <= 1.0 {
		// Distance from center (0 at center, 0.5 at edges)
		centerDist := g.light.PosX - 0.5
		if centerDist < 0 {
			centerDist = -centerDist
		}
		// Intensity: 1.0 at center (dist=0), 0.0 at edges (dist=0.5)
		// Use smooth curve for natural falloff
		normalizedDist := centerDist / 0.5 // 0 at center, 1 at edges
		g.light.Intensity = 1.0 - normalizedDist*normalizedDist // Quadratic falloff
	} else {
		// Sun off-screen = no light
		g.light.Intensity = 0.0
	}
}

func (g *Game) cleanupDead() {
	const maxDeadTime = 600 // Remove after ~10 seconds at normal speed

	// Collect entities to remove (can't modify during query)
	var toRemove []ecs.Entity

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, _ := query.Get()

		if org.Dead {
			org.DeadTime++
			if org.DeadTime > maxDeadTime {
				toRemove = append(toRemove, query.Entity())
			}
		}
	}

	// Remove dead entities
	for _, e := range toRemove {
		g.world.RemoveEntity(e)
	}
}

func (g *Game) Draw() {
	rl.BeginDrawing()

	// Draw animated water background
	g.waterBackground.Draw(float32(g.tick) * 0.016) // Convert tick to approximate seconds

	// Draw flow field particles (on top of water)
	g.flowRenderer.Draw(g.flowField.Particles, g.tick)

	// Collect occluders from organisms for shadow casting
	occluders := g.collectOccluders()

	// Draw sun with shadows
	g.sunRenderer.Draw(g.light, occluders)

	// Draw all organisms
	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, cells := query.Get()
		g.drawOrganism(pos, org, cells)
	}

	// Draw spores
	g.drawSpores()

	// Draw effect particles
	g.particleRenderer.Draw(g.particles.Particles)

	// Draw ambient darkness overlay (based on sun intensity)
	g.sunRenderer.DrawAmbientDarkness(g.light.Intensity)

	// Draw UI
	g.drawUI()

	// Draw tooltip for hovered organism
	g.drawTooltip()

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

	// Type header
	if traits.IsFlora(org.Traits) {
		lines = append(lines, "FLORA")
	} else if org.Traits.Has(traits.Carnivore) && org.Traits.Has(traits.Herbivore) {
		lines = append(lines, "OMNIVORE")
	} else if org.Traits.Has(traits.Carnivore) {
		lines = append(lines, "CARNIVORE")
	} else if org.Traits.Has(traits.Herbivore) {
		lines = append(lines, "HERBIVORE")
	} else if org.Traits.Has(traits.Carrion) {
		lines = append(lines, "SCAVENGER")
	} else {
		lines = append(lines, "ORGANISM")
	}

	lines = append(lines, "")

	// Stats
	lines = append(lines, fmt.Sprintf("Energy: %.0f / %.0f", org.Energy, org.MaxEnergy))
	lines = append(lines, fmt.Sprintf("Cells: %d", cells.Count))
	lines = append(lines, fmt.Sprintf("Speed: %.2f", org.MaxSpeed))

	if org.Dead {
		lines = append(lines, "STATUS: DEAD")
	}

	lines = append(lines, "")

	// Traits
	traitNames := traits.TraitNames(org.Traits)
	if len(traitNames) > 0 {
		lines = append(lines, "Traits:")
		// Group traits into rows of 2
		for i := 0; i < len(traitNames); i += 2 {
			if i+1 < len(traitNames) {
				lines = append(lines, fmt.Sprintf("  %s, %s", traitNames[i], traitNames[i+1]))
			} else {
				lines = append(lines, fmt.Sprintf("  %s", traitNames[i]))
			}
		}
	}

	// Mutations
	var mutations []string
	for i := uint8(0); i < cells.Count; i++ {
		if cells.Cells[i].Mutation != traits.NoMutation {
			mutName := traits.MutationName(cells.Cells[i].Mutation)
			if mutName != "" {
				// Check if already in list
				found := false
				for _, m := range mutations {
					if m == mutName {
						found = true
						break
					}
				}
				if !found {
					mutations = append(mutations, mutName)
				}
			}
		}
	}
	if len(mutations) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Mutations: "+strings.Join(mutations, ", "))
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

	// Draw text
	r, gr, b := traits.GetTraitColor(org.Traits)
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

func (g *Game) drawOrganism(pos *components.Position, org *components.Organism, cells *components.CellBuffer) {
	r, gr, b := traits.GetTraitColor(org.Traits)
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

	// Draw each cell
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if !cell.Alive {
			continue
		}

		cellX := pos.X + float32(cell.GridX)*org.CellSize
		cellY := pos.Y + float32(cell.GridY)*org.CellSize

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

		// Draw cell
		rl.DrawRectangle(
			int32(cellX-org.CellSize/2),
			int32(cellY-org.CellSize/2),
			int32(org.CellSize),
			int32(org.CellSize),
			cellColor,
		)

		// Draw mutation indicator (also affected by lighting)
		if cell.Mutation != traits.NoMutation {
			var mutColor rl.Color
			switch cell.Mutation {
			case traits.Disease:
				mutColor = rl.Color{R: uint8(100 * light), G: uint8(50 * light), B: uint8(100 * light), A: 200}
			case traits.Rage:
				mutColor = rl.Color{R: uint8(255 * light), G: uint8(100 * light), B: uint8(50 * light), A: 200}
			case traits.Growth:
				mutColor = rl.Color{R: uint8(100 * light), G: uint8(255 * light), B: uint8(100 * light), A: 200}
			case traits.Splitting:
				mutColor = rl.Color{R: uint8(200 * light), G: uint8(200 * light), B: uint8(50 * light), A: 200}
			}
			rl.DrawCircle(int32(cellX), int32(cellY), 1, mutColor)
		}
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

func (g *Game) drawUI() {
	// Count organisms
	floraCount := 0
	faunaCount := 0
	totalCells := 0

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()
		if !org.Dead {
			if traits.IsFlora(org.Traits) {
				floraCount++
			} else if traits.IsFauna(org.Traits) {
				faunaCount++
			}
			totalCells += int(cells.Count)
		}
	}

	// Draw stats
	rl.DrawText("Primordial Soup", 10, 10, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Flora: %d | Fauna: %d | Cells: %d", floraCount, faunaCount, totalCells), 10, 35, 16, rl.LightGray)
	rl.DrawText(fmt.Sprintf("Tick: %d | Speed: %dx | FPS: %d | Spores: %d", g.tick, g.stepsPerFrame, rl.GetFPS(), g.spores.Count()), 10, 55, 16, rl.LightGray)

	statusText := "Running"
	if g.paused {
		statusText = "PAUSED"
	}
	rl.DrawText(statusText, 10, 75, 16, rl.Yellow)

	// Performance stats (right side)
	if *perfLog {
		x := int32(screenWidth - 200)
		y := int32(10)
		rl.DrawText("System Performance", x, y, 16, rl.White)
		y += 20

		total := g.perf.Total()
		rl.DrawText(fmt.Sprintf("Total: %s", total.Round(time.Microsecond)), x, y, 14, rl.Yellow)
		y += 16

		for i, name := range g.perf.SortedNames() {
			if i >= 12 {
				break // Limit display
			}
			avg := g.perf.Avg(name)
			pct := float64(0)
			if total > 0 {
				pct = float64(avg) / float64(total) * 100
			}
			color := rl.LightGray
			if pct > 20 {
				color = rl.Red
			} else if pct > 10 {
				color = rl.Orange
			}
			rl.DrawText(fmt.Sprintf("%-16s %6s %5.1f%%", name, avg.Round(time.Microsecond), pct), x, y, 12, color)
			y += 14
		}
	}

	// Controls
	rl.DrawText("SPACE: Pause | < >: Speed | Click: Add Herbivore | F: Flora | C: Carnivore", 10, int32(screenHeight-25), 14, rl.Gray)
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

func logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if logWriter != nil {
		fmt.Fprintln(logWriter, msg)
	} else {
		fmt.Println(msg)
	}
}

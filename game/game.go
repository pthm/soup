package game

import (
	"fmt"
	"log/slog"
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/inspector"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
	"github.com/pthm-cable/soup/telemetry"
)

// Screen dimensions
const (
	ScreenWidth  = 1280
	ScreenHeight = 720
)

// Simulation constants
const (
	DT           = 1.0 / 60.0 // seconds per tick
	GridCellSize = 64.0       // spatial grid cell size
)

// Physics constants - mapped from entity Capabilities
const (
	DefaultMaxSpeed     = 80.0
	DefaultAcceleration = 0.1
	DefaultTurnRateMax  = 3.5
)

// Initial population
const (
	InitialPopulation = 20
)

// Reproduction constants
const (
	PreyReproThreshold float32 = 0.85
	PredReproThreshold float32 = 0.90
	MaturityAge        float32 = 8.0  // seconds
	PreyCooldown       float32 = 8.0  // seconds between reproductions
	PredCooldown       float32 = 10.0 // seconds between reproductions
	MaxPrey                    = 400
	MaxPred                    = 120
)

// Mutation parameters for sparse mutation
const (
	MutationRate     = 0.05
	MutationSigma    = 0.08
	MutationBigRate  = 0.01
	MutationBigSigma = 0.40
)

// normalizeAngle wraps angle to [-pi, pi].
func normalizeAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

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
	neighborBuf []systems.Neighbor        // reused each entity in behavior loop
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

	g := &Game{
		world:  world,
		rng:    rand.New(rand.NewSource(opts.Seed)),
		width:  float32(ScreenWidth),
		height: float32(ScreenHeight),
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
	g.collector = telemetry.NewCollector(opts.StatsWindowSec, DT)
	g.bookmarkDetector = telemetry.NewBookmarkDetector(10) // 10 windows of history
	g.lifetimeTracker = telemetry.NewLifetimeTracker()
	g.perfCollector = telemetry.NewPerfCollector(600) // 10 seconds at 60 ticks/sec

	// Spatial grid
	g.spatialGrid = systems.NewSpatialGrid(g.width, g.height, GridCellSize)

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

// spawnInitialPopulation creates the starting entities.
func (g *Game) spawnInitialPopulation() {
	for i := 0; i < InitialPopulation; i++ {
		x := g.rng.Float32() * g.width
		y := g.rng.Float32() * g.height
		heading := g.rng.Float32() * 2 * math.Pi

		// Alternate between prey and predator
		kind := components.KindPrey
		if i%4 == 0 {
			kind = components.KindPredator
		}

		g.spawnEntity(x, y, heading, kind)
	}
}

// spawnEntity creates a new entity with a fresh brain.
func (g *Game) spawnEntity(x, y, heading float32, kind components.Kind) ecs.Entity {
	id := g.nextID
	g.nextID++

	pos := components.Position{X: x, Y: y}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: heading, AngVel: 0}
	body := components.Body{Radius: 10}
	energy := components.Energy{Value: 0.8, Age: 0, Alive: true}
	caps := components.DefaultCapabilities()
	org := components.Organism{ID: id, Kind: kind, ReproCooldown: MaturityAge}

	// Create brain
	brain := neural.NewFFNN(g.rng)
	g.brains[id] = brain

	entity := g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
	g.aliveCount++

	// Register with lifetime tracker
	g.lifetimeTracker.Register(id, g.tick)

	// Track population by kind
	if kind == components.KindPrey {
		g.numPrey++
	} else {
		g.numPred++
	}

	return entity
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

// flushTelemetry checks if the stats window should be flushed and handles bookmarks.
func (g *Game) flushTelemetry() {
	if !g.collector.ShouldFlush(g.tick) {
		return
	}

	// Sample energy distributions and resource utilization
	preyEnergies, predEnergies, meanResource := g.sampleEnergyDistributions()

	// Flush the stats window
	stats := g.collector.Flush(g.tick, g.numPrey, g.numPred, preyEnergies, predEnergies, meanResource)

	// Log stats if enabled
	if g.logStats {
		stats.LogStats()
		g.perfCollector.Stats().LogStats()
	}

	// Check for bookmarks
	bookmarks := g.bookmarkDetector.Check(stats)
	for _, bm := range bookmarks {
		if g.logStats {
			bm.LogBookmark()
		}

		// Save snapshot on bookmark
		if g.snapshotDir != "" {
			g.saveSnapshot(&bm)
		}
	}
}

// sampleEnergyDistributions collects energy values for percentile calculation.
func (g *Game) sampleEnergyDistributions() (preyEnergies, predEnergies []float64, meanResource float64) {
	var resourceSum float64
	var preyCount int

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		if org.Kind == components.KindPrey {
			preyEnergies = append(preyEnergies, float64(energy.Value))
			resourceSum += float64(g.resourceField.Sample(pos.X, pos.Y))
			preyCount++
		} else {
			predEnergies = append(predEnergies, float64(energy.Value))
		}

		// Update lifetime peak energy
		g.lifetimeTracker.UpdateEnergy(org.ID, energy.Value)
	}

	if preyCount > 0 {
		meanResource = resourceSum / float64(preyCount)
	}

	return preyEnergies, predEnergies, meanResource
}

// saveSnapshot creates and saves a snapshot to disk.
func (g *Game) saveSnapshot(bookmark *telemetry.Bookmark) {
	snapshot := g.createSnapshot(bookmark)

	path, err := telemetry.SaveSnapshot(snapshot, g.snapshotDir)
	if err != nil {
		slog.Error("failed to save snapshot", "error", err)
		return
	}

	slog.Info("snapshot saved", "path", path, "tick", g.tick)
}

// createSnapshot builds a snapshot from the current state.
func (g *Game) createSnapshot(bookmark *telemetry.Bookmark) *telemetry.Snapshot {
	snapshot := &telemetry.Snapshot{
		Version:     telemetry.SnapshotVersion,
		RNGSeed:     g.rngSeed,
		WorldWidth:  g.width,
		WorldHeight: g.height,
		// Resource field is now procedural (GPU noise), no hotspots to serialize
		ResourceHotspots: nil,
		ResourceSigma:    0,
		Tick:             g.tick,
		Bookmark:         bookmark,
	}

	// Collect entity states
	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, rot, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Get brain weights
		brain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Get lifetime stats
		var lifetime *telemetry.LifetimeStatsJSON
		if ls := g.lifetimeTracker.Get(org.ID); ls != nil {
			lifetime = ls.ToJSON()
		}

		state := telemetry.EntityState{
			ID:            org.ID,
			Kind:          org.Kind,
			X:             pos.X,
			Y:             pos.Y,
			VelX:          vel.X,
			VelY:          vel.Y,
			Heading:       rot.Heading,
			Energy:        energy.Value,
			Age:           energy.Age,
			ReproCooldown: org.ReproCooldown,
			Brain:         brain.MarshalWeights(),
			Lifetime:      lifetime,
		}

		snapshot.Entities = append(snapshot.Entities, state)
	}

	return snapshot
}

// updateSpatialGrid rebuilds the spatial index.
func (g *Game) updateSpatialGrid() {
	g.spatialGrid.Clear()

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, _, _ := query.Get()

		if energy.Alive {
			g.spatialGrid.Insert(entity, pos.X, pos.Y)
		}
	}
}

// updateBehaviorAndPhysics runs brains and applies movement.
func (g *Game) updateBehaviorAndPhysics() {
	// Check if we have a selected entity for inspector (headless mode has no inspector)
	var selectedEntity ecs.Entity
	var hasSelection bool
	if g.inspector != nil {
		selectedEntity, hasSelection = g.inspector.Selected()
	}

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, vel, rot, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Get brain
		brain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Query neighbors into reusable buffer (avoids allocation)
		g.neighborBuf = g.spatialGrid.QueryRadiusInto(
			g.neighborBuf[:0], // reset but keep capacity
			pos.X, pos.Y, caps.VisionRange, entity, g.posMap,
		)

		// Compute sensors using precomputed neighbor data (avoids double distance calc)
		sensorInputs := systems.ComputeSensorsFromNeighbors(
			*vel, *rot, *energy, *caps,
			g.neighborBuf,
			g.orgMap,
			g.resourceField,
			*pos,
		)

		// Fill reusable input buffer (avoids allocation)
		inputs := sensorInputs.FillSlice(g.inputBuf[:])

		// Run brain (capture activations if this is the selected entity)
		var turn, thrust float32
		if hasSelection && entity == selectedEntity {
			var act *neural.Activations
			turn, thrust, _, act = brain.ForwardWithCapture(inputs)
			g.inspector.SetSensorData(&sensorInputs)
			g.inspector.SetActivations(act)
		} else {
			turn, thrust, _ = brain.Forward(inputs)
		}

		// Scale outputs by capabilities
		turnRate := turn * caps.MaxTurnRate * DT
		if turnRate > caps.MaxTurnRate*DT {
			turnRate = caps.MaxTurnRate * DT
		} else if turnRate < -caps.MaxTurnRate*DT {
			turnRate = -caps.MaxTurnRate * DT
		}

		// Apply angular velocity to heading (heading-as-state)
		// Minimum turn rate (30%) even at zero throttle enables arrival behavior
		effectiveTurnRate := turnRate * max(thrust, 0.3)
		rot.Heading += effectiveTurnRate
		rot.Heading = normalizeAngle(rot.Heading)

		// Compute desired velocity from heading
		targetSpeed := thrust * caps.MaxSpeed * DT
		desiredVelX := float32(math.Cos(float64(rot.Heading))) * targetSpeed
		desiredVelY := float32(math.Sin(float64(rot.Heading))) * targetSpeed

		// Smooth velocity change
		accelFactor := caps.MaxAccel * DT * 0.01
		vel.X += (desiredVelX - vel.X) * accelFactor
		vel.Y += (desiredVelY - vel.Y) * accelFactor

		// Apply drag
		dragFactor := float32(math.Exp(-float64(caps.Drag * DT)))
		vel.X *= dragFactor
		vel.Y *= dragFactor

		// Clamp speed
		speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		maxSpeed := caps.MaxSpeed * DT
		if speed > maxSpeed {
			scale := maxSpeed / speed
			vel.X *= scale
			vel.Y *= scale
		}

		// Update position
		pos.X += vel.X
		pos.Y += vel.Y

		// Toroidal wrap
		pos.X = mod(pos.X, g.width)
		pos.Y = mod(pos.Y, g.height)
	}
}

// updateFeeding handles predator attacks.
func (g *Game) updateFeeding() {
	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, caps, org := query.Get()

		if !energy.Alive || org.Kind != components.KindPredator {
			continue
		}

		// Check if brain exists
		_, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Query nearby prey within bite range
		neighbors := g.spatialGrid.QueryRadius(pos.X, pos.Y, caps.BiteRange, entity, g.posMap)

		for _, neighbor := range neighbors {
			// Get neighbor components
			nOrg := g.orgMap.Get(neighbor)
			if nOrg == nil || nOrg.Kind != components.KindPrey {
				continue
			}

			// Get prey energy directly via mapper
			nEnergy := g.energyMap.Get(neighbor)
			if nEnergy != nil && nEnergy.Alive {
				// Record bite attempt
				g.collector.RecordBiteAttempt()
				g.lifetimeTracker.RecordBiteAttempt(org.ID)

				preyWasAlive := nEnergy.Alive

				// Simple bite: transfer energy
				transferred := systems.TransferEnergy(energy, nEnergy, 0.1)

				if transferred > 0 {
					// Record successful bite
					g.collector.RecordBiteHit()
					g.lifetimeTracker.RecordBiteHit(org.ID)

					// Check for kill
					if preyWasAlive && !nEnergy.Alive {
						g.collector.RecordKill()
						g.lifetimeTracker.RecordKill(org.ID)
					}
				}

				break // one bite per tick
			}
		}
	}
}

// updateEnergy applies metabolic costs and prey foraging.
func (g *Game) updateEnergy() {
	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Prey gain energy from resource field
		if org.Kind == components.KindPrey {
			r := g.resourceField.Sample(pos.X, pos.Y)
			systems.UpdatePreyForage(energy, *vel, *caps, r, DT)
		}

		// Apply metabolic costs (per-kind)
		systems.UpdateEnergy(energy, *vel, *caps, org.Kind, false, DT)
	}
}

// cleanupDead removes dead entities and their brains.
func (g *Game) cleanupDead() {
	// First pass: collect dead entities (must complete before modifying)
	type deadInfo struct {
		entity ecs.Entity
		id     uint32
		kind   components.Kind
	}
	var toRemove []deadInfo

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			toRemove = append(toRemove, deadInfo{entity: entity, id: org.ID, kind: org.Kind})
		}
	}

	// Second pass: remove entities (query iteration complete)
	for _, dead := range toRemove {
		// Record death in telemetry
		g.collector.RecordDeath(dead.kind)
		g.lifetimeTracker.Remove(dead.id)

		g.entityMapper.Remove(dead.entity)
		delete(g.brains, dead.id)
		g.aliveCount--
		g.deadCount++

		// Track population by kind
		if dead.kind == components.KindPrey {
			g.numPrey--
		} else {
			g.numPred--
		}
	}

	// Respawn if population drops too low
	if g.aliveCount < 5 && g.tick > 100 {
		for i := 0; i < 5; i++ {
			x := g.rng.Float32() * g.width
			y := g.rng.Float32() * g.height
			heading := g.rng.Float32() * 2 * math.Pi
			kind := components.KindPrey
			if g.rng.Float32() < 0.25 {
				kind = components.KindPredator
			}
			g.spawnEntity(x, y, heading, kind)
		}
	}
}

// updateCooldowns decrements reproduction cooldowns.
func (g *Game) updateCooldowns() {
	query := g.entityFilter.Query()
	for query.Next() {
		_, _, _, _, energy, _, org := query.Get()

		if energy.Alive && org.ReproCooldown > 0 {
			org.ReproCooldown -= DT
			if org.ReproCooldown < 0 {
				org.ReproCooldown = 0
			}
		}
	}
}

// updateReproduction handles asexual reproduction with mutation.
func (g *Game) updateReproduction() {
	// Collect births to spawn after iteration
	type birthInfo struct {
		x, y, heading float32
		kind          components.Kind
		parentBrain   *neural.FFNN
		parentID      uint32
	}
	var births []birthInfo

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Check population caps
		if org.Kind == components.KindPrey && g.numPrey >= MaxPrey {
			continue
		}
		if org.Kind == components.KindPredator && g.numPred >= MaxPred {
			continue
		}

		// Check reproduction thresholds
		var threshold, cooldown float32
		if org.Kind == components.KindPredator {
			threshold = PredReproThreshold
			cooldown = PredCooldown
		} else {
			threshold = PreyReproThreshold
			cooldown = PreyCooldown
		}

		if energy.Value < threshold || energy.Age < MaturityAge || org.ReproCooldown > 0 {
			continue
		}

		// Reproduction: parent pays energy, child spawns nearby
		parentBrain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Energy split: parent keeps 55%, child gets 45%
		childEnergy := energy.Value * 0.45
		energy.Value *= 0.55

		// Set cooldown
		org.ReproCooldown = cooldown

		// Queue child spawn
		offset := float32(15 + g.rng.Float32()*10)
		childX := mod(pos.X+(g.rng.Float32()-0.5)*offset*2, g.width)
		childY := mod(pos.Y+(g.rng.Float32()-0.5)*offset*2, g.height)
		childHeading := rot.Heading + (g.rng.Float32()-0.5)*0.5

		births = append(births, birthInfo{
			x:           childX,
			y:           childY,
			heading:     childHeading,
			kind:        org.Kind,
			parentBrain: parentBrain,
			parentID:    org.ID,
		})

		// Store child energy temporarily (will apply after spawn)
		_ = childEnergy
	}

	// Spawn children outside query
	for _, b := range births {
		child := g.spawnEntity(b.x, b.y, b.heading, b.kind)
		childOrg := g.orgMap.Get(child)
		childEnergy := g.energyMap.Get(child)

		if childOrg != nil && childEnergy != nil {
			// Inherit mutated brain
			childBrain := b.parentBrain.Clone()
			childBrain.MutateSparse(g.rng, MutationRate, MutationSigma, MutationBigRate, MutationBigSigma)
			g.brains[childOrg.ID] = childBrain

			// Set child energy from parent split
			childEnergy.Value = 0.35 // Fixed starting energy for children
			childEnergy.Age = 0

			// Record birth in telemetry
			g.collector.RecordBirth(b.kind)
			g.lifetimeTracker.RecordChild(b.parentID)
		}
	}
}

// handleInput processes keyboard input.
func (g *Game) handleInput() {
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}

	// Speed control with < > keys (comma and period)
	if rl.IsKeyPressed(rl.KeyComma) && g.speed > 1 {
		g.speed--
	}
	if rl.IsKeyPressed(rl.KeyPeriod) && g.speed < 10 {
		g.speed++
	}

	// Debug mode toggle
	if rl.IsKeyPressed(rl.KeyD) {
		g.debugMode = !g.debugMode
		if g.debugMode {
			g.debugShowResource = true // Default to showing resource overlay
		}
	}

	// Debug sub-options (only when debug mode is active)
	if g.debugMode {
		if rl.IsKeyPressed(rl.KeyR) {
			g.debugShowResource = !g.debugShowResource
		}
	}

	// Inspector input
	mousePos := rl.GetMousePosition()
	g.inspector.HandleInput(mousePos.X, mousePos.Y, g.posMap, g.bodyMap, g.orgMap, g.entityFilter)
}

// mod returns positive modulo (Go's % can return negative).
func mod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
}

// Draw renders the game.
func (g *Game) Draw() {
	g.perfCollector.RecordFrame()

	rl.BeginDrawing()
	rl.ClearBackground(rl.Black)

	// Water background
	g.water.Draw(float32(g.tick) * 0.01)

	// Debug overlays (drawn before entities so entities appear on top)
	if g.debugMode && g.debugShowResource {
		g.gpuResourceField.DrawOverlayHeatmap(180) // Heatmap with good visibility
	}

	// Draw entities
	g.drawEntities()

	// Draw selection highlight and vision cone
	g.inspector.DrawSelectionHighlight(g.posMap, g.bodyMap, g.rotMap, g.capsMap)

	// Draw HUD
	rl.DrawText(fmt.Sprintf("Tick: %d", g.tick), 10, 10, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Prey: %d  Pred: %d  Dead: %d", g.numPrey, g.numPred, g.deadCount), 10, 35, 20, rl.White)
	rl.DrawText(fmt.Sprintf("Speed: %dx  [</>]", g.speed), 10, 60, 20, rl.White)
	if g.paused {
		rl.DrawText("PAUSED", 10, 85, 20, rl.Yellow)
	}

	// Debug menu
	if g.debugMode {
		g.drawDebugMenu()
	}

	// Draw inspector panel
	g.inspector.Draw(g.posMap, g.velMap, g.rotMap, g.bodyMap, g.energyMap, g.capsMap, g.orgMap, g.brains)

	rl.EndDrawing()
}

// drawEntities renders all entities as oriented triangles.
func (g *Game) drawEntities() {
	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, body, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Color by kind
		color := rl.Green
		if org.Kind == components.KindPredator {
			color = rl.Red
		}

		// Dim based on energy
		alpha := uint8(100 + int(energy.Value*155))
		color.A = alpha

		drawOrientedTriangle(pos.X, pos.Y, rot.Heading, body.Radius, color)
	}
}

// drawDebugMenu renders the debug overlay menu.
func (g *Game) drawDebugMenu() {
	// Semi-transparent background panel
	panelX := int32(g.width) - 200
	panelY := int32(10)
	panelW := int32(190)
	panelH := int32(80)

	rl.DrawRectangle(panelX, panelY, panelW, panelH, rl.Color{R: 0, G: 0, B: 0, A: 180})
	rl.DrawRectangleLines(panelX, panelY, panelW, panelH, rl.Yellow)

	// Title
	rl.DrawText("DEBUG [D to close]", panelX+10, panelY+8, 14, rl.Yellow)

	// Resource overlay toggle
	resourceStatus := "OFF"
	resourceColor := rl.Gray
	if g.debugShowResource {
		resourceStatus = "ON"
		resourceColor = rl.Green
	}
	rl.DrawText(fmt.Sprintf("[R] Resource: %s", resourceStatus), panelX+10, panelY+30, 14, resourceColor)

	// Performance stats
	stats := g.perfCollector.Stats()
	rl.DrawText(fmt.Sprintf("Tick: %v  TPS: %.0f", stats.AvgTickDuration, stats.TicksPerSecond), panelX+10, panelY+55, 12, rl.White)
}

// drawOrientedTriangle draws a triangle pointing in the heading direction.
func drawOrientedTriangle(x, y, heading, radius float32, color rl.Color) {
	cos := float32(math.Cos(float64(heading)))
	sin := float32(math.Sin(float64(heading)))

	// Front point
	frontX := x + cos*radius*1.5
	frontY := y + sin*radius*1.5

	// Back left
	backAngle := heading + math.Pi*0.8
	backLeftX := x + float32(math.Cos(float64(backAngle)))*radius
	backLeftY := y + float32(math.Sin(float64(backAngle)))*radius

	// Back right
	backAngle = heading - math.Pi*0.8
	backRightX := x + float32(math.Cos(float64(backAngle)))*radius
	backRightY := y + float32(math.Sin(float64(backAngle)))*radius

	v1 := rl.Vector2{X: frontX, Y: frontY}
	v2 := rl.Vector2{X: backLeftX, Y: backLeftY}
	v3 := rl.Vector2{X: backRightX, Y: backRightY}

	// DrawTriangle requires counter-clockwise winding (v1, v3, v2)
	rl.DrawTriangle(v1, v3, v2, color)
	rl.DrawTriangleLines(v1, v2, v3, rl.White)
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

package game

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/inspector"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/renderer"
	"github.com/pthm-cable/soup/systems"
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

	// Resource field
	resourceField *systems.ResourceField

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

	// Window dimensions
	width, height float32
}

// NewGame creates a new game instance.
func NewGame() *Game {
	world := ecs.NewWorld()

	g := &Game{
		world:  world,
		rng:    rand.New(rand.NewSource(42)),
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
	}

	// Spatial grid
	g.spatialGrid = systems.NewSpatialGrid(g.width, g.height, GridCellSize)

	// Resource field (12 hotspots)
	g.resourceField = systems.NewResourceField(g.width, g.height, 12, g.rng)

	// GPU flow field
	g.flow = renderer.NewGPUFlowField(g.width, g.height)

	// Water background
	g.water = renderer.NewWaterBackground(int32(g.width), int32(g.height))

	// Inspector
	g.inspector = inspector.NewInspector(int32(g.width), int32(g.height))

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
	// Update flow field
	g.flow.Update(g.tick, float32(g.tick)*0.01)

	// 1. Update spatial grid
	g.updateSpatialGrid()

	// 2. Run behavior (sensors + brains) and physics
	g.updateBehaviorAndPhysics()

	// 3. Handle feeding (predator bites)
	g.updateFeeding()

	// 4. Update energy and check deaths
	g.updateEnergy()

	// 5. Update cooldowns
	g.updateCooldowns()

	// 6. Handle reproduction
	g.updateReproduction()

	// 7. Cleanup dead entities
	g.cleanupDead()

	g.tick++
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
	// Check if we have a selected entity for inspector
	selectedEntity, hasSelection := g.inspector.Selected()

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

		// Query neighbors for sensors
		neighbors := g.spatialGrid.QueryRadius(pos.X, pos.Y, caps.VisionRange, entity, g.posMap)

		// Compute sensors
		sensorInputs := systems.ComputeSensors(
			*pos, *vel, *rot, *energy, *caps, org.Kind,
			neighbors,
			g.posMap, g.orgMap,
			g.resourceField,
			g.width, g.height,
		)

		// Run brain (capture activations if this is the selected entity)
		var turn, thrust float32
		if hasSelection && entity == selectedEntity {
			var act *neural.Activations
			turn, thrust, _, act = brain.ForwardWithCapture(sensorInputs.AsSlice())
			g.inspector.SetSensorData(&sensorInputs)
			g.inspector.SetActivations(act)
		} else {
			turn, thrust, _ = brain.Forward(sensorInputs.AsSlice())
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
				// Simple bite: transfer energy
				systems.TransferEnergy(energy, nEnergy, 0.1)
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
	rl.BeginDrawing()
	rl.ClearBackground(rl.Black)

	// Water background
	g.water.Draw(float32(g.tick) * 0.01)

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
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
}

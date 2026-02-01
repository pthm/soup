package game

import (
	"runtime"
	"sync"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// entitySnapshot captures read-only state for parallel processing.
type entitySnapshot struct {
	Entity ecs.Entity
	ID     uint32
	Kind   components.Kind
	Pos    components.Position
	Vel    components.Velocity
	Rot    components.Rotation
	Energy components.Energy
	Caps   components.Capabilities
	Brain  *neural.FFNN
}

// intent captures computed outputs to apply after parallel phase.
type intent struct {
	NewHeading float32
	NewVelX    float32
	NewVelY    float32
	NewPosX    float32
	NewPosY    float32
	Thrust     float32 // for accel cost calculation
}

// workerScratch holds per-worker reusable buffers.
type workerScratch struct {
	Neighbors  []systems.Neighbor
	Inputs     [systems.NumInputs]float32
	SectorBins systems.SectorBins
}

// parallelState holds resources for parallel behavior computation.
type parallelState struct {
	snapshots  []entitySnapshot
	intents    []intent
	scratches  []workerScratch
	numWorkers int
}

func newParallelState() *parallelState {
	numWorkers := runtime.GOMAXPROCS(0)
	scratches := make([]workerScratch, numWorkers)
	for i := range scratches {
		scratches[i].Neighbors = make([]systems.Neighbor, 0, 64)
	}
	return &parallelState{
		numWorkers: numWorkers,
		scratches:  scratches,
		snapshots:  make([]entitySnapshot, 0, 512),
		intents:    make([]intent, 0, 512),
	}
}

// updateBehaviorAndPhysicsParallel is the parallelized version.
func (g *Game) updateBehaviorAndPhysicsParallel() {
	cfg := config.Cfg()
	dt := cfg.Derived.DT32

	// Phase A: Build snapshots (single-threaded)
	g.parallel.snapshots = g.parallel.snapshots[:0]

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, vel, rot, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		brain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		g.parallel.snapshots = append(g.parallel.snapshots, entitySnapshot{
			Entity: entity,
			ID:     org.ID,
			Kind:   org.Kind,
			Pos:    *pos,
			Vel:    *vel,
			Rot:    *rot,
			Energy: *energy,
			Caps:   *caps,
			Brain:  brain,
		})
	}

	n := len(g.parallel.snapshots)
	if n == 0 {
		return
	}

	// Resize intents slice
	if cap(g.parallel.intents) < n {
		g.parallel.intents = make([]intent, n)
	}
	g.parallel.intents = g.parallel.intents[:n]

	// Phase B: Parallel compute (workers process chunks)
	numWorkers := g.parallel.numWorkers
	chunkSize := (n + numWorkers - 1) / numWorkers

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID, i0, i1 int) {
			defer wg.Done()
			scratch := &g.parallel.scratches[workerID]
			g.computeChunk(i0, i1, scratch, dt)
		}(w, start, end)
	}
	wg.Wait()

	// Phase C: Apply intents (single-threaded, preserves determinism)
	var selectedEntity any
	var hasSelection bool
	if g.inspector != nil {
		selectedEntity, hasSelection = g.inspector.Selected()
	}

	for i, snap := range g.parallel.snapshots {
		intent := &g.parallel.intents[i]

		// Get live component pointers
		pos := g.posMap.Get(snap.Entity)
		vel := g.velMap.Get(snap.Entity)
		rot := g.rotMap.Get(snap.Entity)

		if pos == nil || vel == nil || rot == nil {
			continue
		}

		// Apply computed physics
		rot.Heading = intent.NewHeading
		vel.X = intent.NewVelX
		vel.Y = intent.NewVelY
		pos.X = intent.NewPosX
		pos.Y = intent.NewPosY

		// Store thrust for accel cost calculation
		energy := g.energyMap.Get(snap.Entity)
		if energy != nil {
			energy.LastThrust = intent.Thrust
		}

		// Inspector capture (rare path, only for selected entity)
		if hasSelection && snap.Entity == selectedEntity {
			scratch := &g.parallel.scratches[0]
			scratch.Neighbors = g.spatialGrid.QueryRadiusInto(
				scratch.Neighbors[:0],
				snap.Pos.X, snap.Pos.Y, snap.Caps.VisionRange,
				snap.Entity, g.posMap,
			)
			sensorInputs := systems.ComputeSensorsBounded(
				snap.Vel, snap.Rot, snap.Energy, snap.Caps, snap.Kind,
				scratch.Neighbors, g.orgMap, g.resourceField, snap.Pos,
				&scratch.SectorBins,
			)
			inputs := sensorInputs.FillSlice(scratch.Inputs[:])
			_, _, _, act := snap.Brain.ForwardWithCapture(inputs)
			g.inspector.SetSensorData(&sensorInputs)
			g.inspector.SetActivations(act)
		}
	}
}

// computeChunk processes a range of entities for a single worker.
func (g *Game) computeChunk(i0, i1 int, scratch *workerScratch, dt float32) {
	for i := i0; i < i1; i++ {
		snap := &g.parallel.snapshots[i]
		intent := &g.parallel.intents[i]

		// Query neighbors (read-only spatial grid access)
		scratch.Neighbors = g.spatialGrid.QueryRadiusInto(
			scratch.Neighbors[:0],
			snap.Pos.X, snap.Pos.Y, snap.Caps.VisionRange,
			snap.Entity, g.posMap,
		)

		// Compute sensors (bounded to top-k per sector)
		sensorInputs := systems.ComputeSensorsBounded(
			snap.Vel, snap.Rot, snap.Energy, snap.Caps, snap.Kind,
			scratch.Neighbors, g.orgMap, g.resourceField, snap.Pos,
			&scratch.SectorBins,
		)

		// Fill input buffer and run brain
		inputs := sensorInputs.FillSlice(scratch.Inputs[:])
		turn, thrust, _ := snap.Brain.Forward(inputs)

		// Apply thrust deadzone
		thrustDeadzone := float32(config.Cfg().Capabilities.ThrustDeadzone)
		if thrust < thrustDeadzone {
			thrust = 0
		}

		// Store for accel cost calculation
		intent.Thrust = thrust

		// Compute physics (pure math, no shared state)
		caps := &snap.Caps

		// Turn rate with clamping
		turnRate := turn * caps.MaxTurnRate * dt
		maxTurn := caps.MaxTurnRate * dt
		if turnRate > maxTurn {
			turnRate = maxTurn
		} else if turnRate < -maxTurn {
			turnRate = -maxTurn
		}

		// Heading update
		effectiveTurnRate := turnRate * max(thrust, 0.3)
		newHeading := snap.Rot.Heading + effectiveTurnRate
		newHeading = normalizeAngle(newHeading)

		// Velocity computation
		targetSpeed := thrust * caps.MaxSpeed * dt
		desiredVelX := fastCos(newHeading) * targetSpeed
		desiredVelY := fastSin(newHeading) * targetSpeed

		accelFactor := caps.MaxAccel * dt * 0.01
		newVelX := snap.Vel.X + (desiredVelX-snap.Vel.X)*accelFactor
		newVelY := snap.Vel.Y + (desiredVelY-snap.Vel.Y)*accelFactor

		// Drag
		dragFactor := fastExp(-caps.Drag * dt)
		newVelX *= dragFactor
		newVelY *= dragFactor

		// Speed clamp
		speed := fastSqrt(newVelX*newVelX + newVelY*newVelY)
		maxSpeed := caps.MaxSpeed * dt
		if speed > maxSpeed {
			scale := maxSpeed / speed
			newVelX *= scale
			newVelY *= scale
		}

		// Position + wrap
		newPosX := mod(snap.Pos.X+newVelX, g.worldWidth)
		newPosY := mod(snap.Pos.Y+newVelY, g.worldHeight)

		intent.NewHeading = newHeading
		intent.NewVelX = newVelX
		intent.NewVelY = newVelY
		intent.NewPosX = newPosX
		intent.NewPosY = newPosY
	}
}

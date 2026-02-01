package systems

import (
	"math"
	"math/rand"
	"runtime"
	"sync"
	"time"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

// Behavior system constants
const (
	// Flow field parameters
	flowScale      = 0.003  // Spatial scale for flow field noise
	flowTimeScale  = 0.0001 // Temporal scale for flow field evolution
	flowStrength   = 0.4    // Base flow field strength
	flowDriftY     = 0.05   // Constant downward drift
	flowSideEffect = 0.02   // Amplitude of side-to-side drift

	// Dead organism physics
	deadFlowMultiplier = 1.5  // Dead organisms affected more by flow
	deadSinkRate       = 0.02 // Downward drift rate for dead organisms

	// Capability thresholds
	minSensorGain    = float32(0.1)
	minActuatorGain  = float32(0.1)
	capabilityScale  = 4.0 // Denominator for capability scaling

	// Parallel processing threshold
	minOrganismsForParallel = 100 // Below this, goroutine overhead exceeds benefits

	// Boid field parameters (matching neural/config.go)
	boidSeparationRadius = 3.0  // In body lengths
	boidExpectedNeighbors = 10.0
)

// BehaviorPerfStats tracks subsystem timings within the behavior system.
type BehaviorPerfStats struct {
	EntityListNs int64 // Time building entity lists
	VisionNs     int64 // Time in vision scanning
	BrainNs      int64 // Time in neural network evaluation
	Count        int   // Number of organisms processed
}

// organismTask holds data needed for parallel brain evaluation.
type organismTask struct {
	// Input data (read-only during parallel phase)
	entity           ecs.Entity
	posX, posY       float32
	velX, velY       float32 // Velocity for speed/threat calculations
	heading          float32
	energy           float32
	maxEnergy        float32
	perceptionRadius float32
	maxSpeed         float32
	cellSize         float32
	shapeMetrics     components.ShapeMetrics
	obb              components.CollisionOBB
	brain            *neural.BrainController
	cells            *components.CellBuffer
	faunaIdx         int // Index in faunaPos/faunaOrgs arrays

	// Species and capabilities for field computation
	speciesID         int
	digestiveSpectrum float32
	composition       float32
	structuralArmor   float32
	bodyRadius        float32

	// Output data (written during parallel phase)
	outputs      neural.BehaviorOutputs
	lastInputs   [30]float32 // Store inputs for debugging
	flowX, flowY float32
	hasBrain     bool
}

// BehaviorSystem handles organism steering behaviors using direct neural control.
type BehaviorSystem struct {
	filter      ecs.Filter3[components.Position, components.Velocity, components.Organism]
	brainMap    *ecs.Map[components.Brain]
	cellsMap    *ecs.Map[components.CellBuffer]
	neuralMap   *ecs.Map[components.NeuralGenome] // For species ID lookups
	noise       *PerlinNoise
	tick        int32
	floraSystem *FloraSystem // Flora system for food field queries

	// Parallel processing
	numWorkers    int
	taskBuffer    []organismTask
	floraEntities []neural.EntityInfo   // Pre-built flora list for parallel workers
	faunaVel      []components.Velocity // Fauna velocities for threat calculations
	faunaSpecies  []int                 // Fauna species IDs for boid filtering

	// Performance tracking
	perfEnabled bool
	perfStats   BehaviorPerfStats
}

// NewBehaviorSystem creates a new behavior system.
func NewBehaviorSystem(w *ecs.World) *BehaviorSystem {
	numWorkers := runtime.NumCPU()
	if numWorkers < 1 {
		numWorkers = 1
	}
	return &BehaviorSystem{
		filter:     *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		brainMap:   ecs.NewMap[components.Brain](w),
		cellsMap:   ecs.NewMap[components.CellBuffer](w),
		neuralMap:  ecs.NewMap[components.NeuralGenome](w),
		noise:      NewPerlinNoise(rand.Int63()),
		numWorkers: numWorkers,
	}
}

// SetFloraSystem sets the flora system reference for vision queries.
func (s *BehaviorSystem) SetFloraSystem(fs *FloraSystem) {
	s.floraSystem = fs
}

// SetPerfEnabled enables or disables subsystem performance tracking.
func (s *BehaviorSystem) SetPerfEnabled(enabled bool) {
	s.perfEnabled = enabled
}

// GetPerfStats returns the accumulated performance stats and resets them.
func (s *BehaviorSystem) GetPerfStats() BehaviorPerfStats {
	stats := s.perfStats
	s.perfStats = BehaviorPerfStats{}
	return stats
}

// Update runs the behavior system with actuator-driven neural control.
func (s *BehaviorSystem) Update(w *ecs.World, bounds Bounds, floraPositions, faunaPositions []components.Position, floraOrgs, faunaOrgs []*components.Organism, grid *SpatialGrid) {
	s.tick++
	// Cache bounds for toroidal field computations
	worldBounds = bounds

	query := s.filter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, vel, org := query.Get()

		// All ECS organisms are fauna (flora are in FloraSystem)

		// Dead organisms only get flow field influence
		if org.Dead {
			flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
			vel.X += flowX * deadFlowMultiplier
			vel.Y += flowY * deadFlowMultiplier
			vel.Y += deadSinkRate // Slight downward drift (sinking)
			continue
		}

		// Get cell data for actuator-driven movement
		var cells *components.CellBuffer
		if s.cellsMap.Has(entity) {
			cells = s.cellsMap.Get(entity)
		}

		// Get brain outputs or defaults
		var outputs neural.BehaviorOutputs
		if s.brainMap.Has(entity) {
			outputs = s.getBrainOutputs(entity, pos, vel, org, faunaPositions, faunaOrgs, grid)
		} else {
			outputs = neural.DefaultOutputs()
		}

		// Store brain outputs
		org.UTurn = outputs.UTurn
		org.UThrottle = outputs.UThrottle
		org.AttackIntent = outputs.AttackIntent
		org.MateIntent = outputs.MateIntent
		org.BreedIntent = outputs.MateIntent // Alias for compatibility

		// HEADING-AS-STATE: Integrate heading from turn rate
		// Turn rate is scaled by morphology - asymmetric actuators affect turning
		// Allow some turning even at low throttle to enable arrival behavior
		effectiveTurnRate := getEffectiveTurnRate(neural.TurnRateMax, outputs.UTurn, cells)
		turnThrottle := outputs.UThrottle
		if turnThrottle < minTurnThrottle {
			turnThrottle = minTurnThrottle
		}
		org.Heading += outputs.UTurn * effectiveTurnRate * turnThrottle

		// Normalize heading to [-π, π]
		for org.Heading > math.Pi {
			org.Heading -= 2 * math.Pi
		}
		for org.Heading < -math.Pi {
			org.Heading += 2 * math.Pi
		}

		// Calculate organism center using OBB offset (offset is in local space, must be rotated)
		cosH := float32(math.Cos(float64(org.Heading)))
		sinH := float32(math.Sin(float64(org.Heading)))
		centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
		centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

		// Get flow field influence
		flowX, flowY := s.getFlowFieldForce(centerX, centerY, org, bounds)

		// Calculate effective max speed and acceleration based on actuator capability
		effectiveMaxSpeed := getEffectiveMaxSpeed(org.MaxSpeed, cells)
		maxAccel := getMaxAcceleration(cells, org.ShapeMetrics.Drag)

		// HEADING-AS-STATE: Desired velocity is along heading, scaled by throttle
		// Throttle effectiveness is scaled by morphology - rear actuators provide better thrust
		effectiveThrottle := getEffectiveThrust(outputs.UThrottle, cells)
		desiredVelX := cosH * effectiveThrottle * effectiveMaxSpeed
		desiredVelY := sinH * effectiveThrottle * effectiveMaxSpeed

		// Compute acceleration toward desired velocity
		accelX := desiredVelX - vel.X
		accelY := desiredVelY - vel.Y

		// Clamp acceleration magnitude
		accelMag := float32(math.Sqrt(float64(accelX*accelX + accelY*accelY)))
		if accelMag > maxAccel {
			scale := maxAccel / accelMag
			accelX *= scale
			accelY *= scale
			accelMag = maxAccel
		}

		// Apply acceleration and flow field
		vel.X += accelX + flowX
		vel.Y += accelY + flowY

		// Clamp to effective max speed
		speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		if speed > effectiveMaxSpeed {
			scale := effectiveMaxSpeed / speed
			vel.X *= scale
			vel.Y *= scale
		}

		// Heading is now controlled by brain, not derived from velocity

		// Track acceleration magnitude for energy cost
		org.ActiveThrust = accelMag

		if s.perfEnabled {
			s.perfStats.Count++
		}
	}
}

// UpdateParallel runs the behavior system with parallel brain evaluation.
// Uses a 3-phase approach: collect data, parallel compute, apply results.
func (s *BehaviorSystem) UpdateParallel(w *ecs.World, bounds Bounds, floraPositions, faunaPositions []components.Position, floraOrgs, faunaOrgs []*components.Organism, grid *SpatialGrid) {
	s.tick++
	// Cache bounds for toroidal field computations
	worldBounds = bounds

	// Phase 1: Build flora entity list once (shared read-only data)
	s.floraEntities = s.floraEntities[:0]
	if s.floraSystem != nil {
		nearbyFlora := s.floraSystem.GetAllFlora()
		for _, ref := range nearbyFlora {
			s.floraEntities = append(s.floraEntities, neural.EntityInfo{
				X:               ref.X,
				Y:               ref.Y,
				Composition:     1.0,
				DigestiveSpec:   0.0,
				StructuralArmor: DefaultFloraArmor(),
				GeneticDistance: -1,
				IsFlora:         true,
			})
		}
	}

	// Build fauna velocity and species arrays for field calculations
	// These match the indices in faunaPositions/faunaOrgs
	s.faunaVel = s.faunaVel[:0]
	s.faunaSpecies = s.faunaSpecies[:0]

	// Phase 2: Collect organism data into tasks
	s.taskBuffer = s.taskBuffer[:0]
	deadTasks := make([]struct {
		vel *components.Velocity
		org *components.Organism
		pos *components.Position
	}, 0)

	query := s.filter.Query()
	faunaIdx := 0
	for query.Next() {
		entity := query.Entity()
		pos, vel, org := query.Get()

		// Collect velocity and species for all fauna (including dead) to match faunaPos indices
		s.faunaVel = append(s.faunaVel, *vel)

		speciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}
		s.faunaSpecies = append(s.faunaSpecies, speciesID)

		if org.Dead {
			deadTasks = append(deadTasks, struct {
				vel *components.Velocity
				org *components.Organism
				pos *components.Position
			}{vel, org, pos})
			faunaIdx++
			continue
		}

		var brain *neural.BrainController
		hasBrain := false
		if s.brainMap.Has(entity) {
			b := s.brainMap.Get(entity)
			if b != nil && b.Controller != nil {
				brain = b.Controller
				hasBrain = true
			}
		}

		var cells *components.CellBuffer
		if s.cellsMap.Has(entity) {
			cells = s.cellsMap.Get(entity)
		}

		// Compute capabilities for field calculations
		var caps components.Capabilities
		var cellCount int
		if cells != nil {
			caps = cells.ComputeCapabilities()
			cellCount = int(cells.Count)
		} else {
			cellCount = 1
		}

		s.taskBuffer = append(s.taskBuffer, organismTask{
			entity:            entity,
			posX:              pos.X,
			posY:              pos.Y,
			velX:              vel.X,
			velY:              vel.Y,
			heading:           org.Heading,
			energy:            org.Energy,
			maxEnergy:         org.MaxEnergy,
			perceptionRadius:  org.PerceptionRadius,
			maxSpeed:          org.MaxSpeed,
			cellSize:          org.CellSize,
			shapeMetrics:      org.ShapeMetrics,
			obb:               org.OBB,
			brain:             brain,
			cells:             cells,
			faunaIdx:          faunaIdx,
			speciesID:         speciesID,
			digestiveSpectrum: caps.DigestiveSpectrum(),
			composition:       caps.Composition(),
			structuralArmor:   caps.StructuralArmor,
			bodyRadius:        computeBodyRadius(cellCount, org.CellSize),
			hasBrain:          hasBrain,
		})
		faunaIdx++
	}

	// Process dead organisms (no parallelization needed - simple flow calculation)
	for _, d := range deadTasks {
		flowX, flowY := s.getFlowFieldForce(d.pos.X, d.pos.Y, d.org, bounds)
		d.vel.X += flowX * deadFlowMultiplier
		d.vel.Y += flowY * deadFlowMultiplier
		d.vel.Y += deadSinkRate
	}

	if len(s.taskBuffer) == 0 {
		return
	}

	// For small populations, skip parallel overhead
	if len(s.taskBuffer) < minOrganismsForParallel {
		for i := range s.taskBuffer {
			s.processTaskRange(i, i+1, faunaPositions, faunaOrgs, grid, bounds)
		}
	} else {
		// Phase 3: Parallel brain evaluation
		numTasks := len(s.taskBuffer)
		numWorkers := s.numWorkers
		if numWorkers > numTasks {
			numWorkers = numTasks
		}

		var wg sync.WaitGroup
		chunkSize := (numTasks + numWorkers - 1) / numWorkers

		for w := 0; w < numWorkers; w++ {
			start := w * chunkSize
			end := start + chunkSize
			if end > numTasks {
				end = numTasks
			}
			if start >= end {
				break
			}

			wg.Add(1)
			go func(start, end int) {
				defer wg.Done()
				s.processTaskRange(start, end, faunaPositions, faunaOrgs, grid, bounds)
			}(start, end)
		}
		wg.Wait()
	}

	// Phase 4: Apply results back to ECS components
	query2 := s.filter.Query()
	taskIdx := 0
	for query2.Next() {
		entity := query2.Entity()
		_, vel, org := query2.Get()

		if org.Dead {
			continue
		}

		// Find matching task (entities should be in same order)
		for taskIdx < len(s.taskBuffer) && s.taskBuffer[taskIdx].entity != entity {
			taskIdx++
		}
		if taskIdx >= len(s.taskBuffer) {
			break
		}
		task := &s.taskBuffer[taskIdx]
		taskIdx++

		// Apply brain outputs
		outputs := task.outputs
		org.UTurn = outputs.UTurn
		org.UThrottle = outputs.UThrottle
		org.AttackIntent = outputs.AttackIntent
		org.MateIntent = outputs.MateIntent
		org.BreedIntent = outputs.MateIntent // Alias for compatibility
		org.LastInputs = task.lastInputs     // Store for debugging

		// HEADING-AS-STATE: Integrate heading from turn rate
		// Turn rate is scaled by morphology - asymmetric actuators affect turning
		// Allow some turning even at low throttle to enable arrival behavior
		effectiveTurnRate := getEffectiveTurnRate(neural.TurnRateMax, outputs.UTurn, task.cells)
		turnThrottle := outputs.UThrottle
		if turnThrottle < minTurnThrottle {
			turnThrottle = minTurnThrottle
		}
		org.Heading += outputs.UTurn * effectiveTurnRate * turnThrottle

		// Normalize heading to [-π, π]
		for org.Heading > math.Pi {
			org.Heading -= 2 * math.Pi
		}
		for org.Heading < -math.Pi {
			org.Heading += 2 * math.Pi
		}

		cosH := float32(math.Cos(float64(org.Heading)))
		sinH := float32(math.Sin(float64(org.Heading)))

		// Calculate effective max speed and acceleration based on actuator capability
		effectiveMaxSpeed := getEffectiveMaxSpeed(org.MaxSpeed, task.cells)
		maxAccel := getMaxAcceleration(task.cells, org.ShapeMetrics.Drag)

		// HEADING-AS-STATE: Desired velocity is along heading, scaled by throttle
		// Throttle effectiveness is scaled by morphology - rear actuators provide better thrust
		effectiveThrottle := getEffectiveThrust(outputs.UThrottle, task.cells)
		desiredVelX := cosH * effectiveThrottle * effectiveMaxSpeed
		desiredVelY := sinH * effectiveThrottle * effectiveMaxSpeed

		// Compute acceleration toward desired velocity
		accelX := desiredVelX - vel.X
		accelY := desiredVelY - vel.Y

		// Clamp acceleration magnitude
		accelMag := float32(math.Sqrt(float64(accelX*accelX + accelY*accelY)))
		if accelMag > maxAccel {
			scale := maxAccel / accelMag
			accelX *= scale
			accelY *= scale
			accelMag = maxAccel
		}

		// Apply acceleration and flow field
		vel.X += accelX + task.flowX
		vel.Y += accelY + task.flowY

		// Clamp to effective max speed
		speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		if speed > effectiveMaxSpeed {
			scale := effectiveMaxSpeed / speed
			vel.X *= scale
			vel.Y *= scale
		}

		// Heading is now controlled by brain, not derived from velocity

		// Track acceleration magnitude for energy cost
		org.ActiveThrust = accelMag
	}
}

// processTaskRange evaluates brains for a range of tasks (called by worker goroutines).
func (s *BehaviorSystem) processTaskRange(start, end int, faunaPos []components.Position, faunaOrgs []*components.Organism, grid *SpatialGrid, bounds Bounds) {
	for i := start; i < end; i++ {
		task := &s.taskBuffer[i]

		// Calculate flow field (thread-safe - uses Perlin noise)
		flowX, flowY := s.getFlowFieldForceParallel(task.posX, task.posY, task.shapeMetrics, task.energy, task.maxEnergy, task.bodyRadius, bounds)
		task.flowX = flowX
		task.flowY = flowY

		if !task.hasBrain {
			task.outputs = neural.DefaultOutputs()
			continue
		}

		// Calculate effective perception radius
		effectiveRadius := getEffectivePerceptionRadius(task.perceptionRadius, task.cells)

		// Calculate organism center using OBB offset
		cosH := float32(math.Cos(float64(task.heading)))
		sinH := float32(math.Sin(float64(task.heading)))
		centerX := task.posX + task.obb.OffsetX*cosH - task.obb.OffsetY*sinH
		centerY := task.posY + task.obb.OffsetX*sinH + task.obb.OffsetY*cosH

		// Get capabilities
		var caps components.Capabilities
		var cellCount int
		if task.cells != nil {
			caps = task.cells.ComputeCapabilities()
			cellCount = int(task.cells.Count)
		} else {
			cellCount = int((task.maxEnergy - 100) / 50)
			if cellCount < 1 {
				cellCount = 1
			}
		}

		// Compute current speed normalized
		speed := float32(math.Sqrt(float64(task.velX*task.velX + task.velY*task.velY)))
		speedNorm := clampFloat(speed/task.maxSpeed, 0, 1)

		// Gather data for aggregated field computation (thread-safe versions)
		neighbors := gatherSameSpeciesNeighborsSafe(
			centerX, centerY,
			task.faunaIdx,
			task.speciesID,
			effectiveRadius,
			faunaPos, faunaOrgs,
			s.faunaVel, s.faunaSpecies,
			grid,
		)

		foodTargets := gatherFoodTargetsSafe(
			centerX, centerY,
			task.faunaIdx,
			effectiveRadius,
			s.floraEntities,
			faunaPos, faunaOrgs,
			grid,
		)

		threats := gatherThreatsSafe(
			centerX, centerY,
			task.faunaIdx,
			effectiveRadius,
			faunaPos, faunaOrgs,
			grid,
		)

		// Compute aggregated fields
		boidFields := computeBoidFields(
			centerX, centerY,
			task.heading,
			task.bodyRadius,
			effectiveRadius,
			neighbors,
		)

		foodFields := computeFoodFields(
			centerX, centerY,
			task.heading,
			task.digestiveSpectrum,
			effectiveRadius,
			foodTargets,
		)

		threatInfo := computeThreatInfo(
			centerX, centerY,
			task.velX, task.velY,
			task.heading,
			task.maxSpeed,
			effectiveRadius,
			threats,
			task.composition,
			task.structuralArmor,
		)

		approachInfo := computeApproachInfo(
			centerX, centerY,
			task.heading,
			task.bodyRadius,
			effectiveRadius,
			task.digestiveSpectrum,
			foodTargets,
			neighbors,
		)

		// Build sensory inputs with aggregated fields
		sensory := neural.SensoryInputs{
			SpeedNorm:  speedNorm,
			EnergyNorm: task.energy / task.maxEnergy,
			Body: computeBodyDescriptor(
				&components.Organism{CellSize: task.cellSize, ShapeMetrics: task.shapeMetrics},
				&caps, cellCount,
			),
			Boid:             boidFields,
			Food:             foodFields,
			Threat:           threatInfo,
			Approach:         approachInfo,
			MaxSpeed:         task.maxSpeed,
			MaxEnergy:        task.maxEnergy,
			PerceptionRadius: effectiveRadius,
		}

		// Run brain
		inputs := sensory.ToInputs()

		// Store inputs for debugging (convert float64 to float32)
		for i := 0; i < len(inputs) && i < 30; i++ {
			task.lastInputs[i] = float32(inputs[i])
		}

		rawOutputs, err := task.brain.Think(inputs)
		if err != nil {
			task.outputs = neural.DefaultOutputs()
		} else {
			task.outputs = neural.DecodeOutputs(rawOutputs)
		}
	}
}


// getFlowFieldForceParallel calculates flow field without organism pointer (thread-safe).
func (s *BehaviorSystem) getFlowFieldForceParallel(x, y float32, shapeMetrics components.ShapeMetrics, energy, maxEnergy, bodyRadius float32, _ Bounds) (float32, float32) {
	noiseX := s.noise.Noise3D(float64(x)*flowScale, float64(y)*flowScale, float64(s.tick)*flowTimeScale)
	noiseY := s.noise.Noise3D(float64(x)*flowScale+100, float64(y)*flowScale+100, float64(s.tick)*flowTimeScale)

	flowAngle := noiseX * math.Pi * 2
	flowMagnitude := (noiseY + 1) * 0.5
	flowX := float32(math.Cos(flowAngle) * flowMagnitude * flowStrength)
	flowY := float32(math.Sin(flowAngle) * flowMagnitude * flowStrength)

	flowY += flowDriftY
	flowX += float32(math.Sin(float64(s.tick)*flowTimeScale*2)) * flowSideEffect

	// Size-based resistance: larger organisms are less affected by flow
	// Normalize by typical body radius (~5-10 units), clamp to reasonable range
	sizeResistance := float32(math.Min(float64(bodyRadius)/10.0, 1.0)) * 0.5

	// Low drag = more resistance to being pushed by flow (streamlined shapes cut through)
	shapeResistance := (1.0 - shapeMetrics.Drag) * 0.3
	massResistance := float32(math.Min(float64(energy/maxEnergy)/3, 1)) * 0.2
	totalResistance := sizeResistance + shapeResistance + massResistance
	factor := 1 - totalResistance

	return flowX * factor, flowY * factor
}

// getBrainOutputs gathers sensory inputs using aggregated fields and runs the brain.
func (s *BehaviorSystem) getBrainOutputs(
	entity ecs.Entity,
	pos *components.Position,
	vel *components.Velocity,
	org *components.Organism,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) neural.BehaviorOutputs {
	brain := s.brainMap.Get(entity)
	if brain == nil || brain.Controller == nil {
		return neural.DefaultOutputs()
	}

	// Get cell data for sensor weighting and capabilities
	var cells *components.CellBuffer
	if s.cellsMap.Has(entity) {
		cells = s.cellsMap.Get(entity)
	}

	// Calculate effective perception radius based on sensor capability
	effectiveRadius := getEffectivePerceptionRadius(org.PerceptionRadius, cells)

	// Calculate organism center using OBB offset
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	// Get our capabilities
	var caps components.Capabilities
	var cellCount int
	if cells != nil {
		caps = cells.ComputeCapabilities()
		cellCount = int(cells.Count)
	} else {
		cellCount = 1
	}

	// Get species ID for boid field filtering
	speciesID := 0
	if s.neuralMap.Has(entity) {
		if ng := s.neuralMap.Get(entity); ng != nil {
			speciesID = ng.SpeciesID
		}
	}

	// Compute current speed normalized
	speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
	speedNorm := clampFloat(speed/org.MaxSpeed, 0, 1)

	// Body metrics
	bodyRadius := org.BodyRadius
	if bodyRadius < 1 {
		bodyRadius = computeBodyRadius(cellCount, org.CellSize)
	}

	// Build neighbor info for boid fields (sequential version - use regular grid functions)
	// We need to iterate and filter manually since we don't have pre-built arrays
	var neighbors []NeighborInfo
	if speciesID > 0 {
		nearbyIndices := grid.GetNearbyFauna(centerX, centerY, effectiveRadius)
		neighbors = make([]NeighborInfo, 0, len(nearbyIndices))
		for _, idx := range nearbyIndices {
			if idx >= len(faunaOrgs) || faunaOrgs[idx] == org || faunaOrgs[idx].Dead {
				continue
			}
			// Check species - we need to look it up for each neighbor
			// This is slower than the parallel version but correct
			otherBodyRadius := faunaOrgs[idx].BodyRadius
			if otherBodyRadius < 1 {
				otherBodyRadius = faunaOrgs[idx].CellSize
			}
			neighbors = append(neighbors, NeighborInfo{
				PosX:       faunaPos[idx].X,
				PosY:       faunaPos[idx].Y,
				VelX:       0, // Sequential version doesn't have velocity array
				VelY:       0, // Alignment will be less accurate
				BodyRadius: otherBodyRadius,
			})
		}
	}

	// Build food targets
	foodTargets := make([]FoodTarget, 0, 32)
	radiusSq := effectiveRadius * effectiveRadius
	// Add flora from FloraSystem
	if s.floraSystem != nil {
		nearbyFlora := s.floraSystem.GetNearbyFlora(centerX, centerY, effectiveRadius)
		for _, ref := range nearbyFlora {
			// Use toroidal distance for proper wrap-around
			if ToroidalDistanceSq(centerX, centerY, ref.X, ref.Y, worldBounds.Width, worldBounds.Height) <= radiusSq {
				foodTargets = append(foodTargets, FoodTarget{
					PosX:        ref.X,
					PosY:        ref.Y,
					Composition: 1.0, // Plant
					Intensity:   1.0,
				})
			}
		}
	}
	// Add fauna as potential meat
	nearbyFauna := grid.GetNearbyFauna(centerX, centerY, effectiveRadius)
	for _, idx := range nearbyFauna {
		if idx >= len(faunaOrgs) || faunaOrgs[idx] == org {
			continue
		}
		other := faunaOrgs[idx]
		intensity := float32(1.0)
		if other.Dead {
			intensity = other.Energy / other.MaxEnergy
		}
		foodTargets = append(foodTargets, FoodTarget{
			PosX:        faunaPos[idx].X,
			PosY:        faunaPos[idx].Y,
			Composition: 0.0, // Meat
			Intensity:   intensity,
		})
	}

	// Build threats
	threats := make([]neural.EntityInfo, 0, len(nearbyFauna))
	for _, idx := range nearbyFauna {
		if idx >= len(faunaOrgs) || faunaOrgs[idx] == org || faunaOrgs[idx].Dead {
			continue
		}
		threats = append(threats, neural.EntityInfo{
			X:             faunaPos[idx].X,
			Y:             faunaPos[idx].Y,
			Composition:   0.0,
			DigestiveSpec: 0.5, // Unknown diet
			IsFlora:       false,
		})
	}

	// Compute aggregated fields
	var visionStart time.Time
	if s.perfEnabled {
		visionStart = time.Now()
	}

	boidFields := computeBoidFields(
		centerX, centerY,
		org.Heading,
		bodyRadius,
		effectiveRadius,
		neighbors,
	)

	foodFields := computeFoodFields(
		centerX, centerY,
		org.Heading,
		caps.DigestiveSpectrum(),
		effectiveRadius,
		foodTargets,
	)

	threatInfo := computeThreatInfo(
		centerX, centerY,
		vel.X, vel.Y,
		org.Heading,
		org.MaxSpeed,
		effectiveRadius,
		threats,
		caps.Composition(),
		caps.StructuralArmor,
	)

	approachInfo := computeApproachInfo(
		centerX, centerY,
		org.Heading,
		bodyRadius,
		effectiveRadius,
		caps.DigestiveSpectrum(),
		foodTargets,
		neighbors,
	)

	if s.perfEnabled {
		s.perfStats.VisionNs += time.Since(visionStart).Nanoseconds()
	}

	// Build sensory inputs with aggregated fields
	sensory := neural.SensoryInputs{
		SpeedNorm:        speedNorm,
		EnergyNorm:       org.Energy / org.MaxEnergy,
		Body:             computeBodyDescriptor(org, &caps, cellCount),
		Boid:             boidFields,
		Food:             foodFields,
		Threat:           threatInfo,
		Approach:         approachInfo,
		MaxSpeed:         org.MaxSpeed,
		MaxEnergy:        org.MaxEnergy,
		PerceptionRadius: effectiveRadius,
	}

	// Run brain
	var brainStart time.Time
	if s.perfEnabled {
		brainStart = time.Now()
	}
	inputs := sensory.ToInputs()

	// Store inputs for debugging (convert float64 to float32)
	for i := 0; i < len(inputs) && i < 30; i++ {
		org.LastInputs[i] = float32(inputs[i])
	}

	rawOutputs, err := brain.Controller.Think(inputs)
	if s.perfEnabled {
		s.perfStats.BrainNs += time.Since(brainStart).Nanoseconds()
	}
	if err != nil {
		return neural.DefaultOutputs()
	}

	return neural.DecodeOutputs(rawOutputs)
}



func (s *BehaviorSystem) getFlowFieldForce(x, y float32, org *components.Organism, _ Bounds) (float32, float32) {
	noiseX := s.noise.Noise3D(float64(x)*flowScale, float64(y)*flowScale, float64(s.tick)*flowTimeScale)
	noiseY := s.noise.Noise3D(float64(x)*flowScale+100, float64(y)*flowScale+100, float64(s.tick)*flowTimeScale)

	flowAngle := noiseX * math.Pi * 2
	flowMagnitude := (noiseY + 1) * 0.5
	flowX := float32(math.Cos(flowAngle) * flowMagnitude * flowStrength)
	flowY := float32(math.Sin(flowAngle) * flowMagnitude * flowStrength)

	// Add downward drift and side-to-side motion
	flowY += flowDriftY
	flowX += float32(math.Sin(float64(s.tick)*flowTimeScale*2)) * flowSideEffect

	// All ECS organisms are fauna (flora are in FloraSystem)

	// Size-based resistance: larger organisms are less affected by flow
	// Normalize by typical body radius (~5-10 units), clamp to reasonable range
	sizeResistance := float32(math.Min(float64(org.BodyRadius)/10.0, 1.0)) * 0.5

	// Shape-based flow resistance (low drag = more resistance to being pushed)
	shapeResistance := (1.0 - org.ShapeMetrics.Drag) * 0.3
	massResistance := float32(math.Min(float64(org.Energy/org.MaxEnergy)/3, 1)) * 0.2
	totalResistance := sizeResistance + shapeResistance + massResistance
	factor := 1 - totalResistance

	return flowX * factor, flowY * factor
}

// Sensor weighting functions for body-brain coupling

// getTotalSensorGain returns the sum of sensor strengths from all alive cells with sensor capability.
func getTotalSensorGain(cells *components.CellBuffer) float32 {
	if cells == nil {
		return 1.0 // Default for organisms without cell data
	}
	var total float32
	for i := uint8(0); i < cells.Count; i++ {
		total += cells.Cells[i].GetSensorStrength()
	}
	if total < minSensorGain {
		return minSensorGain
	}
	return total
}

// getSensorCount returns the number of alive cells with sensor capability.
func getSensorCount(cells *components.CellBuffer) int {
	if cells == nil {
		return 1
	}
	count := 0
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if cell.Alive && cell.HasFunction(neural.CellTypeSensor) {
			count++
		}
	}
	return count
}

// getEffectivePerceptionRadius scales perception by total sensor gain.
// More/better sensors = better perception range.
func getEffectivePerceptionRadius(baseRadius float32, cells *components.CellBuffer) float32 {
	totalGain := getTotalSensorGain(cells)
	// Scale: 0.5x (no sensors) to 1.5x (4+ sensor gain)
	scale := float32(0.5 + math.Min(1.0, float64(totalGain)/capabilityScale))
	return baseRadius * scale
}

// Actuator helper functions for body-brain coupling

// getTotalActuatorStrength returns the sum of actuator strengths from all alive cells with actuator capability.
func getTotalActuatorStrength(cells *components.CellBuffer) float32 {
	if cells == nil {
		return 1.0 // Default for organisms without cell data
	}
	var total float32
	for i := uint8(0); i < cells.Count; i++ {
		total += cells.Cells[i].GetActuatorStrength()
	}
	if total < minActuatorGain {
		return minActuatorGain
	}
	return total
}

// getActuatorCount returns the number of alive cells with actuator capability.
func getActuatorCount(cells *components.CellBuffer) int {
	if cells == nil {
		return 1
	}
	count := 0
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if cell.Alive && cell.HasFunction(neural.CellTypeActuator) {
			count++
		}
	}
	return count
}

// getEffectiveMaxSpeed scales max speed by actuator capability.
// More/better actuators = faster movement potential.
func getEffectiveMaxSpeed(baseSpeed float32, cells *components.CellBuffer) float32 {
	totalStrength := getTotalActuatorStrength(cells)
	// Scale: 0.5x (minimal actuators) to 1.5x (4+ actuator strength)
	scale := float32(0.5 + math.Min(1.0, float64(totalStrength)/capabilityScale))
	return baseSpeed * scale
}

// getMaxAcceleration computes max acceleration based on actuator strength and drag.
// More actuators = faster acceleration, higher drag = slower acceleration.
func getMaxAcceleration(cells *components.CellBuffer, drag float32) float32 {
	totalStrength := getTotalActuatorStrength(cells)
	// Base acceleration scaled by actuator strength
	baseAccel := float32(0.05 + 0.15*math.Min(1.0, float64(totalStrength)/capabilityScale))
	// Drag reduces acceleration (high drag = sluggish)
	dragFactor := 1.0 - drag*0.5
	if dragFactor < 0.3 {
		dragFactor = 0.3
	}
	return baseAccel * dragFactor
}

// Morphology-based movement constants
const (
	turnBiasScale    = 0.5  // How much turn bias affects turn rate (0.5 = ±50% at max bias)
	thrustBiasScale  = 0.4  // How much thrust bias affects forward speed (0.4 = ±40% at max bias)
	minTurnThrottle  = 0.3  // Minimum throttle factor for turning (allows turning while slowing down)
)

// getEffectiveTurnRate computes the turn rate scaled by morphology.
// Actuators on one side make turning toward that side easier.
// Returns: baseTurnRate * morphologyMultiplier
func getEffectiveTurnRate(baseTurnRate float32, uTurn float32, cells *components.CellBuffer) float32 {
	if cells == nil {
		return baseTurnRate
	}

	metrics := cells.ComputeActuatorMetrics()
	if metrics.TotalStrength < 0.1 {
		return baseTurnRate * 0.5 // Minimal turning without actuators
	}

	// Normalize turn bias by total strength
	normalizedBias := metrics.TurnBias / metrics.TotalStrength

	// Turn bias > 0 means more right-side actuators = better left turn (positive uTurn)
	// Turn bias < 0 means more left-side actuators = better right turn (negative uTurn)
	// When uTurn matches the bias sign, we get a boost; opposite sign gets penalty
	biasEffect := normalizedBias * uTurn * turnBiasScale

	// Base effectiveness from total actuator strength
	baseEffect := float32(0.5 + 0.5*math.Min(1.0, float64(metrics.TotalStrength)/capabilityScale))

	// Combine: base effectiveness + directional bonus/penalty
	// Clamp to [0.3, 1.5] to prevent extreme values
	multiplier := baseEffect + biasEffect
	if multiplier < 0.3 {
		multiplier = 0.3
	}
	if multiplier > 1.5 {
		multiplier = 1.5
	}

	return baseTurnRate * multiplier
}

// getEffectiveThrust computes thrust effectiveness based on actuator placement.
// Rear actuators provide better forward thrust.
// Returns: baseThrottle * morphologyMultiplier
func getEffectiveThrust(baseThrottle float32, cells *components.CellBuffer) float32 {
	if cells == nil || baseThrottle <= 0 {
		return baseThrottle
	}

	metrics := cells.ComputeActuatorMetrics()
	if metrics.TotalStrength < 0.1 {
		return baseThrottle * 0.3 // Minimal thrust without actuators
	}

	// Normalize thrust bias by total strength
	// Positive = more rear actuators = better forward thrust
	normalizedBias := metrics.ThrustBias / metrics.TotalStrength

	// Base effectiveness from total actuator strength
	baseEffect := float32(0.5 + 0.5*math.Min(1.0, float64(metrics.TotalStrength)/capabilityScale))

	// Thrust bonus for rear-heavy actuator placement
	thrustBonus := normalizedBias * thrustBiasScale

	// Combine and clamp to [0.3, 1.4]
	multiplier := baseEffect + thrustBonus
	if multiplier < 0.3 {
		multiplier = 0.3
	}
	if multiplier > 1.4 {
		multiplier = 1.4
	}

	return baseThrottle * multiplier
}

// computeBodyRadius returns sqrt(cellCount) * cellSize for body length normalization.
func computeBodyRadius(cellCount int, cellSize float32) float32 {
	return float32(math.Sqrt(float64(cellCount))) * cellSize
}

// computeBodyDescriptor computes normalized body capability metrics.
func computeBodyDescriptor(org *components.Organism, caps *components.Capabilities, cellCount int) neural.BodyDescriptor {
	bodyRadius := computeBodyRadius(cellCount, org.CellSize)

	return neural.BodyDescriptor{
		SizeNorm:      clampFloat(bodyRadius/neural.MaxBodySize, 0, 1),
		SpeedCapacity: clampFloat(caps.ActuatorWeight/(caps.ActuatorWeight+org.ShapeMetrics.Drag*2+0.1), 0, 1),
		AgilityNorm:   clampFloat(1.0/(1.0+org.ShapeMetrics.Drag), 0, 1),
		SenseStrength: clampFloat(caps.SensorWeight/neural.MaxSensorWeight, 0, 1),
		BiteStrength:  clampFloat(caps.MouthSize/neural.MaxMouthSize, 0, 1),
		ArmorLevel:    caps.StructuralArmor,
	}
}

// worldToLocal converts a world-space direction to local space relative to heading.
func worldToLocal(dx, dy, heading float32) (localFwd, localUp float32) {
	cosH := float32(math.Cos(float64(heading)))
	sinH := float32(math.Sin(float64(heading)))
	// Rotate by -heading to get local coordinates
	localFwd = dx*cosH + dy*sinH
	localUp = -dx*sinH + dy*cosH
	return localFwd, localUp
}

// normalizeVector normalizes a vector and returns its magnitude.
func normalizeVector(x, y float32) (nx, ny, mag float32) {
	mag = float32(math.Sqrt(float64(x*x + y*y)))
	if mag < 0.001 {
		return 0, 0, 0
	}
	return x / mag, y / mag, mag
}

// gatherSameSpeciesNeighborsSafe builds NeighborInfo slice for boid field calculations.
// Thread-safe version for parallel processing - allocates new slices.
func gatherSameSpeciesNeighborsSafe(
	centerX, centerY float32,
	myIdx int,
	mySpeciesID int,
	perceptionRadius float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	faunaVel []components.Velocity,
	faunaSpecies []int,
	grid *SpatialGrid,
) []NeighborInfo {
	if mySpeciesID == 0 {
		return nil // No species = no boid behavior
	}

	nearbyIndices := grid.GetNearbyFaunaSafe(centerX, centerY, perceptionRadius)
	neighbors := make([]NeighborInfo, 0, len(nearbyIndices))

	for _, idx := range nearbyIndices {
		if idx == myIdx || idx >= len(faunaOrgs) {
			continue
		}
		if faunaOrgs[idx].Dead {
			continue
		}
		// Same-species filter
		if idx >= len(faunaSpecies) || faunaSpecies[idx] != mySpeciesID {
			continue
		}

		// Estimate body radius from cell size
		bodyRadius := faunaOrgs[idx].BodyRadius
		if bodyRadius < 1 {
			bodyRadius = faunaOrgs[idx].CellSize
		}

		neighbors = append(neighbors, NeighborInfo{
			PosX:       faunaPos[idx].X,
			PosY:       faunaPos[idx].Y,
			VelX:       faunaVel[idx].X,
			VelY:       faunaVel[idx].Y,
			BodyRadius: bodyRadius,
		})
	}

	return neighbors
}

// gatherFoodTargetsSafe builds FoodTarget slice for food field calculations.
// Thread-safe version for parallel processing.
func gatherFoodTargetsSafe(
	centerX, centerY float32,
	myIdx int,
	perceptionRadius float32,
	floraEntities []neural.EntityInfo,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) []FoodTarget {
	targets := make([]FoodTarget, 0, 32)

	// Add flora within perception radius (using toroidal distance)
	radiusSq := perceptionRadius * perceptionRadius
	for _, flora := range floraEntities {
		if ToroidalDistanceSq(centerX, centerY, flora.X, flora.Y, worldBounds.Width, worldBounds.Height) <= radiusSq {
			targets = append(targets, FoodTarget{
				PosX:        flora.X,
				PosY:        flora.Y,
				Composition: 1.0, // Plant
				Intensity:   1.0,
			})
		}
	}

	// Add nearby fauna as potential meat sources (alive fauna = potential prey)
	nearbyIndices := grid.GetNearbyFaunaSafe(centerX, centerY, perceptionRadius)
	for _, idx := range nearbyIndices {
		if idx == myIdx || idx >= len(faunaOrgs) {
			continue
		}
		org := faunaOrgs[idx]
		// Both dead and alive fauna can be meat sources
		// Dead = carrion, Alive = potential prey (diet compatibility handled later)
		intensity := float32(1.0)
		if org.Dead {
			intensity = org.Energy / org.MaxEnergy // Carrion value based on remaining energy
		}
		targets = append(targets, FoodTarget{
			PosX:        faunaPos[idx].X,
			PosY:        faunaPos[idx].Y,
			Composition: 0.0, // Meat
			Intensity:   intensity,
		})
	}

	return targets
}

// gatherThreatsSafe builds threat entity list for threat calculations.
// Thread-safe version for parallel processing.
func gatherThreatsSafe(
	centerX, centerY float32,
	myIdx int,
	perceptionRadius float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) []neural.EntityInfo {
	nearbyIndices := grid.GetNearbyFaunaSafe(centerX, centerY, perceptionRadius*1.5) // Extended range for threats
	threats := make([]neural.EntityInfo, 0, len(nearbyIndices))

	for _, idx := range nearbyIndices {
		if idx == myIdx || idx >= len(faunaOrgs) {
			continue
		}
		org := faunaOrgs[idx]
		if org.Dead {
			continue // Dead organisms aren't threats
		}

		threats = append(threats, neural.EntityInfo{
			X:             faunaPos[idx].X,
			Y:             faunaPos[idx].Y,
			Composition:   0.0, // Fauna
			DigestiveSpec: 0.5, // Unknown diet - assume neutral
			IsFlora:       false,
		})
	}

	return threats
}

// NeighborInfo holds data about a nearby same-species organism for boid calculations.
type NeighborInfo struct {
	PosX, PosY    float32 // World position
	VelX, VelY    float32 // Velocity
	BodyRadius    float32 // For surface-to-surface distance
	SurfaceDistSq float32 // Surface-to-surface distance squared
}

// worldBounds holds cached world dimensions for toroidal calculations.
var worldBounds Bounds

// computeBoidFields computes cohesion, alignment, separation fields from same-species neighbors.
func computeBoidFields(
	centerX, centerY float32,
	heading float32,
	myBodyRadius float32,
	perceptionRadius float32,
	neighbors []NeighborInfo,
) neural.BoidFields {
	var fields neural.BoidFields

	if len(neighbors) == 0 {
		return fields
	}

	// Accumulators
	var comDx, comDy float32     // Accumulated delta to center of mass (toroidal)
	var avgVelX, avgVelY float32 // Average velocity
	var sepX, sepY float32       // Separation force
	var cohesionCount int
	var alignCount int
	var sepCount int

	separationRadiusSq := float32(boidSeparationRadius * boidSeparationRadius * myBodyRadius * myBodyRadius)

	for _, n := range neighbors {
		// Compute toroidal delta to neighbor
		dx, dy := ToroidalDelta(centerX, centerY, n.PosX, n.PosY, worldBounds.Width, worldBounds.Height)
		centerDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		surfaceDist := centerDist - myBodyRadius - n.BodyRadius
		if surfaceDist < 0 {
			surfaceDist = 0
		}

		// Cohesion: accumulate delta to neighbor (not absolute position)
		comDx += dx
		comDy += dy
		cohesionCount++

		// Alignment: accumulate velocities
		velMag := float32(math.Sqrt(float64(n.VelX*n.VelX + n.VelY*n.VelY)))
		if velMag > 0.1 {
			avgVelX += n.VelX / velMag
			avgVelY += n.VelY / velMag
			alignCount++
		}

		// Separation: repulsion from close neighbors (surface distance)
		surfaceDistSq := surfaceDist * surfaceDist
		if surfaceDistSq < separationRadiusSq && centerDist > 0.1 {
			// Inverse distance weighting
			weight := 1.0 - (surfaceDist / (boidSeparationRadius * myBodyRadius))
			if weight < 0 {
				weight = 0
			}
			sepX -= (dx / centerDist) * weight
			sepY -= (dy / centerDist) * weight
			sepCount++
		}
	}

	// Compute cohesion direction (toward center of mass using accumulated deltas)
	if cohesionCount > 0 {
		comDx /= float32(cohesionCount)
		comDy /= float32(cohesionCount)
		localFwd, localUp := worldToLocal(comDx, comDy, heading)
		nx, ny, mag := normalizeVector(localFwd, localUp)
		fields.CohesionFwd = nx
		fields.CohesionUp = ny
		// Normalize magnitude by perception radius (in body lengths)
		fields.CohesionMag = clampFloat(mag/(perceptionRadius), 0, 1)
	}

	// Compute alignment (average heading)
	if alignCount > 0 {
		avgVelX /= float32(alignCount)
		avgVelY /= float32(alignCount)
		localFwd, localUp := worldToLocal(avgVelX, avgVelY, heading)
		nx, ny, _ := normalizeVector(localFwd, localUp)
		fields.AlignmentFwd = nx
		fields.AlignmentUp = ny
	}

	// Compute separation
	if sepCount > 0 {
		localFwd, localUp := worldToLocal(sepX, sepY, heading)
		nx, ny, mag := normalizeVector(localFwd, localUp)
		fields.SeparationFwd = nx
		fields.SeparationUp = ny
		fields.SeparationMag = clampFloat(mag, 0, 1)
	}

	// Density
	fields.DensitySame = clampFloat(float32(len(neighbors))/boidExpectedNeighbors, 0, 1)

	return fields
}

// FoodTarget holds data about a potential food source for field computation.
type FoodTarget struct {
	PosX, PosY  float32
	Composition float32 // 1.0 = plant, 0.0 = meat
	Intensity   float32 // Inverse distance weighted
}

// computeFoodFields computes plant and meat attraction fields.
func computeFoodFields(
	centerX, centerY float32,
	heading float32,
	digestiveSpectrum float32, // 0=herbivore, 1=carnivore
	perceptionRadius float32,
	foods []FoodTarget,
) neural.FoodFields {
	var fields neural.FoodFields

	if len(foods) == 0 {
		return fields
	}

	// Accumulators for weighted direction
	var plantX, plantY, plantTotal float32
	var meatX, meatY, meatTotal float32
	var plantCount, meatCount int

	// Plant preference for herbivores, meat preference for carnivores
	plantWeight := 1.0 - digestiveSpectrum // Herbivores prefer plants
	meatWeight := digestiveSpectrum        // Carnivores prefer meat

	for _, f := range foods {
		// Use toroidal delta for proper wrap-around
		dx, dy := ToroidalDelta(centerX, centerY, f.PosX, f.PosY, worldBounds.Width, worldBounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist < 1 {
			dist = 1
		}

		// Intensity decreases with distance
		intensity := f.Intensity / (dist * dist)

		if f.Composition > 0.5 {
			// Plant
			plantX += dx * intensity * plantWeight
			plantY += dy * intensity * plantWeight
			plantTotal += intensity * plantWeight
			plantCount++
		} else {
			// Meat
			meatX += dx * intensity * meatWeight
			meatY += dy * intensity * meatWeight
			meatTotal += intensity * meatWeight
			meatCount++
		}
	}

	// Normalize and convert to local space (normalize by type-specific count)
	if plantCount > 0 && plantTotal > 0.01 {
		localFwd, localUp := worldToLocal(plantX, plantY, heading)
		nx, ny, _ := normalizeVector(localFwd, localUp)
		fields.PlantFwd = nx
		fields.PlantUp = ny
		fields.PlantMag = clampFloat(plantTotal/float32(plantCount), 0, 1)
	}

	if meatCount > 0 && meatTotal > 0.01 {
		localFwd, localUp := worldToLocal(meatX, meatY, heading)
		nx, ny, _ := normalizeVector(localFwd, localUp)
		fields.MeatFwd = nx
		fields.MeatUp = ny
		fields.MeatMag = clampFloat(meatTotal/float32(meatCount), 0, 1)
	}

	return fields
}

// computeThreatInfo computes threat proximity and closing speed.
func computeThreatInfo(
	centerX, centerY float32,
	velX, velY float32,
	heading float32,
	maxSpeed float32,
	perceptionRadius float32,
	threats []neural.EntityInfo,
	myComposition float32,
	myArmor float32,
) neural.ThreatInfo {
	var info neural.ThreatInfo

	if len(threats) == 0 {
		return info
	}

	// Find the most threatening entity
	var maxThreat float32
	var threatDx, threatDy float32

	for _, t := range threats {
		if t.IsFlora {
			continue // Plants aren't threats
		}

		// Use toroidal delta for proper wrap-around
		dx, dy := ToroidalDelta(centerX, centerY, t.X, t.Y, worldBounds.Width, worldBounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist > perceptionRadius {
			continue
		}

		// Calculate how threatening this entity is
		threatLevel := neural.ThreatLevel(t.DigestiveSpec, myComposition, myArmor)
		proximity := 1.0 - (dist / perceptionRadius)
		threat := threatLevel * proximity

		if threat > maxThreat {
			maxThreat = threat
			threatDx = dx
			threatDy = dy
		}
	}

	if maxThreat > 0.01 {
		info.Proximity = clampFloat(maxThreat, 0, 1)

		// Compute closing speed (negative = approaching)
		threatDist := float32(math.Sqrt(float64(threatDx*threatDx + threatDy*threatDy)))
		if threatDist > 0.1 && maxSpeed > 0.1 {
			// Direction to threat
			dirX := threatDx / threatDist
			dirY := threatDy / threatDist
			// Closing speed is our velocity dotted with direction to threat
			closingSpeed := (velX*dirX + velY*dirY)
			info.ClosingSpeed = clampFloat(closingSpeed/maxSpeed, -1, 1)
		}
	}

	return info
}

// computeApproachInfo computes close-range pursuit geometry for nearest food and mate.
// Returns approach info with distance (1=close, 0=far) and bearing (0=ahead, ±1=behind).
func computeApproachInfo(
	centerX, centerY float32,
	heading float32,
	bodyRadius float32,
	perceptionRadius float32,
	digestiveSpectrum float32,
	foods []FoodTarget,
	neighbors []NeighborInfo,
) neural.ApproachInfo {
	var info neural.ApproachInfo

	// Find nearest edible food
	// Herbivores prefer plants (composition > 0.5), carnivores prefer meat (composition < 0.5)
	var nearestFoodDist float32 = perceptionRadius + 1
	var nearestFoodDx, nearestFoodDy float32
	foundFood := false

	for _, f := range foods {
		// Check diet compatibility
		isPlant := f.Composition > 0.5
		plantPreference := 1.0 - digestiveSpectrum // 1 for herbivore, 0 for carnivore
		meatPreference := digestiveSpectrum        // 0 for herbivore, 1 for carnivore

		var compatibility float32
		if isPlant {
			compatibility = plantPreference
		} else {
			compatibility = meatPreference
		}

		// Skip foods we can't eat well (< 30% compatibility)
		if compatibility < 0.3 {
			continue
		}

		// Use toroidal delta for proper wrap-around
		dx, dy := ToroidalDelta(centerX, centerY, f.PosX, f.PosY, worldBounds.Width, worldBounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist < nearestFoodDist {
			nearestFoodDist = dist
			nearestFoodDx = dx
			nearestFoodDy = dy
			foundFood = true
		}
	}

	if foundFood {
		// Normalize distance: 1 when touching (dist <= bodyRadius), 0 at perception edge
		surfaceDist := nearestFoodDist - bodyRadius
		if surfaceDist < 0 {
			surfaceDist = 0
		}
		info.NearestFoodDist = 1.0 - clampFloat(surfaceDist/perceptionRadius, 0, 1)

		// Compute bearing using atan2 for linear angle encoding
		// 0 = directly ahead, +0.5 = 90° right, -0.5 = 90° left, ±1 = behind
		if nearestFoodDist > 0.1 {
			dirX := nearestFoodDx / nearestFoodDist
			dirY := nearestFoodDy / nearestFoodDist
			// Heading vector
			headX := float32(math.Cos(float64(heading)))
			headY := float32(math.Sin(float64(heading)))
			// Dot product gives cos(angle), cross gives sin(angle)
			dot := dirX*headX + dirY*headY
			cross := headX*dirY - headY*dirX
			// atan2(-cross, dot) preserves original sign convention: positive = turn right
			relAngle := float32(math.Atan2(float64(-cross), float64(dot)))
			info.NearestFoodBearing = clampFloat(relAngle/math.Pi, -1, 1)
		}
	}

	// Find nearest same-species neighbor (potential mate)
	var nearestMateDist float32 = perceptionRadius + 1
	var nearestMateDx, nearestMateDy float32
	var nearestMateRadius float32
	foundMate := false

	for _, n := range neighbors {
		// Use toroidal delta for proper wrap-around
		dx, dy := ToroidalDelta(centerX, centerY, n.PosX, n.PosY, worldBounds.Width, worldBounds.Height)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist < nearestMateDist {
			nearestMateDist = dist
			nearestMateDx = dx
			nearestMateDy = dy
			nearestMateRadius = n.BodyRadius
			foundMate = true
		}
	}

	if foundMate {
		// Normalize distance: surface-to-surface (self + neighbor radius)
		surfaceDist := nearestMateDist - bodyRadius - nearestMateRadius
		if surfaceDist < 0 {
			surfaceDist = 0
		}
		info.NearestMateDist = 1.0 - clampFloat(surfaceDist/perceptionRadius, 0, 1)

		// Compute bearing using atan2 for linear angle encoding
		// 0 = directly ahead, +0.5 = 90° right, -0.5 = 90° left, ±1 = behind
		if nearestMateDist > 0.1 {
			dirX := nearestMateDx / nearestMateDist
			dirY := nearestMateDy / nearestMateDist
			headX := float32(math.Cos(float64(heading)))
			headY := float32(math.Sin(float64(heading)))
			dot := dirX*headX + dirY*headY
			cross := headX*dirY - headY*dirX
			// atan2(-cross, dot) preserves original sign convention: positive = turn right
			relAngle := float32(math.Atan2(float64(-cross), float64(dot)))
			info.NearestMateBearing = clampFloat(relAngle/math.Pi, -1, 1)
		}
	}

	return info
}

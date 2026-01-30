package systems

import (
	"math"
	"math/rand"

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
	deadFlowMultiplier = 1.5   // Dead organisms affected more by flow
	deadSinkRate       = 0.02  // Downward drift rate for dead organisms

	// Thrust and turning
	thrustScale       = 0.1  // Thrust force multiplier
	turnScale         = 0.15 // Base turn rate multiplier
	defaultTurnScale  = 0.15 // Turn scale when no cells
	actuatorThrustMul = 0.5  // Actuator thrust contribution

	// Capability thresholds
	minSensorGain    = float32(0.1)
	minActuatorGain  = float32(0.1)
	capabilityScale  = 4.0 // Denominator for capability scaling
)

// BehaviorSystem handles organism steering behaviors using direct neural control.
type BehaviorSystem struct {
	filter      ecs.Filter3[components.Position, components.Velocity, components.Organism]
	brainMap    *ecs.Map[components.Brain]
	cellsMap    *ecs.Map[components.CellBuffer]
	noise       *PerlinNoise
	tick        int32
	shadowMap   *ShadowMap
	terrain     *TerrainSystem
	pathfinder  *Pathfinder  // Potential-field navigation layer
	floraSystem *FloraSystem // Lightweight flora system for vision
}

// NewBehaviorSystem creates a new behavior system.
func NewBehaviorSystem(w *ecs.World, shadowMap *ShadowMap, terrain *TerrainSystem) *BehaviorSystem {
	return &BehaviorSystem{
		filter:     *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		brainMap:   ecs.NewMap[components.Brain](w),
		cellsMap:   ecs.NewMap[components.CellBuffer](w),
		noise:      NewPerlinNoise(rand.Int63()),
		shadowMap:  shadowMap,
		terrain:    terrain,
		pathfinder: NewPathfinder(terrain),
	}
}

// SetFloraSystem sets the flora system reference for vision queries.
func (s *BehaviorSystem) SetFloraSystem(fs *FloraSystem) {
	s.floraSystem = fs
}

// Update runs the behavior system with actuator-driven neural control.
func (s *BehaviorSystem) Update(w *ecs.World, bounds Bounds, floraPositions, faunaPositions []components.Position, floraOrgs, faunaOrgs []*components.Organism, grid *SpatialGrid) {
	s.tick++

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
			outputs = s.getBrainOutputs(entity, pos, org, floraPositions, faunaPositions, floraOrgs, faunaOrgs, grid, bounds)
		} else {
			outputs = neural.DefaultOutputs()
		}

		// Store intents for other systems (feeding, breeding, growth, bioluminescence)
		org.DesireAngle = outputs.DesireAngle
		org.DesireDistance = outputs.DesireDistance
		org.EatIntent = outputs.Eat
		org.GrowIntent = outputs.Grow
		org.BreedIntent = outputs.Breed
		org.GlowIntent = outputs.Glow

		// Phase 5b: Compute emitted light from glow intent and bioluminescent capability
		if cells != nil && outputs.Glow > 0 {
			caps := cells.ComputeCapabilities()
			org.EmittedLight = outputs.Glow * caps.BioluminescentCap
		} else {
			org.EmittedLight = 0
		}

		// Use potential-field navigation to convert desire to turn/thrust
		// Pathfinding handles terrain avoidance so brain only learns strategy
		flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)

		// Get collision radius from OBB if available, otherwise estimate
		organismRadius := GetCollisionRadius(&org.OBB, org.CellSize)

		navResult := s.pathfinder.Navigate(
			pos.X, pos.Y,
			org.Heading,
			outputs.DesireAngle, outputs.DesireDistance,
			flowX, flowY,
			organismRadius,
		)

		org.TurnOutput = navResult.Turn
		org.ThrustOutput = navResult.Thrust

		// Calculate actuator-driven forces
		// Actuator positions and strengths determine how Turn/Thrust translate to movement
		thrust, torque := calculateActuatorForces(cells, org.Heading, navResult.Thrust, navResult.Turn)

		// Apply torque to heading and normalize to [0, 2*Pi]
		org.Heading = normalizeHeading(org.Heading + torque)

		// Calculate effective max speed based on actuator capability
		effectiveMaxSpeed := getEffectiveMaxSpeed(org.MaxSpeed, cells)

		// Apply thrust in heading direction
		thrustX := float32(math.Cos(float64(org.Heading))) * thrust * thrustScale
		thrustY := float32(math.Sin(float64(org.Heading))) * thrust * thrustScale

		// Apply flow field (already computed for pathfinding)
		vel.X += thrustX + flowX
		vel.Y += thrustY + flowY

		// Clamp to effective max speed
		speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		if speed > effectiveMaxSpeed {
			scale := effectiveMaxSpeed / speed
			vel.X *= scale
			vel.Y *= scale
		}

		// Track thrust magnitude for energy cost
		org.ActiveThrust = float32(math.Sqrt(float64(thrustX*thrustX + thrustY*thrustY)))
	}
}

// getBrainOutputs gathers sensory inputs using polar vision and runs the brain network.
// Phase 3: Uses 4-cone × 3-channel polar vision instead of nearest-target sensing.
func (s *BehaviorSystem) getBrainOutputs(
	entity ecs.Entity,
	pos *components.Position,
	org *components.Organism,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
	grid *SpatialGrid,
	bounds Bounds,
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

	// Get our capabilities for vision parameters
	var myComposition, myDigestiveSpec, myArmor float32
	if cells != nil {
		caps := cells.ComputeCapabilities()
		myComposition = caps.Composition()
		myDigestiveSpec = caps.DigestiveSpectrum()
		myArmor = caps.StructuralArmor
	} else {
		// Fauna should always have cells - use defaults if somehow missing
		myComposition = 0.0
		myDigestiveSpec = 0.5
		myArmor = 0.0
	}

	// Get light level for vision
	var lightLevel float32 = 0.5
	if s.shadowMap != nil {
		lightLevel = s.shadowMap.SampleLight(pos.X, pos.Y)
	}

	// Build sensor cells for vision weighting
	sensorCells := buildSensorCells(cells)

	// Build vision parameters
	visionParams := neural.VisionParams{
		PosX:            pos.X,
		PosY:            pos.Y,
		Heading:         org.Heading,
		MyComposition:   myComposition,
		MyDigestiveSpec: myDigestiveSpec,
		MyArmor:         myArmor,
		EffectiveRadius: effectiveRadius,
		LightLevel:      lightLevel,
		Sensors:         sensorCells,
	}

	// Build entity list for vision scan
	entities := s.buildEntityList(pos, org, effectiveRadius, floraPos, faunaPos, floraOrgs, faunaOrgs, grid)

	// Perform polar vision scan
	var pv neural.PolarVision
	pv.ScanEntities(visionParams, entities)

	// Phase 3b: Sample directional light for gradient computation
	var lightSampler func(x, y float32) float32
	if s.shadowMap != nil {
		lightSampler = s.shadowMap.SampleLight
	}
	pv.SampleDirectionalLight(pos.X, pos.Y, org.Heading, effectiveRadius, lightSampler)

	// Build sensory inputs with polar vision data
	sensory := neural.SensoryInputs{
		PerceptionRadius: effectiveRadius,
		Energy:           org.Energy,
		MaxEnergy:        org.MaxEnergy,
		MaxCells:         16,
		SensorCount:      getSensorCount(cells),
		TotalSensorGain:  getTotalSensorGain(cells),
		ActuatorCount:    getActuatorCount(cells),
		TotalActuatorStr: getTotalActuatorStrength(cells),
	}

	// Cell count from actual cells if available
	if cells != nil {
		sensory.CellCount = int(cells.Count)
	} else {
		sensory.CellCount = int((org.MaxEnergy - 100) / 50)
		if sensory.CellCount < 1 {
			sensory.CellCount = 1
		}
	}

	// Copy normalized polar vision to sensory inputs
	sensory.FromPolarVision(&pv)

	// Light level
	sensory.LightLevel = lightLevel

	// Flow alignment (Phase 4): dot product of flow direction with heading
	// Positive = flow helping (pushing in direction we're facing)
	// Negative = flow hindering (pushing against us)
	flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
	headingX := float32(math.Cos(float64(org.Heading)))
	headingY := float32(math.Sin(float64(org.Heading)))
	flowMag := float32(math.Sqrt(float64(flowX*flowX + flowY*flowY)))
	if flowMag > 0.001 {
		// Normalize flow and compute dot product
		sensory.FlowAlignment = (flowX*headingX + flowY*headingY) / flowMag
	} else {
		sensory.FlowAlignment = 0
	}

	// Phase 5: Openness scalar for terrain awareness
	// Pathfinding layer handles actual terrain avoidance, brain just gets context
	if s.terrain != nil {
		terrainDist := s.terrain.DistanceToSolid(pos.X, pos.Y)
		if terrainDist < effectiveRadius {
			// Openness = normalized distance to terrain (0 = touching, 1 = far)
			sensory.Openness = terrainDist / effectiveRadius
		} else {
			sensory.Openness = 1.0
		}
	} else {
		sensory.Openness = 1.0
	}

	// Convert to neural inputs and run brain
	inputs := sensory.ToInputs()
	rawOutputs, err := brain.Controller.Think(inputs)
	if err != nil {
		return neural.DefaultOutputs()
	}

	return neural.DecodeOutputs(rawOutputs)
}

// buildSensorCells extracts sensor cell data for vision weighting.
func buildSensorCells(cells *components.CellBuffer) []neural.SensorCell {
	if cells == nil {
		return nil
	}

	var sensorCells []neural.SensorCell
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		if !cell.Alive {
			continue
		}
		strength := cell.GetSensorStrength()
		if strength > 0 {
			sensorCells = append(sensorCells, neural.SensorCell{
				GridX:    cell.GridX,
				GridY:    cell.GridY,
				Strength: strength,
			})
		}
	}
	return sensorCells
}

// buildEntityList creates the entity info list for polar vision scanning.
func (s *BehaviorSystem) buildEntityList(
	pos *components.Position,
	org *components.Organism,
	effectiveRadius float32,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) []neural.EntityInfo {
	var entities []neural.EntityInfo

	// Add nearby flora from FloraSystem
	if s.floraSystem != nil {
		nearbyFlora := s.floraSystem.GetNearbyFlora(pos.X, pos.Y, effectiveRadius)
		for _, ref := range nearbyFlora {
			// Check line of sight
			if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, ref.X, ref.Y) {
				continue
			}
			entities = append(entities, neural.EntityInfo{
				X:               ref.X,
				Y:               ref.Y,
				Composition:     1.0, // Flora is pure photosynthetic
				DigestiveSpec:   0.0, // Flora doesn't eat
				StructuralArmor: DefaultFloraArmor(), // Standard flora armor
				GeneticDistance: -1,  // No genetic comparison with flora
				IsFlora:         true,
			})
		}
	}

	// Add nearby fauna
	nearbyFauna := grid.GetNearbyFauna(pos.X, pos.Y, effectiveRadius*1.5) // Extended range for threats
	for _, i := range nearbyFauna {
		if faunaOrgs[i] == org || faunaOrgs[i].Dead {
			continue
		}
		// Check line of sight
		if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) {
			continue
		}

		other := faunaOrgs[i]

		// Without access to other organisms' cells, use neutral/default values
		// TODO: Access other organisms' cells via entity lookup for true capability-based check.
		var theirDigestive float32 = 0.5   // Neutral - unknown diet
		var theirComposition float32 = 0.0 // Fauna have low photosynthesis
		var theirArmor float32 = 0.0       // Unknown armor

		// Genetic distance uses default since we can't compare cell-based traits
		// Species ID comparison would be more accurate but requires entity lookup
		var geneticDistance float32 = 1.0 // Default: moderately different
		_ = other                          // Suppress unused warning

		entities = append(entities, neural.EntityInfo{
			X:               faunaPos[i].X,
			Y:               faunaPos[i].Y,
			Composition:     theirComposition,
			DigestiveSpec:   theirDigestive,
			StructuralArmor: theirArmor,
			GeneticDistance: geneticDistance,
			IsFlora:         false,
			EmittedLight:    other.EmittedLight, // Phase 5b: bioluminescence
		})
	}

	return entities
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

	// Shape-based flow resistance
	shapeResistance := org.ShapeMetrics.Streamlining * 0.4
	massResistance := float32(math.Min(float64(org.Energy/org.MaxEnergy)/3, 1))
	totalResistance := shapeResistance + massResistance*0.6
	factor := 1 - totalResistance*0.7

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

// calculateActuatorForces computes thrust and torque from actuator cell geometry.
// Actuators at different positions contribute differently to forward thrust vs turning.
func calculateActuatorForces(
	cells *components.CellBuffer,
	heading float32,
	thrustOutput float32, // 0-1 from brain
	turnOutput float32,   // -1 to +1 from brain
) (thrust float32, torque float32) {
	if cells == nil {
		// No cell data - use direct control
		return thrustOutput, turnOutput * defaultTurnScale
	}

	var totalStrength float32
	var weightedTorque float32

	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		strength := cell.GetActuatorStrength()
		if strength == 0 {
			continue
		}

		totalStrength += strength

		// Actuator position relative to center
		dx := float32(cell.GridX)
		dy := float32(cell.GridY)

		// Calculate lateral offset (perpendicular to heading)
		// Positive = right side, Negative = left side
		sinH := float32(math.Sin(float64(heading)))
		cosH := float32(math.Cos(float64(heading)))
		lateralOffset := -dx*sinH + dy*cosH

		// Actuators on opposite sides contribute to turning
		// Turn output > 0 means turn right, so left actuators (negative offset) push harder
		// Turn output < 0 means turn left, so right actuators (positive offset) push harder
		turnContribution := -lateralOffset * turnOutput * strength

		weightedTorque += turnContribution
	}

	if totalStrength < minActuatorGain {
		totalStrength = minActuatorGain
	}

	// Forward thrust proportional to total actuator strength
	thrust = thrustOutput * totalStrength * actuatorThrustMul

	// Torque for turning (normalized to prevent runaway with many actuators)
	torque = weightedTorque / totalStrength * turnScale

	return thrust, torque
}

// getEffectiveMaxSpeed scales max speed by actuator capability.
// More/better actuators = faster movement potential.
func getEffectiveMaxSpeed(baseSpeed float32, cells *components.CellBuffer) float32 {
	totalStrength := getTotalActuatorStrength(cells)
	// Scale: 0.5x (minimal actuators) to 1.5x (4+ actuator strength)
	scale := float32(0.5 + math.Min(1.0, float64(totalStrength)/capabilityScale))
	return baseSpeed * scale
}

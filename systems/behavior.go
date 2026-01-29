package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/traits"
)

// BehaviorSystem handles organism steering behaviors using direct neural control.
type BehaviorSystem struct {
	filter    ecs.Filter3[components.Position, components.Velocity, components.Organism]
	brainMap  *ecs.Map[components.Brain]
	cellsMap  *ecs.Map[components.CellBuffer]
	noise     *PerlinNoise
	tick      int32
	shadowMap *ShadowMap
	terrain   *TerrainSystem
}

// NewBehaviorSystem creates a new behavior system.
func NewBehaviorSystem(w *ecs.World, shadowMap *ShadowMap, terrain *TerrainSystem) *BehaviorSystem {
	return &BehaviorSystem{
		filter:    *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		brainMap:  ecs.NewMap[components.Brain](w),
		cellsMap:  ecs.NewMap[components.CellBuffer](w),
		noise:     NewPerlinNoise(rand.Int63()),
		shadowMap: shadowMap,
		terrain:   terrain,
	}
}

// Update runs the behavior system with actuator-driven neural control.
func (s *BehaviorSystem) Update(w *ecs.World, bounds Bounds, floraPositions, faunaPositions []components.Position, floraOrgs, faunaOrgs []*components.Organism, grid *SpatialGrid) {
	s.tick++

	query := s.filter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, vel, org := query.Get()

		// Skip stationary flora (unless dead - dead flora drifts)
		if traits.IsFlora(org.Traits) && !org.Traits.Has(traits.Floating) && !org.Dead {
			continue
		}

		// Dead organisms only get flow field influence
		if org.Dead {
			flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
			vel.X += flowX * 1.5
			vel.Y += flowY * 1.5
			vel.Y += 0.02 // Slight downward drift (sinking)
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

		// Store intents for other systems (feeding, breeding, growth)
		org.DesireAngle = outputs.DesireAngle
		org.DesireDistance = outputs.DesireDistance
		org.EatIntent = outputs.Eat
		org.GrowIntent = outputs.Grow
		org.BreedIntent = outputs.Breed

		// Phase 4: Convert desire to turn/thrust (temporary until Phase 5 pathfinding)
		turn, thrustIntent := outputs.ToTurnThrust()
		org.TurnOutput = turn
		org.ThrustOutput = thrustIntent

		// Calculate actuator-driven forces
		// Actuator positions and strengths determine how Turn/Thrust translate to movement
		thrust, torque := calculateActuatorForces(cells, org.Heading, thrustIntent, turn)

		// Apply torque to heading
		org.Heading += torque

		// Normalize heading to [0, 2*Pi]
		for org.Heading < 0 {
			org.Heading += 2 * math.Pi
		}
		for org.Heading >= 2*math.Pi {
			org.Heading -= 2 * math.Pi
		}

		// Calculate effective max speed based on actuator capability
		effectiveMaxSpeed := getEffectiveMaxSpeed(org.MaxSpeed, cells)

		// Apply thrust in heading direction
		thrustX := float32(math.Cos(float64(org.Heading))) * thrust * 0.1
		thrustY := float32(math.Sin(float64(org.Heading))) * thrust * 0.1

		// Apply flow field
		flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
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
		// Fallback: estimate from traits
		myComposition = 0.0 // Fauna-like
		if org.Traits.Has(traits.Carnivore) {
			myDigestiveSpec = 0.85
		} else if org.Traits.Has(traits.Herbivore) {
			myDigestiveSpec = 0.15
		} else {
			myDigestiveSpec = 0.5
		}
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

	// Terrain sensing and openness (Phase 4b)
	if s.terrain != nil {
		terrainDist := s.terrain.DistanceToSolid(pos.X, pos.Y)
		if terrainDist < effectiveRadius {
			sensory.TerrainFound = true
			sensory.TerrainDistance = terrainDist

			gx, gy := s.terrain.GetGradient(pos.X, pos.Y)
			cosH := float32(math.Cos(float64(-org.Heading)))
			sinH := float32(math.Sin(float64(-org.Heading)))
			sensory.TerrainGradientX = gx*cosH - gy*sinH
			sensory.TerrainGradientY = gx*sinH + gy*cosH

			// Phase 4b: Openness = normalized distance to terrain
			// 0 = touching terrain, 1 = terrain at edge of perception
			sensory.Openness = terrainDist / effectiveRadius
		} else {
			// No terrain within perception range = fully open
			sensory.Openness = 1.0
		}
	} else {
		// No terrain system = assume fully open
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

	// Add nearby flora
	nearbyFlora := grid.GetNearbyFlora(pos.X, pos.Y, effectiveRadius)
	for _, i := range nearbyFlora {
		if floraOrgs[i].Dead {
			continue
		}
		// Check line of sight
		if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y) {
			continue
		}
		entities = append(entities, neural.EntityInfo{
			X:               floraPos[i].X,
			Y:               floraPos[i].Y,
			Composition:     1.0, // Flora is pure photosynthetic
			DigestiveSpec:   0.0, // Flora doesn't eat
			StructuralArmor: 0.1, // Standard flora armor
			GeneticDistance: -1,  // No genetic comparison with flora
			IsFlora:         true,
		})
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

		// Estimate other organism's capabilities from traits
		// (In full model, we'd access their cells)
		var theirDigestive float32 = 0.5
		if other.Traits.Has(traits.Carnivore) {
			theirDigestive = 0.85
		} else if other.Traits.Has(traits.Herbivore) {
			theirDigestive = 0.15
		}

		// Estimate composition (fauna have low photo, high actuator)
		var theirComposition float32 = 0.0

		// Estimate armor (simplified - could be enhanced with cell access)
		var theirArmor float32 = 0.0

		// Calculate genetic distance for friend channel
		// Use trait similarity as a proxy for genetic distance
		// Same diet type = more similar = lower distance
		var geneticDistance float32 = 2.0 // Default: somewhat different
		if org.Traits.Has(traits.Carnivore) == other.Traits.Has(traits.Carnivore) &&
			org.Traits.Has(traits.Herbivore) == other.Traits.Has(traits.Herbivore) {
			geneticDistance = 0.5 // Same diet type = more similar
		}

		entities = append(entities, neural.EntityInfo{
			X:               faunaPos[i].X,
			Y:               faunaPos[i].Y,
			Composition:     theirComposition,
			DigestiveSpec:   theirDigestive,
			StructuralArmor: theirArmor,
			GeneticDistance: geneticDistance,
			IsFlora:         false,
		})
	}

	return entities
}

// findMate finds a compatible mate for breeding.
func (s *BehaviorSystem) findMate(pos *components.Position, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism, grid *SpatialGrid) (float32, float32, bool) {
	maxDist := org.PerceptionRadius
	maxDistSq := maxDist * maxDist
	closestDistSq := maxDistSq
	var closestX, closestY float32
	found := false

	nearby := grid.GetNearbyFauna(pos.X, pos.Y, maxDist)
	for _, i := range nearby {
		other := faunaOrgs[i]
		if other == org || other.Dead {
			continue
		}
		// Opposite gender check
		isMale := org.Traits.Has(traits.Male)
		otherMale := other.Traits.Has(traits.Male)
		if isMale == otherMale {
			continue
		}

		distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if distSq < closestDistSq {
			closestDistSq = distSq
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

// findMateWeighted finds a mate using sensor-weighted perception.
func (s *BehaviorSystem) findMateWeighted(
	pos *components.Position,
	org *components.Organism,
	cells *components.CellBuffer,
	effectiveRadius float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) (float32, float32, bool) {
	maxDistSq := effectiveRadius * effectiveRadius
	closestDistSq := maxDistSq
	var closestX, closestY float32
	found := false

	nearby := grid.GetNearbyFauna(pos.X, pos.Y, effectiveRadius)
	for _, i := range nearby {
		other := faunaOrgs[i]
		if other == org || other.Dead {
			continue
		}
		// Opposite gender check
		isMale := org.Traits.Has(traits.Male)
		otherMale := other.Traits.Has(traits.Male)
		if isMale == otherMale {
			continue
		}

		distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if distSq > maxDistSq {
			continue
		}

		// Check line of sight - can't see through terrain
		if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) {
			continue
		}

		// Apply sensor directional weighting - targets in sensor-covered directions are easier to detect
		targetAngle := float32(math.Atan2(float64(faunaPos[i].Y-pos.Y), float64(faunaPos[i].X-pos.X))) - org.Heading
		sensorBonus := sensorWeightedIntensity(cells, org.Heading, targetAngle, 1.0)

		// Better sensor coverage = effectively shorter distance (easier to detect)
		effectiveDistSq := distSq / (sensorBonus * sensorBonus)

		if effectiveDistSq < closestDistSq {
			closestDistSq = effectiveDistSq
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

// countNearbySameType counts the number of nearby organisms of the same diet type.
func (s *BehaviorSystem) countNearbySameType(pos *components.Position, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism, grid *SpatialGrid) int {
	herdRadius := org.PerceptionRadius * 1.5
	count := 0

	nearby := grid.GetNearbyFauna(pos.X, pos.Y, herdRadius)
	for _, i := range nearby {
		other := faunaOrgs[i]
		if other == org || other.Dead {
			continue
		}
		// Same type check (carnivore with carnivore, herbivore with herbivore)
		if org.Traits.Has(traits.Carnivore) != other.Traits.Has(traits.Carnivore) {
			continue
		}
		count++
	}

	return count
}

func (s *BehaviorSystem) findFood(pos *components.Position, org *components.Organism, floraPos, faunaPos []components.Position, floraOrgs, faunaOrgs []*components.Organism, grid *SpatialGrid) (float32, float32, bool) {
	const fov = math.Pi // 180 degree vision
	maxDist := org.PerceptionRadius
	maxDistSq := maxDist * maxDist
	closestDistSq := maxDistSq
	var closestX, closestY float32
	found := false

	// Herbivores seek flora
	if org.Traits.Has(traits.Herbivore) {
		nearby := grid.GetNearbyFlora(pos.X, pos.Y, maxDist)
		for _, i := range nearby {
			if floraOrgs[i].Dead {
				continue
			}
			if !canSeeSq(pos.X, pos.Y, org.Heading, floraPos[i].X, floraPos[i].Y, fov, maxDistSq) {
				continue
			}
			distSq := distanceSq(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y)
			if distSq < closestDistSq {
				closestDistSq = distSq
				closestX = floraPos[i].X
				closestY = floraPos[i].Y
				found = true
			}
		}
	}

	// Carnivores seek fauna
	if org.Traits.Has(traits.Carnivore) {
		nearby := grid.GetNearbyFauna(pos.X, pos.Y, maxDist)
		for _, i := range nearby {
			if faunaOrgs[i] == org || faunaOrgs[i].Dead {
				continue
			}
			if !canSeeSq(pos.X, pos.Y, org.Heading, faunaPos[i].X, faunaPos[i].Y, fov, maxDistSq) {
				continue
			}
			distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
			if distSq < closestDistSq {
				closestDistSq = distSq
				closestX = faunaPos[i].X
				closestY = faunaPos[i].Y
				found = true
			}
		}
	}

	return closestX, closestY, found
}

// findFoodWeighted finds food using sensor-weighted perception and capability matching.
// Uses the organism's digestive spectrum to determine what counts as food.
func (s *BehaviorSystem) findFoodWeighted(
	pos *components.Position,
	org *components.Organism,
	cells *components.CellBuffer,
	effectiveRadius float32,
	floraPos, faunaPos []components.Position,
	floraOrgs, faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) (float32, float32, bool) {
	maxDistSq := effectiveRadius * effectiveRadius
	closestEffectiveDistSq := maxDistSq
	var closestX, closestY float32
	found := false

	// Compute our digestive spectrum from cells
	var myDigestive float32 = 0.5 // Default: omnivore
	if cells != nil {
		caps := cells.ComputeCapabilities()
		myDigestive = caps.DigestiveSpectrum()
	}

	// Flora composition is 1.0 (pure photosynthetic)
	// Edibility for flora = 1 - |digestive + 1.0 - 1| = 1 - |digestive|
	// So herbivores (digestive~0) have high edibility, carnivores (digestive~1) have low
	floraEdibility := neural.Edibility(myDigestive, 1.0)

	// Check if we can eat flora (edibility > typical flora armor of 0.1)
	canEatFlora := floraEdibility > 0.1

	if canEatFlora {
		nearby := grid.GetNearbyFlora(pos.X, pos.Y, effectiveRadius)
		for _, i := range nearby {
			if floraOrgs[i].Dead {
				continue
			}

			distSq := distanceSq(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y)
			if distSq > maxDistSq {
				continue
			}

			// Check line of sight - can't see through terrain
			if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y) {
				continue
			}

			// Calculate direction to target and apply sensor weighting
			targetAngle := float32(math.Atan2(float64(floraPos[i].Y-pos.Y), float64(floraPos[i].X-pos.X))) - org.Heading
			sensorBonus := sensorWeightedIntensity(cells, org.Heading, targetAngle, 1.0)

			// Better sensor coverage and higher edibility = effectively shorter distance
			effectiveDistSq := distSq / (sensorBonus * sensorBonus * floraEdibility)

			if effectiveDistSq < closestEffectiveDistSq {
				closestEffectiveDistSq = effectiveDistSq
				closestX = floraPos[i].X
				closestY = floraPos[i].Y
				found = true
			}
		}
	}

	// Fauna composition is ~0 (pure actuator, no photosynthesis)
	// Edibility for fauna = 1 - |digestive + 0 - 1| = 1 - |digestive - 1|
	// So carnivores (digestive~1) have high edibility, herbivores (digestive~0) have low
	faunaEdibility := neural.Edibility(myDigestive, 0.0)

	// Check if we can eat fauna (edibility > 0 with no armor consideration for detection)
	canEatFauna := faunaEdibility > 0.2

	if canEatFauna {
		nearby := grid.GetNearbyFauna(pos.X, pos.Y, effectiveRadius)
		for _, i := range nearby {
			if faunaOrgs[i] == org || faunaOrgs[i].Dead {
				continue
			}

			distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
			if distSq > maxDistSq {
				continue
			}

			// Check line of sight - can't see through terrain
			if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) {
				continue
			}

			targetAngle := float32(math.Atan2(float64(faunaPos[i].Y-pos.Y), float64(faunaPos[i].X-pos.X))) - org.Heading
			sensorBonus := sensorWeightedIntensity(cells, org.Heading, targetAngle, 1.0)

			// Better sensor coverage and higher edibility = effectively shorter distance
			effectiveDistSq := distSq / (sensorBonus * sensorBonus * faunaEdibility)

			if effectiveDistSq < closestEffectiveDistSq {
				closestEffectiveDistSq = effectiveDistSq
				closestX = faunaPos[i].X
				closestY = faunaPos[i].Y
				found = true
			}
		}
	}

	return closestX, closestY, found
}

func (s *BehaviorSystem) findDead(pos *components.Position, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism, floraPos []components.Position, floraOrgs []*components.Organism, grid *SpatialGrid) (float32, float32, bool) {
	maxDist := org.PerceptionRadius
	maxDistSq := maxDist * maxDist
	closestDistSq := maxDistSq
	var closestX, closestY float32
	found := false

	// Find dead fauna
	nearby := grid.GetNearbyFauna(pos.X, pos.Y, maxDist)
	for _, i := range nearby {
		if faunaOrgs[i] == org || !faunaOrgs[i].Dead {
			continue
		}
		distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if distSq < closestDistSq {
			closestDistSq = distSq
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	// Find dead flora
	nearbyFlora := grid.GetNearbyFlora(pos.X, pos.Y, maxDist)
	for _, i := range nearbyFlora {
		if !floraOrgs[i].Dead {
			continue
		}
		distSq := distanceSq(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y)
		if distSq < closestDistSq {
			closestDistSq = distSq
			closestX = floraPos[i].X
			closestY = floraPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

func (s *BehaviorSystem) findPredator(pos *components.Position, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism, grid *SpatialGrid) (float32, float32, bool) {
	// Prey have omnidirectional awareness of predators
	baseDetectDist := org.PerceptionRadius * 1.5
	maxSearchDist := baseDetectDist * 5
	closestDistSq := maxSearchDist * maxSearchDist
	var closestX, closestY float32
	found := false

	nearby := grid.GetNearbyFauna(pos.X, pos.Y, maxSearchDist)
	for _, i := range nearby {
		if faunaOrgs[i] == org {
			continue
		}
		if !faunaOrgs[i].Traits.Has(traits.Carnivore) {
			continue
		}
		if faunaOrgs[i].Dead {
			continue
		}

		// Size visibility: larger predators are easier to spot
		predatorCells := (faunaOrgs[i].MaxEnergy - 100) / 50
		if predatorCells < 1 {
			predatorCells = 1
		}
		sizeMultiplier := float32(math.Sqrt(float64(predatorCells)))

		// Movement visibility: moving predators are easier to detect
		thrust := faunaOrgs[i].ActiveThrust
		movementMultiplier := float32(0.3) + thrust*40
		if movementMultiplier > 1.5 {
			movementMultiplier = 1.5
		}

		detectDist := baseDetectDist * sizeMultiplier * movementMultiplier
		detectDistSq := detectDist * detectDist

		distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if distSq > detectDistSq {
			continue
		}
		if distSq < closestDistSq {
			closestDistSq = distSq
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

// findPredatorWeighted finds predators using sensor-weighted perception and capability matching.
// A predator is anything that can eat us based on digestive spectrum vs our composition.
func (s *BehaviorSystem) findPredatorWeighted(
	pos *components.Position,
	org *components.Organism,
	cells *components.CellBuffer,
	effectiveRadius float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) (float32, float32, bool) {
	// Compute our composition (how flora-like vs fauna-like we are)
	var myComposition float32 = 0.0 // Default: pure fauna (no photosynthesis)
	var myArmor float32 = 0.0
	if cells != nil {
		caps := cells.ComputeCapabilities()
		myComposition = caps.Composition()
		myArmor = caps.StructuralArmor
	}

	// Base detection range scaled by sensor capability
	baseDetectDist := effectiveRadius * 1.5
	maxSearchDist := baseDetectDist * 3
	closestEffectiveDistSq := maxSearchDist * maxSearchDist
	var closestX, closestY float32
	found := false

	nearby := grid.GetNearbyFauna(pos.X, pos.Y, maxSearchDist)
	for _, i := range nearby {
		if faunaOrgs[i] == org || faunaOrgs[i].Dead {
			continue
		}

		// Estimate their digestive spectrum from traits as a proxy
		// (In full capability model, we'd have their cells to compute this)
		// Carnivore trait suggests high digestive spectrum (~1.0)
		// Herbivore trait suggests low digestive spectrum (~0.0)
		var theirDigestive float32 = 0.5 // Default: omnivore
		if faunaOrgs[i].Traits.Has(traits.Carnivore) {
			theirDigestive = 0.85
		} else if faunaOrgs[i].Traits.Has(traits.Herbivore) {
			theirDigestive = 0.15
		}

		// Check if they can eat us (are they a threat?)
		threatLevel := neural.ThreatLevel(theirDigestive, myComposition, myArmor)
		if threatLevel <= 0 {
			continue // They can't eat us, not a threat
		}

		// Size visibility: larger predators are easier to spot
		predatorCells := (faunaOrgs[i].MaxEnergy - 100) / 50
		if predatorCells < 1 {
			predatorCells = 1
		}
		sizeMultiplier := float32(math.Sqrt(float64(predatorCells)))

		// Movement visibility: moving predators are easier to detect
		thrust := faunaOrgs[i].ActiveThrust
		movementMultiplier := float32(0.3) + thrust*40
		if movementMultiplier > 1.5 {
			movementMultiplier = 1.5
		}

		detectDist := baseDetectDist * sizeMultiplier * movementMultiplier
		detectDistSq := detectDist * detectDist

		distSq := distanceSq(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if distSq > detectDistSq {
			continue
		}

		// Check line of sight - can't see through terrain
		if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) {
			continue
		}

		// Apply sensor weighting - predators in sensor-covered directions detected better
		targetAngle := float32(math.Atan2(float64(faunaPos[i].Y-pos.Y), float64(faunaPos[i].X-pos.X))) - org.Heading

		// Predator detection is partially omnidirectional (survival instinct)
		// but sensors still provide a bonus for covered directions
		// Higher threat level also makes them easier to detect (more alarming)
		sensorBonus := 0.5 + 0.5*sensorWeightedIntensity(cells, org.Heading, targetAngle, 1.0)
		effectiveDistSq := distSq / (sensorBonus * sensorBonus * (0.5 + threatLevel*0.5))

		if effectiveDistSq < closestEffectiveDistSq {
			closestEffectiveDistSq = effectiveDistSq
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

func (s *BehaviorSystem) getFlowFieldForce(x, y float32, org *components.Organism, _ Bounds) (float32, float32) {
	const flowScale = 0.003
	const timeScale = 0.0001
	const baseStrength = 0.4

	noiseX := s.noise.Noise3D(float64(x)*flowScale, float64(y)*flowScale, float64(s.tick)*timeScale)
	noiseY := s.noise.Noise3D(float64(x)*flowScale+100, float64(y)*flowScale+100, float64(s.tick)*timeScale)

	flowAngle := noiseX * math.Pi * 2
	flowMagnitude := (noiseY + 1) * 0.5
	flowX := float32(math.Cos(flowAngle) * flowMagnitude * baseStrength)
	flowY := float32(math.Sin(flowAngle) * flowMagnitude * baseStrength)

	// Add downward drift
	flowY += 0.05
	flowX += float32(math.Sin(float64(s.tick)*0.0002)) * 0.02

	// Floating flora: very weak flow effect
	if traits.IsFlora(org.Traits) && org.Traits.Has(traits.Floating) {
		return flowX * 0.05, flowY * 0.05
	}

	// Shape-based flow resistance
	shapeResistance := org.ShapeMetrics.Streamlining * 0.4
	massResistance := float32(math.Min(float64(org.Energy/org.MaxEnergy)/3, 1))
	totalResistance := shapeResistance + massResistance*0.6
	factor := 1 - totalResistance*0.7

	return flowX * factor, flowY * factor
}

// Helper functions

func canSeeSq(px, py, heading, tx, ty, fov, maxDistSq float32) bool {
	dx := tx - px
	dy := ty - py
	distSq := dx*dx + dy*dy

	if distSq > maxDistSq {
		return false
	}

	angleToTarget := float32(math.Atan2(float64(dy), float64(dx)))
	angleDiff := angleToTarget - heading
	for angleDiff > math.Pi {
		angleDiff -= math.Pi * 2
	}
	for angleDiff < -math.Pi {
		angleDiff += math.Pi * 2
	}

	return float32(math.Abs(float64(angleDiff))) <= fov/2
}

func distance(x1, y1, x2, y2 float32) float32 {
	dx := x2 - x1
	dy := y2 - y1
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

func distanceSq(x1, y1, x2, y2 float32) float32 {
	dx := x2 - x1
	dy := y2 - y1
	return dx*dx + dy*dy
}

// Sensor weighting functions for body-brain coupling

// getTotalSensorGain returns the sum of sensor strengths from all alive cells with sensor capability.
func getTotalSensorGain(cells *components.CellBuffer) float32 {
	if cells == nil {
		return 1.0 // Default for organisms without cell data
	}
	var total float32
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		total += cell.GetSensorStrength()
	}
	if total < 0.1 {
		return 0.1 // Minimum sensor capability
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
	scale := float32(0.5 + math.Min(1.0, float64(totalGain)/4.0))
	return baseRadius * scale
}

// sensorWeightedIntensity weights a stimulus intensity by sensor geometry.
// Sensors facing the stimulus direction contribute more.
func sensorWeightedIntensity(
	cells *components.CellBuffer,
	heading float32,
	stimulusAngle float32, // Angle to stimulus relative to organism heading
	rawIntensity float32,
) float32 {
	if cells == nil {
		return rawIntensity
	}

	var totalWeight float32
	var weightedIntensity float32

	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		sensorStrength := cell.GetSensorStrength()
		if sensorStrength == 0 {
			continue
		}

		// Sensor's facing direction based on its grid position (outward from center)
		sensorAngle := float32(math.Atan2(float64(cell.GridY), float64(cell.GridX)))

		// Angular difference between sensor facing and stimulus direction
		angleDiff := math.Abs(float64(normalizeAngle(sensorAngle - stimulusAngle)))

		// Sensors facing the stimulus contribute more (cosine falloff)
		// Angle diff of 0 = full contribution, Pi = zero contribution
		directionalWeight := float32(math.Max(0, math.Cos(angleDiff)))

		weight := sensorStrength * directionalWeight
		totalWeight += weight
		weightedIntensity += weight * rawIntensity
	}

	if totalWeight < 0.01 {
		// No sensors facing this direction - reduced perception
		return rawIntensity * 0.3
	}

	return weightedIntensity / totalWeight
}

// normalizeAngle wraps an angle to [-Pi, Pi].
func normalizeAngle(angle float32) float32 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// Actuator helper functions for body-brain coupling

// getTotalActuatorStrength returns the sum of actuator strengths from all alive cells with actuator capability.
func getTotalActuatorStrength(cells *components.CellBuffer) float32 {
	if cells == nil {
		return 1.0 // Default for organisms without cell data
	}
	var total float32
	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		total += cell.GetActuatorStrength()
	}
	if total < 0.1 {
		return 0.1 // Minimum actuator capability
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
		return thrustOutput, turnOutput * 0.15
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

	if totalStrength < 0.1 {
		totalStrength = 0.1
	}

	// Forward thrust proportional to total actuator strength
	thrust = thrustOutput * totalStrength * 0.5

	// Torque for turning (normalized to prevent runaway with many actuators)
	torque = weightedTorque / totalStrength * 0.15

	return thrust, torque
}

// getEffectiveMaxSpeed scales max speed by actuator capability.
// More/better actuators = faster movement potential.
func getEffectiveMaxSpeed(baseSpeed float32, cells *components.CellBuffer) float32 {
	totalStrength := getTotalActuatorStrength(cells)
	// Scale: 0.5x (minimal actuators) to 1.5x (4+ actuator strength)
	scale := float32(0.5 + math.Min(1.0, float64(totalStrength)/4.0))
	return baseSpeed * scale
}

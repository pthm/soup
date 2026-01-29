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

		// Store intents for other systems (feeding, breeding)
		org.EatIntent = outputs.Eat
		org.MateIntent = outputs.Mate
		org.TurnOutput = outputs.Turn
		org.ThrustOutput = outputs.Thrust

		// Calculate actuator-driven forces
		// Actuator positions and strengths determine how Turn/Thrust translate to movement
		thrust, torque := calculateActuatorForces(cells, org.Heading, outputs.Thrust, outputs.Turn)

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

// getBrainOutputs gathers sensory inputs and runs the brain network.
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

	// Get cell data for sensor weighting
	var cells *components.CellBuffer
	if s.cellsMap.Has(entity) {
		cells = s.cellsMap.Get(entity)
	}

	// Calculate effective perception radius based on sensor capability
	effectiveRadius := getEffectivePerceptionRadius(org.PerceptionRadius, cells)

	// Gather sensory inputs
	sensory := neural.SensoryInputs{
		PerceptionRadius:  effectiveRadius,
		Energy:            org.Energy,
		MaxEnergy:         org.MaxEnergy,
		MaxCells:          16,
		SensorCount:       getSensorCount(cells),
		TotalSensorGain:   getTotalSensorGain(cells),
		ActuatorCount:     getActuatorCount(cells),
		TotalActuatorStr:  getTotalActuatorStrength(cells),
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

	// Food detection with sensor weighting
	foodX, foodY, foundFood := s.findFoodWeighted(pos, org, cells, effectiveRadius, floraPos, faunaPos, floraOrgs, faunaOrgs, grid)
	if foundFood {
		sensory.FoodFound = true
		rawDist := distance(pos.X, pos.Y, foodX, foodY)
		rawAngle := float32(math.Atan2(float64(foodY-pos.Y), float64(foodX-pos.X))) - org.Heading

		// Apply sensor weighting - sensors facing the food perceive it better
		sensory.FoodDistance = rawDist
		sensory.FoodAngle = rawAngle
	}

	// Predator detection with sensor weighting
	predX, predY, foundPred := s.findPredatorWeighted(pos, org, cells, effectiveRadius, faunaPos, faunaOrgs, grid)
	if foundPred {
		sensory.PredatorFound = true
		rawDist := distance(pos.X, pos.Y, predX, predY)
		rawAngle := float32(math.Atan2(float64(predY-pos.Y), float64(predX-pos.X))) - org.Heading

		sensory.PredatorDistance = rawDist
		sensory.PredatorAngle = rawAngle
	}

	// Mate detection with sensor weighting
	mateX, mateY, foundMate := s.findMateWeighted(pos, org, cells, effectiveRadius, faunaPos, faunaOrgs, grid)
	if foundMate {
		sensory.MateFound = true
		rawDist := distance(pos.X, pos.Y, mateX, mateY)
		rawAngle := float32(math.Atan2(float64(mateY-pos.Y), float64(mateX-pos.X))) - org.Heading

		sensory.MateDistance = rawDist
		sensory.MateAngle = rawAngle
	}

	// Herd count (nearby same-type organisms)
	sensory.HerdCount = s.countNearbySameType(pos, org, faunaPos, faunaOrgs, grid)

	// Light level - weighted by sensors facing upward (positive Y sensors)
	if s.shadowMap != nil {
		rawLight := s.shadowMap.SampleLight(pos.X, pos.Y)
		sensory.LightLevel = sensorWeightedIntensity(cells, org.Heading, -math.Pi/2, rawLight)
	} else {
		sensory.LightLevel = 0.5
	}

	// Flow field
	flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
	sensory.FlowX = flowX
	sensory.FlowY = flowY

	// Terrain sensing - detect nearby walls/obstacles
	if s.terrain != nil {
		terrainDist := s.terrain.DistanceToSolid(pos.X, pos.Y)
		// Consider terrain "found" if within effective perception radius
		if terrainDist < effectiveRadius {
			sensory.TerrainFound = true
			sensory.TerrainDistance = terrainDist

			// Get gradient (direction away from terrain) and convert to heading-relative
			gx, gy := s.terrain.GetGradient(pos.X, pos.Y)

			// Rotate gradient into heading-relative coordinates
			// If heading is 0 (facing right), gradient stays as-is
			// If heading is Pi/2 (facing down), we rotate gradient -Pi/2
			cosH := float32(math.Cos(float64(-org.Heading)))
			sinH := float32(math.Sin(float64(-org.Heading)))
			sensory.TerrainGradientX = gx*cosH - gy*sinH
			sensory.TerrainGradientY = gx*sinH + gy*cosH
		}
	}

	// Convert to neural inputs and run brain
	inputs := sensory.ToInputs()
	rawOutputs, err := brain.Controller.Think(inputs)
	if err != nil {
		return neural.DefaultOutputs()
	}

	return neural.DecodeOutputs(rawOutputs)
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

// findFoodWeighted finds food using sensor-weighted perception.
// Sensors facing the food direction contribute more to detection.
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

	// Herbivores seek flora
	if org.Traits.Has(traits.Herbivore) {
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

			// Better sensor coverage = effectively shorter distance
			effectiveDistSq := distSq / (sensorBonus * sensorBonus)

			if effectiveDistSq < closestEffectiveDistSq {
				closestEffectiveDistSq = effectiveDistSq
				closestX = floraPos[i].X
				closestY = floraPos[i].Y
				found = true
			}
		}
	}

	// Carnivores seek fauna
	if org.Traits.Has(traits.Carnivore) {
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
			effectiveDistSq := distSq / (sensorBonus * sensorBonus)

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

// findPredatorWeighted finds predators using sensor-weighted perception.
// Predator detection is semi-omnidirectional but sensors still help.
func (s *BehaviorSystem) findPredatorWeighted(
	pos *components.Position,
	org *components.Organism,
	cells *components.CellBuffer,
	effectiveRadius float32,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	grid *SpatialGrid,
) (float32, float32, bool) {
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
		if !faunaOrgs[i].Traits.Has(traits.Carnivore) {
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

		// Check line of sight - can't see through terrain
		if s.terrain != nil && !s.terrain.HasLineOfSight(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y) {
			continue
		}

		// Apply sensor weighting - predators in sensor-covered directions detected better
		targetAngle := float32(math.Atan2(float64(faunaPos[i].Y-pos.Y), float64(faunaPos[i].X-pos.X))) - org.Heading

		// Predator detection is partially omnidirectional (survival instinct)
		// but sensors still provide a bonus for covered directions
		sensorBonus := 0.5 + 0.5*sensorWeightedIntensity(cells, org.Heading, targetAngle, 1.0)
		effectiveDistSq := distSq / (sensorBonus * sensorBonus)

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

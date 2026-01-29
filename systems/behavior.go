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
	noise     *PerlinNoise
	tick      int32
	shadowMap *ShadowMap
}

// NewBehaviorSystem creates a new behavior system.
func NewBehaviorSystem(w *ecs.World, shadowMap *ShadowMap) *BehaviorSystem {
	return &BehaviorSystem{
		filter:    *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		brainMap:  ecs.NewMap[components.Brain](w),
		noise:     NewPerlinNoise(rand.Int63()),
		shadowMap: shadowMap,
	}
}

// Update runs the behavior system with direct neural control.
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

		// Direct heading control from Turn output
		// Turn is in range [-1, 1], scale to radians per tick
		const maxTurnRate = 0.15 // Max radians per tick
		org.Heading += outputs.Turn * maxTurnRate

		// Normalize heading to [0, 2*Pi]
		for org.Heading < 0 {
			org.Heading += 2 * math.Pi
		}
		for org.Heading >= 2*math.Pi {
			org.Heading -= 2 * math.Pi
		}

		// Direct velocity from Thrust output
		thrust := outputs.Thrust * org.MaxSpeed
		thrustX := float32(math.Cos(float64(org.Heading))) * thrust * 0.1
		thrustY := float32(math.Sin(float64(org.Heading))) * thrust * 0.1

		// Apply flow field
		flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
		vel.X += thrustX + flowX
		vel.Y += thrustY + flowY

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

	// Gather sensory inputs
	sensory := neural.SensoryInputs{
		PerceptionRadius: org.PerceptionRadius,
		Energy:           org.Energy,
		MaxEnergy:        org.MaxEnergy,
		MaxCells:         16,
	}

	// Estimate cell count from energy capacity
	sensory.CellCount = int((org.MaxEnergy - 100) / 50)
	if sensory.CellCount < 1 {
		sensory.CellCount = 1
	}

	// Food detection
	foodX, foodY, foundFood := s.findFood(pos, org, floraPos, faunaPos, floraOrgs, faunaOrgs, grid)
	if foundFood {
		sensory.FoodFound = true
		sensory.FoodDistance = distance(pos.X, pos.Y, foodX, foodY)
		sensory.FoodAngle = float32(math.Atan2(float64(foodY-pos.Y), float64(foodX-pos.X))) - org.Heading
	}

	// Predator detection
	predX, predY, foundPred := s.findPredator(pos, org, faunaPos, faunaOrgs, grid)
	if foundPred {
		sensory.PredatorFound = true
		sensory.PredatorDistance = distance(pos.X, pos.Y, predX, predY)
		sensory.PredatorAngle = float32(math.Atan2(float64(predY-pos.Y), float64(predX-pos.X))) - org.Heading
	}

	// Mate detection
	mateX, mateY, foundMate := s.findMate(pos, org, faunaPos, faunaOrgs, grid)
	if foundMate {
		sensory.MateFound = true
		sensory.MateDistance = distance(pos.X, pos.Y, mateX, mateY)
		sensory.MateAngle = float32(math.Atan2(float64(mateY-pos.Y), float64(mateX-pos.X))) - org.Heading
	}

	// Herd count (nearby same-type organisms)
	sensory.HerdCount = s.countNearbySameType(pos, org, faunaPos, faunaOrgs, grid)

	// Light level
	if s.shadowMap != nil {
		sensory.LightLevel = s.shadowMap.SampleLight(pos.X, pos.Y)
	} else {
		sensory.LightLevel = 0.5
	}

	// Flow field
	flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
	sensory.FlowX = flowX
	sensory.FlowY = flowY

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

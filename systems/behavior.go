package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// BehaviorSystem handles organism steering behaviors.
type BehaviorSystem struct {
	filter ecs.Filter3[components.Position, components.Velocity, components.Organism]
	noise  *PerlinNoise
	tick   int32
}

// NewBehaviorSystem creates a new behavior system.
func NewBehaviorSystem(w *ecs.World) *BehaviorSystem {
	return &BehaviorSystem{
		filter: *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		noise:  NewPerlinNoise(rand.Int63()),
	}
}

// Update runs the behavior system.
func (s *BehaviorSystem) Update(w *ecs.World, bounds Bounds, floraPositions, faunaPositions []components.Position, floraOrgs, faunaOrgs []*components.Organism) {
	s.tick++

	query := s.filter.Query()
	for query.Next() {
		pos, vel, org := query.Get()

		// Skip stationary flora
		if traits.IsFlora(org.Traits) && !org.Traits.Has(traits.Floating) {
			continue
		}

		// Skip dead organisms
		if org.Dead {
			continue
		}

		var steerX, steerY float32

		// Find and seek food
		foodX, foodY, foundFood := s.findFood(pos, org, floraPositions, faunaPositions, floraOrgs, faunaOrgs)
		if foundFood {
			seekX, seekY := seek(pos.X, pos.Y, foodX, foodY, vel.X, vel.Y, org.MaxSpeed)
			steerX += seekX * 1.5
			steerY += seekY * 1.5
		}

		// Flee from predators (if herbivore)
		if org.Traits.Has(traits.Herbivore) && !org.Traits.Has(traits.Carnivore) {
			predX, predY, foundPred := s.findPredator(pos, org, faunaPositions, faunaOrgs)
			if foundPred {
				fleeX, fleeY := flee(pos.X, pos.Y, predX, predY, vel.X, vel.Y, org.MaxSpeed)
				steerX += fleeX * 2
				steerY += fleeY * 2
			}
		}

		// Herding behavior
		if org.Traits.Has(traits.Herding) {
			herdX, herdY := s.flockWithHerd(pos, vel, org, faunaPositions, faunaOrgs)
			steerX += herdX * 1.2
			steerY += herdY * 1.2
		}

		// Wander if no other behavior
		steerMag := float32(math.Sqrt(float64(steerX*steerX + steerY*steerY)))
		if steerMag < 0.01 {
			org.Heading += (rand.Float32() - 0.5) * 0.3
			steerX = float32(math.Cos(float64(org.Heading))) * 0.4
			steerY = float32(math.Sin(float64(org.Heading))) * 0.4
		}

		// Apply flow field
		flowX, flowY := s.getFlowFieldForce(pos.X, pos.Y, org, bounds)
		steerX += flowX
		steerY += flowY

		// Limit steering force
		steerMag = float32(math.Sqrt(float64(steerX*steerX + steerY*steerY)))
		if steerMag > org.MaxForce {
			scale := org.MaxForce / steerMag
			steerX *= scale
			steerY *= scale
		}

		// Apply steering
		vel.X += steerX
		vel.Y += steerY
	}
}

func (s *BehaviorSystem) findFood(pos *components.Position, org *components.Organism, floraPos, faunaPos []components.Position, floraOrgs, faunaOrgs []*components.Organism) (float32, float32, bool) {
	vision := traits.GetVisionParams(org.Traits)
	maxDist := org.PerceptionRadius * vision.RangeMultiplier
	closestDist := maxDist
	var closestX, closestY float32
	found := false

	// Herbivores seek flora
	if org.Traits.Has(traits.Herbivore) {
		for i := range floraPos {
			if !canSee(pos.X, pos.Y, org.Heading, floraPos[i].X, floraPos[i].Y, vision.FOV, maxDist) {
				continue
			}
			dist := distance(pos.X, pos.Y, floraPos[i].X, floraPos[i].Y)
			if dist < closestDist {
				closestDist = dist
				closestX = floraPos[i].X
				closestY = floraPos[i].Y
				found = true
			}
		}
	}

	// Carnivores seek fauna
	if org.Traits.Has(traits.Carnivore) {
		for i := range faunaPos {
			if faunaOrgs[i] == org {
				continue
			}
			if !canSee(pos.X, pos.Y, org.Heading, faunaPos[i].X, faunaPos[i].Y, vision.FOV, maxDist) {
				continue
			}
			dist := distance(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
			if dist < closestDist {
				closestDist = dist
				closestX = faunaPos[i].X
				closestY = faunaPos[i].Y
				found = true
			}
		}
	}

	return closestX, closestY, found
}

func (s *BehaviorSystem) findPredator(pos *components.Position, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism) (float32, float32, bool) {
	vision := traits.GetVisionParams(org.Traits)
	maxDist := org.PerceptionRadius * vision.RangeMultiplier
	closestDist := maxDist
	var closestX, closestY float32
	found := false

	for i := range faunaPos {
		if faunaOrgs[i] == org {
			continue
		}
		if !faunaOrgs[i].Traits.Has(traits.Carnivore) {
			continue
		}
		dist := distance(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if dist < closestDist {
			closestDist = dist
			closestX = faunaPos[i].X
			closestY = faunaPos[i].Y
			found = true
		}
	}

	return closestX, closestY, found
}

func (s *BehaviorSystem) flockWithHerd(pos *components.Position, vel *components.Velocity, org *components.Organism, faunaPos []components.Position, faunaOrgs []*components.Organism) (float32, float32) {
	herdRadius := org.PerceptionRadius * 1.5
	var sepX, sepY, cohX, cohY float32
	var count int

	for i := range faunaPos {
		if faunaOrgs[i] == org || faunaOrgs[i].Dead {
			continue
		}
		if !faunaOrgs[i].Traits.Has(traits.Herding) {
			continue
		}
		// Same type check
		if org.Traits.Has(traits.Carnivore) != faunaOrgs[i].Traits.Has(traits.Carnivore) {
			continue
		}

		dist := distance(pos.X, pos.Y, faunaPos[i].X, faunaPos[i].Y)
		if dist > herdRadius || dist == 0 {
			continue
		}

		// Separation
		dx := pos.X - faunaPos[i].X
		dy := pos.Y - faunaPos[i].Y
		sepX += dx / dist
		sepY += dy / dist

		// Cohesion
		cohX += faunaPos[i].X
		cohY += faunaPos[i].Y

		count++
	}

	if count == 0 {
		return 0, 0
	}

	// Average separation
	sepX /= float32(count)
	sepY /= float32(count)

	// Cohesion: seek center
	cohX /= float32(count)
	cohY /= float32(count)
	cohX, cohY = seek(pos.X, pos.Y, cohX, cohY, vel.X, vel.Y, org.MaxSpeed)

	return sepX*1.5 + cohX*0.8, sepY*1.5 + cohY*0.8
}

func (s *BehaviorSystem) getFlowFieldForce(x, y float32, org *components.Organism, bounds Bounds) (float32, float32) {
	const flowScale = 0.003
	const timeScale = 0.0001 // Slowed down to match flow particles
	const baseStrength = 0.4  // Reduced for gentler movement

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

	// Larger organisms resist flow better - but we don't have cell count here
	// Use energy as proxy for mass
	mass := org.Energy / org.MaxEnergy
	resistance := float32(math.Min(float64(mass)/3, 1))
	factor := 1 - resistance*0.8

	return flowX * factor, flowY * factor
}

// Helper functions

func seek(px, py, tx, ty, vx, vy, maxSpeed float32) (float32, float32) {
	dx := tx - px
	dy := ty - py
	mag := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if mag > 0 {
		dx = dx / mag * maxSpeed
		dy = dy / mag * maxSpeed
	}
	return dx - vx, dy - vy
}

func flee(px, py, tx, ty, vx, vy, maxSpeed float32) (float32, float32) {
	sx, sy := seek(px, py, tx, ty, vx, vy, maxSpeed)
	return -sx, -sy
}

func canSee(px, py, heading, tx, ty, fov, maxDist float32) bool {
	dx := tx - px
	dy := ty - py
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	if dist > maxDist {
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

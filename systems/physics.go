// Package systems contains ECS systems for the simulation.
package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Physics constants
const (
	deadMaxSpeed          = float32(0.5)  // Maximum drift speed for dead organisms
	deadFriction          = float32(0.96) // Velocity dampening for dead organisms
	aliveFrictionMin      = float32(0.96) // Base friction for alive organisms
	aliveFrictionRange    = float32(0.03) // Additional friction from streamlining (0 to this)
	headingUpdateMinVelSq = float32(0.01) // Minimum velocity squared to update heading
)

// PhysicsSystem updates entity positions based on velocity.
// All ECS organisms are fauna - flora are managed separately by FloraSystem.
type PhysicsSystem struct {
	filter ecs.Filter3[components.Position, components.Velocity, components.Organism]
	bounds Bounds
}

// Bounds represents the simulation bounds.
type Bounds struct {
	Width, Height float32
}

// Occluder represents something that blocks light.
type Occluder struct {
	X, Y, Width, Height float32
	Density             float32 // 0-1, how much light is blocked (1 = fully solid, 0.3 = sparse like foliage)
}

// NewPhysicsSystem creates a physics system.
func NewPhysicsSystem(w *ecs.World, bounds Bounds) *PhysicsSystem {
	return &PhysicsSystem{
		filter: *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		bounds: bounds,
	}
}

// Update runs the physics system.
func (s *PhysicsSystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		pos, vel, org := query.Get()

		// All ECS organisms are fauna (flora are in FloraSystem)

		// Dead organisms drift slowly with reduced max speed
		maxSpeed := org.MaxSpeed
		if org.Dead {
			maxSpeed = deadMaxSpeed
		}

		// Limit velocity
		velMag := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		if velMag > maxSpeed {
			scale := maxSpeed / velMag
			vel.X *= scale
			vel.Y *= scale
		}

		// Update position
		pos.X += vel.X
		pos.Y += vel.Y

		// Shape-based friction: low drag organisms coast further
		baseFriction := aliveFrictionMin
		if org.Dead {
			baseFriction = deadFriction
		} else {
			// Low drag = coasts further (higher friction multiplier = less slowdown)
			// Drag 0.2 (fish) -> bonus 0.8 * range, Drag 1.0 (plate) -> bonus 0 * range
			streamlined := clampFloat(1.0-org.ShapeMetrics.Drag, 0, 1)
			baseFriction = aliveFrictionMin + streamlined*aliveFrictionRange
		}
		vel.X *= baseFriction
		vel.Y *= baseFriction

		// Heading is now brain-controlled state (heading-as-state model)
		// No longer derived from velocity - this eliminates the feedback loop
		// that caused jittering with constant UFwd/UUp outputs

		// Toroidal wrap-around (all edges) into [0, width) / [0, height)
		for pos.X < 0 {
			pos.X += s.bounds.Width
		}
		for pos.X >= s.bounds.Width {
			pos.X -= s.bounds.Width
		}
		for pos.Y < 0 {
			pos.Y += s.bounds.Height
		}
		for pos.Y >= s.bounds.Height {
			pos.Y -= s.bounds.Height
		}
	}
}

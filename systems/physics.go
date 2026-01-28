// Package systems contains ECS systems for the simulation.
package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// PhysicsSystem updates entity positions based on velocity.
type PhysicsSystem struct {
	filter  ecs.Filter3[components.Position, components.Velocity, components.Organism]
	bounds  Bounds
}

// Bounds represents the simulation bounds.
type Bounds struct {
	Width, Height float32
}

// NewPhysicsSystem creates a new physics system.
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

		// Skip stationary flora
		if traits.IsFlora(org.Traits) && !org.Traits.Has(traits.Floating) {
			continue
		}

		// Limit velocity
		velMag := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
		if velMag > org.MaxSpeed {
			scale := org.MaxSpeed / velMag
			vel.X *= scale
			vel.Y *= scale
		}

		// Update position
		pos.X += vel.X
		pos.Y += vel.Y

		// Apply friction
		vel.X *= 0.98
		vel.Y *= 0.98

		// Update heading
		if vel.X*vel.X+vel.Y*vel.Y > 0.01 {
			org.Heading = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}

		// Keep in bounds
		cellRadius := org.CellSize * float32(3) // Approximate organism radius
		if pos.X < cellRadius {
			pos.X = cellRadius
		}
		if pos.X > s.bounds.Width-cellRadius {
			pos.X = s.bounds.Width - cellRadius
		}

		// Rooted organisms stay at bottom
		if org.Traits.Has(traits.Rooted) {
			pos.Y = s.bounds.Height - org.CellSize
		} else {
			if pos.Y < cellRadius {
				pos.Y = cellRadius
			}
			if pos.Y > s.bounds.Height-cellRadius {
				pos.Y = s.bounds.Height - cellRadius
			}
		}
	}
}

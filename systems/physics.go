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
	terrain *TerrainSystem
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

// NewPhysicsSystem creates a new physics system.
func NewPhysicsSystem(w *ecs.World, bounds Bounds) *PhysicsSystem {
	return &PhysicsSystem{
		filter: *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		bounds: bounds,
	}
}

// NewPhysicsSystemWithTerrain creates a physics system with terrain collision.
func NewPhysicsSystemWithTerrain(w *ecs.World, bounds Bounds, terrain *TerrainSystem) *PhysicsSystem {
	return &PhysicsSystem{
		filter:  *ecs.NewFilter3[components.Position, components.Velocity, components.Organism](w),
		bounds:  bounds,
		terrain: terrain,
	}
}

// Update runs the physics system.
func (s *PhysicsSystem) Update(w *ecs.World) {
	query := s.filter.Query()
	for query.Next() {
		pos, vel, org := query.Get()

		// Skip stationary flora (but not if dead - dead flora can drift)
		if traits.IsFlora(org.Traits) && !org.Traits.Has(traits.Floating) && !org.Dead {
			continue
		}

		// Dead organisms drift slowly with reduced max speed
		maxSpeed := org.MaxSpeed
		if org.Dead {
			maxSpeed = 0.5 // Slow drift for corpses
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

		// Terrain collision - slide along walls instead of bouncing
		// Use OBB collision if available, fall back to circle collision
		if s.terrain != nil {
			var collides bool
			var nx, ny float32

			// Check if OBB is valid (has non-zero half-extents)
			if org.OBB.HalfWidth > 0 && org.OBB.HalfHeight > 0 {
				collides = s.terrain.CheckOBBCollision(pos.X, pos.Y, org.Heading, &org.OBB)
				if collides {
					openX, openY, normalX, normalY := s.terrain.FindNearestOpenOBB(pos.X, pos.Y, org.Heading, &org.OBB)
					pos.X, pos.Y = openX, openY
					nx, ny = normalX, normalY
				}
			} else {
				// Fallback to circle collision for organisms without OBB
				radius := org.CellSize * 3
				collides = s.terrain.CheckCircleCollision(pos.X, pos.Y, radius)
				if collides {
					openX, openY, normalX, normalY := s.terrain.FindNearestOpen(pos.X, pos.Y, radius)
					pos.X, pos.Y = openX, openY
					nx, ny = normalX, normalY
				}
			}

			if collides {
				// Project velocity onto wall (slide along it)
				// Remove the component going into the wall
				dot := vel.X*nx + vel.Y*ny
				if dot < 0 { // Only if moving into wall
					vel.X -= dot * nx
					vel.Y -= dot * ny
				}
				// Apply friction from wall contact
				vel.X *= 0.8
				vel.Y *= 0.8
			}
		}

		// Shape-based friction: streamlined organisms coast further
		baseFriction := float32(0.98)
		if org.Dead {
			baseFriction = 0.96
		} else {
			// Streamlined organisms coast further (higher friction = less slowdown)
			baseFriction = 0.96 + org.ShapeMetrics.Streamlining*0.03 // 0.96 to 0.99
		}
		vel.X *= baseFriction
		vel.Y *= baseFriction

		// Update heading (not for dead)
		if !org.Dead && vel.X*vel.X+vel.Y*vel.Y > 0.01 {
			org.Heading = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}

		// Horizontal wrap-around
		if pos.X < 0 {
			pos.X += s.bounds.Width
		}
		if pos.X > s.bounds.Width {
			pos.X -= s.bounds.Width
		}

		// Rooted organisms stay at bottom (unless dead)
		if org.Traits.Has(traits.Rooted) && !org.Dead {
			pos.Y = s.bounds.Height - org.CellSize
		} else {
			// Vertical bounds (no wrap - top and bottom are walls)
			cellRadius := org.CellSize * float32(3)
			if pos.Y < cellRadius {
				pos.Y = cellRadius
				vel.Y *= -0.3 // Bounce slightly
			}
			if pos.Y > s.bounds.Height-cellRadius {
				pos.Y = s.bounds.Height - cellRadius
				vel.Y *= -0.3 // Bounce slightly
			}
		}
	}
}

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
	wallFriction          = float32(0.5)  // Velocity multiplier on wall contact (strong brake)
	bounceCoeff           = float32(0.3)  // Velocity multiplier when bouncing off top/bottom
	fallbackCircleScale   = float32(3)    // Multiplier for cellSize when OBB unavailable
	headingUpdateMinVelSq = float32(0.01) // Minimum velocity squared to update heading
)

// PhysicsSystem updates entity positions based on velocity.
// All ECS organisms are fauna - flora are managed separately by FloraSystem.
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

		// Terrain collision - slide along walls instead of bouncing
		// Use OBB collision if available, fall back to circle collision
		if s.terrain != nil {
			var collides bool
			var nx, ny float32

			// Check if OBB is valid (has non-zero half-extents)
			if org.OBB.HalfWidth > 0 && org.OBB.HalfHeight > 0 {
				// X+ is forward in local grid space, aligned with heading
				collides = s.terrain.CheckOBBCollision(pos.X, pos.Y, org.Heading, &org.OBB)
				if collides {
					openX, openY, normalX, normalY := s.terrain.FindNearestOpenOBB(pos.X, pos.Y, org.Heading, &org.OBB)
					pos.X, pos.Y = openX, openY
					nx, ny = normalX, normalY
				}
			} else {
				// Fallback to circle collision for organisms without OBB
				radius := org.CellSize * fallbackCircleScale
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
				vel.X *= wallFriction
				vel.Y *= wallFriction
			}
		}

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

		// Update heading (not for dead)
		if !org.Dead && vel.X*vel.X+vel.Y*vel.Y > headingUpdateMinVelSq {
			org.Heading = float32(math.Atan2(float64(vel.Y), float64(vel.X)))
		}

		// Horizontal wrap-around
		if pos.X < 0 {
			pos.X += s.bounds.Width
		}
		if pos.X > s.bounds.Width {
			pos.X -= s.bounds.Width
		}

		// Vertical bounds (no wrap - top and bottom are walls)
		// Stop vertical motion and apply friction (no bounce - just settle)
		cellRadius := org.CellSize * fallbackCircleScale
		if pos.Y < cellRadius {
			pos.Y = cellRadius
			if vel.Y < 0 {
				vel.Y = 0 // Stop downward motion
			}
			vel.X *= wallFriction // Friction from floor contact
		}
		if pos.Y > s.bounds.Height-cellRadius {
			pos.Y = s.bounds.Height - cellRadius
			if vel.Y > 0 {
				vel.Y = 0 // Stop upward motion
			}
			vel.X *= wallFriction // Friction from ceiling contact
		}
	}
}

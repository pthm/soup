package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/traits"
)

// SporeEntity represents a spore in flight.
type SporeEntity struct {
	X, Y           float32
	VelX, VelY     float32
	ParentTraits   traits.Trait
	Lifespan       int32
	LandedTimer    int32 // Ticks since landing
	Landed         bool
	Rooted         bool
}

// SporeSystem handles flora reproduction via spores.
type SporeSystem struct {
	Spores    []SporeEntity
	noise     *PerlinNoise
	bounds    Bounds
	maxSpores int
	terrain   *TerrainSystem
}

// NewSporeSystem creates a new spore system.
func NewSporeSystem(bounds Bounds) *SporeSystem {
	return &SporeSystem{
		Spores:    make([]SporeEntity, 0, 200),
		noise:     NewPerlinNoise(rand.Int63()),
		bounds:    bounds,
		maxSpores: 200,
	}
}

// NewSporeSystemWithTerrain creates a spore system with terrain awareness.
func NewSporeSystemWithTerrain(bounds Bounds, terrain *TerrainSystem) *SporeSystem {
	return &SporeSystem{
		Spores:    make([]SporeEntity, 0, 200),
		noise:     NewPerlinNoise(rand.Int63()),
		bounds:    bounds,
		maxSpores: 200,
		terrain:   terrain,
	}
}

// SpawnSpore creates a new spore from a parent flora.
func (s *SporeSystem) SpawnSpore(x, y float32, parentTraits traits.Trait) {
	if len(s.Spores) >= s.maxSpores {
		return
	}

	// Initial velocity: slight upward drift with random horizontal
	velX := (rand.Float32() - 0.5) * 0.3
	velY := -0.2 - rand.Float32()*0.3 // Upward

	s.Spores = append(s.Spores, SporeEntity{
		X:            x,
		Y:            y,
		VelX:         velX,
		VelY:         velY,
		ParentTraits: parentTraits,
		Lifespan:     600,
		LandedTimer:  0,
		Landed:       false,
		Rooted:       false,
	})
}

// Update processes all spores.
func (s *SporeSystem) Update(tick int32, createOrganism func(x, y float32, t traits.Trait, energy float32) ecs.Entity) {
	alive := 0
	for i := range s.Spores {
		spore := &s.Spores[i]

		spore.Lifespan--
		if spore.Lifespan <= 0 {
			continue
		}

		if spore.Landed {
			// Count down to germination
			spore.LandedTimer++
			if spore.LandedTimer >= 50 {
				// Germinate into new flora
				s.germinate(spore, createOrganism)
				continue // Remove spore after germination
			}
		} else {
			// Drift with Perlin noise
			s.updateDrift(spore, tick)

			// Check for landing on terrain
			if s.terrain != nil {
				// Check if spore hit terrain
				if s.terrain.IsSolid(spore.X, spore.Y+2) {
					// Find the surface position
					for spore.Y > 0 && s.terrain.IsSolid(spore.X, spore.Y) {
						spore.Y--
					}
					spore.Landed = true
					spore.Rooted = true // Landing on terrain = rooted
					spore.VelX = 0
					spore.VelY = 0
				}
			}

			// Check for landing (non-terrain cases)
			if !spore.Landed {
				if spore.Rooted {
					// Rooted spores land when reaching bottom
					if spore.Y >= s.bounds.Height-4 {
						spore.Y = s.bounds.Height - 4
						spore.Landed = true
						spore.VelX = 0
						spore.VelY = 0
					}
				} else {
					// Floating spores have random chance to settle
					velMag := float32(math.Sqrt(float64(spore.VelX*spore.VelX + spore.VelY*spore.VelY)))
					if velMag < 0.1 && rand.Float32() < 0.002 {
						spore.Landed = true
					}
				}
			}
		}

		// Keep spore
		s.Spores[alive] = s.Spores[i]
		alive++
	}
	s.Spores = s.Spores[:alive]
}

func (s *SporeSystem) updateDrift(spore *SporeEntity, tick int32) {
	const noiseScale = 0.01
	const timeScale = 0.002

	// Perlin noise for organic movement
	noiseX := s.noise.Noise3D(float64(spore.X)*noiseScale, float64(spore.Y)*noiseScale, float64(tick)*timeScale)
	noiseY := s.noise.Noise3D(float64(spore.X)*noiseScale+100, float64(spore.Y)*noiseScale+100, float64(tick)*timeScale)

	// Apply noise force
	spore.VelX += float32(noiseX) * 0.02
	spore.VelY += float32(noiseY) * 0.02

	// Slight downward bias for rooted spores
	if spore.Rooted {
		spore.VelY += 0.005
	}

	// Drag
	spore.VelX *= 0.95
	spore.VelY *= 0.95

	// Cap velocity
	velMag := float32(math.Sqrt(float64(spore.VelX*spore.VelX + spore.VelY*spore.VelY)))
	if velMag > 0.8 {
		scale := 0.8 / velMag
		spore.VelX *= scale
		spore.VelY *= scale
	}

	// Update position
	newX := spore.X + spore.VelX
	newY := spore.Y + spore.VelY

	// Check terrain collision before moving
	if s.terrain != nil && s.terrain.IsSolid(newX, newY) {
		// Bounce off terrain
		if s.terrain.IsSolid(newX, spore.Y) {
			spore.VelX *= -0.5
			newX = spore.X
		}
		if s.terrain.IsSolid(spore.X, newY) {
			spore.VelY *= -0.3
			newY = spore.Y
		}
	}

	spore.X = newX
	spore.Y = newY

	// Horizontal wrap-around
	if spore.X < 0 {
		spore.X = s.bounds.Width
	}
	if spore.X > s.bounds.Width {
		spore.X = 0
	}

	// Vertical bounds
	if spore.Y < 0 {
		spore.Y = 0
		spore.VelY = -spore.VelY * 0.5
	}
	if spore.Y > s.bounds.Height {
		spore.Y = s.bounds.Height
		spore.VelY = 0
	}
}

func (s *SporeSystem) germinate(spore *SporeEntity, createOrganism func(x, y float32, t traits.Trait, energy float32) ecs.Entity) {
	// Build new flora traits
	newTraits := traits.Flora

	// Check if landed on terrain
	onTerrain := s.terrain != nil && s.terrain.IsSolid(spore.X, spore.Y+4)
	onSeafloor := spore.Y >= s.bounds.Height-10

	// If landed on terrain or seafloor, always root
	if onTerrain || onSeafloor || spore.Rooted {
		newTraits = newTraits.Add(traits.Rooted)
	} else {
		// Inherit parent traits with 80% probability each
		if spore.ParentTraits.Has(traits.Rooted) && rand.Float32() < 0.8 {
			newTraits = newTraits.Add(traits.Rooted)
		} else if spore.ParentTraits.Has(traits.Floating) && rand.Float32() < 0.8 {
			newTraits = newTraits.Add(traits.Floating)
		}

		// If neither rooted nor floating, default to floating
		if !newTraits.Has(traits.Rooted) && !newTraits.Has(traits.Floating) {
			newTraits = newTraits.Add(traits.Floating)
		}
	}

	// Create the new organism
	createOrganism(spore.X, spore.Y, newTraits, 60)
}

// Count returns the current number of active spores.
func (s *SporeSystem) Count() int {
	return len(s.Spores)
}

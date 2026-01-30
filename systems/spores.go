package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
)

// Spore system constants
const (
	sporeMaxCount         = 200              // Maximum active spores
	sporeLifespan         = 600              // Ticks before spore expires
	sporeLandedGermTime   = 50               // Ticks after landing before germination
	sporeGerminateEnergy  = float32(60)      // Initial energy for germinated flora
	sporeMaxVelocity      = float32(0.8)     // Maximum spore drift speed
	sporeDrag             = float32(0.95)    // Velocity dampening per tick
	sporeNoiseScale       = 0.01             // Perlin noise spatial scale
	sporeTimeScale        = 0.002            // Perlin noise temporal scale
	sporeTopZoneFraction  = float32(0.25)    // Top zone where rooting is blocked
	sporeSettleChance     = float32(0.002)   // Per-tick chance for floating spore to settle
	sporeSettleMinVel     = float32(0.1)     // Max velocity to allow settling
)

// SporeEntity represents a spore in flight.
type SporeEntity struct {
	X, Y         float32
	VelX, VelY   float32
	ParentRooted bool  // Whether parent was rooted (influences germination)
	Lifespan     int32
	LandedTimer  int32 // Ticks since landing
	Landed       bool
	Rooted       bool // Whether this spore will germinate as rooted
}

// SporeSystem handles flora reproduction via spores.
type SporeSystem struct {
	Spores    []SporeEntity
	noise     *PerlinNoise
	bounds    Bounds
	maxSpores int
	terrain   *TerrainSystem
}

// NewSporeSystemWithTerrain creates a spore system with terrain awareness.
func NewSporeSystemWithTerrain(bounds Bounds, terrain *TerrainSystem) *SporeSystem {
	return &SporeSystem{
		Spores:    make([]SporeEntity, 0, sporeMaxCount),
		noise:     NewPerlinNoise(rand.Int63()),
		bounds:    bounds,
		maxSpores: sporeMaxCount,
		terrain:   terrain,
	}
}

// SpawnSpore creates a new spore from a parent flora.
func (s *SporeSystem) SpawnSpore(x, y float32, parentRooted bool) {
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
		ParentRooted: parentRooted,
		Lifespan:     sporeLifespan,
		LandedTimer:  0,
		Landed:       false,
		Rooted:       false,
	})
}

// FloraCreator is called to create new flora when spores germinate.
// Takes position, whether it should be rooted, and initial energy.
type FloraCreator func(x, y float32, isRooted bool, energy float32) ecs.Entity

// Update processes all spores.
func (s *SporeSystem) Update(tick int32, createFlora FloraCreator) {
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
			if spore.LandedTimer >= sporeLandedGermTime {
				// Germinate into new flora
				s.germinate(spore, createFlora)
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
					// Only allow rooting below the top zone of screen
					// (don't let flora blot out the sun at the top)
					if spore.Y > s.bounds.Height*sporeTopZoneFraction {
						spore.Landed = true
						spore.Rooted = true // Landing on terrain = rooted
						spore.VelX = 0
						spore.VelY = 0
					} else {
						// Bounce off terrain near top - become floating
						spore.VelY = 0.3 // Push down
						spore.Rooted = false
					}
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
					if velMag < sporeSettleMinVel && rand.Float32() < sporeSettleChance {
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
	// Perlin noise for organic movement
	noiseX := s.noise.Noise3D(float64(spore.X)*sporeNoiseScale, float64(spore.Y)*sporeNoiseScale, float64(tick)*sporeTimeScale)
	noiseY := s.noise.Noise3D(float64(spore.X)*sporeNoiseScale+100, float64(spore.Y)*sporeNoiseScale+100, float64(tick)*sporeTimeScale)

	// Apply noise force
	spore.VelX += float32(noiseX) * 0.02
	spore.VelY += float32(noiseY) * 0.02

	// Slight downward bias for rooted spores
	if spore.Rooted {
		spore.VelY += 0.005
	}

	// Drag
	spore.VelX *= sporeDrag
	spore.VelY *= sporeDrag

	// Cap velocity
	velMag := float32(math.Sqrt(float64(spore.VelX*spore.VelX + spore.VelY*spore.VelY)))
	if velMag > sporeMaxVelocity {
		scale := sporeMaxVelocity / velMag
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

func (s *SporeSystem) germinate(spore *SporeEntity, createFlora FloraCreator) {
	// Check if in top zone where rooting is blocked
	inTopZone := spore.Y < s.bounds.Height*sporeTopZoneFraction

	// Determine if flora should be rooted based on location
	isRooted := s.shouldRoot(spore, inTopZone)

	// Create the new flora
	createFlora(spore.X, spore.Y, isRooted, sporeGerminateEnergy)
}

// shouldRoot determines whether a germinating spore should become rooted flora.
func (s *SporeSystem) shouldRoot(spore *SporeEntity, inTopZone bool) bool {
	if inTopZone {
		return false // Never root in top zone
	}

	// Check if on terrain or seafloor
	onTerrain := s.terrain != nil && s.terrain.IsSolid(spore.X, spore.Y+4)
	onSeafloor := spore.Y >= s.bounds.Height-10

	if onTerrain || onSeafloor || spore.Rooted {
		return true
	}

	// Inherit parent's rooted status with 80% probability
	const inheritRootChance = 0.8
	return spore.ParentRooted && rand.Float32() < inheritRootChance
}

// Count returns the current number of active spores.
func (s *SporeSystem) Count() int {
	return len(s.Spores)
}

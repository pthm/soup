package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
)

// Spore system constants
const (
	sporeMaxCount        = 200          // Maximum active spores
	sporeLifespan        = 600          // Ticks before spore expires
	sporeLandedGermTime  = 50           // Ticks after settling before germination
	sporeGerminateEnergy = float32(60)  // Initial energy for germinated flora
	sporeMaxVelocity     = float32(0.8) // Maximum spore drift speed
	sporeDrag            = float32(0.95)
	sporeNoiseScale      = 0.01
	sporeTimeScale       = 0.002
	sporeSettleChance    = float32(0.003) // Per-tick chance to settle
	sporeSettleMinVel    = float32(0.15)  // Max velocity to allow settling
)

// SporeEntity represents a spore in flight.
type SporeEntity struct {
	X, Y        float32
	VelX, VelY  float32
	Lifespan    int32
	LandedTimer int32 // Ticks since settling
	Settled     bool  // Whether spore has settled
}

// SporeSystem handles flora reproduction via spores.
type SporeSystem struct {
	Spores    []SporeEntity
	noise     *PerlinNoise
	bounds    Bounds
	maxSpores int
}

// NewSporeSystemWithTerrain creates a spore system (terrain param kept for compatibility).
func NewSporeSystemWithTerrain(bounds Bounds, terrain *TerrainSystem) *SporeSystem {
	return &SporeSystem{
		Spores:    make([]SporeEntity, 0, sporeMaxCount),
		noise:     NewPerlinNoise(rand.Int63()),
		bounds:    bounds,
		maxSpores: sporeMaxCount,
	}
}

// SpawnSpore creates a new spore from a parent flora.
func (s *SporeSystem) SpawnSpore(x, y float32) {
	if len(s.Spores) >= s.maxSpores {
		return
	}

	// Initial velocity: slight upward drift with random horizontal
	velX := (rand.Float32() - 0.5) * 0.4
	velY := -0.15 - rand.Float32()*0.25 // Upward

	s.Spores = append(s.Spores, SporeEntity{
		X:           x,
		Y:           y,
		VelX:        velX,
		VelY:        velY,
		Lifespan:    sporeLifespan,
		LandedTimer: 0,
		Settled:     false,
	})
}

// FloraCreator is called to create new flora when spores germinate.
// Takes position and initial energy.
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

		if spore.Settled {
			// Count down to germination
			spore.LandedTimer++
			if spore.LandedTimer >= sporeLandedGermTime {
				// Germinate into new flora (always floating now)
				createFlora(spore.X, spore.Y, false, sporeGerminateEnergy)
				continue // Remove spore after germination
			}
		} else {
			// Drift with Perlin noise
			s.updateDrift(spore, tick)

			// Check for settling (random chance when slow enough)
			velMag := float32(math.Sqrt(float64(spore.VelX*spore.VelX + spore.VelY*spore.VelY)))
			if velMag < sporeSettleMinVel && rand.Float32() < sporeSettleChance {
				spore.Settled = true
				spore.VelX = 0
				spore.VelY = 0
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
	spore.X += spore.VelX
	spore.Y += spore.VelY

	// Toroidal wrap-around (all edges)
	if spore.X < 0 {
		spore.X += s.bounds.Width
	}
	if spore.X > s.bounds.Width {
		spore.X -= s.bounds.Width
	}
	if spore.Y < 0 {
		spore.Y += s.bounds.Height
	}
	if spore.Y > s.bounds.Height {
		spore.Y -= s.bounds.Height
	}
}

// Count returns the current number of active spores.
func (s *SporeSystem) Count() int {
	return len(s.Spores)
}

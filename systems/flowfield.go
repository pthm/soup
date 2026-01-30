package systems

import (
	"math"
	"math/rand"
)

// FlowParticle represents a single flow visualization particle.
type FlowParticle struct {
	X, Y         float32
	VelX, VelY   float32
	Lifespan     int32
	MaxLifespan  int32
	Opacity      float32
	Size         float32
	// Trail history (most recent first)
	TrailX [8]float32
	TrailY [8]float32
	TrailLen uint8
}

// FlowSampler provides flow vectors at world positions.
// Implemented by GPUFlowField for GPU-accelerated flow.
type FlowSampler interface {
	Sample(worldX, worldY float32) (flowX, flowY float32)
}

// FlowFieldSystem manages flow particles for visualization.
type FlowFieldSystem struct {
	Particles      []FlowParticle
	noise          *PerlinNoise
	bounds         Bounds
	targetCount    int
	spawnRate      int
	terrain        *TerrainSystem
	updateInterval int32 // Update every N ticks (1 = every tick)
	batchIndex     int   // Current batch being updated
	batchCount     int   // Number of batches to split particles into
	gpuSampler     FlowSampler // Optional GPU-based flow sampler
}

// NewFlowFieldSystemWithTerrain creates a flow field system with terrain awareness.
func NewFlowFieldSystemWithTerrain(bounds Bounds, targetCount int, terrain *TerrainSystem) *FlowFieldSystem {
	return &FlowFieldSystem{
		Particles:      make([]FlowParticle, 0, targetCount),
		noise:          NewPerlinNoise(rand.Int63()),
		bounds:         bounds,
		targetCount:    targetCount,
		spawnRate:      50,
		terrain:        terrain,
		updateInterval: 2,  // Update every 2 ticks
		batchCount:     4,  // Process 1/4 of particles per update
		batchIndex:     0,
	}
}

// SetGPUSampler sets an optional GPU-based flow sampler.
// When set, flow forces are sampled from GPU-generated texture instead of CPU Perlin noise.
func (s *FlowFieldSystem) SetGPUSampler(sampler FlowSampler) {
	s.gpuSampler = sampler
}

// Update updates all flow particles using batch processing for performance.
func (s *FlowFieldSystem) Update(tick int32) {
	// Spawn new particles if below target (always do this)
	if len(s.Particles) < s.targetCount {
		for i := 0; i < s.spawnRate && len(s.Particles) < s.targetCount; i++ {
			lifespan := int32(800 + rand.Intn(600))
			x := rand.Float32() * s.bounds.Width
			y := rand.Float32() * s.bounds.Height

			// Reject positions inside terrain
			if s.terrain != nil && s.terrain.IsSolid(x, y) {
				continue
			}

			s.Particles = append(s.Particles, FlowParticle{
				X:           x,
				Y:           y,
				VelX:        0,
				VelY:        0,
				Lifespan:    lifespan,
				MaxLifespan: lifespan,
				Opacity:     0.15 + rand.Float32()*0.15,
				Size:        0.5 + rand.Float32()*0.3,
				TrailLen:    0,
			})
		}
	}

	// Determine which batch to update this tick
	// Each tick, we fully update 1/batchCount of the particles (flow force + physics)
	// Other particles just get position updates (cheap)
	currentBatch := s.batchIndex
	s.batchIndex = (s.batchIndex + 1) % s.batchCount

	// Update existing particles
	alive := 0
	for i := range s.Particles {
		p := &s.Particles[i]

		p.Lifespan--
		if p.Lifespan <= 0 {
			continue
		}

		// Determine if this particle is in the current batch for full update
		inCurrentBatch := (i % s.batchCount) == currentBatch

		// Shift trail history and add current position (always)
		for j := len(p.TrailX) - 1; j > 0; j-- {
			p.TrailX[j] = p.TrailX[j-1]
			p.TrailY[j] = p.TrailY[j-1]
		}
		p.TrailX[0] = p.X
		p.TrailY[0] = p.Y
		if p.TrailLen < uint8(len(p.TrailX)) {
			p.TrailLen++
		}

		// Only recalculate flow force for current batch (expensive)
		if inCurrentBatch {
			flowX, flowY := s.getFlowForce(p.X, p.Y, tick)
			p.VelX += flowX
			p.VelY += flowY
		}

		// Friction (always)
		p.VelX *= 0.95
		p.VelY *= 0.95

		// Limit velocity (always, but skip sqrt if clearly under limit)
		velSq := p.VelX*p.VelX + p.VelY*p.VelY
		if velSq > 2.25 { // 1.5^2
			velMag := float32(math.Sqrt(float64(velSq)))
			scale := 1.5 / velMag
			p.VelX *= scale
			p.VelY *= scale
		}

		// Update position (always)
		p.X += p.VelX
		p.Y += p.VelY

		// Terrain collision - only check for current batch (expensive)
		if inCurrentBatch && s.terrain != nil && s.terrain.IsSolid(p.X, p.Y) {
			// Move back
			p.X -= p.VelX
			p.Y -= p.VelY
			// Reflect velocity
			p.VelX *= -0.5
			p.VelY *= -0.5
			// Clear trail to avoid lines through terrain
			p.TrailLen = 0
		}

		// Wrap at edges (always, cheap)
		wrapped := false
		if p.X < 0 {
			p.X = s.bounds.Width
			wrapped = true
		}
		if p.X > s.bounds.Width {
			p.X = 0
			wrapped = true
		}
		if p.Y < 0 {
			p.Y = s.bounds.Height
			wrapped = true
		}
		if p.Y > s.bounds.Height {
			p.Y = 0
			wrapped = true
		}
		if wrapped {
			p.TrailLen = 0
		}

		// Keep alive particle
		s.Particles[alive] = s.Particles[i]
		alive++
	}
	s.Particles = s.Particles[:alive]
}

func (s *FlowFieldSystem) getFlowForce(x, y float32, tick int32) (float32, float32) {
	// Use GPU sampler if available (much faster - just a texture lookup)
	if s.gpuSampler != nil {
		return s.gpuSampler.Sample(x, y)
	}

	// Fallback to CPU Perlin noise calculation
	const flowScale = 0.003
	const timeScale = 0.0001 // Slowed down 5x
	const baseStrength = 0.08 // Reduced strength for gentler movement

	noiseX := s.noise.Noise3D(float64(x)*flowScale, float64(y)*flowScale, float64(tick)*timeScale)
	noiseY := s.noise.Noise3D(float64(x)*flowScale+100, float64(y)*flowScale+100, float64(tick)*timeScale)

	flowAngle := noiseX * math.Pi * 2
	flowMagnitude := (noiseY + 1) * 0.5
	flowX := float32(math.Cos(flowAngle) * flowMagnitude * baseStrength)
	flowY := float32(math.Sin(flowAngle) * flowMagnitude * baseStrength)

	// Add slight downward drift
	flowY += 0.01
	flowX += float32(math.Sin(float64(tick)*0.0002)) * 0.005

	// Deflect flow around terrain
	if s.terrain != nil {
		dist := s.terrain.DistanceToSolid(x, y)
		if dist < 40 { // Within influence range
			gradX, gradY := s.terrain.GetGradient(x, y)
			blend := 1.0 - dist/40.0
			flowX += gradX * blend * 0.1
			flowY += gradY * blend * 0.1
		}
	}

	return flowX, flowY
}

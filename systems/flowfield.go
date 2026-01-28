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

// FlowFieldSystem manages flow particles for visualization.
type FlowFieldSystem struct {
	Particles     []FlowParticle
	noise         *PerlinNoise
	bounds        Bounds
	targetCount   int
	spawnRate     int
}

// NewFlowFieldSystem creates a new flow field visualization system.
func NewFlowFieldSystem(bounds Bounds, targetCount int) *FlowFieldSystem {
	return &FlowFieldSystem{
		Particles:   make([]FlowParticle, 0, targetCount),
		noise:       NewPerlinNoise(rand.Int63()),
		bounds:      bounds,
		targetCount: targetCount,
		spawnRate:   50,
	}
}

// Update updates all flow particles.
func (s *FlowFieldSystem) Update(tick int32) {
	// Spawn new particles if below target
	if len(s.Particles) < s.targetCount {
		for i := 0; i < s.spawnRate && len(s.Particles) < s.targetCount; i++ {
			lifespan := int32(800 + rand.Intn(600))
			x := rand.Float32() * s.bounds.Width
			y := rand.Float32() * s.bounds.Height
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

	// Update existing particles
	alive := 0
	for i := range s.Particles {
		p := &s.Particles[i]

		p.Lifespan--
		if p.Lifespan <= 0 {
			continue
		}

		// Shift trail history and add current position
		for j := len(p.TrailX) - 1; j > 0; j-- {
			p.TrailX[j] = p.TrailX[j-1]
			p.TrailY[j] = p.TrailY[j-1]
		}
		p.TrailX[0] = p.X
		p.TrailY[0] = p.Y
		if p.TrailLen < uint8(len(p.TrailX)) {
			p.TrailLen++
		}

		// Apply flow field force
		flowX, flowY := s.getFlowForce(p.X, p.Y, tick)
		p.VelX += flowX
		p.VelY += flowY

		// Friction
		p.VelX *= 0.95
		p.VelY *= 0.95

		// Limit velocity
		velMag := float32(math.Sqrt(float64(p.VelX*p.VelX + p.VelY*p.VelY)))
		if velMag > 1.5 {
			scale := 1.5 / velMag
			p.VelX *= scale
			p.VelY *= scale
		}

		// Update position
		p.X += p.VelX
		p.Y += p.VelY

		// Wrap at edges (and clear trail to avoid long lines across screen)
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

	return flowX, flowY
}

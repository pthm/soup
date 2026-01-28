package systems

import (
	"math"
	"math/rand"
)

// ParticleType identifies the type of effect particle.
type ParticleType uint8

const (
	ParticleDisease ParticleType = iota
	ParticleDeath
	ParticleSplit
)

// EffectParticle represents a visual feedback particle.
type EffectParticle struct {
	X, Y       float32
	VelX, VelY float32
	Life       int32
	MaxLife    int32
	Type       ParticleType
	Size       float32
}

// ParticleSystem manages effect particles for visual feedback.
type ParticleSystem struct {
	Particles   []EffectParticle
	maxParticles int
}

// NewParticleSystem creates a new particle system.
func NewParticleSystem() *ParticleSystem {
	return &ParticleSystem{
		Particles:   make([]EffectParticle, 0, 500),
		maxParticles: 500,
	}
}

// Update processes all particles.
func (s *ParticleSystem) Update() {
	alive := 0
	for i := range s.Particles {
		p := &s.Particles[i]

		p.Life--
		if p.Life <= 0 {
			continue
		}

		// Apply physics based on type
		switch p.Type {
		case ParticleDisease:
			// Float upward
			p.VelY -= 0.01
		case ParticleDeath:
			// Sink downward
			p.VelY += 0.02
		case ParticleSplit:
			// Slight gravity
			p.VelY += 0.005
		}

		// Drag
		p.VelX *= 0.95
		p.VelY *= 0.95

		// Update position
		p.X += p.VelX
		p.Y += p.VelY

		// Keep particle
		s.Particles[alive] = s.Particles[i]
		alive++
	}
	s.Particles = s.Particles[:alive]
}

// EmitDisease emits a disease particle (15% chance per call).
func (s *ParticleSystem) EmitDisease(x, y float32) {
	if rand.Float32() > 0.15 {
		return
	}
	s.emit(x, y, ParticleDisease)
}

// EmitDeath emits a death particle (8% chance per call).
func (s *ParticleSystem) EmitDeath(x, y float32) {
	if rand.Float32() > 0.08 {
		return
	}
	s.emit(x, y, ParticleDeath)
}

// EmitSplit emits a burst of split particles (8-14 particles).
func (s *ParticleSystem) EmitSplit(x, y float32) {
	count := 8 + rand.Intn(7) // 8-14 particles
	for i := 0; i < count; i++ {
		s.emitSplitParticle(x, y)
	}
}

func (s *ParticleSystem) emit(x, y float32, ptype ParticleType) {
	if len(s.Particles) >= s.maxParticles {
		return
	}

	var velX, velY float32
	var life int32
	var size float32

	switch ptype {
	case ParticleDisease:
		velX = (rand.Float32() - 0.5) * 0.3
		velY = -rand.Float32() * 0.3 // Upward
		life = 60 + rand.Int31n(40)
		size = 1.5 + rand.Float32()
	case ParticleDeath:
		velX = (rand.Float32() - 0.5) * 0.2
		velY = rand.Float32() * 0.2 // Downward
		life = 80 + rand.Int31n(60)
		size = 2 + rand.Float32()
	default:
		velX = (rand.Float32() - 0.5) * 0.5
		velY = (rand.Float32() - 0.5) * 0.5
		life = 40 + rand.Int31n(30)
		size = 2 + rand.Float32()
	}

	s.Particles = append(s.Particles, EffectParticle{
		X:       x + (rand.Float32()-0.5)*4,
		Y:       y + (rand.Float32()-0.5)*4,
		VelX:    velX,
		VelY:    velY,
		Life:    life,
		MaxLife: life,
		Type:    ptype,
		Size:    size,
	})
}

func (s *ParticleSystem) emitSplitParticle(x, y float32) {
	if len(s.Particles) >= s.maxParticles {
		return
	}

	// Radial burst
	angle := rand.Float32() * 2 * math.Pi
	speed := 0.5 + rand.Float32()*0.8
	velX := float32(math.Cos(float64(angle))) * speed
	velY := float32(math.Sin(float64(angle))) * speed

	life := int32(30 + rand.Intn(30))

	s.Particles = append(s.Particles, EffectParticle{
		X:       x + (rand.Float32()-0.5)*6,
		Y:       y + (rand.Float32()-0.5)*6,
		VelX:    velX,
		VelY:    velY,
		Life:    life,
		MaxLife: life,
		Type:    ParticleSplit,
		Size:    2 + rand.Float32()*1.5,
	})
}

// Count returns the current number of active particles.
func (s *ParticleSystem) Count() int {
	return len(s.Particles)
}

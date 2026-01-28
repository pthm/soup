package renderer

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/systems"
)

// ParticleRenderer renders effect particles.
type ParticleRenderer struct{}

// NewParticleRenderer creates a new particle renderer.
func NewParticleRenderer() *ParticleRenderer {
	return &ParticleRenderer{}
}

// Draw renders all particles.
func (r *ParticleRenderer) Draw(particles []systems.EffectParticle) {
	for i := range particles {
		p := &particles[i]

		// Calculate life ratio for fade
		lifeRatio := float32(p.Life) / float32(p.MaxLife)

		// Get color based on type
		var color rl.Color
		switch p.Type {
		case systems.ParticleDisease:
			// Purple
			color = rl.Color{
				R: 150,
				G: 50,
				B: 150,
				A: uint8(lifeRatio * 180),
			}
		case systems.ParticleDeath:
			// Grey/brown
			color = rl.Color{
				R: 100,
				G: 80,
				B: 60,
				A: uint8(lifeRatio * 150),
			}
		case systems.ParticleSplit:
			// Orange
			color = rl.Color{
				R: 255,
				G: 150,
				B: 50,
				A: uint8(lifeRatio * 200),
			}
		}

		// Draw particle as circle
		size := p.Size * lifeRatio
		if size < 0.5 {
			size = 0.5
		}
		rl.DrawCircle(int32(p.X), int32(p.Y), size, color)
	}
}

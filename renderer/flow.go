// Package renderer provides rendering utilities.
package renderer

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/systems"
)

// FlowRenderer renders flow particles with trails.
type FlowRenderer struct {
	width  int32
	height int32
}

// NewFlowRenderer creates a new flow renderer.
// fadeAmount is kept for API compatibility but not used in direct rendering mode.
func NewFlowRenderer(width, height int32, fadeAmount float32) *FlowRenderer {
	return &FlowRenderer{
		width:  width,
		height: height,
	}
}

// Draw renders all flow particles directly to screen with additive blending.
func (r *FlowRenderer) Draw(particles []systems.FlowParticle, tick int32) {
	// Draw particle trails with additive blending
	rl.BeginBlendMode(rl.BlendAdditive)

	for i := range particles {
		p := &particles[i]

		// Need at least 1 trail point to draw
		if p.TrailLen < 1 {
			continue
		}

		lifeRatio := float32(p.Lifespan) / float32(p.MaxLifespan)

		// Fade in over first 20% of life (quadratic)
		fadeIn := float32(math.Min(float64(lifeRatio)*5, 1))
		fadeIn *= fadeIn

		// Gentle fade out at end
		fadeOut := float32(math.Min(float64(1-lifeRatio)*3+0.7, 1))

		// Pulse/shimmer effect
		timeOffset := p.X*0.01 + p.Y*0.01
		pulse := float32(math.Sin(float64(tick)*0.03+float64(timeOffset))*0.5 + 0.5)
		modulation := 0.3 + pulse*0.7

		baseAlpha := p.Opacity * fadeIn * fadeOut * float32(modulation) * 120

		if baseAlpha < 2 {
			continue
		}

		// Draw trail segments with fading alpha
		// Current position to first trail point
		trailFade := float32(1.0)
		color := rl.Color{
			R: 50,
			G: 100,
			B: 130,
			A: uint8(baseAlpha * trailFade),
		}
		rl.DrawLineEx(
			rl.Vector2{X: p.X, Y: p.Y},
			rl.Vector2{X: p.TrailX[0], Y: p.TrailY[0]},
			p.Size*2,
			color,
		)

		// Draw rest of trail with decreasing alpha
		for j := uint8(0); j < p.TrailLen-1; j++ {
			trailFade = 1.0 - float32(j+1)/float32(p.TrailLen)
			trailFade *= trailFade // Quadratic falloff

			alpha := baseAlpha * trailFade
			if alpha < 1 {
				continue
			}

			color := rl.Color{
				R: 50,
				G: 100,
				B: 130,
				A: uint8(alpha),
			}
			rl.DrawLineEx(
				rl.Vector2{X: p.TrailX[j], Y: p.TrailY[j]},
				rl.Vector2{X: p.TrailX[j+1], Y: p.TrailY[j+1]},
				p.Size*2*trailFade,
				color,
			)
		}
	}

	rl.EndBlendMode()
}

// Unload frees resources.
func (r *FlowRenderer) Unload() {
	// Nothing to unload in direct rendering mode
}

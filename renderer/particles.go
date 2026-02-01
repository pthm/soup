package renderer

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// ParticleRenderer renders floating particles with glow effects.
// Particles are drawn to a render texture, then a shader adds glow and twinkling.
type ParticleRenderer struct {
	shader        rl.Shader
	timeLoc       int32
	resolutionLoc int32

	renderTarget rl.RenderTexture2D
	width        int32
	height       int32
	initialized  bool

	// Particle rendering settings
	BaseColor  rl.Color // Base particle color (default: soft green)
	DotRadius  float32  // Radius of each particle dot
	GlowAlpha  uint8    // Alpha for the glow (lower = more subtle)
}

// NewParticleRenderer creates a new particle renderer.
func NewParticleRenderer(width, height int32) *ParticleRenderer {
	return &ParticleRenderer{
		width:     width,
		height:    height,
		BaseColor: rl.Color{R: 100, G: 220, B: 120, A: 255}, // Soft algae green
		DotRadius: 2.0,
		GlowAlpha: 60, // Fairly subtle
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (p *ParticleRenderer) Init() {
	if p.initialized {
		return
	}

	// Create render texture for particles
	p.renderTarget = rl.LoadRenderTexture(p.width, p.height)

	// Load glow shader
	p.shader = rl.LoadShader("", "shaders/particles.fs")
	p.timeLoc = rl.GetShaderLocation(p.shader, "time")
	p.resolutionLoc = rl.GetShaderLocation(p.shader, "resolution")

	// Set resolution uniform
	resolution := []float32{float32(p.width), float32(p.height)}
	rl.SetShaderValue(p.shader, p.resolutionLoc, resolution, rl.ShaderUniformVec2)

	p.initialized = true
}

// BeginParticles starts particle rendering. Call this before drawing particles.
func (p *ParticleRenderer) BeginParticles() {
	if !p.initialized {
		p.Init()
	}

	rl.BeginTextureMode(p.renderTarget)
	rl.ClearBackground(rl.Color{R: 0, G: 0, B: 0, A: 0}) // Transparent background
}

// DrawParticle draws a single particle at world position with given mass.
// The mass affects the brightness (higher mass = brighter).
// screenX, screenY should already be transformed to screen coordinates.
func (p *ParticleRenderer) DrawParticle(screenX, screenY, mass float32) {
	// Mass affects alpha (brightness)
	// Typical mass range is 0.001 to 0.05, scale to visible alpha
	alpha := mass * 2000 // Scale up small mass values
	if alpha > 1.0 {
		alpha = 1.0
	}
	if alpha < 0.1 {
		alpha = 0.1
	}

	color := p.BaseColor
	color.A = uint8(float32(p.GlowAlpha) * alpha)

	rl.DrawCircle(int32(screenX), int32(screenY), p.DotRadius, color)
}

// DrawParticleScaled draws a particle with camera zoom applied.
func (p *ParticleRenderer) DrawParticleScaled(screenX, screenY, mass, zoom float32) {
	alpha := mass * 2000
	if alpha > 1.0 {
		alpha = 1.0
	}
	if alpha < 0.1 {
		alpha = 0.1
	}

	color := p.BaseColor
	color.A = uint8(float32(p.GlowAlpha) * alpha)

	radius := p.DotRadius * zoom
	if radius < 1.0 {
		radius = 1.0
	}
	if radius > 4.0 {
		radius = 4.0
	}

	rl.DrawCircle(int32(screenX), int32(screenY), radius, color)
}

// EndParticles finishes particle rendering to the texture.
func (p *ParticleRenderer) EndParticles() {
	rl.EndTextureMode()
}

// Draw renders the particle texture with glow shader to the screen.
// Uses additive blending to create the glowing effect.
func (p *ParticleRenderer) Draw(time float32) {
	if !p.initialized {
		return
	}

	// Update time uniform for twinkling
	rl.SetShaderValue(p.shader, p.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Enable additive blending for glow effect
	rl.BeginBlendMode(rl.BlendAdditive)

	// Draw the render texture with the glow shader
	rl.BeginShaderMode(p.shader)

	// Render textures are flipped vertically in OpenGL, so we draw with negative height
	srcRect := rl.Rectangle{X: 0, Y: 0, Width: float32(p.width), Height: -float32(p.height)}
	dstRect := rl.Rectangle{X: 0, Y: 0, Width: float32(p.width), Height: float32(p.height)}
	rl.DrawTexturePro(p.renderTarget.Texture, srcRect, dstRect, rl.Vector2{}, 0, rl.White)

	rl.EndShaderMode()
	rl.EndBlendMode()
}

// Unload frees resources.
func (p *ParticleRenderer) Unload() {
	if p.initialized {
		rl.UnloadShader(p.shader)
		rl.UnloadRenderTexture(p.renderTarget)
		p.initialized = false
	}
}

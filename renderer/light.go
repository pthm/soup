package renderer

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// LightRenderer renders the potential field as dappled caustic sunlight.
// Uses GPU double-buffering for smooth transitions between potential field updates.
// Renders in two passes: shadow (darkening) then caustics (additive).
type LightRenderer struct {
	// Caustics shader (additive pass)
	shader        rl.Shader
	timeLoc       int32
	resolutionLoc int32
	cameraPosLoc  int32
	cameraZoomLoc int32
	worldSizeLoc  int32
	blendLoc      int32
	prevTexLoc    int32

	// Shadow shader (alpha blend pass for darkening)
	shadowShader        rl.Shader
	shadowResolutionLoc int32
	shadowCameraPosLoc  int32
	shadowCameraZoomLoc int32
	shadowWorldSizeLoc  int32
	shadowBlendLoc      int32
	shadowPrevTexLoc    int32
	shadowStrengthLoc   int32

	// Double-buffered textures for smooth GPU blending
	potentialTex [2]rl.Texture2D
	currentIdx   int // Index of current (newest) texture
	texW, texH   int

	// Blend state
	blendProgress float32 // 0 = showing previous, 1 = showing current
	blendSpeed    float32 // Units per second
	firstUpdate   bool    // Track first update to initialize both textures

	// Shadow settings
	shadowStrength float32 // How dark the shadows get (0-1)

	screenW, screenH float32
	initialized      bool
}

// NewLightRenderer creates a new light renderer.
// updateIntervalSec is the potential field update interval in seconds;
// the blend speed is derived from this to ensure smooth transitions.
func NewLightRenderer(screenW, screenH int32, updateIntervalSec float32) *LightRenderer {
	blendSpeed := float32(1.0)
	if updateIntervalSec > 0 {
		blendSpeed = 1.0 / updateIntervalSec // Blend completes exactly when next update arrives
	}
	return &LightRenderer{
		screenW:        float32(screenW),
		screenH:        float32(screenH),
		blendSpeed:     blendSpeed,
		shadowStrength: 0.35, // Moderate shadow darkness
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (l *LightRenderer) Init(potW, potH int) {
	if l.initialized {
		return
	}

	l.texW = potW
	l.texH = potH

	// Create two textures for double-buffered blending
	img := rl.GenImageColor(potW, potH, rl.Black)
	for i := 0; i < 2; i++ {
		l.potentialTex[i] = rl.LoadTextureFromImage(img)
		rl.SetTextureFilter(l.potentialTex[i], rl.FilterBilinear)
		rl.SetTextureWrap(l.potentialTex[i], rl.WrapRepeat)
	}
	rl.UnloadImage(img)

	l.currentIdx = 0
	l.blendProgress = 1.0 // Start fully blended
	l.firstUpdate = true

	// Load caustics shader
	l.shader = rl.LoadShader("", "shaders/light.fs")
	l.timeLoc = rl.GetShaderLocation(l.shader, "time")
	l.resolutionLoc = rl.GetShaderLocation(l.shader, "resolution")
	l.cameraPosLoc = rl.GetShaderLocation(l.shader, "cameraPos")
	l.cameraZoomLoc = rl.GetShaderLocation(l.shader, "cameraZoom")
	l.worldSizeLoc = rl.GetShaderLocation(l.shader, "worldSize")
	l.blendLoc = rl.GetShaderLocation(l.shader, "blend")
	l.prevTexLoc = rl.GetShaderLocation(l.shader, "texture1")

	// Load shadow shader
	l.shadowShader = rl.LoadShader("", "shaders/light_shadow.fs")
	l.shadowResolutionLoc = rl.GetShaderLocation(l.shadowShader, "resolution")
	l.shadowCameraPosLoc = rl.GetShaderLocation(l.shadowShader, "cameraPos")
	l.shadowCameraZoomLoc = rl.GetShaderLocation(l.shadowShader, "cameraZoom")
	l.shadowWorldSizeLoc = rl.GetShaderLocation(l.shadowShader, "worldSize")
	l.shadowBlendLoc = rl.GetShaderLocation(l.shadowShader, "blend")
	l.shadowPrevTexLoc = rl.GetShaderLocation(l.shadowShader, "texture1")
	l.shadowStrengthLoc = rl.GetShaderLocation(l.shadowShader, "shadowStrength")

	// Set static uniforms
	resolution := []float32{l.screenW, l.screenH}
	rl.SetShaderValue(l.shader, l.resolutionLoc, resolution, rl.ShaderUniformVec2)
	rl.SetShaderValue(l.shadowShader, l.shadowResolutionLoc, resolution, rl.ShaderUniformVec2)

	l.initialized = true
}

// Resize updates screen dimensions for the light shaders.
func (l *LightRenderer) Resize(w, h float32) {
	if w == l.screenW && h == l.screenH {
		return
	}
	l.screenW = w
	l.screenH = h
	if l.initialized {
		resolution := []float32{w, h}
		rl.SetShaderValue(l.shader, l.resolutionLoc, resolution, rl.ShaderUniformVec2)
		rl.SetShaderValue(l.shadowShader, l.shadowResolutionLoc, resolution, rl.ShaderUniformVec2)
	}
}

// UpdatePotential uploads new potential field data to the GPU texture.
// Swaps buffers and starts blending from previous to new.
func (l *LightRenderer) UpdatePotential(data []float32, w, h int) {
	if !l.initialized {
		l.Init(w, h)
	}

	// Convert data to pixels
	pixels := make([]color.RGBA, len(data))
	for i, val := range data {
		if val < 0 {
			val = 0
		}
		if val > 1 {
			val = 1
		}
		gray := uint8(val * 255)
		pixels[i] = color.RGBA{R: gray, G: gray, B: gray, A: 255}
	}

	if l.firstUpdate {
		// First update: initialize both textures to avoid blending from black
		rl.UpdateTexture(l.potentialTex[0], pixels)
		rl.UpdateTexture(l.potentialTex[1], pixels)
		l.blendProgress = 1.0 // No transition needed
		l.firstUpdate = false
	} else {
		// Swap to the other texture buffer
		l.currentIdx = 1 - l.currentIdx
		// Reset blend to start transition
		l.blendProgress = 0.0
		// Upload to current texture
		rl.UpdateTexture(l.potentialTex[l.currentIdx], pixels)
	}
}

// Update advances the blend progress. Call once per frame before drawing.
func (l *LightRenderer) Update(dt float32) {
	if !l.initialized {
		return
	}

	if l.blendProgress < 1.0 {
		l.blendProgress += dt * l.blendSpeed
		if l.blendProgress > 1.0 {
			l.blendProgress = 1.0
		}
	}
}

// DrawShadow renders the shadow layer (darkening low-potential areas).
// Call this early in the render order, before particles and entities.
func (l *LightRenderer) DrawShadow(cameraX, cameraY, cameraZoom, worldW, worldH float32) {
	if !l.initialized {
		return
	}

	prevIdx := 1 - l.currentIdx
	srcRect := rl.Rectangle{X: 0, Y: 0, Width: float32(l.texW), Height: float32(l.texH)}
	dstRect := rl.Rectangle{X: 0, Y: 0, Width: l.screenW, Height: l.screenH}

	rl.BeginBlendMode(rl.BlendAlpha)
	rl.BeginShaderMode(l.shadowShader)

	rl.SetShaderValue(l.shadowShader, l.shadowCameraPosLoc, []float32{cameraX, cameraY}, rl.ShaderUniformVec2)
	rl.SetShaderValue(l.shadowShader, l.shadowCameraZoomLoc, []float32{cameraZoom}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shadowShader, l.shadowWorldSizeLoc, []float32{worldW, worldH}, rl.ShaderUniformVec2)
	rl.SetShaderValue(l.shadowShader, l.shadowBlendLoc, []float32{l.blendProgress}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shadowShader, l.shadowStrengthLoc, []float32{l.shadowStrength}, rl.ShaderUniformFloat)
	rl.SetShaderValueTexture(l.shadowShader, l.shadowPrevTexLoc, l.potentialTex[prevIdx])

	rl.DrawTexturePro(l.potentialTex[l.currentIdx], srcRect, dstRect, rl.Vector2{}, 0, rl.White)

	rl.EndShaderMode()
	rl.EndBlendMode()
}

// DrawCaustics renders the caustic light layer (additive glow).
// Call this late in the render order, after particles and entities.
func (l *LightRenderer) DrawCaustics(time float32, cameraX, cameraY, cameraZoom, worldW, worldH float32) {
	if !l.initialized {
		return
	}

	prevIdx := 1 - l.currentIdx
	srcRect := rl.Rectangle{X: 0, Y: 0, Width: float32(l.texW), Height: float32(l.texH)}
	dstRect := rl.Rectangle{X: 0, Y: 0, Width: l.screenW, Height: l.screenH}

	rl.BeginBlendMode(rl.BlendAdditive)
	rl.BeginShaderMode(l.shader)

	rl.SetShaderValue(l.shader, l.timeLoc, []float32{time}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shader, l.cameraPosLoc, []float32{cameraX, cameraY}, rl.ShaderUniformVec2)
	rl.SetShaderValue(l.shader, l.cameraZoomLoc, []float32{cameraZoom}, rl.ShaderUniformFloat)
	rl.SetShaderValue(l.shader, l.worldSizeLoc, []float32{worldW, worldH}, rl.ShaderUniformVec2)
	rl.SetShaderValue(l.shader, l.blendLoc, []float32{l.blendProgress}, rl.ShaderUniformFloat)
	rl.SetShaderValueTexture(l.shader, l.prevTexLoc, l.potentialTex[prevIdx])

	rl.DrawTexturePro(l.potentialTex[l.currentIdx], srcRect, dstRect, rl.Vector2{}, 0, rl.White)

	rl.EndShaderMode()
	rl.EndBlendMode()
}

// Unload frees resources.
func (l *LightRenderer) Unload() {
	if l.initialized {
		rl.UnloadShader(l.shader)
		rl.UnloadShader(l.shadowShader)
		for i := 0; i < 2; i++ {
			rl.UnloadTexture(l.potentialTex[i])
		}
		l.initialized = false
	}
}

package renderer

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// LightRenderer renders the potential field as dappled caustic sunlight.
// Uses CPU-side interpolation for smooth transitions when the potential field updates.
type LightRenderer struct {
	shader        rl.Shader
	timeLoc       int32
	resolutionLoc int32

	potentialTex rl.Texture2D
	texW, texH   int

	// Double-buffered data for smooth CPU-side blending
	current  []float32 // Target values
	display  []float32 // Currently displayed (interpolated)
	blending bool

	screenW, screenH float32
	initialized      bool
}

// NewLightRenderer creates a new light renderer.
func NewLightRenderer(screenW, screenH int32) *LightRenderer {
	return &LightRenderer{
		screenW: float32(screenW),
		screenH: float32(screenH),
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (l *LightRenderer) Init(potW, potH int) {
	if l.initialized {
		return
	}

	l.texW = potW
	l.texH = potH

	// Create texture for potential field
	img := rl.GenImageColor(potW, potH, rl.Black)
	l.potentialTex = rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)

	// Set texture filtering for smooth interpolation
	rl.SetTextureFilter(l.potentialTex, rl.FilterBilinear)
	rl.SetTextureWrap(l.potentialTex, rl.WrapRepeat)

	// Allocate blend buffers
	size := potW * potH
	l.current = make([]float32, size)
	l.display = make([]float32, size)

	// Load light shader
	l.shader = rl.LoadShader("", "shaders/light.fs")
	l.timeLoc = rl.GetShaderLocation(l.shader, "time")
	l.resolutionLoc = rl.GetShaderLocation(l.shader, "resolution")

	// Set resolution uniform
	resolution := []float32{l.screenW, l.screenH}
	rl.SetShaderValue(l.shader, l.resolutionLoc, resolution, rl.ShaderUniformVec2)

	l.initialized = true
}

// UpdatePotential sets new target potential field data.
func (l *LightRenderer) UpdatePotential(data []float32, w, h int) {
	if !l.initialized {
		l.Init(w, h)
	}

	// Copy new data to current buffer
	copy(l.current, data)
	l.blending = true
}

// Draw renders the light background, blending toward current values.
func (l *LightRenderer) Draw(time, dt float32) {
	if !l.initialized {
		return
	}

	// Blend display toward current (exponential smoothing)
	if l.blending {
		blendRate := float32(3.0) * dt // Smooth over ~0.3 seconds
		if blendRate > 1.0 {
			blendRate = 1.0
		}

		allDone := true
		for i := range l.display {
			diff := l.current[i] - l.display[i]
			if diff > 0.001 || diff < -0.001 {
				l.display[i] += diff * blendRate
				allDone = false
			} else {
				l.display[i] = l.current[i]
			}
		}
		if allDone {
			l.blending = false
		}

		// Upload blended values to texture
		l.uploadTexture()
	}

	// Update time uniform
	rl.SetShaderValue(l.shader, l.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Draw fullscreen quad with potential texture and shader
	rl.BeginShaderMode(l.shader)

	srcRect := rl.Rectangle{X: 0, Y: 0, Width: float32(l.texW), Height: float32(l.texH)}
	dstRect := rl.Rectangle{X: 0, Y: 0, Width: l.screenW, Height: l.screenH}
	rl.DrawTexturePro(l.potentialTex, srcRect, dstRect, rl.Vector2{}, 0, rl.White)

	rl.EndShaderMode()
}

// uploadTexture converts display buffer to pixels and uploads.
func (l *LightRenderer) uploadTexture() {
	pixels := make([]color.RGBA, len(l.display))
	for i, val := range l.display {
		if val < 0 {
			val = 0
		}
		if val > 1 {
			val = 1
		}
		gray := uint8(val * 255)
		pixels[i] = color.RGBA{R: gray, G: gray, B: gray, A: 255}
	}
	rl.UpdateTexture(l.potentialTex, pixels)
}

// Unload frees resources.
func (l *LightRenderer) Unload() {
	if l.initialized {
		rl.UnloadShader(l.shader)
		rl.UnloadTexture(l.potentialTex)
		l.initialized = false
	}
}

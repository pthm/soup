package renderer

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// WaterBackground renders an animated Perlin noise water background.
type WaterBackground struct {
	shader        rl.Shader
	timeLoc       int32
	resolutionLoc int32
	width         float32
	height        float32
	initialized   bool
}

// NewWaterBackground creates a new water background renderer.
func NewWaterBackground(width, height int32) *WaterBackground {
	return &WaterBackground{
		width:  float32(width),
		height: float32(height),
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (w *WaterBackground) Init() {
	if w.initialized {
		return
	}

	// Load water shader
	w.shader = rl.LoadShader("", "shaders/water.fs")
	w.timeLoc = rl.GetShaderLocation(w.shader, "time")
	w.resolutionLoc = rl.GetShaderLocation(w.shader, "resolution")

	// Set resolution uniform
	resolution := []float32{w.width, w.height}
	rl.SetShaderValue(w.shader, w.resolutionLoc, resolution, rl.ShaderUniformVec2)

	w.initialized = true
}

// Draw renders the animated water background.
func (w *WaterBackground) Draw(time float32) {
	if !w.initialized {
		w.Init()
	}

	// Update time uniform
	rl.SetShaderValue(w.shader, w.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Draw fullscreen quad with shader
	rl.BeginShaderMode(w.shader)
	rl.DrawRectangle(0, 0, int32(w.width), int32(w.height), rl.White)
	rl.EndShaderMode()
}

// Unload frees resources.
func (w *WaterBackground) Unload() {
	if w.initialized {
		rl.UnloadShader(w.shader)
		w.initialized = false
	}
}

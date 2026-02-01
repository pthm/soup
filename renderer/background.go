package renderer

import rl "github.com/gen2brain/raylib-go/raylib"

// BackgroundRenderer renders a soft simplex noise texture as the background.
type BackgroundRenderer struct {
	shader        rl.Shader
	timeLoc       int32
	resolutionLoc int32
	cameraPosLoc  int32
	cameraZoomLoc int32
	worldSizeLoc  int32
	baseColorLoc  int32

	screenW, screenH float32
	baseColor        [3]float32
	initialized      bool
}

// NewBackgroundRenderer creates a new background renderer.
func NewBackgroundRenderer(screenW, screenH int32, baseR, baseG, baseB uint8) *BackgroundRenderer {
	return &BackgroundRenderer{
		screenW: float32(screenW),
		screenH: float32(screenH),
		baseColor: [3]float32{
			float32(baseR) / 255.0,
			float32(baseG) / 255.0,
			float32(baseB) / 255.0,
		},
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (b *BackgroundRenderer) Init() {
	if b.initialized {
		return
	}

	b.shader = rl.LoadShader("", "shaders/background.fs")
	b.timeLoc = rl.GetShaderLocation(b.shader, "time")
	b.resolutionLoc = rl.GetShaderLocation(b.shader, "resolution")
	b.cameraPosLoc = rl.GetShaderLocation(b.shader, "cameraPos")
	b.cameraZoomLoc = rl.GetShaderLocation(b.shader, "cameraZoom")
	b.worldSizeLoc = rl.GetShaderLocation(b.shader, "worldSize")
	b.baseColorLoc = rl.GetShaderLocation(b.shader, "baseColor")

	// Set static uniforms
	resolution := []float32{b.screenW, b.screenH}
	rl.SetShaderValue(b.shader, b.resolutionLoc, resolution, rl.ShaderUniformVec2)
	rl.SetShaderValue(b.shader, b.baseColorLoc, b.baseColor[:], rl.ShaderUniformVec3)

	b.initialized = true
}

// Draw renders the background with noise texture.
func (b *BackgroundRenderer) Draw(time float32, cameraX, cameraY, cameraZoom, worldW, worldH float32) {
	if !b.initialized {
		b.Init()
	}

	rl.BeginShaderMode(b.shader)

	rl.SetShaderValue(b.shader, b.timeLoc, []float32{time}, rl.ShaderUniformFloat)
	rl.SetShaderValue(b.shader, b.cameraPosLoc, []float32{cameraX, cameraY}, rl.ShaderUniformVec2)
	rl.SetShaderValue(b.shader, b.cameraZoomLoc, []float32{cameraZoom}, rl.ShaderUniformFloat)
	rl.SetShaderValue(b.shader, b.worldSizeLoc, []float32{worldW, worldH}, rl.ShaderUniformVec2)

	// Draw fullscreen quad
	rl.DrawRectangle(0, 0, int32(b.screenW), int32(b.screenH), rl.White)

	rl.EndShaderMode()
}

// Unload frees resources.
func (b *BackgroundRenderer) Unload() {
	if b.initialized {
		rl.UnloadShader(b.shader)
		b.initialized = false
	}
}

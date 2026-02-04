package renderer

import (
	"image/color"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// ResourceFogRenderer renders the resource field as a soft green fog.
// Uses a shader for organic-looking algae coloring with texture.
type ResourceFogRenderer struct {
	shader        rl.Shader
	resolutionLoc int32
	cameraPosLoc  int32
	cameraZoomLoc int32
	worldSizeLoc  int32
	timeLoc       int32

	// Resource texture
	resourceTex rl.Texture2D
	texW, texH  int

	screenW, screenH float32
	initialized      bool
}

// NewResourceFogRenderer creates a new resource fog renderer.
func NewResourceFogRenderer(screenW, screenH int32) *ResourceFogRenderer {
	return &ResourceFogRenderer{
		screenW: float32(screenW),
		screenH: float32(screenH),
	}
}

// Init initializes the renderer (must be called after raylib window is created).
func (r *ResourceFogRenderer) Init(gridW, gridH int) {
	if r.initialized {
		return
	}

	r.texW = gridW
	r.texH = gridH

	// Create resource texture
	img := rl.GenImageColor(gridW, gridH, rl.Black)
	r.resourceTex = rl.LoadTextureFromImage(img)
	rl.SetTextureFilter(r.resourceTex, rl.FilterBilinear)
	rl.SetTextureWrap(r.resourceTex, rl.WrapRepeat)
	rl.UnloadImage(img)

	// Load shader
	r.shader = rl.LoadShader("", "shaders/resource_fog.fs")

	// Get uniform locations
	r.resolutionLoc = rl.GetShaderLocation(r.shader, "resolution")
	r.cameraPosLoc = rl.GetShaderLocation(r.shader, "cameraPos")
	r.cameraZoomLoc = rl.GetShaderLocation(r.shader, "cameraZoom")
	r.worldSizeLoc = rl.GetShaderLocation(r.shader, "worldSize")
	r.timeLoc = rl.GetShaderLocation(r.shader, "time")

	r.initialized = true
}

// Resize updates screen dimensions for the resource fog renderer.
func (r *ResourceFogRenderer) Resize(w, h float32) {
	if w == r.screenW && h == r.screenH {
		return
	}
	r.screenW = w
	r.screenH = h
}

// UpdateResource uploads new resource field data to the GPU texture.
func (r *ResourceFogRenderer) UpdateResource(data []float32, w, h int) {
	if !r.initialized {
		r.Init(w, h)
	}
	if len(data) != w*h {
		return
	}

	// Convert float32 resource values to RGBA pixels (store in R channel)
	pixels := make([]color.RGBA, w*h)
	for i, val := range data {
		// Clamp and convert to 0-255
		v := val
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		pixels[i] = color.RGBA{R: uint8(v * 255), G: 0, B: 0, A: 255}
	}

	rl.UpdateTexture(r.resourceTex, pixels)
}

// Draw renders the resource fog layer.
func (r *ResourceFogRenderer) Draw(time float32, cameraX, cameraY, cameraZoom, worldW, worldH float32) {
	if !r.initialized {
		return
	}

	// Set shader uniforms
	resolution := []float32{r.screenW, r.screenH}
	rl.SetShaderValue(r.shader, r.resolutionLoc, resolution, rl.ShaderUniformVec2)

	cameraPos := []float32{cameraX, cameraY}
	rl.SetShaderValue(r.shader, r.cameraPosLoc, cameraPos, rl.ShaderUniformVec2)

	rl.SetShaderValue(r.shader, r.cameraZoomLoc, []float32{cameraZoom}, rl.ShaderUniformFloat)

	worldSize := []float32{worldW, worldH}
	rl.SetShaderValue(r.shader, r.worldSizeLoc, worldSize, rl.ShaderUniformVec2)

	rl.SetShaderValue(r.shader, r.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Calculate destination rect to fill screen
	srcRect := rl.Rectangle{X: 0, Y: 0, Width: float32(r.texW), Height: float32(r.texH)}
	dstRect := rl.Rectangle{X: 0, Y: 0, Width: r.screenW, Height: r.screenH}

	// Draw with alpha blending
	rl.BeginShaderMode(r.shader)
	rl.DrawTexturePro(r.resourceTex, srcRect, dstRect, rl.Vector2{}, 0, rl.White)
	rl.EndShaderMode()
}

// Unload frees GPU resources.
func (r *ResourceFogRenderer) Unload() {
	if !r.initialized {
		return
	}
	rl.UnloadShader(r.shader)
	rl.UnloadTexture(r.resourceTex)
	r.initialized = false
}

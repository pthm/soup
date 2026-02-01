package renderer

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/config"
)

// GPUFlowField generates flow field vectors on the GPU and caches them for CPU sampling.
// This provides a unified flow field for the entire scene that flora, fauna, and
// particles can all sample from.
type GPUFlowField struct {
	shader        rl.Shader
	flowTarget    rl.RenderTexture2D
	timeLoc       int32
	resolutionLoc int32

	// Cached flow data for CPU sampling
	flowData     []float32 // [x0, y0, x1, y1, ...] interleaved
	width        int
	height       int
	screenWidth  float32
	screenHeight float32

	// Update tracking
	lastUpdate int32
}

// NewGPUFlowField creates a GPU-accelerated flow field generator.
func NewGPUFlowField(screenWidth, screenHeight float32) *GPUFlowField {
	cfg := config.Cfg().GPU
	textureSize := cfg.FlowTextureSize

	gf := &GPUFlowField{
		width:        textureSize,
		height:       textureSize,
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
		flowData:     make([]float32, textureSize*textureSize*2),
	}

	// Load the flow field shader
	gf.shader = rl.LoadShader("", "shaders/flowfield.fs")
	gf.timeLoc = rl.GetShaderLocation(gf.shader, "time")
	gf.resolutionLoc = rl.GetShaderLocation(gf.shader, "resolution")

	// Set resolution uniform
	resolution := []float32{screenWidth, screenHeight}
	rl.SetShaderValue(gf.shader, gf.resolutionLoc, resolution, rl.ShaderUniformVec2)

	// Create render target for flow field
	gf.flowTarget = rl.LoadRenderTexture(int32(textureSize), int32(textureSize))

	return gf
}

// Update regenerates the flow field texture if needed and reads it back to CPU.
func (gf *GPUFlowField) Update(tick int32, time float32) {
	// Only update periodically
	updateInterval := int32(config.Cfg().GPU.FlowUpdateInterval)
	if tick-gf.lastUpdate < updateInterval {
		return
	}
	gf.lastUpdate = tick

	// Render flow field to texture
	rl.BeginTextureMode(gf.flowTarget)
	rl.ClearBackground(rl.Black)

	// Set time uniform
	rl.SetShaderValue(gf.shader, gf.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Draw fullscreen quad with shader
	rl.BeginShaderMode(gf.shader)
	rl.DrawRectangle(0, 0, int32(gf.width), int32(gf.height), rl.White)
	rl.EndShaderMode()

	rl.EndTextureMode()

	// Read back texture to CPU for fast sampling
	gf.readbackFlowData()
}

// readbackFlowData copies the flow texture to CPU memory.
func (gf *GPUFlowField) readbackFlowData() {
	// Get image from render texture
	img := rl.LoadImageFromTexture(gf.flowTarget.Texture)
	defer rl.UnloadImage(img)

	// Extract pixel data
	colors := rl.LoadImageColors(img)
	defer rl.UnloadImageColors(colors)

	// Decode flow vectors from RGBA
	for i := 0; i < gf.width*gf.height; i++ {
		c := colors[i]
		// Decode: R,G store flow values mapped from [-0.5, 0.5] to [0, 255]
		flowX := (float32(c.R)/255.0 - 0.5)
		flowY := (float32(c.G)/255.0 - 0.5)
		gf.flowData[i*2] = flowX
		gf.flowData[i*2+1] = flowY
	}
}

// Sample returns the flow vector at a world position.
// This is a fast O(1) lookup into the cached flow data.
func (gf *GPUFlowField) Sample(worldX, worldY float32) (float32, float32) {
	// Map world coords to texture coords
	texX := int(worldX / gf.screenWidth * float32(gf.width))
	texY := int(worldY / gf.screenHeight * float32(gf.height))

	// Wrap around edges (toroidal)
	texX = ((texX % gf.width) + gf.width) % gf.width
	texY = ((texY % gf.height) + gf.height) % gf.height

	idx := (texY*gf.width + texX) * 2
	return gf.flowData[idx], gf.flowData[idx+1]
}

// Unload releases GPU resources.
func (gf *GPUFlowField) Unload() {
	rl.UnloadShader(gf.shader)
	rl.UnloadRenderTexture(gf.flowTarget)
}

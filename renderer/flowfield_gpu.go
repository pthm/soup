package renderer

import (
	"unsafe"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// FlowTextureSize is the resolution of the flow field texture
	FlowTextureSize = 128
	// FlowUpdateInterval is how often to regenerate the flow texture (in ticks)
	FlowUpdateInterval = 60
)

// GPUFlowField generates flow field vectors on the GPU and caches them for CPU sampling.
type GPUFlowField struct {
	shader       rl.Shader
	terrainTex   rl.Texture2D
	flowTarget   rl.RenderTexture2D
	timeLoc      int32
	resolutionLoc int32
	terrainLoc   int32

	// Cached flow data for CPU sampling
	flowData     []float32 // [x0, y0, x1, y1, ...] interleaved
	width        int
	height       int
	screenWidth  float32
	screenHeight float32

	// Update tracking
	lastUpdate   int32
	needsReadback bool
}

// NewGPUFlowField creates a GPU-accelerated flow field generator.
func NewGPUFlowField(screenWidth, screenHeight float32, terrainDistanceFunc func(x, y float32) float32) *GPUFlowField {
	gf := &GPUFlowField{
		width:        FlowTextureSize,
		height:       FlowTextureSize,
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
		flowData:     make([]float32, FlowTextureSize*FlowTextureSize*2),
	}

	// Load the flow field shader
	gf.shader = rl.LoadShader("", "shaders/flowfield.fs")
	gf.timeLoc = rl.GetShaderLocation(gf.shader, "time")
	gf.resolutionLoc = rl.GetShaderLocation(gf.shader, "resolution")
	gf.terrainLoc = rl.GetShaderLocation(gf.shader, "terrainTex")

	// Set resolution uniform
	resolution := []float32{screenWidth, screenHeight}
	rl.SetShaderValue(gf.shader, gf.resolutionLoc, resolution, rl.ShaderUniformVec2)

	// Create render target for flow field
	gf.flowTarget = rl.LoadRenderTexture(int32(FlowTextureSize), int32(FlowTextureSize))

	// Generate terrain distance texture
	gf.generateTerrainTexture(terrainDistanceFunc)

	return gf
}

// generateTerrainTexture creates a texture encoding terrain distances.
func (gf *GPUFlowField) generateTerrainTexture(distanceFunc func(x, y float32) float32) {
	// Create image data
	pixels := make([]uint8, FlowTextureSize*FlowTextureSize*4) // RGBA

	for y := 0; y < FlowTextureSize; y++ {
		for x := 0; x < FlowTextureSize; x++ {
			// Map texture coords to world coords
			worldX := float32(x) / float32(FlowTextureSize) * gf.screenWidth
			worldY := float32(y) / float32(FlowTextureSize) * gf.screenHeight

			// Get distance to terrain (0 = inside, positive = distance)
			dist := distanceFunc(worldX, worldY)

			// Normalize: 0 = solid/touching, 1 = far (100+ pixels away)
			normalizedDist := dist / 100.0
			if normalizedDist > 1.0 {
				normalizedDist = 1.0
			}
			if normalizedDist < 0.0 {
				normalizedDist = 0.0
			}

			idx := (y*FlowTextureSize + x) * 4
			pixels[idx+0] = uint8(normalizedDist * 255) // R = distance
			pixels[idx+1] = 0                            // G unused
			pixels[idx+2] = 0                            // B unused
			pixels[idx+3] = 255                          // A = opaque
		}
	}

	// Create texture from pixel data
	img := rl.Image{
		Data:    unsafe.Pointer(&pixels[0]),
		Width:   int32(FlowTextureSize),
		Height:  int32(FlowTextureSize),
		Mipmaps: 1,
		Format:  rl.UncompressedR8g8b8a8,
	}
	gf.terrainTex = rl.LoadTextureFromImage(&img)
}

// Update regenerates the flow field texture if needed and reads it back to CPU.
func (gf *GPUFlowField) Update(tick int32, time float32) {
	// Only update periodically
	if tick-gf.lastUpdate < FlowUpdateInterval {
		return
	}
	gf.lastUpdate = tick

	// Render flow field to texture
	rl.BeginTextureMode(gf.flowTarget)
	rl.ClearBackground(rl.Black)

	// Set uniforms
	rl.SetShaderValue(gf.shader, gf.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Bind terrain texture
	rl.SetShaderValueTexture(gf.shader, gf.terrainLoc, gf.terrainTex)

	// Draw fullscreen quad with shader
	rl.BeginShaderMode(gf.shader)
	rl.DrawRectangle(0, 0, int32(FlowTextureSize), int32(FlowTextureSize), rl.White)
	rl.EndShaderMode()

	rl.EndTextureMode()

	// Read back texture to CPU
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
	for i := 0; i < FlowTextureSize*FlowTextureSize; i++ {
		c := colors[i]
		// Decode: R,G stored flow values mapped from [-0.5, 0.5] to [0, 255]
		flowX := (float32(c.R)/255.0 - 0.5)
		flowY := (float32(c.G)/255.0 - 0.5)
		gf.flowData[i*2] = flowX
		gf.flowData[i*2+1] = flowY
	}
}

// Sample returns the flow vector at a world position.
func (gf *GPUFlowField) Sample(worldX, worldY float32) (float32, float32) {
	// Map world coords to texture coords
	texX := int(worldX / gf.screenWidth * float32(gf.width))
	texY := int(worldY / gf.screenHeight * float32(gf.height))

	// Clamp to bounds
	if texX < 0 {
		texX = 0
	}
	if texX >= gf.width {
		texX = gf.width - 1
	}
	if texY < 0 {
		texY = 0
	}
	if texY >= gf.height {
		texY = gf.height - 1
	}

	idx := (texY*gf.width + texX) * 2
	return gf.flowData[idx], gf.flowData[idx+1]
}

// Unload releases GPU resources.
func (gf *GPUFlowField) Unload() {
	rl.UnloadShader(gf.shader)
	rl.UnloadTexture(gf.terrainTex)
	rl.UnloadRenderTexture(gf.flowTarget)
}

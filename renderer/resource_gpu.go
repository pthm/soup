package renderer

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/config"
)

// GPUResourceField generates and caches a resource field texture.
// Computed once at startup (or periodically if time-varying).
type GPUResourceField struct {
	shader        rl.Shader
	renderTarget  rl.RenderTexture2D
	timeLoc       int32
	resolutionLoc int32

	// Cached resource data for CPU sampling [0,1]
	resourceData []float32
	width        int
	height       int
	screenWidth  float32
	screenHeight float32

	// Whether field has been computed
	initialized bool
}

// NewGPUResourceField creates a GPU-accelerated resource field.
func NewGPUResourceField(screenWidth, screenHeight float32) *GPUResourceField {
	textureSize := config.Cfg().GPU.ResourceTextureSize

	rf := &GPUResourceField{
		width:        textureSize,
		height:       textureSize,
		screenWidth:  screenWidth,
		screenHeight: screenHeight,
		resourceData: make([]float32, textureSize*textureSize),
	}

	// Load the resource field shader
	rf.shader = rl.LoadShader("", "shaders/resource.fs")
	rf.timeLoc = rl.GetShaderLocation(rf.shader, "time")
	rf.resolutionLoc = rl.GetShaderLocation(rf.shader, "resolution")

	// Set resolution uniform to texture size (shader uses gl_FragCoord which is in texture pixels)
	resolution := []float32{float32(textureSize), float32(textureSize)}
	rl.SetShaderValue(rf.shader, rf.resolutionLoc, resolution, rl.ShaderUniformVec2)

	// Create render target
	rf.renderTarget = rl.LoadRenderTexture(int32(textureSize), int32(textureSize))

	return rf
}

// Initialize computes the resource field once.
// Call this after graphics are initialized but before simulation starts.
func (rf *GPUResourceField) Initialize(time float32) {
	if rf.initialized {
		return
	}
	rf.Regenerate(time)
	rf.initialized = true
}

// Regenerate recomputes the resource field.
// Call this if you want the field to change over time.
func (rf *GPUResourceField) Regenerate(time float32) {
	// Render resource field to texture
	rl.BeginTextureMode(rf.renderTarget)
	rl.ClearBackground(rl.Black)

	// Set time uniform
	rl.SetShaderValue(rf.shader, rf.timeLoc, []float32{time}, rl.ShaderUniformFloat)

	// Draw fullscreen quad with shader
	rl.BeginShaderMode(rf.shader)
	rl.DrawRectangle(0, 0, int32(rf.width), int32(rf.height), rl.White)
	rl.EndShaderMode()

	rl.EndTextureMode()

	// Read back texture to CPU
	rf.readbackData()
}

// readbackData copies the resource texture to CPU memory.
func (rf *GPUResourceField) readbackData() {
	img := rl.LoadImageFromTexture(rf.renderTarget.Texture)
	defer rl.UnloadImage(img)

	colors := rl.LoadImageColors(img)
	defer rl.UnloadImageColors(colors)

	// Extract resource values from R channel
	for i := 0; i < rf.width*rf.height; i++ {
		rf.resourceData[i] = float32(colors[i].R) / 255.0
	}
}

// Sample returns the resource value at a world position.
// O(1) lookup into cached data. Returns value in [0, 1].
func (rf *GPUResourceField) Sample(worldX, worldY float32) float32 {
	// Map world coords to texture coords
	texX := int(worldX / rf.screenWidth * float32(rf.width))
	texY := int(worldY / rf.screenHeight * float32(rf.height))

	// Wrap around edges (toroidal)
	texX = ((texX % rf.width) + rf.width) % rf.width
	texY = ((texY % rf.height) + rf.height) % rf.height

	return rf.resourceData[texY*rf.width+texX]
}

// SampleBilinear returns bilinearly interpolated resource value.
// Slightly more expensive but smoother gradients.
func (rf *GPUResourceField) SampleBilinear(worldX, worldY float32) float32 {
	// Map world coords to texture coords (floating point)
	fx := worldX / rf.screenWidth * float32(rf.width)
	fy := worldY / rf.screenHeight * float32(rf.height)

	// Integer coords and fractions
	x0 := int(fx)
	y0 := int(fy)
	fracX := fx - float32(x0)
	fracY := fy - float32(y0)

	// Wrap all coords (toroidal)
	x0 = ((x0 % rf.width) + rf.width) % rf.width
	y0 = ((y0 % rf.height) + rf.height) % rf.height
	x1 := (x0 + 1) % rf.width
	y1 := (y0 + 1) % rf.height

	// Sample four corners
	v00 := rf.resourceData[y0*rf.width+x0]
	v10 := rf.resourceData[y0*rf.width+x1]
	v01 := rf.resourceData[y1*rf.width+x0]
	v11 := rf.resourceData[y1*rf.width+x1]

	// Bilinear interpolation
	v0 := v00 + (v10-v00)*fracX
	v1 := v01 + (v11-v01)*fracX
	return v0 + (v1-v0)*fracY
}

// Width returns the world width.
func (rf *GPUResourceField) Width() float32 {
	return rf.screenWidth
}

// Height returns the world height.
func (rf *GPUResourceField) Height() float32 {
	return rf.screenHeight
}

// DrawOverlay renders the resource field as a semi-transparent heatmap overlay.
// Uses green tint for high resources.
func (rf *GPUResourceField) DrawOverlay(alpha uint8) {
	// Draw the render texture scaled to screen size
	// The texture is upside down (OpenGL convention), so we flip it
	srcRect := rl.Rectangle{
		X:      0,
		Y:      float32(rf.height),
		Width:  float32(rf.width),
		Height: -float32(rf.height), // Negative to flip
	}
	dstRect := rl.Rectangle{
		X:      0,
		Y:      0,
		Width:  rf.screenWidth,
		Height: rf.screenHeight,
	}

	// Full white tint - let the texture grayscale show through with full intensity
	tint := rl.Color{R: 255, G: 255, B: 255, A: alpha}
	rl.DrawTexturePro(rf.renderTarget.Texture, srcRect, dstRect, rl.Vector2{}, 0, tint)
}

// DrawOverlayHeatmap renders the resource field with a proper heatmap color scale.
// Uses the cached CPU data for accurate color mapping.
func (rf *GPUResourceField) DrawOverlayHeatmap(alpha uint8) {
	cellW := rf.screenWidth / float32(rf.width)
	cellH := rf.screenHeight / float32(rf.height)

	for y := 0; y < rf.height; y++ {
		for x := 0; x < rf.width; x++ {
			value := rf.resourceData[y*rf.width+x]

			// Color scale: black -> dark green -> bright green -> yellow
			var color rl.Color
			if value < 0.3 {
				// Dark: black to dark green
				t := value / 0.3
				color = rl.Color{R: 0, G: uint8(50 * t), B: 0, A: alpha}
			} else if value < 0.6 {
				// Mid: dark green to bright green
				t := (value - 0.3) / 0.3
				color = rl.Color{R: 0, G: uint8(50 + 150*t), B: 0, A: alpha}
			} else {
				// Hot: bright green to yellow
				t := (value - 0.6) / 0.4
				color = rl.Color{R: uint8(255 * t), G: 200, B: 0, A: alpha}
			}

			rl.DrawRectangle(
				int32(float32(x)*cellW),
				int32(float32(y)*cellH),
				int32(cellW)+1,
				int32(cellH)+1,
				color,
			)
		}
	}
}

// Unload releases GPU resources.
func (rf *GPUResourceField) Unload() {
	rl.UnloadShader(rf.shader)
	rl.UnloadRenderTexture(rf.renderTarget)
}

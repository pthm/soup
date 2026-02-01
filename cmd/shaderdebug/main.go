// Shader debug tool - renders a shader to a PNG file for inspection.
//
// Usage: go run ./cmd/shaderdebug -shader shaders/resource.fs -out debug.png
package main

import (
	"flag"
	"fmt"
	"os"

	rl "github.com/gen2brain/raylib-go/raylib"
)

func main() {
	shaderPath := flag.String("shader", "shaders/resource.fs", "Path to fragment shader")
	outPath := flag.String("out", "debug.png", "Output PNG path")
	width := flag.Int("width", 512, "Render width")
	height := flag.Int("height", 512, "Render height")
	flag.Parse()

	// Initialize raylib with hidden window
	rl.SetConfigFlags(rl.FlagWindowHidden)
	rl.InitWindow(int32(*width), int32(*height), "Shader Debug")
	defer rl.CloseWindow()

	// Load the shader
	shader := rl.LoadShader("", *shaderPath)
	if shader.ID == 0 {
		fmt.Fprintf(os.Stderr, "Failed to load shader: %s\n", *shaderPath)
		os.Exit(1)
	}
	defer rl.UnloadShader(shader)

	// Get uniform locations
	timeLoc := rl.GetShaderLocation(shader, "time")
	resolutionLoc := rl.GetShaderLocation(shader, "resolution")

	// Set uniforms
	rl.SetShaderValue(shader, timeLoc, []float32{0.0}, rl.ShaderUniformFloat)
	rl.SetShaderValue(shader, resolutionLoc, []float32{float32(*width), float32(*height)}, rl.ShaderUniformVec2)

	// Create render texture
	target := rl.LoadRenderTexture(int32(*width), int32(*height))
	defer rl.UnloadRenderTexture(target)

	// Render shader to texture
	rl.BeginTextureMode(target)
	rl.ClearBackground(rl.Black)
	rl.BeginShaderMode(shader)
	rl.DrawRectangle(0, 0, int32(*width), int32(*height), rl.White)
	rl.EndShaderMode()
	rl.EndTextureMode()

	// Get image from texture and flip it (OpenGL convention)
	img := rl.LoadImageFromTexture(target.Texture)
	rl.ImageFlipVertical(img)

	// Export to PNG
	success := rl.ExportImage(*img, *outPath)
	rl.UnloadImage(img)

	if success {
		fmt.Printf("Shader rendered to: %s (%dx%d)\n", *outPath, *width, *height)
	} else {
		fmt.Fprintf(os.Stderr, "Failed to export image\n")
		os.Exit(1)
	}
}

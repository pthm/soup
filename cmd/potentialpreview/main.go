// Potential field preview tool - interactive visualization with sliders.
//
// Usage: go run ./cmd/potentialpreview
package main

import (
	"fmt"
	"image/color"
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	gui "github.com/gen2brain/raylib-go/raygui"
)

const (
	windowWidth  = 1000
	windowHeight = 720
	previewSize  = 512
	panelWidth   = windowWidth - previewSize - 30
)

// PotentialParams holds the FBM parameters
type PotentialParams struct {
	Scale      float32
	Octaves    int
	Lacunarity float32
	Gain       float32
	Contrast   float32
	DriftX     float32
	DriftY     float32
	Seed       uint32
}

func main() {
	rl.InitWindow(windowWidth, windowHeight, "Potential Field Preview")
	defer rl.CloseWindow()
	rl.SetTargetFPS(30)

	// Initialize with default values from config
	params := PotentialParams{
		Scale:      4.1,
		Octaves:    5,
		Lacunarity: 3.3,
		Gain:       0.66,
		Contrast:   4.69,
		DriftX:     0.0035,
		DriftY:     0.0017,
		Seed:       12345,
	}

	// Create texture for rendering
	gridSize := 256
	potentialGrid := make([]float32, gridSize*gridSize)
	img := rl.GenImageColor(gridSize, gridSize, rl.Black)
	texture := rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
	defer rl.UnloadTexture(texture)

	// Time for animation
	var time float32 = 0
	animating := false

	// Generate initial field
	generatePotential(potentialGrid, gridSize, params, time)
	updateTexture(texture, potentialGrid, gridSize)

	// GUI state
	needsRegen := true

	for !rl.WindowShouldClose() {
		// Animation
		if animating {
			time += rl.GetFrameTime()
			needsRegen = true
		}

		// Regenerate if needed
		if needsRegen {
			generatePotential(potentialGrid, gridSize, params, time)
			updateTexture(texture, potentialGrid, gridSize)
			needsRegen = false
		}

		rl.BeginDrawing()
		rl.ClearBackground(rl.RayWhite)

		// Draw preview
		rl.DrawTexturePro(
			texture,
			rl.Rectangle{X: 0, Y: 0, Width: float32(gridSize), Height: float32(gridSize)},
			rl.Rectangle{X: 10, Y: 10, Width: previewSize, Height: previewSize},
			rl.Vector2{X: 0, Y: 0},
			0,
			rl.White,
		)
		rl.DrawRectangleLines(10, 10, previewSize, previewSize, rl.DarkGray)

		// Draw stats
		var totalMass float32
		var minVal, maxVal float32 = 1, 0
		for _, v := range potentialGrid {
			totalMass += v
			if v < minVal {
				minVal = v
			}
			if v > maxVal {
				maxVal = v
			}
		}
		avgMass := totalMass / float32(len(potentialGrid))

		statsY := int32(previewSize + 25)
		rl.DrawText(fmt.Sprintf("Min: %.3f  Max: %.3f  Avg: %.3f", minVal, maxVal, avgMass), 15, statsY, 16, rl.DarkGray)
		rl.DrawText(fmt.Sprintf("Time: %.1f", time), 15, statsY+20, 16, rl.DarkGray)

		// Control panel
		panelX := float32(previewSize + 20)
		panelY := float32(10)

		rl.DrawText("Potential Field Parameters", int32(panelX), int32(panelY), 20, rl.DarkGray)
		panelY += 35

		// Scale slider
		rl.DrawText("Scale (base noise frequency)", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newScale := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"1.0", "20.0",
			params.Scale, 1.0, 20.0,
		)
		rl.DrawText(fmt.Sprintf("%.1f", params.Scale), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newScale != params.Scale {
			params.Scale = newScale
			needsRegen = true
		}
		panelY += 35

		// Octaves slider
		rl.DrawText("Octaves (FBM detail level)", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newOctaves := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"1", "6",
			float32(params.Octaves), 1, 6,
		)
		rl.DrawText(fmt.Sprintf("%d", params.Octaves), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if int(newOctaves) != params.Octaves {
			params.Octaves = int(newOctaves)
			needsRegen = true
		}
		panelY += 35

		// Lacunarity slider
		rl.DrawText("Lacunarity (frequency multiplier)", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newLacunarity := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"1.5", "4.0",
			params.Lacunarity, 1.5, 4.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", params.Lacunarity), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newLacunarity != params.Lacunarity {
			params.Lacunarity = newLacunarity
			needsRegen = true
		}
		panelY += 35

		// Gain slider
		rl.DrawText("Gain (amplitude multiplier)", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newGain := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"0.2", "0.9",
			params.Gain, 0.2, 0.9,
		)
		rl.DrawText(fmt.Sprintf("%.2f", params.Gain), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newGain != params.Gain {
			params.Gain = newGain
			needsRegen = true
		}
		panelY += 35

		// Contrast slider
		rl.DrawText("Contrast (exponent - higher = sparser)", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newContrast := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"1.0", "5.0",
			params.Contrast, 1.0, 5.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", params.Contrast), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newContrast != params.Contrast {
			params.Contrast = newContrast
			needsRegen = true
		}
		panelY += 35

		// Separator
		rl.DrawLine(int32(panelX), int32(panelY), int32(panelX)+int32(panelWidth)-20, int32(panelY), rl.LightGray)
		panelY += 15

		// Drift section
		rl.DrawText("Drift (animation speed)", int32(panelX), int32(panelY), 16, rl.DarkGray)
		panelY += 25

		// Drift X slider
		rl.DrawText("Drift X", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newDriftX := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"0", "0.02",
			params.DriftX, 0, 0.02,
		)
		rl.DrawText(fmt.Sprintf("%.4f", params.DriftX), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newDriftX != params.DriftX {
			params.DriftX = newDriftX
		}
		panelY += 35

		// Drift Y slider
		rl.DrawText("Drift Y", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newDriftY := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"0", "0.02",
			params.DriftY, 0, 0.02,
		)
		rl.DrawText(fmt.Sprintf("%.4f", params.DriftY), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if newDriftY != params.DriftY {
			params.DriftY = newDriftY
		}
		panelY += 35

		// Seed slider
		rl.DrawText("Seed", int32(panelX), int32(panelY), 14, rl.Gray)
		panelY += 18
		newSeed := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: float32(panelWidth - 80), Height: 20},
			"0", "99999",
			float32(params.Seed), 0, 99999,
		)
		rl.DrawText(fmt.Sprintf("%d", params.Seed), int32(panelX+float32(panelWidth-70)), int32(panelY+2), 16, rl.DarkGray)
		if uint32(newSeed) != params.Seed {
			params.Seed = uint32(newSeed)
			needsRegen = true
		}
		panelY += 45

		// Buttons
		if gui.Button(rl.Rectangle{X: panelX, Y: panelY, Width: 120, Height: 30}, toggleText(animating, "Stop", "Animate")) {
			animating = !animating
		}

		if gui.Button(rl.Rectangle{X: panelX + 130, Y: panelY, Width: 120, Height: 30}, "Reset Time") {
			time = 0
			needsRegen = true
		}
		panelY += 45

		if gui.Button(rl.Rectangle{X: panelX, Y: panelY, Width: 120, Height: 30}, "Random Seed") {
			params.Seed = uint32(rl.GetRandomValue(0, 99999))
			needsRegen = true
		}

		if gui.Button(rl.Rectangle{X: panelX + 130, Y: panelY, Width: 120, Height: 30}, "Reset All") {
			params = PotentialParams{
				Scale:      4.1,
				Octaves:    5,
				Lacunarity: 3.3,
				Gain:       0.66,
				Contrast:   4.69,
				DriftX:     0.0035,
				DriftY:     0.0017,
				Seed:       12345,
			}
			time = 0
			needsRegen = true
		}
		panelY += 55

		// Output YAML
		rl.DrawText("YAML Config:", int32(panelX), int32(panelY), 16, rl.DarkGray)
		panelY += 25
		yamlLines := []string{
			"potential:",
			fmt.Sprintf("  scale: %.1f", params.Scale),
			fmt.Sprintf("  octaves: %d", params.Octaves),
			fmt.Sprintf("  lacunarity: %.1f", params.Lacunarity),
			fmt.Sprintf("  gain: %.2f", params.Gain),
			fmt.Sprintf("  contrast: %.2f", params.Contrast),
			fmt.Sprintf("  drift_x: %.4f", params.DriftX),
			fmt.Sprintf("  drift_y: %.4f", params.DriftY),
		}
		for _, line := range yamlLines {
			rl.DrawText(line, int32(panelX), int32(panelY), 14, rl.Gray)
			panelY += 16
		}

		// Instructions
		rl.DrawText("Press C to copy YAML to clipboard", int32(panelX), int32(windowHeight-30), 12, rl.LightGray)

		// Copy to clipboard on C key
		if rl.IsKeyPressed(rl.KeyC) {
			yaml := fmt.Sprintf(`potential:
  scale: %.1f
  octaves: %d
  lacunarity: %.1f
  gain: %.2f
  contrast: %.2f
  drift_x: %.4f
  drift_y: %.4f`,
				params.Scale, params.Octaves, params.Lacunarity, params.Gain,
				params.Contrast, params.DriftX, params.DriftY)
			rl.SetClipboardText(yaml)
		}

		rl.EndDrawing()
	}
}

func toggleText(cond bool, ifTrue, ifFalse string) string {
	if cond {
		return ifTrue
	}
	return ifFalse
}

// generatePotential fills the grid using FBM - same algorithm as particle_resource.go
func generatePotential(grid []float32, size int, params PotentialParams, t float32) {
	du := fract(t * params.DriftX)
	dv := fract(t * params.DriftY)

	for y := 0; y < size; y++ {
		v := (float32(y) + 0.5) / float32(size)
		v = fract(v + dv)
		for x := 0; x < size; x++ {
			u := (float32(x) + 0.5) / float32(size)
			u = fract(u + du)

			grid[y*size+x] = fbm2D(u, v, params)
		}
	}
}

// fbm2D generates tileable 2D FBM
func fbm2D(u, v float32, params PotentialParams) float32 {
	sum := float32(0)
	amp := float32(0.5)
	freq := params.Scale

	for o := 0; o < params.Octaves; o++ {
		sum += amp * valueNoise2D(u, v, freq, params.Seed)
		freq *= params.Lacunarity
		amp *= params.Gain
	}

	// Contrast shaping
	return clamp01(float32(math.Pow(float64(sum), float64(params.Contrast))))
}

// valueNoise2D generates tileable value noise at given frequency
func valueNoise2D(u, v float32, freq float32, seed uint32) float32 {
	x := u * freq
	y := v * freq

	ix := int(math.Floor(float64(x)))
	iy := int(math.Floor(float64(y)))

	fx := x - float32(ix)
	fy := y - float32(iy)

	f := int(freq)
	if f < 1 {
		f = 1
	}

	i00x := modInt(ix, f)
	i10x := modInt(ix+1, f)
	i00y := modInt(iy, f)
	i01y := modInt(iy+1, f)

	a := hash2D(i00x, i00y, seed)
	b := hash2D(i10x, i00y, seed)
	c := hash2D(i00x, i01y, seed)
	d := hash2D(i10x, i01y, seed)

	ux := smoothstep(fx)
	uy := smoothstep(fy)

	ab := a + (b-a)*ux
	cd := c + (d-c)*ux
	return ab + (cd-ab)*uy
}

func hash2D(ix, iy int, seed uint32) float32 {
	x := uint32(ix)
	y := uint32(iy)
	h := x*374761393 + y*668265263 + seed*1442695041
	h = (h ^ (h >> 13)) * 1274126177
	h ^= (h >> 16)
	return float32(h&0x00FFFFFF) / float32(0x01000000)
}

func fract(x float32) float32 {
	return x - float32(math.Floor(float64(x)))
}

func modInt(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func smoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

// updateTexture updates the GPU texture from the grid values
func updateTexture(texture rl.Texture2D, grid []float32, size int) {
	pixels := make([]color.RGBA, size*size)
	for i, v := range grid {
		// Use a color gradient: dark blue -> cyan -> yellow -> white
		var r, g, b uint8
		if v < 0.25 {
			// Dark blue to blue
			t := v / 0.25
			r = uint8(10 + t*30)
			g = uint8(20 + t*60)
			b = uint8(60 + t*100)
		} else if v < 0.5 {
			// Blue to cyan
			t := (v - 0.25) / 0.25
			r = uint8(40 + t*20)
			g = uint8(80 + t*120)
			b = uint8(160 + t*40)
		} else if v < 0.75 {
			// Cyan to yellow-green
			t := (v - 0.5) / 0.25
			r = uint8(60 + t*140)
			g = uint8(200 - t*40)
			b = uint8(200 - t*150)
		} else {
			// Yellow-green to white
			t := (v - 0.75) / 0.25
			r = uint8(200 + t*55)
			g = uint8(160 + t*95)
			b = uint8(50 + t*205)
		}
		pixels[i] = color.RGBA{R: r, G: g, B: b, A: 255}
	}
	rl.UpdateTexture(texture, pixels)
}

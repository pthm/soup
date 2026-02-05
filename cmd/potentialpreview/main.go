// Potential/Resource field preview tool - interactive visualization with grazing simulation.
// Shows how capacity (potential) and resource fields interact with grazing organisms.
//
// Usage: go run ./cmd/potentialpreview
package main

import (
	"fmt"
	"image/color"
	"math"
	"math/rand"

	rl "github.com/gen2brain/raylib-go/raylib"
	gui "github.com/gen2brain/raylib-go/raygui"
	opensimplex "github.com/ojrac/opensimplex-go"
)

const (
	windowWidth  = 1400
	windowHeight = 800
	previewSize  = 400
	panelWidth   = 380
	gridSize     = 128
)

// PotentialParams holds the FBM parameters
type PotentialParams struct {
	Scale      float64
	Octaves    int
	Lacunarity float64
	Gain       float64
	Contrast   float64
	TimeSpeed  float64
	RotXW      float64
	RotYZ      float64
	Seed       int64
}

// ResourceParams holds resource regeneration parameters
type ResourceParams struct {
	RegenRate float64
	DecayRate float64
}

// Walker is a simple grazing organism
type Walker struct {
	X, Y      float64 // Position [0, 1]
	Angle     float64 // Heading
	Speed     float64 // Movement speed
	TurnRate  float64 // How much it turns
	GrazeRate float64 // How fast it grazes
}

func main() {
	rl.InitWindow(windowWidth, windowHeight, "Potential & Resource Field Preview")
	defer rl.CloseWindow()
	rl.SetTargetFPS(60)

	// Initialize parameters (matches defaults.yaml)
	potParams := PotentialParams{
		Scale:      0.70,
		Octaves:    6,
		Lacunarity: 3.0,
		Gain:       0.77,
		Contrast:   8.0,
		TimeSpeed:  0.062,
		RotXW:      0.7,
		RotYZ:      0.53,
		Seed:       12345,
	}

	resParams := ResourceParams{
		RegenRate: 0.430,
		DecayRate: 0.169,
	}

	// Create noise generator
	noise := opensimplex.New(potParams.Seed)

	// Create grids
	capGrid := make([]float32, gridSize*gridSize)
	resGrid := make([]float32, gridSize*gridSize)

	// Create textures
	img := rl.GenImageColor(gridSize, gridSize, rl.Black)
	capTexture := rl.LoadTextureFromImage(img)
	resTexture := rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
	defer rl.UnloadTexture(capTexture)
	defer rl.UnloadTexture(resTexture)

	// Walkers
	walkers := make([]Walker, 0)
	numWalkers := 10
	grazeRadius := 1

	// Time and state
	var time float64 = 0
	animating := true
	lastSeed := potParams.Seed

	// Initialize grids
	generateCapacity(capGrid, gridSize, potParams, noise, time)
	copy(resGrid, capGrid) // Start resource at capacity
	updateCapTexture(capTexture, capGrid, gridSize)
	updateResTexture(resTexture, resGrid, gridSize)

	// Spawn initial walkers
	for i := 0; i < numWalkers; i++ {
		walkers = append(walkers, newWalker())
	}

	for !rl.WindowShouldClose() {
		dt := float64(rl.GetFrameTime())

		// Check if seed changed
		if potParams.Seed != lastSeed {
			noise = opensimplex.New(potParams.Seed)
			lastSeed = potParams.Seed
		}

		// Update simulation
		if animating {
			// Advance time and update capacity
			time += dt * potParams.TimeSpeed
			generateCapacity(capGrid, gridSize, potParams, noise, time)

			// Regenerate or decay resource towards capacity
			regenRate := float32(resParams.RegenRate * dt)
			decayRate := float32(resParams.DecayRate * dt)
			for i := range resGrid {
				cap := capGrid[i]
				res := resGrid[i]
				if res < cap {
					res = res + (cap-res)*regenRate
				} else if res > cap {
					res = res + (cap-res)*decayRate
				}
				resGrid[i] = res
			}

			// Update walkers
			for i := range walkers {
				updateWalker(&walkers[i], resGrid, gridSize, grazeRadius, dt)
			}

			// Update textures
			updateCapTexture(capTexture, capGrid, gridSize)
			updateResTexture(resTexture, resGrid, gridSize)
		}

		// Adjust walker count
		for len(walkers) < numWalkers {
			walkers = append(walkers, newWalker())
		}
		for len(walkers) > numWalkers {
			walkers = walkers[:len(walkers)-1]
		}

		rl.BeginDrawing()
		rl.ClearBackground(rl.RayWhite)

		// Draw capacity field
		rl.DrawText("Capacity (Potential)", 15, 10, 18, rl.DarkGray)
		rl.DrawTexturePro(
			capTexture,
			rl.Rectangle{X: 0, Y: 0, Width: float32(gridSize), Height: float32(gridSize)},
			rl.Rectangle{X: 10, Y: 35, Width: previewSize, Height: previewSize},
			rl.Vector2{X: 0, Y: 0}, 0, rl.White,
		)
		rl.DrawRectangleLines(10, 35, previewSize, previewSize, rl.DarkGray)

		// Draw resource field
		rl.DrawText("Resource (what organisms eat)", 15+previewSize+10, 10, 18, rl.DarkGray)
		rl.DrawTexturePro(
			resTexture,
			rl.Rectangle{X: 0, Y: 0, Width: float32(gridSize), Height: float32(gridSize)},
			rl.Rectangle{X: float32(10 + previewSize + 10), Y: 35, Width: previewSize, Height: previewSize},
			rl.Vector2{X: 0, Y: 0}, 0, rl.White,
		)
		rl.DrawRectangleLines(10+int32(previewSize)+10, 35, previewSize, previewSize, rl.DarkGray)

		// Draw walkers on resource field
		resFieldX := float32(10 + previewSize + 10)
		resFieldY := float32(35)
		for _, w := range walkers {
			wx := resFieldX + float32(w.X)*previewSize
			wy := resFieldY + float32(w.Y)*previewSize
			rl.DrawCircle(int32(wx), int32(wy), 4, rl.Red)
			// Draw heading indicator
			hx := wx + float32(math.Cos(w.Angle))*8
			hy := wy + float32(math.Sin(w.Angle))*8
			rl.DrawLine(int32(wx), int32(wy), int32(hx), int32(hy), rl.DarkGray)
		}

		// Stats
		statsY := int32(35 + previewSize + 10)
		capTotal, capMin, capMax := gridStats(capGrid)
		resTotal, resMin, resMax := gridStats(resGrid)
		rl.DrawText(fmt.Sprintf("Cap - Min:%.2f Max:%.2f Avg:%.3f", capMin, capMax, capTotal/float32(len(capGrid))), 15, statsY, 14, rl.DarkGray)
		rl.DrawText(fmt.Sprintf("Res - Min:%.2f Max:%.2f Avg:%.3f", resMin, resMax, resTotal/float32(len(resGrid))), 15, statsY+18, 14, rl.DarkGray)
		rl.DrawText(fmt.Sprintf("Time: %.2f  Deficit: %.1f%%", time, 100*(1-resTotal/capTotal)), 15, statsY+36, 14, rl.DarkGray)

		// Control panel
		panelX := float32(10 + previewSize*2 + 30)
		panelY := float32(10)

		rl.DrawText("Potential Field", int32(panelX), int32(panelY), 18, rl.DarkGray)
		panelY += 28

		// Scale
		rl.DrawText("Scale", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newScale := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0.5", "10",
			float32(potParams.Scale), 0.5, 10.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.Scale), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.Scale = float64(newScale)
		panelY += 24

		// Octaves
		rl.DrawText("Octaves", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newOctaves := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"1", "8",
			float32(potParams.Octaves), 1, 8,
		)
		rl.DrawText(fmt.Sprintf("%d", potParams.Octaves), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.Octaves = int(newOctaves)
		panelY += 24

		// Lacunarity
		rl.DrawText("Lacunarity", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newLac := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"1.5", "4",
			float32(potParams.Lacunarity), 1.5, 4.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.Lacunarity), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.Lacunarity = float64(newLac)
		panelY += 24

		// Gain
		rl.DrawText("Gain", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newGain := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0.3", "0.8",
			float32(potParams.Gain), 0.3, 0.8,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.Gain), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.Gain = float64(newGain)
		panelY += 24

		// Contrast
		rl.DrawText("Contrast (sparser hotspots)", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newContrast := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"1", "8",
			float32(potParams.Contrast), 1.0, 8.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.Contrast), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.Contrast = float64(newContrast)
		panelY += 24

		// Time Speed
		rl.DrawText("Time Speed (evolution rate)", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newTS := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "0.2",
			float32(potParams.TimeSpeed), 0, 0.2,
		)
		rl.DrawText(fmt.Sprintf("%.3f", potParams.TimeSpeed), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.TimeSpeed = float64(newTS)
		panelY += 24

		// RotXW
		rl.DrawText("Rotation XW", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newRXW := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "2",
			float32(potParams.RotXW), 0, 2.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.RotXW), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.RotXW = float64(newRXW)
		panelY += 24

		// RotYZ
		rl.DrawText("Rotation YZ", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newRYZ := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "2",
			float32(potParams.RotYZ), 0, 2.0,
		)
		rl.DrawText(fmt.Sprintf("%.2f", potParams.RotYZ), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		potParams.RotYZ = float64(newRYZ)
		panelY += 30

		// Separator
		rl.DrawLine(int32(panelX), int32(panelY), int32(panelX+panelWidth-20), int32(panelY), rl.LightGray)
		panelY += 15

		// Resource section
		rl.DrawText("Resource Field", int32(panelX), int32(panelY), 18, rl.DarkGray)
		panelY += 28

		// Regen Rate
		rl.DrawText("Regen Rate (when below cap)", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newRegen := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "1.0",
			float32(resParams.RegenRate), 0, 1.0,
		)
		rl.DrawText(fmt.Sprintf("%.3f", resParams.RegenRate), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		resParams.RegenRate = float64(newRegen)
		panelY += 24

		// Decay Rate
		rl.DrawText("Decay Rate (when above cap)", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newDecay := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "0.5",
			float32(resParams.DecayRate), 0, 0.5,
		)
		rl.DrawText(fmt.Sprintf("%.3f", resParams.DecayRate), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		resParams.DecayRate = float64(newDecay)
		panelY += 30

		// Separator
		rl.DrawLine(int32(panelX), int32(panelY), int32(panelX+panelWidth-20), int32(panelY), rl.LightGray)
		panelY += 15

		// Walkers section
		rl.DrawText("Walkers (grazers)", int32(panelX), int32(panelY), 18, rl.DarkGray)
		panelY += 28

		// Number of walkers
		rl.DrawText("Count", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newNum := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "50",
			float32(numWalkers), 0, 50,
		)
		rl.DrawText(fmt.Sprintf("%d", numWalkers), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		numWalkers = int(newNum)
		panelY += 24

		// Graze radius
		rl.DrawText("Graze Radius (cells)", int32(panelX), int32(panelY), 12, rl.Gray)
		panelY += 14
		newGR := gui.SliderBar(
			rl.Rectangle{X: panelX, Y: panelY, Width: panelWidth - 70, Height: 18},
			"0", "3",
			float32(grazeRadius), 0, 3,
		)
		rl.DrawText(fmt.Sprintf("%d", grazeRadius), int32(panelX+panelWidth-60), int32(panelY), 14, rl.DarkGray)
		grazeRadius = int(newGR)
		panelY += 35

		// Buttons
		if gui.Button(rl.Rectangle{X: panelX, Y: panelY, Width: 90, Height: 26}, toggleText(animating, "Pause", "Run")) {
			animating = !animating
		}

		if gui.Button(rl.Rectangle{X: panelX + 100, Y: panelY, Width: 90, Height: 26}, "Reset Res") {
			copy(resGrid, capGrid)
		}

		if gui.Button(rl.Rectangle{X: panelX + 200, Y: panelY, Width: 90, Height: 26}, "Random") {
			potParams.Seed = int64(rl.GetRandomValue(0, 99999))
		}
		panelY += 35

		if gui.Button(rl.Rectangle{X: panelX, Y: panelY, Width: 90, Height: 26}, "Reset All") {
			potParams = PotentialParams{
				Scale: 0.70, Octaves: 6, Lacunarity: 3.0, Gain: 0.77,
				Contrast: 8.0, TimeSpeed: 0.062, RotXW: 0.7, RotYZ: 0.53, Seed: 12345,
			}
			resParams.RegenRate = 0.430
			resParams.DecayRate = 0.169
			numWalkers = 10
			grazeRadius = 1
			time = 0
			generateCapacity(capGrid, gridSize, potParams, noise, time)
			copy(resGrid, capGrid)
			walkers = walkers[:0]
			for i := 0; i < numWalkers; i++ {
				walkers = append(walkers, newWalker())
			}
		}
		panelY += 45

		// YAML output
		rl.DrawText("YAML (press C to copy):", int32(panelX), int32(panelY), 12, rl.DarkGray)
		panelY += 18
		yamlLines := []string{
			"potential:",
			fmt.Sprintf("  scale: %.2f", potParams.Scale),
			fmt.Sprintf("  octaves: %d", potParams.Octaves),
			fmt.Sprintf("  lacunarity: %.2f", potParams.Lacunarity),
			fmt.Sprintf("  gain: %.2f", potParams.Gain),
			fmt.Sprintf("  contrast: %.2f", potParams.Contrast),
			fmt.Sprintf("  time_speed: %.3f", potParams.TimeSpeed),
			"resource:",
			fmt.Sprintf("  regen_rate: %.3f", resParams.RegenRate),
			fmt.Sprintf("  decay_rate: %.3f", resParams.DecayRate),
		}
		for _, line := range yamlLines {
			rl.DrawText(line, int32(panelX), int32(panelY), 12, rl.Gray)
			panelY += 14
		}

		// Keyboard shortcuts
		if rl.IsKeyPressed(rl.KeyC) {
			yaml := fmt.Sprintf(`potential:
  scale: %.2f
  octaves: %d
  lacunarity: %.2f
  gain: %.2f
  contrast: %.2f
  time_speed: %.3f
resource:
  regen_rate: %.3f
  decay_rate: %.3f`,
				potParams.Scale, potParams.Octaves, potParams.Lacunarity, potParams.Gain,
				potParams.Contrast, potParams.TimeSpeed, resParams.RegenRate, resParams.DecayRate)
			rl.SetClipboardText(yaml)
		}

		if rl.IsKeyPressed(rl.KeySpace) {
			animating = !animating
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

func newWalker() Walker {
	return Walker{
		X:         rand.Float64(),
		Y:         rand.Float64(),
		Angle:     rand.Float64() * 2 * math.Pi,
		Speed:     0.05 + rand.Float64()*0.1,
		TurnRate:  0.5 + rand.Float64()*1.0,
		GrazeRate: 0.3 + rand.Float64()*0.4,
	}
}

func updateWalker(w *Walker, resGrid []float32, gridSize, grazeRadius int, dt float64) {
	// Random walk with bias towards higher resource
	w.Angle += (rand.Float64() - 0.5) * w.TurnRate * dt * 10

	// Move
	w.X += math.Cos(w.Angle) * w.Speed * dt
	w.Y += math.Sin(w.Angle) * w.Speed * dt

	// Wrap around
	w.X = math.Mod(w.X+1, 1)
	w.Y = math.Mod(w.Y+1, 1)

	// Graze
	graze(resGrid, gridSize, w.X, w.Y, w.GrazeRate, dt, grazeRadius)
}

func graze(grid []float32, size int, x, y float64, rate, dt float64, radius int) {
	cx := int(x * float64(size))
	cy := int(y * float64(size))

	want := float32(rate * dt)
	if want <= 0 {
		return
	}

	// Compute kernel weights
	var wsum float32
	for oy := -radius; oy <= radius; oy++ {
		for ox := -radius; ox <= radius; ox++ {
			d := float32(absInt(ox) + absInt(oy))
			w := float32(radius+1) - d
			if w > 0 {
				wsum += w
			}
		}
	}
	if wsum <= 0 {
		return
	}

	// Remove resource
	for oy := -radius; oy <= radius; oy++ {
		yy := modInt(cy+oy, size)
		for ox := -radius; ox <= radius; ox++ {
			xx := modInt(cx+ox, size)

			d := float32(absInt(ox) + absInt(oy))
			w := float32(radius+1) - d
			if w <= 0 {
				continue
			}
			share := want * (w / wsum)

			i := yy*size + xx
			avail := grid[i]
			take := share
			if take > avail {
				take = avail
			}
			if take > 0 {
				grid[i] = avail - take
			}
		}
	}
}

func generateCapacity(grid []float32, size int, params PotentialParams, noise opensimplex.Noise, t float64) {
	for y := 0; y < size; y++ {
		v := (float64(y) + 0.5) / float64(size)
		for x := 0; x < size; x++ {
			u := (float64(x) + 0.5) / float64(size)
			grid[y*size+x] = fbmTiled(u, v, t, params, noise)
		}
	}
}

func fbmTiled(u, v, t float64, params PotentialParams, noise opensimplex.Noise) float32 {
	sum := 0.0
	amp := 0.5
	freq := params.Scale

	twoPi := 2.0 * math.Pi
	angleU := u * twoPi
	angleV := v * twoPi

	baseX := math.Cos(angleU)
	baseY := math.Sin(angleU)
	baseZ := math.Cos(angleV)
	baseW := math.Sin(angleV)

	rotXW := t * params.RotXW
	rotYZ := t * params.RotYZ

	cosXW := math.Cos(rotXW)
	sinXW := math.Sin(rotXW)
	cosYZ := math.Cos(rotYZ)
	sinYZ := math.Sin(rotYZ)

	nx := baseX*cosXW - baseW*sinXW
	nw := baseX*sinXW + baseW*cosXW
	ny := baseY*cosYZ - baseZ*sinYZ
	nz := baseY*sinYZ + baseZ*cosYZ

	for o := 0; o < params.Octaves; o++ {
		n := (noise.Eval4(nx*freq, ny*freq, nz*freq, nw*freq) + 1) * 0.5
		sum += amp * n
		freq *= params.Lacunarity
		amp *= params.Gain
	}

	return clamp01(float32(math.Pow(sum, params.Contrast)))
}

func gridStats(grid []float32) (total, min, max float32) {
	min = 1
	max = 0
	for _, v := range grid {
		total += v
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return
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

func modInt(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func updateCapTexture(texture rl.Texture2D, grid []float32, size int) {
	pixels := make([]color.RGBA, size*size)
	for i, v := range grid {
		// Blue-cyan-white gradient for capacity
		var r, g, b uint8
		if v < 0.5 {
			t := v / 0.5
			r = uint8(20 + t*40)
			g = uint8(40 + t*120)
			b = uint8(80 + t*100)
		} else {
			t := (v - 0.5) / 0.5
			r = uint8(60 + t*195)
			g = uint8(160 + t*95)
			b = uint8(180 + t*75)
		}
		pixels[i] = color.RGBA{R: r, G: g, B: b, A: 255}
	}
	rl.UpdateTexture(texture, pixels)
}

func updateResTexture(texture rl.Texture2D, grid []float32, size int) {
	pixels := make([]color.RGBA, size*size)
	for i, v := range grid {
		// Green gradient for resource (darker = depleted)
		var r, g, b uint8
		if v < 0.3 {
			t := v / 0.3
			r = uint8(20 + t*20)
			g = uint8(30 + t*60)
			b = uint8(20 + t*20)
		} else if v < 0.6 {
			t := (v - 0.3) / 0.3
			r = uint8(40 + t*40)
			g = uint8(90 + t*80)
			b = uint8(40 + t*20)
		} else {
			t := (v - 0.6) / 0.4
			r = uint8(80 + t*100)
			g = uint8(170 + t*85)
			b = uint8(60 + t*140)
		}
		pixels[i] = color.RGBA{R: r, G: g, B: b, A: 255}
	}
	rl.UpdateTexture(texture, pixels)
}

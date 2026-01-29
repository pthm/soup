package renderer

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/systems"
)

// TerrainRenderer renders the terrain with organic edges and color variation.
type TerrainRenderer struct {
	width       int32
	height      int32
	initialized bool
}

// NewTerrainRenderer creates a new terrain renderer.
func NewTerrainRenderer(width, height int32) *TerrainRenderer {
	return &TerrainRenderer{
		width:  width,
		height: height,
	}
}

// Draw renders the terrain.
func (r *TerrainRenderer) Draw(terrain *systems.TerrainSystem, tick int32) {
	if terrain == nil {
		return
	}

	grid := terrain.Grid()
	gridW := terrain.GridWidth()
	gridH := terrain.GridHeight()
	cellSize := terrain.CellSize()
	noise := terrain.Noise()

	// Draw terrain cells
	for gy := 0; gy < gridH; gy++ {
		for gx := 0; gx < gridW; gx++ {
			cell := grid[gy][gx]
			if cell == systems.TerrainEmpty {
				continue
			}

			// Base position
			baseX := float32(gx) * cellSize
			baseY := float32(gy) * cellSize

			// Apply noise-based edge perturbation for organic look
			edgeNoise := float32(noise.Noise2D(float64(gx)*0.3, float64(gy)*0.3+1000))
			offsetX := edgeNoise * cellSize * 0.15
			offsetY := float32(noise.Noise2D(float64(gx)*0.3+500, float64(gy)*0.3)) * cellSize * 0.15

			// Depth-based color - darker at bottom
			normalizedY := float32(gy) / float32(gridH)
			depthDarken := 1.0 - normalizedY*0.4 // 1.0 at top, 0.6 at bottom

			// Choose color based on terrain type
			var baseColor rl.Color
			switch cell {
			case systems.TerrainRock:
				// Dark rocky colors with variation
				rockNoise := float32(noise.Noise2D(float64(gx)*0.5+200, float64(gy)*0.5+200))
				gray := uint8(40 + rockNoise*15)
				baseColor = rl.Color{
					R: uint8(float32(gray) * depthDarken),
					G: uint8(float32(gray+5) * depthDarken),
					B: uint8(float32(gray+10) * depthDarken),
					A: 255,
				}
			case systems.TerrainCoral:
				// Coral colors - orange/pink with variation
				coralNoise := float32(noise.Noise2D(float64(gx)*0.4+400, float64(gy)*0.4+400))
				baseR := uint8(180 + coralNoise*30)
				baseG := uint8(100 + coralNoise*20)
				baseB := uint8(80 + coralNoise*30)
				baseColor = rl.Color{
					R: uint8(float32(baseR) * depthDarken),
					G: uint8(float32(baseG) * depthDarken),
					B: uint8(float32(baseB) * depthDarken),
					A: 255,
				}
			}

			// Draw main cell with slight offset for organic look
			drawX := baseX + offsetX
			drawY := baseY + offsetY
			drawW := cellSize + cellSize*0.1
			drawH := cellSize + cellSize*0.1

			rl.DrawRectangle(int32(drawX), int32(drawY), int32(drawW), int32(drawH), baseColor)

			// Draw edge highlights/shadows for depth
			r.drawCellEdges(gx, gy, baseX, baseY, cellSize, grid, gridW, gridH, baseColor, noise)
		}
	}
}

// drawCellEdges adds visual depth with edge highlights and shadows.
func (r *TerrainRenderer) drawCellEdges(gx, gy int, baseX, baseY, cellSize float32,
	grid [][]systems.TerrainCell, gridW, gridH int, baseColor rl.Color, noise *systems.PerlinNoise) {

	// Check neighbors for edge detection
	hasTop := gy > 0 && grid[gy-1][gx] != systems.TerrainEmpty
	hasBottom := gy < gridH-1 && grid[gy+1][gx] != systems.TerrainEmpty
	hasLeft := gx > 0 && grid[gy][gx-1] != systems.TerrainEmpty
	hasRight := gx < gridW-1 && grid[gy][gx+1] != systems.TerrainEmpty

	edgeThickness := cellSize * 0.15

	// Top edge highlight (light from above)
	if !hasTop {
		highlightColor := rl.Color{
			R: uint8(math.Min(float64(baseColor.R)+40, 255)),
			G: uint8(math.Min(float64(baseColor.G)+40, 255)),
			B: uint8(math.Min(float64(baseColor.B)+45, 255)),
			A: 200,
		}
		rl.DrawRectangle(int32(baseX), int32(baseY), int32(cellSize), int32(edgeThickness), highlightColor)
	}

	// Bottom edge shadow
	if !hasBottom {
		shadowColor := rl.Color{
			R: uint8(float32(baseColor.R) * 0.6),
			G: uint8(float32(baseColor.G) * 0.6),
			B: uint8(float32(baseColor.B) * 0.6),
			A: 200,
		}
		rl.DrawRectangle(int32(baseX), int32(baseY+cellSize-edgeThickness), int32(cellSize), int32(edgeThickness), shadowColor)
	}

	// Left edge (slight highlight)
	if !hasLeft {
		highlightColor := rl.Color{
			R: uint8(math.Min(float64(baseColor.R)+20, 255)),
			G: uint8(math.Min(float64(baseColor.G)+20, 255)),
			B: uint8(math.Min(float64(baseColor.B)+25, 255)),
			A: 150,
		}
		rl.DrawRectangle(int32(baseX), int32(baseY), int32(edgeThickness), int32(cellSize), highlightColor)
	}

	// Right edge shadow
	if !hasRight {
		shadowColor := rl.Color{
			R: uint8(float32(baseColor.R) * 0.7),
			G: uint8(float32(baseColor.G) * 0.7),
			B: uint8(float32(baseColor.B) * 0.7),
			A: 150,
		}
		rl.DrawRectangle(int32(baseX+cellSize-edgeThickness), int32(baseY), int32(edgeThickness), int32(cellSize), shadowColor)
	}

	// Add noise-based texture spots
	spotNoise := noise.Noise2D(float64(gx)*2+600, float64(gy)*2+600)
	if spotNoise > 0.5 {
		spotX := baseX + cellSize*0.3 + float32(spotNoise)*cellSize*0.4
		spotY := baseY + cellSize*0.3 + float32(noise.Noise2D(float64(gx)*2+700, float64(gy)*2+700))*cellSize*0.4
		spotSize := cellSize * 0.15

		spotColor := rl.Color{
			R: uint8(float32(baseColor.R) * 0.8),
			G: uint8(float32(baseColor.G) * 0.8),
			B: uint8(float32(baseColor.B) * 0.8),
			A: 100,
		}
		rl.DrawCircle(int32(spotX), int32(spotY), spotSize, spotColor)
	}
}

// Unload frees resources.
func (r *TerrainRenderer) Unload() {
	// Nothing to unload
}

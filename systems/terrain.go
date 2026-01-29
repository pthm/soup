package systems

import (
	"math"
)

// TerrainCell represents the type of terrain in a cell.
type TerrainCell uint8

const (
	TerrainEmpty TerrainCell = iota
	TerrainRock              // Solid, dark
	TerrainCoral             // Solid, colored
)

const (
	// Target cell size in pixels (2px for high resolution terrain)
	terrainCellSize = 2.0
)

// TerrainSystem manages procedural terrain with collision detection.
type TerrainSystem struct {
	grid          [][]TerrainCell
	cellSize      float32 // Square cells
	width         float32
	height        float32
	gridWidth     int
	gridHeight    int
	occluderCache []Occluder
	noise         *PerlinNoise
}

// NewTerrainSystem creates a new terrain system and generates terrain.
func NewTerrainSystem(screenWidth, screenHeight float32, seed int64) *TerrainSystem {
	// Calculate grid dimensions based on screen size and target cell size
	gridWidth := int(screenWidth / terrainCellSize)
	gridHeight := int(screenHeight / terrainCellSize)

	// Allocate the grid as a slice of slices
	grid := make([][]TerrainCell, gridHeight)
	for y := range grid {
		grid[y] = make([]TerrainCell, gridWidth)
	}

	t := &TerrainSystem{
		grid:       grid,
		cellSize:   terrainCellSize,
		width:      screenWidth,
		height:     screenHeight,
		gridWidth:  gridWidth,
		gridHeight: gridHeight,
		noise:      NewPerlinNoise(seed),
	}
	t.Generate(seed)
	t.buildOccluderCache()
	return t
}

// Generate creates procedural terrain using Perlin noise.
func (t *TerrainSystem) Generate(seed int64) {
	// Clear grid
	for y := 0; y < t.gridHeight; y++ {
		for x := 0; x < t.gridWidth; x++ {
			t.grid[y][x] = TerrainEmpty
		}
	}

	// 1. Sea floor: 1D Perlin along X, fills bottom 10-20%
	t.generateSeaFloor()

	// 2. Floating islands: 2D Perlin (threshold > 0.55), Y range 150-550px
	t.generateFloatingIslands()

	// 3. Coral outcrops: High-freq Perlin, small features near floor
	t.generateCoralOutcrops()

	// 4. Cave carving: Inverse Perlin pass removes cells for connectivity
	t.carveCaves()

	// Ensure edges remain open for organism movement
	t.clearEdges()
}

// generateSeaFloor creates the bottom terrain layer.
func (t *TerrainSystem) generateSeaFloor() {
	const noiseScale = 0.08

	for x := 0; x < t.gridWidth; x++ {
		// Sample 1D noise along X
		noiseVal := t.noise.Noise2D(float64(x)*noiseScale, 0)
		// Map noise [-1,1] to height variation [10%, 20%] of grid height
		heightRatio := 0.10 + (noiseVal+1)*0.05 // 10-20%
		floorHeight := int(float64(t.gridHeight) * heightRatio)

		for y := t.gridHeight - 1; y >= t.gridHeight-floorHeight; y-- {
			// Deeper cells are rock, shallow cells can be coral
			if y > t.gridHeight-floorHeight/2 {
				t.grid[y][x] = TerrainRock
			} else {
				// Random coral patches on top of floor
				coralNoise := t.noise.Noise2D(float64(x)*0.2, float64(y)*0.2+100)
				if coralNoise > 0.3 {
					t.grid[y][x] = TerrainCoral
				} else {
					t.grid[y][x] = TerrainRock
				}
			}
		}
	}
}

// generateFloatingIslands creates islands in the mid-water region.
func (t *TerrainSystem) generateFloatingIslands() {
	const noiseScale = 0.06
	const threshold = 0.55

	// Y range in grid cells: approximately 150-550px mapped to grid
	minY := int(150 / t.cellSize)
	maxY := int(550 / t.cellSize)
	if minY < 2 {
		minY = 2
	}
	if maxY >= t.gridHeight-2 {
		maxY = t.gridHeight - 3
	}

	for y := minY; y <= maxY; y++ {
		for x := 0; x < t.gridWidth; x++ {
			// Sample 2D Perlin noise
			noiseVal := t.noise.Noise2D(float64(x)*noiseScale, float64(y)*noiseScale+50)

			// Higher threshold creates smaller, more separated islands
			if noiseVal > threshold {
				// Make islands more rock-like
				t.grid[y][x] = TerrainRock
			}
		}
	}
}

// generateCoralOutcrops adds small coral features near the sea floor.
func (t *TerrainSystem) generateCoralOutcrops() {
	const noiseScale = 0.15 // Higher frequency
	const threshold = 0.6

	// Only in bottom 30% of screen
	startY := t.gridHeight * 7 / 10

	for y := startY; y < t.gridHeight; y++ {
		for x := 0; x < t.gridWidth; x++ {
			if t.grid[y][x] != TerrainEmpty {
				continue // Already has terrain
			}

			// High-frequency noise for small features
			noiseVal := t.noise.Noise2D(float64(x)*noiseScale+200, float64(y)*noiseScale)

			if noiseVal > threshold {
				t.grid[y][x] = TerrainCoral
			}
		}
	}
}

// carveCaves removes some terrain cells to create caves and passages.
func (t *TerrainSystem) carveCaves() {
	const noiseScale = 0.1
	const threshold = 0.65

	for y := 0; y < t.gridHeight; y++ {
		for x := 0; x < t.gridWidth; x++ {
			if t.grid[y][x] == TerrainEmpty {
				continue
			}

			// Inverse Perlin - high values carve out caves
			noiseVal := t.noise.Noise2D(float64(x)*noiseScale+300, float64(y)*noiseScale+300)

			if noiseVal > threshold {
				t.grid[y][x] = TerrainEmpty
			}
		}
	}
}

// clearEdges ensures screen edges have some open space for organism movement.
func (t *TerrainSystem) clearEdges() {
	// Clear top 2 rows (but not bottom - keep seabed intact)
	for y := 0; y < 2; y++ {
		for x := 0; x < t.gridWidth; x++ {
			t.grid[y][x] = TerrainEmpty
		}
	}

	// Clear left and right edges only in the upper portion (above seabed)
	// Don't clear the bottom 15% where the seabed is
	seabedStartY := t.gridHeight * 85 / 100
	for y := 0; y < seabedStartY; y++ {
		for x := 0; x < 2; x++ {
			t.grid[y][x] = TerrainEmpty
		}
		for x := t.gridWidth - 2; x < t.gridWidth; x++ {
			t.grid[y][x] = TerrainEmpty
		}
	}
}

// buildOccluderCache creates occluders for shadow casting.
func (t *TerrainSystem) buildOccluderCache() {
	t.occluderCache = t.occluderCache[:0]

	for y := 0; y < t.gridHeight; y++ {
		for x := 0; x < t.gridWidth; x++ {
			if t.grid[y][x] == TerrainEmpty {
				continue
			}

			// Create an occluder for each solid cell (full density for solid terrain)
			t.occluderCache = append(t.occluderCache, Occluder{
				X:       float32(x) * t.cellSize,
				Y:       float32(y) * t.cellSize,
				Width:   t.cellSize,
				Height:  t.cellSize,
				Density: 1.0,
			})
		}
	}
}

// GetOccluders returns cached occluders for shadow map integration.
func (t *TerrainSystem) GetOccluders() []Occluder {
	return t.occluderCache
}

// IsSolid returns true if the world position is inside solid terrain.
func (t *TerrainSystem) IsSolid(x, y float32) bool {
	gx := int(x / t.cellSize)
	gy := int(y / t.cellSize)

	if gx < 0 || gx >= t.gridWidth || gy < 0 || gy >= t.gridHeight {
		return false
	}

	return t.grid[gy][gx] != TerrainEmpty
}

// GetCell returns the terrain cell type at the given world position.
func (t *TerrainSystem) GetCell(x, y float32) TerrainCell {
	gx := int(x / t.cellSize)
	gy := int(y / t.cellSize)

	if gx < 0 || gx >= t.gridWidth || gy < 0 || gy >= t.gridHeight {
		return TerrainEmpty
	}

	return t.grid[gy][gx]
}

// CheckCircleCollision returns true if a circle intersects solid terrain.
func (t *TerrainSystem) CheckCircleCollision(x, y, radius float32) bool {
	// Check grid cells that the circle could overlap
	minGX := int((x - radius) / t.cellSize)
	maxGX := int((x + radius) / t.cellSize)
	minGY := int((y - radius) / t.cellSize)
	maxGY := int((y + radius) / t.cellSize)

	// Clamp to grid bounds
	if minGX < 0 {
		minGX = 0
	}
	if maxGX >= t.gridWidth {
		maxGX = t.gridWidth - 1
	}
	if minGY < 0 {
		minGY = 0
	}
	if maxGY >= t.gridHeight {
		maxGY = t.gridHeight - 1
	}

	radiusSq := radius * radius

	for gy := minGY; gy <= maxGY; gy++ {
		for gx := minGX; gx <= maxGX; gx++ {
			if t.grid[gy][gx] == TerrainEmpty {
				continue
			}

			// Check circle vs AABB collision
			// Find closest point on cell to circle center
			cellMinX := float32(gx) * t.cellSize
			cellMaxX := cellMinX + t.cellSize
			cellMinY := float32(gy) * t.cellSize
			cellMaxY := cellMinY + t.cellSize

			closestX := x
			if x < cellMinX {
				closestX = cellMinX
			} else if x > cellMaxX {
				closestX = cellMaxX
			}

			closestY := y
			if y < cellMinY {
				closestY = cellMinY
			} else if y > cellMaxY {
				closestY = cellMaxY
			}

			dx := x - closestX
			dy := y - closestY
			distSq := dx*dx + dy*dy

			if distSq < radiusSq {
				return true
			}
		}
	}

	return false
}

// FindNearestOpen finds the nearest open position and returns the collision normal.
func (t *TerrainSystem) FindNearestOpen(x, y, radius float32) (openX, openY, normalX, normalY float32) {
	// Start from current position
	openX, openY = x, y

	// Find the gradient (direction away from terrain)
	gx, gy := t.GetGradient(x, y)

	// If no gradient, use a default
	if gx == 0 && gy == 0 {
		gx, gy = 0, -1 // Push up by default
	}

	// Push out along gradient
	pushDist := radius + t.cellSize*0.5
	openX = x + gx*pushDist
	openY = y + gy*pushDist

	// Ensure we're actually in open space
	for i := 0; i < 5 && t.CheckCircleCollision(openX, openY, radius); i++ {
		openX += gx * t.cellSize
		openY += gy * t.cellSize
	}

	normalX = gx
	normalY = gy

	return
}

// GetGradient returns the direction away from the nearest solid terrain.
func (t *TerrainSystem) GetGradient(x, y float32) (gx, gy float32) {
	// Sample solid distances in 4 cardinal directions
	sampleDist := t.cellSize * 2

	leftDist := t.distanceToSolidInDirection(x, y, -1, 0, sampleDist*3)
	rightDist := t.distanceToSolidInDirection(x, y, 1, 0, sampleDist*3)
	upDist := t.distanceToSolidInDirection(x, y, 0, -1, sampleDist*3)
	downDist := t.distanceToSolidInDirection(x, y, 0, 1, sampleDist*3)

	// Gradient points toward more open space
	gx = rightDist - leftDist
	gy = downDist - upDist

	// Normalize
	mag := float32(math.Sqrt(float64(gx*gx + gy*gy)))
	if mag > 0.001 {
		gx /= mag
		gy /= mag
	}

	return
}

// distanceToSolidInDirection returns the distance to solid terrain in a direction.
func (t *TerrainSystem) distanceToSolidInDirection(x, y, dx, dy, maxDist float32) float32 {
	step := t.cellSize * 0.5
	dist := float32(0)

	for dist < maxDist {
		dist += step
		testX := x + dx*dist
		testY := y + dy*dist

		if t.IsSolid(testX, testY) {
			return dist
		}
	}

	return maxDist
}

// DistanceToSolid returns the approximate distance to the nearest solid cell.
func (t *TerrainSystem) DistanceToSolid(x, y float32) float32 {
	// Quick check if we're inside solid
	if t.IsSolid(x, y) {
		return 0
	}

	// Sample in 8 directions
	minDist := t.cellSize * 10 // Default max
	directions := [][2]float32{
		{1, 0}, {-1, 0}, {0, 1}, {0, -1},
		{0.707, 0.707}, {-0.707, 0.707}, {0.707, -0.707}, {-0.707, -0.707},
	}

	for _, dir := range directions {
		d := t.distanceToSolidInDirection(x, y, dir[0], dir[1], minDist)
		if d < minDist {
			minDist = d
		}
	}

	return minDist
}

// HasLineOfSight returns true if there is no solid terrain between two points.
// Uses a simple raycast with steps smaller than cell size.
func (t *TerrainSystem) HasLineOfSight(x1, y1, x2, y2 float32) bool {
	dx := x2 - x1
	dy := y2 - y1
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	if dist < 0.001 {
		return true // Same point
	}

	// Step size should be smaller than cell size to avoid missing walls
	stepSize := t.cellSize * 0.4
	steps := int(dist/stepSize) + 1

	// Normalize direction
	dx /= dist
	dy /= dist

	// Check points along the line (skip start and end points)
	for i := 1; i < steps; i++ {
		checkX := x1 + dx*float32(i)*stepSize
		checkY := y1 + dy*float32(i)*stepSize

		if t.IsSolid(checkX, checkY) {
			return false
		}
	}

	return true
}

// CellSize returns the size of a terrain cell (square) in world coordinates.
func (t *TerrainSystem) CellSize() float32 {
	return t.cellSize
}

// CellWidth returns the width of a terrain cell in world coordinates.
func (t *TerrainSystem) CellWidth() float32 {
	return t.cellSize
}

// CellHeight returns the height of a terrain cell in world coordinates.
func (t *TerrainSystem) CellHeight() float32 {
	return t.cellSize
}

// Grid returns a reference to the terrain grid for rendering.
func (t *TerrainSystem) Grid() [][]TerrainCell {
	return t.grid
}

// GridWidth returns the terrain grid width.
func (t *TerrainSystem) GridWidth() int {
	return t.gridWidth
}

// GridHeight returns the terrain grid height.
func (t *TerrainSystem) GridHeight() int {
	return t.gridHeight
}

// Noise returns the noise generator for rendering effects.
func (t *TerrainSystem) Noise() *PerlinNoise {
	return t.noise
}

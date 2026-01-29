package systems

import "github.com/pthm-cable/soup/components"

const (
	spatialGridSize = 64 // 64x64 grid cells
)

// SpatialGrid provides O(1) neighbor lookups for organisms.
type SpatialGrid struct {
	cellWidth  float32
	cellHeight float32
	width      float32
	height     float32

	// Grid cells containing indices into position arrays
	floraGrid [spatialGridSize][spatialGridSize][]int
	faunaGrid [spatialGridSize][spatialGridSize][]int

	// Pre-allocated result buffers to avoid allocations in hot path
	floraBuf []int
	faunaBuf []int
}

// NewSpatialGrid creates a new spatial grid.
func NewSpatialGrid(width, height float32) *SpatialGrid {
	return &SpatialGrid{
		cellWidth:  width / spatialGridSize,
		cellHeight: height / spatialGridSize,
		width:      width,
		height:     height,
		floraBuf:   make([]int, 0, 128),
		faunaBuf:   make([]int, 0, 128),
	}
}

// Update rebuilds the grid with current positions.
func (sg *SpatialGrid) Update(floraPos, faunaPos []components.Position) {
	// Clear grid
	for y := 0; y < spatialGridSize; y++ {
		for x := 0; x < spatialGridSize; x++ {
			sg.floraGrid[y][x] = sg.floraGrid[y][x][:0]
			sg.faunaGrid[y][x] = sg.faunaGrid[y][x][:0]
		}
	}

	// Insert flora
	for i := range floraPos {
		gx, gy := sg.worldToGrid(floraPos[i].X, floraPos[i].Y)
		sg.floraGrid[gy][gx] = append(sg.floraGrid[gy][gx], i)
	}

	// Insert fauna
	for i := range faunaPos {
		gx, gy := sg.worldToGrid(faunaPos[i].X, faunaPos[i].Y)
		sg.faunaGrid[gy][gx] = append(sg.faunaGrid[gy][gx], i)
	}
}

func (sg *SpatialGrid) worldToGrid(x, y float32) (int, int) {
	gx := int(x / sg.cellWidth)
	gy := int(y / sg.cellHeight)

	// Clamp to grid bounds
	if gx < 0 {
		gx = 0
	}
	if gx >= spatialGridSize {
		gx = spatialGridSize - 1
	}
	if gy < 0 {
		gy = 0
	}
	if gy >= spatialGridSize {
		gy = spatialGridSize - 1
	}

	return gx, gy
}

// GetNearbyFauna returns indices of fauna within radius of position.
func (sg *SpatialGrid) GetNearbyFauna(x, y, radius float32) []int {
	return sg.getNearby(x, y, radius, false)
}

func (sg *SpatialGrid) getNearby(x, y, radius float32, flora bool) []int {
	// Calculate grid cell range to check
	cellRadius := int(radius/sg.cellWidth) + 1

	cx, cy := sg.worldToGrid(x, y)

	minX := cx - cellRadius
	maxX := cx + cellRadius
	minY := cy - cellRadius
	maxY := cy + cellRadius

	// Clamp to grid bounds
	if minX < 0 {
		minX = 0
	}
	if maxX >= spatialGridSize {
		maxX = spatialGridSize - 1
	}
	if minY < 0 {
		minY = 0
	}
	if maxY >= spatialGridSize {
		maxY = spatialGridSize - 1
	}

	// Use pre-allocated buffer, reset to zero length
	var result *[]int
	if flora {
		sg.floraBuf = sg.floraBuf[:0]
		result = &sg.floraBuf
	} else {
		sg.faunaBuf = sg.faunaBuf[:0]
		result = &sg.faunaBuf
	}

	// Collect all indices from nearby cells
	for gy := minY; gy <= maxY; gy++ {
		for gx := minX; gx <= maxX; gx++ {
			if flora {
				*result = append(*result, sg.floraGrid[gy][gx]...)
			} else {
				*result = append(*result, sg.faunaGrid[gy][gx]...)
			}
		}
	}

	return *result
}

package systems

// NavGrid stores a navigation grid for A* pathfinding.
// Cells are marked as blocked (true) or open (false).
type NavGrid struct {
	cells    []bool  // true = blocked
	cellSize float32 // pixels per cell (16px)
	width    int     // grid width in cells
	height   int     // grid height in cells
}

// SizeClass categorizes organisms by collision radius for navigation grid inflation.
type SizeClass uint8

const (
	SizeSmall  SizeClass = iota // radius < 12px
	SizeMedium                   // radius 12-24px
	SizeLarge                    // radius > 24px
	NumSizeClasses
)

// NavGridCellSize is the navigation grid cell size in pixels.
const NavGridCellSize = 16.0

// GetSizeClass returns the appropriate size class for a given collision radius.
func GetSizeClass(radius float32) SizeClass {
	if radius < 12 {
		return SizeSmall
	}
	if radius < 24 {
		return SizeMedium
	}
	return SizeLarge
}

// NewNavGridFromTerrain creates a navigation grid from terrain, inflated by a radius.
// Inflation marks cells as blocked if terrain is within 'inflation' pixels.
func NewNavGridFromTerrain(terrain *TerrainSystem, inflation float32) *NavGrid {
	w := int(terrain.width / NavGridCellSize)
	h := int(terrain.height / NavGridCellSize)

	grid := &NavGrid{
		cells:    make([]bool, w*h),
		cellSize: NavGridCellSize,
		width:    w,
		height:   h,
	}

	// For each nav grid cell, check if terrain is nearby
	for gy := 0; gy < h; gy++ {
		for gx := 0; gx < w; gx++ {
			// Center of this nav cell in world coordinates
			centerX := (float32(gx) + 0.5) * NavGridCellSize
			centerY := (float32(gy) + 0.5) * NavGridCellSize

			// Check if any terrain is within inflation distance
			blocked := false

			// Convert to terrain grid coordinates
			tMinX := int((centerX - inflation) / terrain.cellSize)
			tMaxX := int((centerX + inflation) / terrain.cellSize)
			tMinY := int((centerY - inflation) / terrain.cellSize)
			tMaxY := int((centerY + inflation) / terrain.cellSize)

			// Clamp to terrain bounds
			if tMinX < 0 {
				tMinX = 0
			}
			if tMaxX >= terrain.gridWidth {
				tMaxX = terrain.gridWidth - 1
			}
			if tMinY < 0 {
				tMinY = 0
			}
			if tMaxY >= terrain.gridHeight {
				tMaxY = terrain.gridHeight - 1
			}

			// Check terrain cells in range
			for ty := tMinY; ty <= tMaxY && !blocked; ty++ {
				for tx := tMinX; tx <= tMaxX && !blocked; tx++ {
					if terrain.grid[ty][tx] != TerrainEmpty {
						// Check actual distance to terrain cell center
						tcX := (float32(tx) + 0.5) * terrain.cellSize
						tcY := (float32(ty) + 0.5) * terrain.cellSize
						dx := centerX - tcX
						dy := centerY - tcY
						distSq := dx*dx + dy*dy
						// Use slightly less than inflation to avoid edge cases
						if distSq < (inflation+terrain.cellSize)*(inflation+terrain.cellSize) {
							blocked = true
						}
					}
				}
			}

			// Also block cells too close to top/bottom bounds
			if centerY < inflation || centerY > terrain.height-inflation {
				blocked = true
			}

			grid.cells[gy*w+gx] = blocked
		}
	}

	return grid
}

// IsBlocked returns true if the given nav grid cell is blocked.
func (g *NavGrid) IsBlocked(gx, gy int) bool {
	if gx < 0 || gx >= g.width || gy < 0 || gy >= g.height {
		return true // Out of bounds is blocked
	}
	return g.cells[gy*g.width+gx]
}

// IsBlockedWorld returns true if the world position is in a blocked cell.
func (g *NavGrid) IsBlockedWorld(x, y float32) bool {
	gx := int(x / g.cellSize)
	gy := int(y / g.cellSize)
	return g.IsBlocked(gx, gy)
}

// WorldToGrid converts world coordinates to nav grid coordinates.
func (g *NavGrid) WorldToGrid(x, y float32) (gx, gy int) {
	gx = int(x / g.cellSize)
	gy = int(y / g.cellSize)
	return
}

// GridToWorld converts nav grid coordinates to world coordinates (cell center).
func (g *NavGrid) GridToWorld(gx, gy int) (x, y float32) {
	x = (float32(gx) + 0.5) * g.cellSize
	y = (float32(gy) + 0.5) * g.cellSize
	return
}

// NavGridSet holds navigation grids for different organism size classes.
type NavGridSet struct {
	grids [NumSizeClasses]*NavGrid
}

// NewNavGridSet creates navigation grids for all size classes from terrain.
func NewNavGridSet(terrain *TerrainSystem) *NavGridSet {
	set := &NavGridSet{}

	// Create grids with different inflation values for each size class
	// Small: 8px inflation (for organisms with radius < 12px)
	// Medium: 16px inflation (for organisms with radius 12-24px)
	// Large: 28px inflation (for organisms with radius > 24px)
	inflations := [NumSizeClasses]float32{8, 16, 28}

	for i := SizeClass(0); i < NumSizeClasses; i++ {
		set.grids[i] = NewNavGridFromTerrain(terrain, inflations[i])
	}

	return set
}

// GetGrid returns the navigation grid for a given size class.
func (s *NavGridSet) GetGrid(sc SizeClass) *NavGrid {
	if sc >= NumSizeClasses {
		sc = SizeLarge
	}
	return s.grids[sc]
}


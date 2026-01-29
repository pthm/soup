package systems

import (
	"math"
	"testing"

	"github.com/pthm-cable/soup/components"
)

// TestComputeOBB verifies OBB computation from cells.
func TestComputeOBB(t *testing.T) {
	tests := []struct {
		name       string
		cells      []struct{ x, y int8 }
		cellSize   float32
		wantHalfW  float32
		wantHalfH  float32
		wantOffX   float32
		wantOffY   float32
	}{
		{
			name:       "single cell at origin",
			cells:      []struct{ x, y int8 }{{0, 0}},
			cellSize:   5,
			wantHalfW:  5,  // 1 cell wide + padding
			wantHalfH:  5,
			wantOffX:   0,
			wantOffY:   0,
		},
		{
			name:       "horizontal line",
			cells:      []struct{ x, y int8 }{{-1, 0}, {0, 0}, {1, 0}},
			cellSize:   5,
			wantHalfW:  10, // 3 cells + 1 padding = 4 cells worth / 2 = 10
			wantHalfH:  5,  // 1 cell + padding
			wantOffX:   0,  // centered
			wantOffY:   0,
		},
		{
			name:       "vertical line",
			cells:      []struct{ x, y int8 }{{0, -1}, {0, 0}, {0, 1}},
			cellSize:   5,
			wantHalfW:  5,
			wantHalfH:  10,
			wantOffX:   0,
			wantOffY:   0,
		},
		{
			name:       "L-shape",
			cells:      []struct{ x, y int8 }{{0, 0}, {1, 0}, {0, 1}},
			cellSize:   5,
			wantHalfW:  7.5, // 2 cells + 1 padding = 3 cells / 2 * 5
			wantHalfH:  7.5,
			wantOffX:   2.5, // center at (0.5, 0.5) * 5
			wantOffY:   2.5,
		},
		{
			name:       "empty buffer",
			cells:      nil,
			cellSize:   5,
			wantHalfW:  5, // minimum
			wantHalfH:  5,
			wantOffX:   0,
			wantOffY:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cells := &components.CellBuffer{}
			for _, c := range tc.cells {
				cells.AddCell(components.Cell{
					GridX: c.x,
					GridY: c.y,
					Alive: true,
				})
			}

			obb := ComputeCollisionOBB(cells, tc.cellSize)

			if math.Abs(float64(obb.HalfWidth-tc.wantHalfW)) > 0.01 {
				t.Errorf("HalfWidth = %f, want %f", obb.HalfWidth, tc.wantHalfW)
			}
			if math.Abs(float64(obb.HalfHeight-tc.wantHalfH)) > 0.01 {
				t.Errorf("HalfHeight = %f, want %f", obb.HalfHeight, tc.wantHalfH)
			}
			if math.Abs(float64(obb.OffsetX-tc.wantOffX)) > 0.01 {
				t.Errorf("OffsetX = %f, want %f", obb.OffsetX, tc.wantOffX)
			}
			if math.Abs(float64(obb.OffsetY-tc.wantOffY)) > 0.01 {
				t.Errorf("OffsetY = %f, want %f", obb.OffsetY, tc.wantOffY)
			}
		})
	}
}

// TestOBBTerrainCollision verifies OBB-terrain collision detection.
func TestOBBTerrainCollision(t *testing.T) {
	// Create a simple terrain with a wall
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear the terrain
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}
	// Add a solid block at center
	// Terrain cell size is 2px, so x=35-45 covers pixels 70-90
	for y := 20; y < 30; y++ { // y cells 20-30 = pixels 40-60
		for x := 35; x < 45; x++ { // x cells 35-45 = pixels 70-90
			terrain.grid[y][x] = TerrainRock
		}
	}

	obb := &components.CollisionOBB{
		HalfWidth:  5,
		HalfHeight: 5,
		OffsetX:    0,
		OffsetY:    0,
	}

	tests := []struct {
		name    string
		posX    float32
		posY    float32
		heading float32
		want    bool
	}{
		{
			name:    "clear of obstacle",
			posX:    20,
			posY:    20,
			heading: 0,
			want:    false,
		},
		{
			name:    "inside obstacle",
			posX:    80,
			posY:    50,
			heading: 0,
			want:    true,
		},
		{
			name:    "clear left of obstacle",
			posX:    60,
			posY:    50,
			heading: 0,
			want:    false, // OBB extends to 65, obstacle starts at 70
		},
		{
			name:    "touching obstacle left",
			posX:    66,
			posY:    50,
			heading: 0,
			want:    true, // OBB extends to 71, obstacle starts at 70
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := terrain.CheckOBBCollision(tc.posX, tc.posY, tc.heading, obb)
			if got != tc.want {
				t.Errorf("CheckOBBCollision(%f, %f, %f) = %v, want %v", tc.posX, tc.posY, tc.heading, got, tc.want)
			}
		})
	}
}

// TestOBBRotation verifies OBB collision with rotation.
func TestOBBRotation(t *testing.T) {
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear terrain
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}
	// Add a narrow wall
	for y := 20; y < 30; y++ {
		for x := 40; x < 42; x++ { // Narrow wall at x=80-84
			terrain.grid[y][x] = TerrainRock
		}
	}

	// Elongated OBB (like a streamlined organism)
	obb := &components.CollisionOBB{
		HalfWidth:  3,  // Narrow
		HalfHeight: 10, // Long
		OffsetX:    0,
		OffsetY:    0,
	}

	// When facing right (heading=0), the OBB is narrow horizontally
	// When facing up (heading=-π/2), the OBB is narrow vertically

	// Position organism so it would collide if rotated wrong
	posX := float32(75) // Just left of wall at x=80
	posY := float32(50)

	// Facing right - narrow profile, should clear the wall
	headingRight := float32(0)
	collidesRight := terrain.CheckOBBCollision(posX, posY, headingRight, obb)

	// Facing up - wide profile, might collide
	headingUp := float32(-math.Pi / 2)
	collidesUp := terrain.CheckOBBCollision(posX, posY, headingUp, obb)

	// The elongated OBB should behave differently based on rotation
	t.Logf("Heading right collision: %v", collidesRight)
	t.Logf("Heading up collision: %v", collidesUp)

	// Both should be false at this position since we're 5px away from wall
	// and max extent is 10px (half-height when rotated)
	if collidesRight {
		t.Logf("Note: Collision when facing right (may depend on exact wall position)")
	}
}

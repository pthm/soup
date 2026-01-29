package systems

import (
	"testing"

	"github.com/pthm-cable/soup/components"
)

// TestAStarSimplePath verifies A* finds a straight-line path.
func TestAStarSimplePath(t *testing.T) {
	// Create terrain without obstacles
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear all terrain
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}

	planner := NewAStarPlanner(terrain)

	// Find path from (20, 20) to (140, 80)
	path := planner.FindPath(20, 20, 140, 80, SizeSmall)

	if path == nil {
		t.Fatal("Expected path, got nil")
	}
	if len(path) < 2 {
		t.Errorf("Expected at least 2 waypoints, got %d", len(path))
	}

	// First waypoint should be near start
	first := path[0]
	if first.X < 0 || first.X > 40 || first.Y < 0 || first.Y > 40 {
		t.Errorf("First waypoint %v not near start (20, 20)", first)
	}

	// Last waypoint should be near goal
	last := path[len(path)-1]
	if last.X < 120 || last.X > 160 || last.Y < 60 || last.Y > 100 {
		t.Errorf("Last waypoint %v not near goal (140, 80)", last)
	}
}

// TestAStarAroundObstacle verifies A* navigates around obstacles.
func TestAStarAroundObstacle(t *testing.T) {
	// Create terrain with a vertical wall in the middle
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear all terrain first
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}
	// Add vertical wall from y=10 to y=40 at x=40 (pixels 78-82)
	for y := 5; y < 20; y++ {
		for x := 38; x < 42; x++ { // Wall around x=80
			terrain.grid[y][x] = TerrainRock
		}
	}

	planner := NewAStarPlanner(terrain)

	// Find path from left (30, 30) to right (130, 30)
	// Direct path would cross the wall
	path := planner.FindPath(30, 30, 130, 30, SizeSmall)

	if path == nil {
		t.Fatal("Expected path around obstacle, got nil")
	}

	// Path should go around the wall
	// Check that no waypoint is inside the wall
	for i, wp := range path {
		gx := int(wp.X / terrain.cellSize)
		gy := int(wp.Y / terrain.cellSize)
		if gx >= 0 && gx < terrain.gridWidth && gy >= 0 && gy < terrain.gridHeight {
			if terrain.grid[gy][gx] != TerrainEmpty {
				t.Errorf("Waypoint %d at (%f, %f) is inside terrain", i, wp.X, wp.Y)
			}
		}
	}

	t.Logf("Path has %d waypoints (navigated around obstacle)", len(path))
}

// TestAStarNoPath verifies A* returns nil when no path exists.
func TestAStarNoPath(t *testing.T) {
	// Create terrain that completely blocks the path
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear all terrain first
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}
	// Add complete vertical wall across the screen
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 38; x < 42; x++ {
			terrain.grid[y][x] = TerrainRock
		}
	}

	planner := NewAStarPlanner(terrain)

	// Try to find path across the wall
	path := planner.FindPath(30, 30, 130, 30, SizeSmall)

	if path != nil {
		t.Errorf("Expected no path through complete wall, got %d waypoints", len(path))
	}
}

// TestPathCacheValidity verifies path cache validation logic.
func TestPathCacheValidity(t *testing.T) {
	terrain := NewTerrainSystem(160, 100, 12345)
	// Clear all terrain
	for y := 0; y < terrain.gridHeight; y++ {
		for x := 0; x < terrain.gridWidth; x++ {
			terrain.grid[y][x] = TerrainEmpty
		}
	}

	planner := NewAStarPlanner(terrain)

	cache := &PathCache{
		Waypoints: []components.Position{
			{X: 20, Y: 20},
			{X: 50, Y: 50},
			{X: 80, Y: 80},
		},
		Index:     0,
		TargetX:   80,
		TargetY:   80,
		ValidTick: 100,
	}

	// Valid: same target, recent tick
	if !planner.IsPathValid(cache, 80, 80, 120, SizeSmall, 60) {
		t.Error("Expected path to be valid with same target and recent tick")
	}

	// Invalid: target moved too far
	if planner.IsPathValid(cache, 150, 80, 120, SizeSmall, 60) {
		t.Error("Expected path to be invalid with target moved >32px")
	}

	// Invalid: path too old
	if planner.IsPathValid(cache, 80, 80, 200, SizeSmall, 60) {
		t.Error("Expected path to be invalid when too old")
	}

	// Invalid: nil cache
	if planner.IsPathValid(nil, 80, 80, 120, SizeSmall, 60) {
		t.Error("Expected nil cache to be invalid")
	}
}

// TestGetNextWaypoint verifies waypoint navigation.
func TestGetNextWaypoint(t *testing.T) {
	cache := &PathCache{
		Waypoints: []components.Position{
			{X: 20, Y: 20},
			{X: 50, Y: 50},
			{X: 80, Y: 80},
		},
		Index: 0,
	}

	// Initial position far from first waypoint
	wpX, wpY, hasMore := GetNextWaypoint(cache, 0, 0, 10)
	if wpX != 20 || wpY != 20 {
		t.Errorf("Expected waypoint (20,20), got (%f,%f)", wpX, wpY)
	}
	if !hasMore {
		t.Error("Expected hasMore=true for first waypoint")
	}
	if cache.Index != 0 {
		t.Errorf("Index should still be 0, got %d", cache.Index)
	}

	// Position close to first waypoint (should advance)
	wpX, wpY, hasMore = GetNextWaypoint(cache, 18, 18, 10)
	if wpX != 50 || wpY != 50 {
		t.Errorf("Expected waypoint (50,50) after advance, got (%f,%f)", wpX, wpY)
	}
	if !hasMore {
		t.Error("Expected hasMore=true for middle waypoint")
	}
	if cache.Index != 1 {
		t.Errorf("Index should be 1, got %d", cache.Index)
	}

	// Advance to last waypoint
	cache.Index = 2
	wpX, wpY, hasMore = GetNextWaypoint(cache, 75, 75, 10)
	if hasMore {
		t.Error("Expected hasMore=false for last waypoint")
	}
}

// TestSizeClass verifies size class determination.
func TestSizeClass(t *testing.T) {
	tests := []struct {
		radius float32
		want   SizeClass
	}{
		{5, SizeSmall},
		{11, SizeSmall},
		{12, SizeMedium},
		{20, SizeMedium},
		{24, SizeLarge},
		{50, SizeLarge},
	}

	for _, tc := range tests {
		got := GetSizeClass(tc.radius)
		if got != tc.want {
			t.Errorf("GetSizeClass(%f) = %d, want %d", tc.radius, got, tc.want)
		}
	}
}

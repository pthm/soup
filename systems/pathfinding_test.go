package systems

import (
	"math"
	"testing"
)

// TestNavigateNoTerrain verifies pathfinding follows desire vector when no terrain exists.
func TestNavigateNoTerrain(t *testing.T) {
	pf := NewPathfinder(nil) // No terrain

	tests := []struct {
		name           string
		desireAngle    float32
		desireDistance float32
		expectTurnSign int  // -1, 0, or +1
		expectThrust   bool // true if thrust should be > 0
	}{
		{
			name:           "forward desire",
			desireAngle:    0,
			desireDistance: 1.0,
			expectTurnSign: 0,
			expectThrust:   true,
		},
		{
			name:           "turn right desire",
			desireAngle:    math.Pi / 2,
			desireDistance: 0.8,
			expectTurnSign: +1,
			expectThrust:   true, // With no terrain, thrust passes through directly
		},
		{
			name:           "turn left desire",
			desireAngle:    -math.Pi / 2,
			desireDistance: 0.8,
			expectTurnSign: -1,
			expectThrust:   true, // With no terrain, thrust passes through directly
		},
		{
			name:           "slight right turn with thrust",
			desireAngle:    math.Pi / 6, // 30 degrees
			desireDistance: 0.8,
			expectTurnSign: +1,
			expectThrust:   true, // cos(30°) ≈ 0.87, so thrust is possible
		},
		{
			name:           "no movement",
			desireAngle:    0,
			desireDistance: 0,
			expectTurnSign: 0,
			expectThrust:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := pf.Navigate(
				100, 100, // position
				0,        // heading (facing right)
				tc.desireAngle,
				tc.desireDistance,
				0, 0, // no flow
				5,    // organism radius
			)

			// Check turn direction
			turnSign := 0
			if result.Turn > 0.01 {
				turnSign = 1
			} else if result.Turn < -0.01 {
				turnSign = -1
			}

			if turnSign != tc.expectTurnSign {
				t.Errorf("Turn sign = %d, want %d (Turn = %f)", turnSign, tc.expectTurnSign, result.Turn)
			}

			// Check thrust
			hasThrust := result.Thrust > 0.01
			if hasThrust != tc.expectThrust {
				t.Errorf("Thrust > 0 = %v, want %v (Thrust = %f)", hasThrust, tc.expectThrust, result.Thrust)
			}
		})
	}
}

// TestNavigateSimple verifies the simple navigation mode.
func TestNavigateSimple(t *testing.T) {
	pf := NewPathfinder(nil)

	result := pf.NavigateSimple(math.Pi/4, 0.75)

	// Turn should be angle / π (normalized to [-1, 1])
	expectedTurn := float32(math.Pi/4) / math.Pi
	if math.Abs(float64(result.Turn-expectedTurn)) > 0.001 {
		t.Errorf("Turn = %f, want %f", result.Turn, expectedTurn)
	}

	// Thrust should equal desire distance
	if math.Abs(float64(result.Thrust-0.75)) > 0.001 {
		t.Errorf("Thrust = %f, want 0.75", result.Thrust)
	}
}

// TestNavigateAroundObstacle verifies pathfinding steers around blocked paths.
func TestNavigateAroundObstacle(t *testing.T) {
	// Create mock terrain with a wall at y > 100
	terrain := &mockTerrain{
		solidY: 100,
	}

	pf := NewPathfinder(terrain)

	// Organism at y=80, heading down toward wall
	// Desired direction is blocked
	result := pf.Navigate(
		100, 80,   // position above wall
		math.Pi/2, // heading down (toward solid at y>100)
		0,         // desire straight ahead (into wall)
		1.0,       // full urgency
		0, 0,      // no flow
		5,         // radius
	)

	// With context steering, the organism should:
	// 1. Detect that straight ahead is blocked
	// 2. Find an alternative clear direction
	// 3. Either turn or reduce thrust

	// The thrust should be reduced since direct path is blocked
	if result.Thrust > 0.8 {
		t.Errorf("Expected reduced thrust when path blocked, got %f", result.Thrust)
	}
}

// TestNavigateClearPath verifies full thrust when path is clear.
func TestNavigateClearPath(t *testing.T) {
	// Create mock terrain with wall far below
	terrain := &mockTerrain{
		solidY: 500, // Wall is far away
	}

	pf := NewPathfinder(terrain)

	// Organism moving horizontally, wall is far below
	result := pf.Navigate(
		100, 100, // position
		0,        // heading right (away from wall)
		0,        // desire straight ahead
		1.0,      // full urgency
		0, 0,     // no flow
		5,        // radius
	)

	// Path is clear, should have full thrust
	if result.Thrust < 0.9 {
		t.Errorf("Expected high thrust on clear path, got %f", result.Thrust)
	}

	// Should not turn significantly
	if math.Abs(float64(result.Turn)) > 0.1 {
		t.Errorf("Expected minimal turn on clear path, got %f", result.Turn)
	}
}

// TestPathfindingResult verifies result clamping.
func TestPathfindingResult(t *testing.T) {
	pf := NewPathfinder(nil)

	// Test extreme angles
	result := pf.NavigateSimple(math.Pi*2, 1.0) // Way beyond [-π, π]

	if result.Turn < -1.0 || result.Turn > 1.0 {
		t.Errorf("Turn not clamped: %f", result.Turn)
	}

	if result.Thrust < 0 || result.Thrust > 1.0 {
		t.Errorf("Thrust not clamped: %f", result.Thrust)
	}
}

// mockTerrain implements TerrainQuerier for testing.
type mockTerrain struct {
	solidY float32 // Y value above which terrain is solid
}

// Ensure mockTerrain implements TerrainQuerier.
var _ TerrainQuerier = (*mockTerrain)(nil)

func (m *mockTerrain) IsSolid(x, y float32) bool {
	return y > m.solidY
}

func (m *mockTerrain) DistanceToSolid(x, y float32) float32 {
	if y > m.solidY {
		return 0
	}
	return m.solidY - y
}

func (m *mockTerrain) GetGradient(x, y float32) (gx, gy float32) {
	// Gradient points away from solid (upward = negative Y)
	return 0, -1
}

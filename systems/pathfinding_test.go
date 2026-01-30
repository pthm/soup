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
			expectThrust:   true, // cos(30) ~= 0.87, so thrust is possible
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

	// Turn should be angle / pi (normalized to [-1, 1])
	expectedTurn := float32(math.Pi/4) / math.Pi
	if math.Abs(float64(result.Turn-expectedTurn)) > 0.001 {
		t.Errorf("Turn = %f, want %f", result.Turn, expectedTurn)
	}

	// Thrust should equal desire distance
	if math.Abs(float64(result.Thrust-0.75)) > 0.001 {
		t.Errorf("Thrust = %f, want 0.75", result.Thrust)
	}
}

// TestNavigateAroundObstacle verifies pathfinding steers around blocked paths using repulsion.
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

	// With potential field navigation, the organism should:
	// 1. Have attraction toward target (into wall)
	// 2. Have repulsion from the wall pushing it back
	// 3. Either turn or reduce thrust

	// The repulsion should create a turn or reduce thrust
	// Either behavior is acceptable for obstacle avoidance
	if result.Thrust > 0.95 && math.Abs(float64(result.Turn)) < 0.01 {
		t.Errorf("Expected reduced thrust or turn when near obstacle, got thrust=%f, turn=%f", result.Thrust, result.Turn)
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

	// Path is clear, should have high thrust
	if result.Thrust < 0.5 {
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
	result := pf.NavigateSimple(math.Pi*2, 1.0) // Way beyond [-pi, pi]

	if result.Turn < -1.0 || result.Turn > 1.0 {
		t.Errorf("Turn not clamped: %f", result.Turn)
	}

	if result.Thrust < 0 || result.Thrust > 1.0 {
		t.Errorf("Thrust not clamped: %f", result.Thrust)
	}
}

// TestRepulsionFalloff verifies closer obstacles produce stronger repulsion.
func TestRepulsionFalloff(t *testing.T) {
	// Create mock terrain with wall at y > 100
	terrain := &mockTerrain{
		solidY: 100,
	}

	pf := NewPathfinder(terrain)

	// Organism close to wall (y=90, only 10px from solid)
	closeResult := pf.Navigate(
		100, 90,   // position very close to wall
		math.Pi/2, // heading down (toward wall)
		0,         // desire straight ahead (into wall)
		1.0,       // full urgency
		0, 0,      // no flow
		5,         // radius
	)

	// Organism farther from wall (y=60, 40px from solid)
	farResult := pf.Navigate(
		100, 60,   // position farther from wall
		math.Pi/2, // heading down (toward wall)
		0,         // desire straight ahead (into wall)
		1.0,       // full urgency
		0, 0,      // no flow
		5,         // radius
	)

	// Close to wall should have less forward thrust or more turn
	// (repulsion fighting attraction more strongly)
	closeForwardness := closeResult.Thrust * float32(math.Cos(float64(closeResult.Turn*math.Pi)))
	farForwardness := farResult.Thrust * float32(math.Cos(float64(farResult.Turn*math.Pi)))

	// The closer organism should have less effective forward movement toward the wall
	if closeForwardness > farForwardness {
		t.Errorf("Expected stronger repulsion when closer: close forwardness=%f, far forwardness=%f",
			closeForwardness, farForwardness)
	}
}

// TestAttractionDeadzone verifies attraction tapers when very close to target.
func TestAttractionDeadzone(t *testing.T) {
	pf := NewPathfinder(nil)

	// Test with small desire distance (close to target)
	smallResult := pf.Navigate(
		100, 100, // position
		0,        // heading
		0,        // desire straight ahead
		0.02,     // very small desire distance (near deadzone)
		0, 0,     // no flow
		5,        // radius
	)

	// Test with larger desire distance
	largeResult := pf.Navigate(
		100, 100, // position
		0,        // heading
		0,        // desire straight ahead
		0.5,      // moderate desire distance
		0, 0,     // no flow
		5,        // radius
	)

	// Small desire distance should produce less thrust
	// (Thrust is scaled by desireDistance in the algorithm)
	if smallResult.Thrust >= largeResult.Thrust {
		t.Errorf("Expected less thrust for small desire distance: small=%f, large=%f",
			smallResult.Thrust, largeResult.Thrust)
	}
}

// TestFlowInfluence verifies flow vector biases the heading.
func TestFlowInfluence(t *testing.T) {
	// Use terrain so Navigate uses potential field instead of simple pass-through
	terrain := &mockTerrain{
		solidY: 500, // Wall far away, no repulsion
	}
	pf := NewPathfinder(terrain)

	// Organism heading right, desire forward, but flow pushing up
	flowResult := pf.Navigate(
		100, 100, // position
		0,        // heading right
		0,        // desire straight ahead
		0.5,      // moderate desire
		0, -0.5,  // strong flow upward (negative Y)
		5,        // radius
	)

	// No flow for comparison
	noFlowResult := pf.Navigate(
		100, 100, // position
		0,        // heading right
		0,        // desire straight ahead
		0.5,      // moderate desire
		0, 0,     // no flow
		5,        // radius
	)

	// With flow, should have some turn to compensate or different thrust
	// The flow influence should cause a different navigation result
	if flowResult.Turn == noFlowResult.Turn && flowResult.Thrust == noFlowResult.Thrust {
		t.Errorf("Expected flow to influence navigation: flow result=%+v, no flow=%+v",
			flowResult, noFlowResult)
	}
}

// TestMaxForceClamping verifies force magnitude is capped.
func TestMaxForceClamping(t *testing.T) {
	pf := NewPathfinder(nil)

	// Test with maximum desire
	result := pf.Navigate(
		100, 100, // position
		0,        // heading
		0,        // desire straight ahead
		1.0,      // max desire distance
		0, 0,     // no flow
		5,        // radius
	)

	// Thrust should be capped at MaxForce (1.0 default) scaled by alignment
	// With perfect alignment (cos(0) = 1), thrust should be <= 1.0
	if result.Thrust > 1.0 {
		t.Errorf("Thrust exceeds max force: %f", result.Thrust)
	}

	// Turn should be within [-MaxTurnRate, MaxTurnRate]
	maxTurnRate := pf.params.MaxTurnRate
	if result.Turn < -maxTurnRate || result.Turn > maxTurnRate {
		t.Errorf("Turn exceeds max turn rate: %f (max=%f)", result.Turn, maxTurnRate)
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

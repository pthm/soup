package systems

import (
	"math"

	"github.com/pthm-cable/soup/components"
)

// TerrainQuerier is the interface for terrain queries needed by pathfinding.
type TerrainQuerier interface {
	DistanceToSolid(x, y float32) float32
	GetGradient(x, y float32) (gx, gy float32)
	IsSolid(x, y float32) bool
}

// PathfindingParams holds tunable parameters for the pathfinding layer.
type PathfindingParams struct {
	// Steering
	MaxTargetDistance float32 // Maximum projection distance for desire
	ProbeDistance     float32 // How far ahead to check for obstacles
	NumProbeRays      int     // Number of directions to probe (more = smoother but slower)

	// Flow field
	FlowInfluence float32 // How much flow affects navigation (0-1)

	// Output limits
	MaxTurnRate float32 // Maximum turn per tick (radians)

	// A* pathfinding
	AStarMinDistance  float32 // Minimum distance to target before using A* (use local steering below this)
	AStarMaxAge       int32   // Maximum ticks before path recomputation
	WaypointArrival   float32 // Distance to waypoint to consider "arrived"
}

// DefaultPathfindingParams returns sensible defaults for pathfinding.
func DefaultPathfindingParams() PathfindingParams {
	return PathfindingParams{
		MaxTargetDistance: 100.0,
		ProbeDistance:     25.0,
		NumProbeRays:      8, // Check 8 directions (45 degree increments)

		FlowInfluence: 0.3,
		MaxTurnRate:   0.3, // ~17 degrees per tick max

		AStarMinDistance: 64.0,  // Use A* for targets beyond 64px
		AStarMaxAge:      120,   // Recompute path every 2 seconds at 60fps
		WaypointArrival:  16.0,  // Waypoint reached at 16px
	}
}

// PathfindingResult holds the computed navigation outputs.
type PathfindingResult struct {
	Turn   float32 // -1 to +1: turn direction and magnitude
	Thrust float32 // 0 to 1: forward thrust magnitude
}

// Pathfinder computes navigation from brain desire to motor commands.
// Uses A* for long-range pathfinding and context steering for local navigation.
type Pathfinder struct {
	terrain TerrainQuerier
	astar   *AStarPlanner
	params  PathfindingParams
	tick    int32
}

// NewPathfinder creates a new pathfinding layer.
func NewPathfinder(terrain TerrainQuerier) *Pathfinder {
	pf := &Pathfinder{
		terrain: terrain,
		params:  DefaultPathfindingParams(),
	}
	// Initialize A* planner if terrain supports it
	if ts, ok := terrain.(*TerrainSystem); ok {
		pf.astar = NewAStarPlanner(ts)
	}
	return pf
}

// NewPathfinderWithParams creates a pathfinder with custom parameters.
func NewPathfinderWithParams(terrain TerrainQuerier, params PathfindingParams) *Pathfinder {
	pf := &Pathfinder{
		terrain: terrain,
		params:  params,
	}
	if ts, ok := terrain.(*TerrainSystem); ok {
		pf.astar = NewAStarPlanner(ts)
	}
	return pf
}

// SetTick updates the pathfinder's tick counter for path cache validation.
func (p *Pathfinder) SetTick(tick int32) {
	p.tick = tick
}

// Navigate computes turn and thrust from brain desire outputs.
// Uses context steering to avoid obstacles while pursuing the desired direction.
func (p *Pathfinder) Navigate(
	posX, posY float32,
	heading float32,
	desireAngle, desireDistance float32,
	flowX, flowY float32,
	organismRadius float32,
) PathfindingResult {
	// If no terrain or no movement desired, simple pass-through
	if p.terrain == nil || desireDistance < 0.01 {
		return p.NavigateSimple(desireAngle, desireDistance)
	}

	// Target direction in world coordinates
	targetAngle := heading + desireAngle

	// Add flow influence to target
	if p.params.FlowInfluence > 0 {
		flowMag := float32(math.Sqrt(float64(flowX*flowX + flowY*flowY)))
		if flowMag > 0.01 {
			flowAngle := float32(math.Atan2(float64(flowY), float64(flowX)))
			// Blend target angle with flow direction
			targetAngle = blendAngles(targetAngle, flowAngle, p.params.FlowInfluence)
		}
	}

	// Find the best direction using context steering
	bestAngle, clearAhead := p.findBestDirection(posX, posY, targetAngle, organismRadius)

	// Calculate turn needed
	angleDiff := normalizeAngleRange(bestAngle - heading)
	turn := angleDiff / math.Pi // Normalize to [-1, 1]

	// Clamp turn rate
	if turn > p.params.MaxTurnRate {
		turn = p.params.MaxTurnRate
	} else if turn < -p.params.MaxTurnRate {
		turn = -p.params.MaxTurnRate
	}

	// Calculate thrust
	// Reduce thrust when turning sharply or when path ahead is blocked
	alignmentFactor := float32(math.Cos(float64(angleDiff)))
	if alignmentFactor < 0 {
		alignmentFactor = 0
	}

	thrust := desireDistance * alignmentFactor
	if !clearAhead {
		// Reduce thrust when we need to navigate around obstacle
		thrust *= 0.5
	}

	return PathfindingResult{
		Turn:   turn,
		Thrust: thrust,
	}
}

// findBestDirection uses context steering to find an unblocked direction
// closest to the desired target angle.
func (p *Pathfinder) findBestDirection(
	posX, posY float32,
	targetAngle float32,
	radius float32,
) (bestAngle float32, clearAhead bool) {
	probeDistance := p.params.ProbeDistance + radius
	numRays := p.params.NumProbeRays
	if numRays < 4 {
		numRays = 4
	}

	// Check if directly ahead is clear
	clearAhead = p.isDirectionClear(posX, posY, targetAngle, probeDistance, radius)
	if clearAhead {
		return targetAngle, true
	}

	// Target direction is blocked - find best alternative
	// Check directions radiating out from target angle
	angleStep := float32(math.Pi * 2 / float64(numRays))

	bestAngle = targetAngle
	bestScore := float32(-1000)

	for i := 0; i < numRays; i++ {
		// Check directions alternating left and right of target
		// This ensures we find the closest clear direction to our target
		var testAngle float32
		offset := float32(i+1) * angleStep / 2
		if i%2 == 0 {
			testAngle = targetAngle + offset
		} else {
			testAngle = targetAngle - offset
		}

		if !p.isDirectionClear(posX, posY, testAngle, probeDistance, radius) {
			continue
		}

		// Score based on how close to target direction
		angleDiffFromTarget := math.Abs(float64(normalizeAngleRange(testAngle - targetAngle)))
		score := float32(math.Pi - angleDiffFromTarget) // Higher score = closer to target

		if score > bestScore {
			bestScore = score
			bestAngle = testAngle
		}
	}

	// If no clear direction found, just try to turn away from obstacle
	if bestScore < 0 {
		// Get gradient away from nearest obstacle
		gx, gy := p.terrain.GetGradient(posX, posY)
		if gx != 0 || gy != 0 {
			bestAngle = float32(math.Atan2(float64(gy), float64(gx)))
		}
	}

	return bestAngle, false
}

// isDirectionClear checks if a direction is clear of obstacles.
func (p *Pathfinder) isDirectionClear(
	posX, posY float32,
	angle float32,
	distance float32,
	radius float32,
) bool {
	// Check several points along the path
	steps := 3
	stepDist := distance / float32(steps)

	dx := float32(math.Cos(float64(angle)))
	dy := float32(math.Sin(float64(angle)))

	for i := 1; i <= steps; i++ {
		checkX := posX + dx*stepDist*float32(i)
		checkY := posY + dy*stepDist*float32(i)

		// Check if this point would collide
		// Sample in a small pattern around the point to account for organism width
		if p.terrain.IsSolid(checkX, checkY) {
			return false
		}
		// Check sides
		perpX := -dy * radius * 0.7
		perpY := dx * radius * 0.7
		if p.terrain.IsSolid(checkX+perpX, checkY+perpY) {
			return false
		}
		if p.terrain.IsSolid(checkX-perpX, checkY-perpY) {
			return false
		}
	}

	return true
}

// blendAngles blends two angles with a weight factor.
func blendAngles(a1, a2, weight float32) float32 {
	// Convert to unit vectors
	x1 := float32(math.Cos(float64(a1))) * (1 - weight)
	y1 := float32(math.Sin(float64(a1))) * (1 - weight)
	x2 := float32(math.Cos(float64(a2))) * weight
	y2 := float32(math.Sin(float64(a2))) * weight

	// Sum and get angle
	return float32(math.Atan2(float64(y1+y2), float64(x1+x2)))
}

// normalizeAngleRange wraps an angle to [-π, π].
func normalizeAngleRange(angle float32) float32 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// NavigateSimple is a simplified navigation for when no terrain avoidance is needed.
func (p *Pathfinder) NavigateSimple(desireAngle, desireDistance float32) PathfindingResult {
	turn := desireAngle / math.Pi
	if turn > 1.0 {
		turn = 1.0
	} else if turn < -1.0 {
		turn = -1.0
	}

	return PathfindingResult{
		Turn:   turn,
		Thrust: desireDistance,
	}
}

// NavigateWithAStar uses A* for long-range navigation with path caching.
// For short distances or when A* is unavailable, falls back to context steering.
func (p *Pathfinder) NavigateWithAStar(
	posX, posY float32,
	heading float32,
	targetX, targetY float32,
	desireDistance float32,
	flowX, flowY float32,
	obb *components.CollisionOBB,
	cellSize float32,
	cache *PathCache,
) PathfindingResult {
	// Calculate distance to target
	dx := targetX - posX
	dy := targetY - posY
	distToTarget := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	// Get collision radius for size class determination
	radius := GetCollisionRadius(obb, cellSize)
	sizeClass := GetSizeClass(radius)

	// For short distances or no A* planner, use local context steering
	if distToTarget < p.params.AStarMinDistance || p.astar == nil {
		// Convert target position to desire angle relative to heading
		targetAngle := float32(math.Atan2(float64(dy), float64(dx)))
		desireAngle := normalizeAngleRange(targetAngle - heading)
		return p.Navigate(posX, posY, heading, desireAngle, desireDistance, flowX, flowY, radius)
	}

	// Check if we need to recompute the A* path
	needsPath := cache == nil || !p.astar.IsPathValid(cache, targetX, targetY, p.tick, sizeClass, p.params.AStarMaxAge)

	if needsPath {
		// Compute new A* path
		waypoints := p.astar.FindPath(posX, posY, targetX, targetY, sizeClass)
		if waypoints != nil && len(waypoints) > 0 {
			cache.Waypoints = waypoints
			cache.Index = 0
			cache.TargetX = targetX
			cache.TargetY = targetY
			cache.ValidTick = p.tick
		} else {
			// A* failed, fall back to direct navigation
			targetAngle := float32(math.Atan2(float64(dy), float64(dx)))
			desireAngle := normalizeAngleRange(targetAngle - heading)
			return p.Navigate(posX, posY, heading, desireAngle, desireDistance, flowX, flowY, radius)
		}
	}

	// Get next waypoint from cached path
	wpX, wpY, hasMore := GetNextWaypoint(cache, posX, posY, p.params.WaypointArrival)

	// Navigate toward waypoint
	wpDx := wpX - posX
	wpDy := wpY - posY
	wpAngle := float32(math.Atan2(float64(wpDy), float64(wpDx)))
	desireAngle := normalizeAngleRange(wpAngle - heading)

	// Adjust thrust based on whether we're at the final waypoint
	adjustedThrust := desireDistance
	if !hasMore {
		// Near final waypoint, slow down
		wpDist := float32(math.Sqrt(float64(wpDx*wpDx + wpDy*wpDy)))
		if wpDist < p.params.WaypointArrival*2 {
			adjustedThrust *= wpDist / (p.params.WaypointArrival * 2)
		}
	}

	return p.Navigate(posX, posY, heading, desireAngle, adjustedThrust, flowX, flowY, radius)
}

// HasAStarPlanner returns true if the pathfinder has an A* planner available.
func (p *Pathfinder) HasAStarPlanner() bool {
	return p.astar != nil
}

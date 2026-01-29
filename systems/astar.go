package systems

import (
	"container/heap"
	"math"

	"github.com/pthm-cable/soup/components"
)

// AStarPlanner provides A* pathfinding using pre-computed navigation grids.
type AStarPlanner struct {
	grids *NavGridSet

	// Reusable data structures (cleared between searches)
	openHeap  *nodeHeap
	closedSet map[int]struct{}
	cameFrom  map[int]int
	gScore    map[int]float32
	fScore    map[int]float32
}

// PathCache stores a computed path and validation info.
type PathCache struct {
	Waypoints []components.Position // Path waypoints in world coordinates
	Index     int                   // Current waypoint index
	TargetX   float32               // Target position when path was computed
	TargetY   float32
	ValidTick int32 // Tick when path was computed
}

// astarNode is a node in the A* search.
type astarNode struct {
	gx, gy int     // Grid coordinates
	f      float32 // f = g + h (priority)
	index  int     // Heap index
}

// nodeHeap implements heap.Interface for A* open set.
type nodeHeap []*astarNode

func (h nodeHeap) Len() int           { return len(h) }
func (h nodeHeap) Less(i, j int) bool { return h[i].f < h[j].f }
func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *nodeHeap) Push(x any) {
	n := x.(*astarNode)
	n.index = len(*h)
	*h = append(*h, n)
}

func (h *nodeHeap) Pop() any {
	old := *h
	n := len(old)
	node := old[n-1]
	old[n-1] = nil
	node.index = -1
	*h = old[0 : n-1]
	return node
}

// NewAStarPlanner creates an A* planner with pre-computed navigation grids.
func NewAStarPlanner(terrain *TerrainSystem) *AStarPlanner {
	return &AStarPlanner{
		grids:     NewNavGridSet(terrain),
		openHeap:  &nodeHeap{},
		closedSet: make(map[int]struct{}, 256),
		cameFrom:  make(map[int]int, 256),
		gScore:    make(map[int]float32, 256),
		fScore:    make(map[int]float32, 256),
	}
}

// FindPath computes a path from start to goal using A*.
// Returns waypoints in world coordinates, or nil if no path found.
func (a *AStarPlanner) FindPath(startX, startY, goalX, goalY float32, sizeClass SizeClass) []components.Position {
	grid := a.grids.GetGrid(sizeClass)

	// Convert to grid coordinates
	startGX, startGY := grid.WorldToGrid(startX, startY)
	goalGX, goalGY := grid.WorldToGrid(goalX, goalY)

	// Check if start or goal is blocked
	if grid.IsBlocked(startGX, startGY) {
		// Try to find nearest unblocked cell for start
		startGX, startGY = a.findNearestOpen(grid, startGX, startGY)
		if startGX < 0 {
			return nil
		}
	}
	if grid.IsBlocked(goalGX, goalGY) {
		// Try to find nearest unblocked cell for goal
		goalGX, goalGY = a.findNearestOpen(grid, goalGX, goalGY)
		if goalGX < 0 {
			return nil
		}
	}

	// Same cell - no path needed
	if startGX == goalGX && startGY == goalGY {
		x, y := grid.GridToWorld(goalGX, goalGY)
		return []components.Position{{X: x, Y: y}}
	}

	// Clear reusable data structures
	*a.openHeap = (*a.openHeap)[:0]
	for k := range a.closedSet {
		delete(a.closedSet, k)
	}
	for k := range a.cameFrom {
		delete(a.cameFrom, k)
	}
	for k := range a.gScore {
		delete(a.gScore, k)
	}
	for k := range a.fScore {
		delete(a.fScore, k)
	}

	// Initialize start node
	startID := startGY*grid.width + startGX
	goalID := goalGY*grid.width + goalGX

	a.gScore[startID] = 0
	a.fScore[startID] = a.heuristic(startGX, startGY, goalGX, goalGY)

	startNode := &astarNode{gx: startGX, gy: startGY, f: a.fScore[startID]}
	heap.Push(a.openHeap, startNode)

	// A* main loop
	maxIterations := grid.width * grid.height // Limit iterations
	iterations := 0

	for a.openHeap.Len() > 0 && iterations < maxIterations {
		iterations++

		current := heap.Pop(a.openHeap).(*astarNode)
		currentID := current.gy*grid.width + current.gx

		// Goal reached
		if currentID == goalID {
			return a.reconstructPath(grid, startID, goalID)
		}

		a.closedSet[currentID] = struct{}{}

		// Check 8-connected neighbors
		neighbors := [][2]int{
			{current.gx - 1, current.gy},     // W
			{current.gx + 1, current.gy},     // E
			{current.gx, current.gy - 1},     // N
			{current.gx, current.gy + 1},     // S
			{current.gx - 1, current.gy - 1}, // NW
			{current.gx + 1, current.gy - 1}, // NE
			{current.gx - 1, current.gy + 1}, // SW
			{current.gx + 1, current.gy + 1}, // SE
		}

		for i, n := range neighbors {
			ngx, ngy := n[0], n[1]

			// Skip if blocked
			if grid.IsBlocked(ngx, ngy) {
				continue
			}

			// For diagonal moves, check that both adjacent cells are open
			// to prevent cutting corners
			if i >= 4 { // Diagonal
				dx := ngx - current.gx
				dy := ngy - current.gy
				if grid.IsBlocked(current.gx+dx, current.gy) || grid.IsBlocked(current.gx, current.gy+dy) {
					continue
				}
			}

			neighborID := ngy*grid.width + ngx

			// Skip if already evaluated
			if _, ok := a.closedSet[neighborID]; ok {
				continue
			}

			// Calculate cost (sqrt(2) for diagonal, 1 for cardinal)
			moveCost := float32(1.0)
			if i >= 4 {
				moveCost = 1.414
			}

			tentativeG := a.gScore[currentID] + moveCost

			// Check if this path is better
			existingG, exists := a.gScore[neighborID]
			if exists && tentativeG >= existingG {
				continue
			}

			// This is a better path
			a.cameFrom[neighborID] = currentID
			a.gScore[neighborID] = tentativeG
			a.fScore[neighborID] = tentativeG + a.heuristic(ngx, ngy, goalGX, goalGY)

			// Add to open set if not already there
			if !exists {
				node := &astarNode{gx: ngx, gy: ngy, f: a.fScore[neighborID]}
				heap.Push(a.openHeap, node)
			}
		}
	}

	// No path found
	return nil
}

// heuristic computes the Euclidean distance heuristic for A*.
func (a *AStarPlanner) heuristic(gx1, gy1, gx2, gy2 int) float32 {
	dx := float32(gx2 - gx1)
	dy := float32(gy2 - gy1)
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// reconstructPath builds the path from cameFrom map.
func (a *AStarPlanner) reconstructPath(grid *NavGrid, startID, goalID int) []components.Position {
	// Build path in reverse
	var pathIDs []int
	current := goalID
	for current != startID {
		pathIDs = append(pathIDs, current)
		var ok bool
		current, ok = a.cameFrom[current]
		if !ok {
			break
		}
	}
	pathIDs = append(pathIDs, startID)

	// Reverse and convert to world coordinates
	path := make([]components.Position, len(pathIDs))
	for i := 0; i < len(pathIDs); i++ {
		id := pathIDs[len(pathIDs)-1-i]
		gx := id % grid.width
		gy := id / grid.width
		x, y := grid.GridToWorld(gx, gy)
		path[i] = components.Position{X: x, Y: y}
	}

	// Simplify path by removing redundant waypoints
	return a.simplifyPath(path, grid)
}

// simplifyPath removes waypoints that are in a straight line.
func (a *AStarPlanner) simplifyPath(path []components.Position, grid *NavGrid) []components.Position {
	if len(path) <= 2 {
		return path
	}

	simplified := make([]components.Position, 0, len(path))
	simplified = append(simplified, path[0])

	for i := 1; i < len(path)-1; i++ {
		prev := path[i-1]
		curr := path[i]
		next := path[i+1]

		// Check if we can skip curr by going directly from prev to next
		// Use line-of-sight check through the nav grid
		if !a.hasLineOfSight(grid, prev.X, prev.Y, next.X, next.Y) {
			simplified = append(simplified, curr)
		}
	}

	simplified = append(simplified, path[len(path)-1])
	return simplified
}

// hasLineOfSight checks if there's a clear line between two points on the nav grid.
func (a *AStarPlanner) hasLineOfSight(grid *NavGrid, x1, y1, x2, y2 float32) bool {
	dx := x2 - x1
	dy := y2 - y1
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	if dist < 0.01 {
		return true
	}

	// Step along the line, checking each nav cell
	stepSize := grid.cellSize * 0.5
	steps := int(dist/stepSize) + 1

	dx /= dist
	dy /= dist

	for i := 0; i <= steps; i++ {
		checkX := x1 + dx*float32(i)*stepSize
		checkY := y1 + dy*float32(i)*stepSize
		if grid.IsBlockedWorld(checkX, checkY) {
			return false
		}
	}

	return true
}

// findNearestOpen finds the nearest unblocked cell to the given position.
// Returns (-1, -1) if no open cell found within search radius.
func (a *AStarPlanner) findNearestOpen(grid *NavGrid, gx, gy int) (int, int) {
	// Spiral search outward
	for radius := 1; radius < 10; radius++ {
		for dy := -radius; dy <= radius; dy++ {
			for dx := -radius; dx <= radius; dx++ {
				// Only check cells at the current radius
				if abs(dx) != radius && abs(dy) != radius {
					continue
				}
				ngx := gx + dx
				ngy := gy + dy
				if !grid.IsBlocked(ngx, ngy) {
					return ngx, ngy
				}
			}
		}
	}
	return -1, -1
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// IsPathValid checks if a cached path is still valid.
// A path is invalid if:
// - Target has moved more than 32px
// - Any waypoint is now blocked
// - Path is older than maxAge ticks
func (a *AStarPlanner) IsPathValid(cache *PathCache, targetX, targetY float32, currentTick int32, sizeClass SizeClass, maxAge int32) bool {
	if cache == nil || len(cache.Waypoints) == 0 {
		return false
	}

	// Check age
	if currentTick-cache.ValidTick > maxAge {
		return false
	}

	// Check if target moved significantly
	dx := targetX - cache.TargetX
	dy := targetY - cache.TargetY
	if dx*dx+dy*dy > 32*32 {
		return false
	}

	// Check if remaining path is still clear
	grid := a.grids.GetGrid(sizeClass)
	for i := cache.Index; i < len(cache.Waypoints); i++ {
		wp := cache.Waypoints[i]
		if grid.IsBlockedWorld(wp.X, wp.Y) {
			return false
		}
	}

	return true
}

// GetNextWaypoint returns the next waypoint to navigate toward.
// Advances the path index if the organism is close enough to the current waypoint.
func GetNextWaypoint(cache *PathCache, posX, posY float32, arrivalDist float32) (wpX, wpY float32, hasMore bool) {
	if cache == nil || cache.Index >= len(cache.Waypoints) {
		return posX, posY, false
	}

	wp := cache.Waypoints[cache.Index]

	// Check if we've reached the current waypoint
	dx := wp.X - posX
	dy := wp.Y - posY
	if dx*dx+dy*dy < arrivalDist*arrivalDist {
		// Advance to next waypoint
		cache.Index++
		if cache.Index >= len(cache.Waypoints) {
			return wp.X, wp.Y, false
		}
		wp = cache.Waypoints[cache.Index]
	}

	return wp.X, wp.Y, cache.Index < len(cache.Waypoints)-1
}

// GetCollisionRadius returns the collision radius for an organism.
// Uses OBB if available, otherwise falls back to CellSize-based radius.
func GetCollisionRadius(obb *components.CollisionOBB, cellSize float32) float32 {
	if obb != nil && (obb.HalfWidth > 0 || obb.HalfHeight > 0) {
		// Use the larger half-extent
		if obb.HalfWidth > obb.HalfHeight {
			return obb.HalfWidth
		}
		return obb.HalfHeight
	}
	return cellSize * 3
}

package systems

import (
	"math"
)

const (
	shadowMapWidth  = 64 // Reduced from 128 for performance
	shadowMapHeight = 64
	occluderGridSize = 16 // Spatial grid for occluders
)

// ShadowMap stores light intensity values for photosynthesis and phototropism.
type ShadowMap struct {
	grid           [shadowMapHeight][shadowMapWidth]float32
	width, height  float32
	updateInterval int32
	lastUpdate     int32
	// Spatial grid for occluder acceleration
	occluderGrid   [occluderGridSize][occluderGridSize][]int // indices into occluders slice
	// Pre-allocated buffers to avoid allocations in hot path
	seenGeneration int           // Incremented each frame to avoid clearing seen array
	occluderSeen   []int         // Stores generation when occluder was last seen
	candidateBuf   []int         // Reusable buffer for candidate occluders
}

// NewShadowMap creates a new shadow map for the given screen dimensions.
func NewShadowMap(screenWidth, screenHeight float32) *ShadowMap {
	sm := &ShadowMap{
		width:          screenWidth,
		height:         screenHeight,
		updateInterval: 8, // Update every 8 ticks (increased from 5)
	}
	// Initialize all cells to full light
	for y := 0; y < shadowMapHeight; y++ {
		for x := 0; x < shadowMapWidth; x++ {
			sm.grid[y][x] = 1.0
		}
	}
	return sm
}

// Update recalculates the shadow map based on sun position and occluders.
func (sm *ShadowMap) Update(tick int32, sunX, sunY float32, occluders []Occluder) {
	// Only update every N ticks for performance
	if tick-sm.lastUpdate < sm.updateInterval {
		return
	}
	sm.lastUpdate = tick

	// Build spatial grid for occluders
	sm.buildOccluderGrid(occluders)

	// Ensure seen buffer is large enough
	if len(sm.occluderSeen) < len(occluders) {
		sm.occluderSeen = make([]int, len(occluders)*2) // 2x to avoid frequent reallocations
	}
	// Pre-allocate candidate buffer
	if cap(sm.candidateBuf) < 64 {
		sm.candidateBuf = make([]int, 0, 64)
	}

	cellWidth := sm.width / shadowMapWidth
	cellHeight := sm.height / shadowMapHeight
	occGridCellW := sm.width / occluderGridSize
	occGridCellH := sm.height / occluderGridSize

	// Pre-compute max distance once
	maxDist := float32(math.Sqrt(float64(sm.width*sm.width + sm.height*sm.height)))

	// For each grid cell, calculate light intensity
	for gy := 0; gy < shadowMapHeight; gy++ {
		for gx := 0; gx < shadowMapWidth; gx++ {
			// Increment generation for this grid cell to invalidate seen markers
			sm.seenGeneration++

			// World position of grid cell center
			worldX := (float32(gx) + 0.5) * cellWidth
			worldY := (float32(gy) + 0.5) * cellHeight

			// Calculate base light from distance to sun
			dx := worldX - sunX
			dy := worldY - sunY
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			normalizedDist := dist / maxDist

			// Clamp to valid range (sun can be off-screen)
			if normalizedDist > 1.0 {
				normalizedDist = 1.0
			}

			// Distance falloff - gentler curve, minimum 0.3 ambient light
			distFactor := float32(math.Pow(float64(1-normalizedDist), 0.8))
			light := 0.3 + distFactor*0.7

			// Vertical gradient - light attenuates with depth (darker at bottom)
			// normalizedY: 0 = top (bright), 1 = bottom (dark)
			normalizedY := worldY / sm.height
			// Depth attenuation: top gets full light, bottom gets ~40% of base light
			depthFactor := 1.0 - normalizedY*0.6
			light *= depthFactor

			// Only check occluders in cells that the ray passes through
			candidates := sm.getOccludersAlongRayFast(worldX, worldY, sunX, sunY, occGridCellW, occGridCellH)

			// Check occlusion from candidate occluders
			for _, idx := range candidates {
				occ := occluders[idx]
				if sm.rayIntersectsAABB(worldX, worldY, sunX, sunY, occ) {
					// Shadow strength based on occluder density
					occluderArea := occ.Width * occ.Height
					density := float32(math.Min(float64(occluderArea)/(15*15), 1.0))
					light *= (1 - density*0.7)

					// Early termination - if very dark, stop checking
					if light < 0.1 {
						light = 0.1
						break
					}
				}
			}

			// Clamp light to valid range
			if light < 0 {
				light = 0
			}
			if light > 1 {
				light = 1
			}

			sm.grid[gy][gx] = light
		}
	}
}

// buildOccluderGrid populates the spatial grid with occluder indices.
func (sm *ShadowMap) buildOccluderGrid(occluders []Occluder) {
	// Clear the grid
	for y := 0; y < occluderGridSize; y++ {
		for x := 0; x < occluderGridSize; x++ {
			sm.occluderGrid[y][x] = sm.occluderGrid[y][x][:0]
		}
	}

	cellW := sm.width / occluderGridSize
	cellH := sm.height / occluderGridSize

	for i, occ := range occluders {
		// Find all grid cells this occluder overlaps
		minGX := int(occ.X / cellW)
		maxGX := int((occ.X + occ.Width) / cellW)
		minGY := int(occ.Y / cellH)
		maxGY := int((occ.Y + occ.Height) / cellH)

		// Clamp to grid bounds
		if minGX < 0 {
			minGX = 0
		}
		if maxGX >= occluderGridSize {
			maxGX = occluderGridSize - 1
		}
		if minGY < 0 {
			minGY = 0
		}
		if maxGY >= occluderGridSize {
			maxGY = occluderGridSize - 1
		}

		// Add to all overlapping cells
		for gy := minGY; gy <= maxGY; gy++ {
			for gx := minGX; gx <= maxGX; gx++ {
				sm.occluderGrid[gy][gx] = append(sm.occluderGrid[gy][gx], i)
			}
		}
	}
}

// getOccludersAlongRay returns indices of occluders in cells along the ray path.
func (sm *ShadowMap) getOccludersAlongRay(x0, y0, x1, y1, cellW, cellH float32) []int {
	// Use a simple approach: collect occluders from the start cell and end cell,
	// plus cells in between using Bresenham-like stepping
	seen := make(map[int]bool)
	var result []int

	// Helper to add occluders from a grid cell
	addCell := func(gx, gy int) {
		if gx < 0 || gx >= occluderGridSize || gy < 0 || gy >= occluderGridSize {
			return
		}
		for _, idx := range sm.occluderGrid[gy][gx] {
			if !seen[idx] {
				seen[idx] = true
				result = append(result, idx)
			}
		}
	}

	// Start and end grid cells
	gx0 := int(x0 / cellW)
	gy0 := int(y0 / cellH)
	gx1 := int(x1 / cellW)
	gy1 := int(y1 / cellH)

	// Clamp
	if gx0 < 0 {
		gx0 = 0
	}
	if gx0 >= occluderGridSize {
		gx0 = occluderGridSize - 1
	}
	if gy0 < 0 {
		gy0 = 0
	}
	if gy0 >= occluderGridSize {
		gy0 = occluderGridSize - 1
	}
	if gx1 < 0 {
		gx1 = 0
	}
	if gx1 >= occluderGridSize {
		gx1 = occluderGridSize - 1
	}
	if gy1 < 0 {
		gy1 = 0
	}
	if gy1 >= occluderGridSize {
		gy1 = occluderGridSize - 1
	}

	// Simple line traversal - add all cells in the bounding box of the ray
	// This is a conservative approximation but fast
	minGX, maxGX := gx0, gx1
	if minGX > maxGX {
		minGX, maxGX = maxGX, minGX
	}
	minGY, maxGY := gy0, gy1
	if minGY > maxGY {
		minGY, maxGY = maxGY, minGY
	}

	for gy := minGY; gy <= maxGY; gy++ {
		for gx := minGX; gx <= maxGX; gx++ {
			addCell(gx, gy)
		}
	}

	return result
}

// getOccludersAlongRayFast is an allocation-free version using pre-allocated buffers.
func (sm *ShadowMap) getOccludersAlongRayFast(x0, y0, x1, y1, cellW, cellH float32) []int {
	// Reset candidate buffer
	sm.candidateBuf = sm.candidateBuf[:0]

	// Start and end grid cells
	gx0 := int(x0 / cellW)
	gy0 := int(y0 / cellH)
	gx1 := int(x1 / cellW)
	gy1 := int(y1 / cellH)

	// Clamp to grid bounds
	if gx0 < 0 {
		gx0 = 0
	}
	if gx0 >= occluderGridSize {
		gx0 = occluderGridSize - 1
	}
	if gy0 < 0 {
		gy0 = 0
	}
	if gy0 >= occluderGridSize {
		gy0 = occluderGridSize - 1
	}
	if gx1 < 0 {
		gx1 = 0
	}
	if gx1 >= occluderGridSize {
		gx1 = occluderGridSize - 1
	}
	if gy1 < 0 {
		gy1 = 0
	}
	if gy1 >= occluderGridSize {
		gy1 = occluderGridSize - 1
	}

	// Get bounding box of the ray
	minGX, maxGX := gx0, gx1
	if minGX > maxGX {
		minGX, maxGX = maxGX, minGX
	}
	minGY, maxGY := gy0, gy1
	if minGY > maxGY {
		minGY, maxGY = maxGY, minGY
	}

	// Collect unique occluders using generation-based deduplication
	gen := sm.seenGeneration
	for gy := minGY; gy <= maxGY; gy++ {
		for gx := minGX; gx <= maxGX; gx++ {
			for _, idx := range sm.occluderGrid[gy][gx] {
				if idx < len(sm.occluderSeen) && sm.occluderSeen[idx] != gen {
					sm.occluderSeen[idx] = gen
					sm.candidateBuf = append(sm.candidateBuf, idx)
				}
			}
		}
	}

	return sm.candidateBuf
}

// rayIntersectsAABB checks if a ray from point to sun intersects an AABB.
func (sm *ShadowMap) rayIntersectsAABB(px, py, sunX, sunY float32, occ Occluder) bool {
	// Ray direction
	dx := sunX - px
	dy := sunY - py

	// AABB bounds
	minX := occ.X
	maxX := occ.X + occ.Width
	minY := occ.Y
	maxY := occ.Y + occ.Height

	// Parametric ray intersection
	var tmin, tmax float32 = 0, 1

	// X slab
	if dx != 0 {
		invD := 1.0 / dx
		t0 := (minX - px) * invD
		t1 := (maxX - px) * invD
		if invD < 0 {
			t0, t1 = t1, t0
		}
		if t0 > tmin {
			tmin = t0
		}
		if t1 < tmax {
			tmax = t1
		}
	} else {
		// Ray is parallel to X axis
		if px < minX || px > maxX {
			return false
		}
	}

	// Y slab
	if dy != 0 {
		invD := 1.0 / dy
		t0 := (minY - py) * invD
		t1 := (maxY - py) * invD
		if invD < 0 {
			t0, t1 = t1, t0
		}
		if t0 > tmin {
			tmin = t0
		}
		if t1 < tmax {
			tmax = t1
		}
	} else {
		// Ray is parallel to Y axis
		if py < minY || py > maxY {
			return false
		}
	}

	return tmax >= tmin && tmax > 0 && tmin < 1
}

// SampleLight returns the light intensity at a world position using bilinear interpolation.
func (sm *ShadowMap) SampleLight(worldX, worldY float32) float32 {
	// Convert world coordinates to grid coordinates
	cellWidth := sm.width / shadowMapWidth
	cellHeight := sm.height / shadowMapHeight

	gx := worldX/cellWidth - 0.5
	gy := worldY/cellHeight - 0.5

	// Clamp to grid bounds
	if gx < 0 {
		gx = 0
	}
	if gy < 0 {
		gy = 0
	}
	if gx >= shadowMapWidth-1 {
		gx = shadowMapWidth - 1.001
	}
	if gy >= shadowMapHeight-1 {
		gy = shadowMapHeight - 1.001
	}

	// Get integer and fractional parts
	x0 := int(gx)
	y0 := int(gy)
	x1 := x0 + 1
	y1 := y0 + 1

	fx := gx - float32(x0)
	fy := gy - float32(y0)

	// Clamp indices
	if x1 >= shadowMapWidth {
		x1 = shadowMapWidth - 1
	}
	if y1 >= shadowMapHeight {
		y1 = shadowMapHeight - 1
	}

	// Bilinear interpolation
	v00 := sm.grid[y0][x0]
	v10 := sm.grid[y0][x1]
	v01 := sm.grid[y1][x0]
	v11 := sm.grid[y1][x1]

	v0 := v00*(1-fx) + v10*fx
	v1 := v01*(1-fx) + v11*fx

	return v0*(1-fy) + v1*fy
}

// SunDirection returns the normalized direction from a world position to the sun.
func (sm *ShadowMap) SunDirection(worldX, worldY, sunX, sunY float32) (float32, float32) {
	dx := sunX - worldX
	dy := sunY - worldY
	mag := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if mag < 0.001 {
		return 0, -1 // Default to up
	}
	return dx / mag, dy / mag
}

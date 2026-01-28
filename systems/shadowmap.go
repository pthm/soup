package systems

import (
	"math"
)

const (
	shadowMapWidth  = 128
	shadowMapHeight = 128
)

// ShadowMap stores light intensity values for photosynthesis and phototropism.
type ShadowMap struct {
	grid           [shadowMapHeight][shadowMapWidth]float32
	width, height  float32
	updateInterval int32
	lastUpdate     int32
}

// NewShadowMap creates a new shadow map for the given screen dimensions.
func NewShadowMap(screenWidth, screenHeight float32) *ShadowMap {
	sm := &ShadowMap{
		width:          screenWidth,
		height:         screenHeight,
		updateInterval: 5, // Update every 5 ticks
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

	cellWidth := sm.width / shadowMapWidth
	cellHeight := sm.height / shadowMapHeight

	// For each grid cell, calculate light intensity
	for gy := 0; gy < shadowMapHeight; gy++ {
		for gx := 0; gx < shadowMapWidth; gx++ {
			// World position of grid cell center
			worldX := (float32(gx) + 0.5) * cellWidth
			worldY := (float32(gy) + 0.5) * cellHeight

			// Calculate base light from distance to sun
			dx := worldX - sunX
			dy := worldY - sunY
			dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
			maxDist := float32(math.Sqrt(float64(sm.width*sm.width + sm.height*sm.height)))
			normalizedDist := dist / maxDist

			// Distance falloff - gentler curve, minimum 0.3 ambient light
			// At sun: 1.0, at farthest corner: 0.3
			distFactor := float32(math.Pow(float64(1-normalizedDist), 0.8))
			light := 0.3 + distFactor*0.7

			// Check occlusion from each occluder
			for _, occ := range occluders {
				if sm.rayIntersectsAABB(worldX, worldY, sunX, sunY, occ) {
					// Shadow strength based on occluder density (using size as proxy)
					occluderArea := occ.Width * occ.Height
					density := float32(math.Min(float64(occluderArea)/(15*15), 1.0))
					light *= (1 - density*0.7)
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

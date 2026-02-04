// Package camera provides a 2D camera system for viewport control.
package camera

import "math"

// Camera controls the viewport into the simulation world.
// Supports pan and zoom with toroidal world wrapping.
type Camera struct {
	// Position is the camera center in world coordinates
	X, Y float32

	// Zoom level (1.0 = 1:1, 2.0 = 2x magnification)
	Zoom float32

	// Viewport dimensions (screen size)
	ViewportW, ViewportH float32

	// World dimensions (for toroidal wrapping)
	WorldW, WorldH float32

	// Zoom constraints
	MinZoom, MaxZoom float32
}

// New creates a camera centered on the world with 1:1 zoom.
func New(viewportW, viewportH, worldW, worldH float32) *Camera {
	// Compute minimum zoom so viewport never exceeds world bounds
	// At zoom Z, visible world area is (viewportW/Z, viewportH/Z)
	// We need viewportW/Z <= worldW AND viewportH/Z <= worldH
	// So Z >= viewportW/worldW AND Z >= viewportH/worldH
	minZoomX := viewportW / worldW
	minZoomY := viewportH / worldH
	minZoom := minZoomX
	if minZoomY > minZoom {
		minZoom = minZoomY
	}

	return &Camera{
		X:         worldW / 2,
		Y:         worldH / 2,
		Zoom:      1.0,
		ViewportW: viewportW,
		ViewportH: viewportH,
		WorldW:    worldW,
		WorldH:    worldH,
		MinZoom:   minZoom,
		MaxZoom:   4.0,
	}
}

// WorldToScreen converts world coordinates to screen coordinates.
// Returns the screen position and whether the point is visible.
// For toroidal worlds, this finds the shortest path to the viewport.
func (c *Camera) WorldToScreen(wx, wy float32) (sx, sy float32) {
	// Calculate delta from camera center using toroidal shortest distance
	dx := toroidalDelta(wx, c.X, c.WorldW)
	dy := toroidalDelta(wy, c.Y, c.WorldH)

	// Apply zoom and center on viewport
	sx = c.ViewportW/2 + dx*c.Zoom
	sy = c.ViewportH/2 + dy*c.Zoom
	return sx, sy
}

// ScreenToWorld converts screen coordinates to world coordinates.
func (c *Camera) ScreenToWorld(sx, sy float32) (wx, wy float32) {
	// Reverse the viewport centering and zoom
	dx := (sx - c.ViewportW/2) / c.Zoom
	dy := (sy - c.ViewportH/2) / c.Zoom

	// Add to camera position and wrap to world bounds
	wx = mod(c.X+dx, c.WorldW)
	wy = mod(c.Y+dy, c.WorldH)
	return wx, wy
}

// IsVisible returns true if a circle at (wx, wy) with given radius
// could be visible on screen (conservative check for culling).
func (c *Camera) IsVisible(wx, wy, radius float32) bool {
	// Calculate shortest delta to camera center
	dx := toroidalDelta(wx, c.X, c.WorldW)
	dy := toroidalDelta(wy, c.Y, c.WorldH)

	// Half-extents of the visible area in world coords, plus margin for radius
	halfW := c.ViewportW/(2*c.Zoom) + radius
	halfH := c.ViewportH/(2*c.Zoom) + radius

	// Check if within visible bounds
	return absf(dx) <= halfW && absf(dy) <= halfH
}

// GhostPositions returns additional screen positions for entities near world edges.
// These "ghost" copies ensure entities appear on both sides during wrapping.
// Returns up to 3 additional positions (plus the primary position makes 4 max for corners).
func (c *Camera) GhostPositions(wx, wy, radius float32) []struct{ X, Y float32 } {
	var ghosts []struct{ X, Y float32 }

	// Calculate the visible extent in world coordinates
	halfW := c.ViewportW / (2 * c.Zoom)
	halfH := c.ViewportH / (2 * c.Zoom)

	// Check if entity is near world edges relative to camera view
	dx := toroidalDelta(wx, c.X, c.WorldW)
	dy := toroidalDelta(wy, c.Y, c.WorldH)

	// Check horizontal wrapping
	needsHorizontalGhost := false
	var hGhostX float32
	if dx > halfW-radius && dx < halfW+radius {
		// Near right edge of view - ghost on left
		needsHorizontalGhost = true
		hGhostX = c.ViewportW/2 + (dx-c.WorldW)*c.Zoom
	} else if dx < -halfW+radius && dx > -halfW-radius {
		// Near left edge of view - ghost on right
		needsHorizontalGhost = true
		hGhostX = c.ViewportW/2 + (dx+c.WorldW)*c.Zoom
	}

	// Check vertical wrapping
	needsVerticalGhost := false
	var vGhostY float32
	if dy > halfH-radius && dy < halfH+radius {
		// Near bottom edge of view - ghost on top
		needsVerticalGhost = true
		vGhostY = c.ViewportH/2 + (dy-c.WorldH)*c.Zoom
	} else if dy < -halfH+radius && dy > -halfH-radius {
		// Near top edge of view - ghost on bottom
		needsVerticalGhost = true
		vGhostY = c.ViewportH/2 + (dy+c.WorldH)*c.Zoom
	}

	// Primary screen position
	sx := c.ViewportW/2 + dx*c.Zoom
	sy := c.ViewportH/2 + dy*c.Zoom

	if needsHorizontalGhost {
		ghosts = append(ghosts, struct{ X, Y float32 }{hGhostX, sy})
	}
	if needsVerticalGhost {
		ghosts = append(ghosts, struct{ X, Y float32 }{sx, vGhostY})
	}
	if needsHorizontalGhost && needsVerticalGhost {
		ghosts = append(ghosts, struct{ X, Y float32 }{hGhostX, vGhostY})
	}

	return ghosts
}

// Resize updates viewport dimensions and recalculates zoom constraints.
func (c *Camera) Resize(viewportW, viewportH float32) {
	if viewportW == c.ViewportW && viewportH == c.ViewportH {
		return
	}
	c.ViewportW = viewportW
	c.ViewportH = viewportH
	minZoomX := viewportW / c.WorldW
	minZoomY := viewportH / c.WorldH
	c.MinZoom = minZoomX
	if minZoomY > c.MinZoom {
		c.MinZoom = minZoomY
	}
	if c.Zoom < c.MinZoom {
		c.Zoom = c.MinZoom
	}
}

// Pan moves the camera by the given delta in screen pixels.
// Automatically wraps around world boundaries.
func (c *Camera) Pan(dx, dy float32) {
	// Convert screen delta to world delta (inverse of zoom)
	c.X = mod(c.X+dx/c.Zoom, c.WorldW)
	c.Y = mod(c.Y+dy/c.Zoom, c.WorldH)
}

// SetZoom sets the zoom level, clamped to min/max.
func (c *Camera) SetZoom(zoom float32) {
	c.Zoom = clamp(zoom, c.MinZoom, c.MaxZoom)
}

// ZoomBy multiplies the current zoom by the given factor.
func (c *Camera) ZoomBy(factor float32) {
	c.SetZoom(c.Zoom * factor)
}

// Reset returns the camera to the default position and zoom.
func (c *Camera) Reset() {
	c.X = c.WorldW / 2
	c.Y = c.WorldH / 2
	c.Zoom = 1.0
}

// VisibleWorldBounds returns the world-coordinate bounds of the visible area.
// Returns (minX, minY, maxX, maxY) in world coordinates.
// Note: For toroidal worlds, min may be > max if the view wraps.
func (c *Camera) VisibleWorldBounds() (minX, minY, maxX, maxY float32) {
	halfW := c.ViewportW / (2 * c.Zoom)
	halfH := c.ViewportH / (2 * c.Zoom)

	minX = c.X - halfW
	maxX = c.X + halfW
	minY = c.Y - halfH
	maxY = c.Y + halfH
	return
}

// toroidalDelta computes the shortest signed distance from 'from' to 'to'
// in a toroidal space of the given size.
func toroidalDelta(to, from, size float32) float32 {
	d := to - from
	if d > size/2 {
		d -= size
	} else if d < -size/2 {
		d += size
	}
	return d
}

// mod computes the positive modulo (Go's % can return negative).
func mod(x, m float32) float32 {
	r := float32(math.Mod(float64(x), float64(m)))
	if r < 0 {
		r += m
	}
	return r
}

// absf returns the absolute value of a float32.
func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// clamp restricts a value to a range.
func clamp(x, min, max float32) float32 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

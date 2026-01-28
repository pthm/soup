package renderer

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/systems"
)

// LightState holds sun/light configuration.
type LightState struct {
	// Normalized position (0-1), e.g. top-right = {0.75, 0.12}
	PosX, PosY float32
	// Light intensity (0-1)
	Intensity float32
}

// SunRenderer renders 2D lighting with shadow casting.
type SunRenderer struct {
	width  float32
	height float32
}

// NewSunRenderer creates a new sun renderer.
func NewSunRenderer(width, height int32) *SunRenderer {
	return &SunRenderer{
		width:  float32(width),
		height: float32(height),
	}
}

// Draw renders the sun glow and shadows.
func (r *SunRenderer) Draw(light LightState, occluders []systems.Occluder) {
	sunX := light.PosX * r.width
	sunY := light.PosY * r.height
	maxDist := float32(math.Sqrt(float64(r.width*r.width + r.height*r.height)))

	// Only draw radial light if sun is somewhat visible
	if light.PosX >= -0.1 && light.PosX <= 1.1 {
		r.drawRadialLight(sunX, sunY, maxDist, light.Intensity)
	}

	// Draw shadows for each occluder (shadows cast even when sun is off-screen)
	for _, occ := range occluders {
		r.drawShadow(sunX, sunY, occ, maxDist)
	}

	// Only draw sun glow if sun is on screen
	if light.PosX >= 0 && light.PosX <= 1.0 && light.PosY >= -0.1 && light.PosY <= 1.0 {
		r.drawSunGlow(sunX, sunY, light.Intensity)
	}
}

// drawRadialLight draws a subtle radial gradient from the light source.
func (r *SunRenderer) drawRadialLight(x, y, maxRadius, intensity float32) {
	steps := 12
	for i := steps; i >= 0; i-- {
		t := float32(i) / float32(steps)
		radius := maxRadius * t * 0.4

		// Fast falloff - light concentrated near source
		falloff := float32(math.Pow(float64(1-t), 4.0))
		alpha := falloff * 0.015 * intensity * 255

		if alpha < 1 {
			continue
		}

		// Warm color
		color := rl.Color{R: 255, G: 200, B: 150, A: uint8(alpha)}
		rl.DrawCircle(int32(x), int32(y), radius, color)
	}
}

// drawShadow draws a shadow polygon cast by an occluder.
func (r *SunRenderer) drawShadow(lightX, lightY float32, occ systems.Occluder, maxDist float32) {
	// Get the four corners of the occluder
	corners := []struct{ x, y float32 }{
		{occ.X, occ.Y},                             // top-left
		{occ.X + occ.Width, occ.Y},                 // top-right
		{occ.X + occ.Width, occ.Y + occ.Height},    // bottom-right
		{occ.X, occ.Y + occ.Height},                // bottom-left
	}

	// Calculate angle from light to each corner
	type angledCorner struct {
		x, y, angle float32
	}
	angledCorners := make([]angledCorner, 4)
	for i, c := range corners {
		angledCorners[i] = angledCorner{
			x:     c.x,
			y:     c.y,
			angle: float32(math.Atan2(float64(c.y-lightY), float64(c.x-lightX))),
		}
	}

	// Sort by angle
	for i := 0; i < len(angledCorners)-1; i++ {
		for j := i + 1; j < len(angledCorners); j++ {
			if angledCorners[j].angle < angledCorners[i].angle {
				angledCorners[i], angledCorners[j] = angledCorners[j], angledCorners[i]
			}
		}
	}

	// The silhouette is formed by the two extreme corners
	minCorner := angledCorners[0]
	maxCorner := angledCorners[len(angledCorners)-1]

	shadowLength := maxDist * 1.5

	// Project corners away from light
	projectPoint := func(cx, cy float32) (float32, float32) {
		dx := cx - lightX
		dy := cy - lightY
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
		if dist < 0.001 {
			return cx, cy
		}
		nx := dx / dist
		ny := dy / dist
		return cx + nx*shadowLength, cy + ny*shadowLength
	}

	minProjX, minProjY := projectPoint(minCorner.x, minCorner.y)
	maxProjX, maxProjY := projectPoint(maxCorner.x, maxCorner.y)

	// Draw main shadow quad
	shadowColor := rl.Color{R: 5, G: 8, B: 12, A: 80}
	rl.DrawTriangle(
		rl.Vector2{X: minCorner.x, Y: minCorner.y},
		rl.Vector2{X: maxCorner.x, Y: maxCorner.y},
		rl.Vector2{X: maxProjX, Y: maxProjY},
		shadowColor,
	)
	rl.DrawTriangle(
		rl.Vector2{X: minCorner.x, Y: minCorner.y},
		rl.Vector2{X: maxProjX, Y: maxProjY},
		rl.Vector2{X: minProjX, Y: minProjY},
		shadowColor,
	)

	// Draw soft penumbra edges
	penumbraOffset := float32(0.08)
	penumbraColor := rl.Color{R: 5, G: 8, B: 12, A: 30}

	// Left penumbra
	leftAngle := float32(math.Atan2(float64(minCorner.y-lightY), float64(minCorner.x-lightX)))
	leftPenumbraX := minCorner.x + float32(math.Cos(float64(leftAngle-penumbraOffset)))*shadowLength
	leftPenumbraY := minCorner.y + float32(math.Sin(float64(leftAngle-penumbraOffset)))*shadowLength

	rl.DrawTriangle(
		rl.Vector2{X: minCorner.x, Y: minCorner.y},
		rl.Vector2{X: minProjX, Y: minProjY},
		rl.Vector2{X: leftPenumbraX, Y: leftPenumbraY},
		penumbraColor,
	)

	// Right penumbra
	rightAngle := float32(math.Atan2(float64(maxCorner.y-lightY), float64(maxCorner.x-lightX)))
	rightPenumbraX := maxCorner.x + float32(math.Cos(float64(rightAngle+penumbraOffset)))*shadowLength
	rightPenumbraY := maxCorner.y + float32(math.Sin(float64(rightAngle+penumbraOffset)))*shadowLength

	rl.DrawTriangle(
		rl.Vector2{X: maxCorner.x, Y: maxCorner.y},
		rl.Vector2{X: maxProjX, Y: maxProjY},
		rl.Vector2{X: rightPenumbraX, Y: rightPenumbraY},
		penumbraColor,
	)
}

// drawSunGlow draws the sun glow effect.
func (r *SunRenderer) drawSunGlow(x, y, intensity float32) {
	// Subtle outer glow layers
	glowLayers := []struct {
		radius float32
		alpha  float32
	}{
		{50, 8},
		{30, 15},
		{18, 25},
		{10, 50},
	}

	for _, layer := range glowLayers {
		alpha := layer.alpha * intensity
		color := rl.Color{R: 255, G: 220, B: 180, A: uint8(alpha)}
		rl.DrawCircle(int32(x), int32(y), layer.radius, color)
	}

	// Bright core
	coreAlpha := uint8(200 * intensity)
	rl.DrawCircle(int32(x), int32(y), 4, rl.Color{R: 255, G: 250, B: 230, A: coreAlpha})
}

// Unload frees resources (none for this renderer).
func (r *SunRenderer) Unload() {}

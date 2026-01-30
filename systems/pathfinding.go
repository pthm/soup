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

	// Flow field
	FlowInfluence float32 // How much flow affects navigation (0-1)

	// Output limits
	MaxTurnRate float32 // Maximum turn per tick (radians)

	// Potential field parameters
	AttractionStrength float32 // Scales pull toward target (1.0)
	RepulsionStrength  float32 // Scales push from obstacles (2.5)
	RepulsionRadius    float32 // Obstacle influence range (40px)
	RepulsionFalloff   float32 // Exponent for repulsion (2.0)
	TargetDeadzone     float32 // Attraction taper distance (8px)
	MaxForce           float32 // Cap on combined vector magnitude (1.0)
	MinThrust          float32 // Minimum thrust when moving (0.05)
}

// DefaultPathfindingParams returns sensible defaults for pathfinding.
func DefaultPathfindingParams() PathfindingParams {
	return PathfindingParams{
		MaxTargetDistance: 100.0,
		FlowInfluence:     0.3,
		MaxTurnRate:       0.3, // ~17 degrees per tick max

		// Potential field defaults
		AttractionStrength: 1.0,
		RepulsionStrength:  2.5,
		RepulsionRadius:    40.0,
		RepulsionFalloff:   2.0,
		TargetDeadzone:     8.0,
		MaxForce:           1.0,
		MinThrust:          0.05,
	}
}

// PathfindingResult holds the computed navigation outputs.
type PathfindingResult struct {
	Turn   float32 // -1 to +1: turn direction and magnitude
	Thrust float32 // 0 to 1: forward thrust magnitude
}

// Pathfinder computes navigation from brain desire to motor commands.
// Uses potential-field navigation for continuous, reactive steering.
type Pathfinder struct {
	terrain TerrainQuerier
	params  PathfindingParams
}

// NewPathfinder creates a new pathfinding layer.
func NewPathfinder(terrain TerrainQuerier) *Pathfinder {
	return &Pathfinder{
		terrain: terrain,
		params:  DefaultPathfindingParams(),
	}
}

// Navigate computes turn and thrust from brain desire outputs using potential-field navigation.
// The algorithm combines attraction toward the target, repulsion from obstacles, and flow influence.
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

	// 1. Project target from desire angle/distance
	targetAngle := heading + desireAngle
	projectionDist := p.params.MaxTargetDistance * desireDistance
	targetX := posX + float32(math.Cos(float64(targetAngle)))*projectionDist
	targetY := posY + float32(math.Sin(float64(targetAngle)))*projectionDist

	// 2. Compute attraction force toward target
	attrX, attrY := p.computeAttraction(posX, posY, targetX, targetY, desireDistance)

	// 3. Compute repulsion force from nearby obstacles
	repX, repY := p.computeRepulsion(posX, posY, organismRadius)

	// 4. Combine forces: attraction + repulsion + flow
	forceX := attrX + repX + flowX*p.params.FlowInfluence
	forceY := attrY + repY + flowY*p.params.FlowInfluence

	// Clamp force magnitude
	forceMag := float32(math.Sqrt(float64(forceX*forceX + forceY*forceY)))
	if forceMag > p.params.MaxForce {
		scale := p.params.MaxForce / forceMag
		forceX *= scale
		forceY *= scale
		forceMag = p.params.MaxForce
	}

	// 5. Convert resultant vector to turn/thrust
	return p.forceToTurnThrust(forceX, forceY, forceMag, heading, desireDistance)
}

// computeAttraction calculates the attraction force toward the target position.
// Force tapers off when very close to target (within deadzone).
func (p *Pathfinder) computeAttraction(posX, posY, targetX, targetY, desireDistance float32) (float32, float32) {
	dx := targetX - posX
	dy := targetY - posY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

	if dist < 0.001 {
		return 0, 0
	}

	// Normalize direction
	dirX := dx / dist
	dirY := dy / dist

	// Calculate attraction magnitude with deadzone taper
	mag := desireDistance * p.params.AttractionStrength
	if dist < p.params.TargetDeadzone {
		// Taper attraction when very close to target
		mag *= dist / p.params.TargetDeadzone
	}

	return dirX * mag, dirY * mag
}

// computeRepulsion calculates repulsion force from nearby obstacles.
// Samples 8 points around the organism to detect obstacles and compute gradient-based repulsion.
func (p *Pathfinder) computeRepulsion(posX, posY, organismRadius float32) (float32, float32) {
	var repX, repY float32

	// Sample 8 points around the organism at organismRadius + RepulsionRadius*0.5
	sampleDist := organismRadius + p.params.RepulsionRadius*0.5
	numSamples := 8
	angleStep := float32(2 * math.Pi / float64(numSamples))

	for i := 0; i < numSamples; i++ {
		angle := float32(i) * angleStep
		sampleX := posX + float32(math.Cos(float64(angle)))*sampleDist
		sampleY := posY + float32(math.Sin(float64(angle)))*sampleDist

		// Get distance to solid at this sample point
		distToSolid := p.terrain.DistanceToSolid(sampleX, sampleY)

		if distToSolid < p.params.RepulsionRadius {
			// Calculate repulsion magnitude with falloff
			// Closer = stronger repulsion
			normalizedDist := (p.params.RepulsionRadius - distToSolid) / p.params.RepulsionRadius
			mag := p.params.RepulsionStrength * float32(math.Pow(float64(normalizedDist), float64(p.params.RepulsionFalloff)))

			// Get repulsion direction from terrain gradient or sample-to-pos direction
			gx, gy := p.terrain.GetGradient(sampleX, sampleY)
			gradMag := float32(math.Sqrt(float64(gx*gx + gy*gy)))

			var dirX, dirY float32
			if gradMag > 0.001 {
				// Use terrain gradient (points away from solid)
				dirX = gx / gradMag
				dirY = gy / gradMag
			} else {
				// Fallback: push away from sample point toward pos
				dirX = posX - sampleX
				dirY = posY - sampleY
				fallbackMag := float32(math.Sqrt(float64(dirX*dirX + dirY*dirY)))
				if fallbackMag > 0.001 {
					dirX /= fallbackMag
					dirY /= fallbackMag
				}
			}

			repX += dirX * mag
			repY += dirY * mag
		}
	}

	return repX, repY
}

// forceToTurnThrust converts a force vector to turn and thrust outputs.
func (p *Pathfinder) forceToTurnThrust(forceX, forceY, forceMag, heading, desireDistance float32) PathfindingResult {
	if forceMag < 0.001 {
		return PathfindingResult{Turn: 0, Thrust: 0}
	}

	// Calculate target angle from force vector
	forceAngle := float32(math.Atan2(float64(forceY), float64(forceX)))

	// Calculate turn needed (angle difference)
	angleDiff := normalizeAngleRange(forceAngle - heading)
	turn := angleDiff / math.Pi // Normalize to [-1, 1]

	// Clamp turn rate
	if turn > p.params.MaxTurnRate {
		turn = p.params.MaxTurnRate
	} else if turn < -p.params.MaxTurnRate {
		turn = -p.params.MaxTurnRate
	}

	// Calculate thrust
	// Reduce thrust when turning sharply (cos of angle diff)
	alignmentFactor := float32(math.Cos(float64(angleDiff)))
	if alignmentFactor < 0 {
		alignmentFactor = 0
	}

	thrust := (forceMag / p.params.MaxForce) * desireDistance * alignmentFactor

	// Ensure minimum thrust when there's desire to move
	if desireDistance > 0.01 && thrust < p.params.MinThrust {
		thrust = p.params.MinThrust
	}

	return PathfindingResult{
		Turn:   turn,
		Thrust: thrust,
	}
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

// normalizeAngleRange wraps an angle to [-pi, pi].
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

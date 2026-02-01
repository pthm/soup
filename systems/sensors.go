package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Number of vision sectors.
const NumSectors = 5

// SensorInputs holds the computed sensor values for one entity.
// Total: K*3 + 2 = 17 floats for K=5 sectors.
type SensorInputs struct {
	Prey  [NumSectors]float32 // density of prey in each sector
	Pred  [NumSectors]float32 // density of predators in each sector
	Wall  [NumSectors]float32 // proximity to wall in each sector
	Energy float32            // self energy [0,1]
	Speed  float32            // self speed normalized [0,1]
}

// AsSlice returns the sensor inputs as a flat slice for the neural network.
func (s *SensorInputs) AsSlice() []float32 {
	result := make([]float32, NumSectors*3+2)
	idx := 0
	for i := 0; i < NumSectors; i++ {
		result[idx] = s.Prey[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		result[idx] = s.Pred[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		result[idx] = s.Wall[i]
		idx++
	}
	result[idx] = s.Energy
	idx++
	result[idx] = s.Speed
	return result
}

// ComputeSensors calculates all sensor inputs for an entity.
func ComputeSensors(
	selfPos components.Position,
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	selfKind components.Kind,
	neighbors []ecs.Entity,
	posMap *ecs.Map1[components.Position],
	orgMap *ecs.Map1[components.Organism],
	worldWidth, worldHeight float32,
) SensorInputs {
	var inputs SensorInputs

	// Self-state
	speed := float32(math.Sqrt(float64(selfVel.X*selfVel.X + selfVel.Y*selfVel.Y)))
	inputs.Speed = clamp01(speed / selfCaps.MaxSpeed)
	inputs.Energy = clamp01(selfEnergy.Value)

	halfFOV := selfCaps.FOV / 2.0
	halfSectors := float32(NumSectors-1) / 2.0 // 2.0 for K=5

	// Process neighbors
	for _, neighbor := range neighbors {
		nPos := posMap.Get(neighbor)
		nOrg := orgMap.Get(neighbor)
		if nPos == nil || nOrg == nil {
			continue
		}

		// Compute delta with toroidal wrapping
		dx, dy := ToroidalDelta(selfPos.X, selfPos.Y, nPos.X, nPos.Y, worldWidth, worldHeight)
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist < 0.001 || dist > selfCaps.VisionRange {
			continue
		}

		// Angle to neighbor relative to heading
		angleToNeighbor := float32(math.Atan2(float64(dy), float64(dx)))
		relativeAngle := normalizeAngle(angleToNeighbor - selfRot.Heading)

		// Check if within FOV
		if relativeAngle < -halfFOV || relativeAngle > halfFOV {
			continue
		}

		// Map to sector
		t := relativeAngle / halfFOV // [-1, 1]
		sectorIdx := int(math.Round(float64(t * halfSectors)))
		sectorIdx += NumSectors / 2 // shift to [0, NumSectors-1]

		if sectorIdx < 0 {
			sectorIdx = 0
		} else if sectorIdx >= NumSectors {
			sectorIdx = NumSectors - 1
		}

		// Distance weight: closer = stronger signal
		weight := clamp01(1.0 - dist/selfCaps.VisionRange)

		// Accumulate by kind
		if nOrg.Kind == components.KindPrey {
			inputs.Prey[sectorIdx] += weight
		} else {
			inputs.Pred[sectorIdx] += weight
		}
	}

	// Normalize accumulated values with smooth saturation
	for i := 0; i < NumSectors; i++ {
		inputs.Prey[i] = smoothSaturate(inputs.Prey[i])
		inputs.Pred[i] = smoothSaturate(inputs.Pred[i])
	}

	// Wall proximity per sector
	inputs.Wall = computeWallProximity(selfPos, selfRot.Heading, selfCaps, worldWidth, worldHeight)

	return inputs
}

// computeWallProximity calculates wall distance for each vision sector.
// For a toroidal world, walls are effectively at infinity (no walls).
// If you want bounded mode, set wallMode = true.
func computeWallProximity(pos components.Position, heading float32, caps components.Capabilities, w, h float32) [NumSectors]float32 {
	var walls [NumSectors]float32

	halfFOV := caps.FOV / 2.0
	halfSectors := float32(NumSectors-1) / 2.0

	for i := 0; i < NumSectors; i++ {
		// Sector direction relative to heading
		t := (float32(i) - float32(NumSectors/2)) / halfSectors
		sectorAngle := heading + t*halfFOV

		dirX := float32(math.Cos(float64(sectorAngle)))
		dirY := float32(math.Sin(float64(sectorAngle)))

		// Distance to wall in this direction (bounded mode)
		var distToWall float32 = caps.VisionRange // default: no wall

		// Check horizontal walls
		if dirX > 0.001 {
			distToWall = minf(distToWall, (w-pos.X)/dirX)
		} else if dirX < -0.001 {
			distToWall = minf(distToWall, -pos.X/dirX)
		}

		// Check vertical walls
		if dirY > 0.001 {
			distToWall = minf(distToWall, (h-pos.Y)/dirY)
		} else if dirY < -0.001 {
			distToWall = minf(distToWall, -pos.Y/dirY)
		}

		// In toroidal mode, there are no walls, so this will always be VisionRange
		// Set to 0 (no wall signal) for toroidal worlds
		walls[i] = 0 // toroidal: no wall proximity
	}

	return walls
}

func normalizeAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

func clamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// smoothSaturate uses 1 - exp(-x) for smooth [0,1] saturation.
func smoothSaturate(x float32) float32 {
	if x <= 0 {
		return 0
	}
	return 1.0 - float32(math.Exp(-float64(x)))
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

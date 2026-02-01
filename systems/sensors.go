package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// Number of vision sectors.
const NumSectors = 5

// NumInputs is the total number of neural network inputs.
const NumInputs = NumSectors*3 + 2 // 17 for K=5

// SensorInputs holds the computed sensor values for one entity.
// Total: K*3 + 2 = 17 floats for K=5 sectors.
type SensorInputs struct {
	Prey     [NumSectors]float32 // density of prey in each sector
	Pred     [NumSectors]float32 // density of predators in each sector
	Resource [NumSectors]float32 // resource level ahead in each sector
	Energy   float32             // self energy [0,1]
	Speed    float32             // self speed normalized [0,1]
}

// FillSlice writes sensor inputs into dst (must be len >= NumInputs).
// Returns dst for convenience. Use this to avoid allocations.
func (s *SensorInputs) FillSlice(dst []float32) []float32 {
	idx := 0
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Prey[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Pred[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Resource[i]
		idx++
	}
	dst[idx] = s.Energy
	idx++
	dst[idx] = s.Speed
	return dst[:NumInputs]
}

// AsSlice returns the sensor inputs as a flat slice for the neural network.
// Deprecated: Use FillSlice to avoid allocations.
func (s *SensorInputs) AsSlice() []float32 {
	result := make([]float32, NumInputs)
	return s.FillSlice(result)
}

// ComputeSensorsFromNeighbors calculates sensor inputs using precomputed neighbor data.
// This is the optimized path that avoids recomputing distances.
func ComputeSensorsFromNeighbors(
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	neighbors []Neighbor,
	orgMap *ecs.Map1[components.Organism],
	resourceField ResourceSampler,
	selfPos components.Position,
) SensorInputs {
	var inputs SensorInputs

	// Self-state
	speed := float32(math.Sqrt(float64(selfVel.X*selfVel.X + selfVel.Y*selfVel.Y)))
	inputs.Speed = clamp01(speed / selfCaps.MaxSpeed)
	inputs.Energy = clamp01(selfEnergy.Value)

	halfFOV := selfCaps.FOV / 2.0
	halfSectors := float32(NumSectors-1) / 2.0 // 2.0 for K=5
	visionRangeSq := selfCaps.VisionRange * selfCaps.VisionRange

	// Process neighbors (already have DX, DY, DistSq from spatial query)
	for i := range neighbors {
		n := &neighbors[i]

		// Skip if too close or too far (DistSq already computed)
		if n.DistSq < 0.001 || n.DistSq > visionRangeSq {
			continue
		}

		// Angle to neighbor relative to heading (use precomputed DX, DY)
		angleToNeighbor := float32(math.Atan2(float64(n.DY), float64(n.DX)))
		relativeAngle := normalizeAngle(angleToNeighbor - selfRot.Heading)

		// Check if within FOV
		if relativeAngle < -halfFOV || relativeAngle > halfFOV {
			continue
		}

		// Get organism kind
		nOrg := orgMap.Get(n.E)
		if nOrg == nil {
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

		// Distance weight using sqrt of precomputed DistSq
		dist := float32(math.Sqrt(float64(n.DistSq)))
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

	// Resource level per sector
	inputs.Resource = computeResourceSensors(selfPos, selfRot.Heading, selfCaps, resourceField)

	return inputs
}

// ComputeSensors calculates all sensor inputs for an entity.
// Deprecated: Use ComputeSensorsFromNeighbors with QueryRadiusInto for better performance.
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
	resourceField ResourceSampler,
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

	// Resource level per sector
	inputs.Resource = computeResourceSensors(selfPos, selfRot.Heading, selfCaps, resourceField)

	return inputs
}

// computeResourceSensors samples the resource field ahead in each sector.
func computeResourceSensors(pos components.Position, heading float32, caps components.Capabilities, field ResourceSampler) [NumSectors]float32 {
	var res [NumSectors]float32

	if field == nil {
		return res
	}

	halfFOV := caps.FOV / 2.0
	half := float32(NumSectors-1) / 2.0
	sampleDist := caps.VisionRange * 0.7 // look ahead

	for i := 0; i < NumSectors; i++ {
		// Sector center angle
		t := (float32(i) - half) / half // [-1, 1]
		sectorAngle := heading + t*halfFOV

		dirX := float32(math.Cos(float64(sectorAngle)))
		dirY := float32(math.Sin(float64(sectorAngle)))

		// Sample point ahead in this sector
		sampleX := pos.X + dirX*sampleDist
		sampleY := pos.Y + dirY*sampleDist

		// Wrap toroidally
		sampleX = wrapMod(sampleX, field.Width())
		sampleY = wrapMod(sampleY, field.Height())

		res[i] = field.Sample(sampleX, sampleY)
	}

	return res
}

// wrapMod returns positive modulo for toroidal wrapping.
func wrapMod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
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

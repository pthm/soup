package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

// NumSectors is the number of vision sectors (compile-time constant for array sizing).
// This value must match config.yaml sensors.num_sectors.
const NumSectors = 8

// NumInputs is the total number of neural network inputs: NumSectors*3 + 2.
const NumInputs = NumSectors*3 + 2 // 26 for K=8

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
// All entities have 360° vision; effectiveness varies by angle and selfKind.
func ComputeSensorsFromNeighbors(
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	selfKind components.Kind,
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

	visionRangeSq := selfCaps.VisionRange * selfCaps.VisionRange
	sectorWidth := float32(2 * math.Pi / NumSectors)

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

		// Get organism kind
		nOrg := orgMap.Get(n.E)
		if nOrg == nil {
			continue
		}

		// Map to sector (full 360°, sector 2 = front)
		// Shift angle from [-π, π] to [0, 2π) then divide by sector width
		shiftedAngle := relativeAngle + math.Pi
		sectorIdx := int(shiftedAngle / sectorWidth)
		if sectorIdx >= NumSectors {
			sectorIdx = NumSectors - 1
		}
		if sectorIdx < 0 {
			sectorIdx = 0
		}

		// Distance weight using sqrt of precomputed DistSq
		dist := float32(math.Sqrt(float64(n.DistSq)))
		distWeight := clamp01(1.0 - dist/selfCaps.VisionRange)

		// Effectiveness weight based on angle and self kind
		effWeight := visionEffectiveness(relativeAngle, selfKind)
		weight := distWeight * effWeight

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

	// Resource level per sector (full 360°)
	inputs.Resource = computeResourceSensors(selfPos, selfRot.Heading, selfCaps, resourceField)

	return inputs
}

// visionEffectiveness returns the signal effectiveness [0,1] for a given angle and entity kind.
// Uses configurable vision zones from config.
func visionEffectiveness(relAngle float32, kind components.Kind) float32 {
	cfg := config.Cfg().Capabilities
	minEff := float32(cfg.MinEffectiveness)

	// Get zones for this kind
	var zones []config.VisionZone
	if kind == components.KindPredator {
		zones = cfg.Predator.VisionZones
	} else {
		zones = cfg.Prey.VisionZones
	}

	// If no zones defined, return minimum effectiveness
	if len(zones) == 0 {
		return minEff
	}

	// Calculate max effectiveness across all zones
	maxEff := float32(0)
	for _, zone := range zones {
		// Angular distance from zone center (handle wraparound)
		angleDist := normalizeAngle(relAngle - float32(zone.Angle))
		absAngleDist := float32(math.Abs(float64(angleDist)))

		// Smooth falloff within zone width using cosine
		// At center (dist=0): 1.0, at edge (dist=width): 0.0, beyond: 0.0
		if absAngleDist < float32(zone.Width) {
			// Cosine falloff: cos(0) = 1, cos(π/2) = 0
			t := absAngleDist / float32(zone.Width) * (math.Pi / 2)
			zoneEff := float32(zone.Power) * float32(math.Cos(float64(t)))
			if zoneEff > maxEff {
				maxEff = zoneEff
			}
		}
	}

	// Combine with minimum effectiveness
	return minEff + (1-minEff)*maxEff
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

	sectorWidth := float32(2 * math.Pi / NumSectors)

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

		// Map to sector (full 360°, sector 2 = front)
		shiftedAngle := relativeAngle + math.Pi
		sectorIdx := int(shiftedAngle / sectorWidth)
		if sectorIdx >= NumSectors {
			sectorIdx = NumSectors - 1
		}
		if sectorIdx < 0 {
			sectorIdx = 0
		}

		// Distance weight: closer = stronger signal
		distWeight := clamp01(1.0 - dist/selfCaps.VisionRange)

		// Effectiveness weight based on angle and self kind
		effWeight := visionEffectiveness(relativeAngle, selfKind)
		weight := distWeight * effWeight

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

	// Resource level per sector (full 360°)
	inputs.Resource = computeResourceSensors(selfPos, selfRot.Heading, selfCaps, resourceField)

	return inputs
}

// computeResourceSensors samples the resource field in each sector (full 360°).
func computeResourceSensors(pos components.Position, heading float32, caps components.Capabilities, field ResourceSampler) [NumSectors]float32 {
	var res [NumSectors]float32

	if field == nil {
		return res
	}

	sectorWidth := float32(2 * math.Pi / NumSectors)
	sampleDist := caps.VisionRange * float32(config.Cfg().Sensors.ResourceSampleDistance)

	for i := 0; i < NumSectors; i++ {
		// Sector center angle (sector 2 = front at 0°)
		// Sector i has center at: (i - NumSectors/2) * sectorWidth relative to heading
		// With shift: center at (i * sectorWidth - π) relative to heading
		relAngle := float32(i)*sectorWidth - math.Pi + sectorWidth/2
		sectorAngle := heading + relAngle

		dirX := float32(math.Cos(float64(sectorAngle)))
		dirY := float32(math.Sin(float64(sectorAngle)))

		// Sample point in this sector's direction
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

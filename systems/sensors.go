package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

// Cached config values for hot path (initialized by InitSensorCache)
var (
	cachedMinEffectiveness   float32
	cachedPreyVisionZones    []config.VisionZone
	cachedPredVisionZones    []config.VisionZone
	cachedResourceSampleDist float32
	cachedForageRate         float32
	cachedTransferEfficiency float32
	cacheInitialized         bool
)

// InitSensorCache caches config values for hot-path access.
// Must be called after config.Init().
func InitSensorCache() {
	cfg := config.Cfg()
	cachedMinEffectiveness = float32(cfg.Capabilities.MinEffectiveness)
	cachedPreyVisionZones = cfg.Capabilities.Prey.VisionZones
	cachedPredVisionZones = cfg.Capabilities.Predator.VisionZones
	cachedResourceSampleDist = float32(cfg.Sensors.ResourceSampleDistance)
	cachedForageRate = float32(cfg.Energy.Prey.ForageRate)
	cachedTransferEfficiency = float32(cfg.Energy.Predator.TransferEfficiency)
	cacheInitialized = true
}

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

	// Self-state (use fast sqrt)
	speedSq := selfVel.X*selfVel.X + selfVel.Y*selfVel.Y
	speed := fastSqrt(speedSq)
	inputs.Speed = clamp01(speed / selfCaps.MaxSpeed)
	inputs.Energy = clamp01(selfEnergy.Value)

	visionRangeSq := selfCaps.VisionRange * selfCaps.VisionRange
	invVisionRange := 1.0 / selfCaps.VisionRange

	// Precompute heading direction for local frame transformation (once per entity)
	// This replaces N*M atan2 calls with 2 sin/cos + N*M simple transforms
	cosH := fastCos(selfRot.Heading)
	sinH := fastSin(selfRot.Heading)

	// Process neighbors (already have DX, DY, DistSq from spatial query)
	for i := range neighbors {
		n := &neighbors[i]

		// Skip if too close or too far (DistSq already computed)
		if n.DistSq < 0.001 || n.DistSq > visionRangeSq {
			continue
		}

		// Get organism kind early to avoid work for nil
		nOrg := orgMap.Get(n.E)
		if nOrg == nil {
			continue
		}

		// Transform neighbor direction to local frame (no atan2!)
		// lx = forward component, ly = leftward component
		lx := n.DX*cosH + n.DY*sinH
		ly := -n.DX*sinH + n.DY*cosH

		// Determine sector using comparisons (no trig, no division)
		sectorIdx := sectorFromLocal(lx, ly)

		// Compute relative angle for vision effectiveness (still needed)
		// But now we can use local coords: relAngle = atan2(ly, lx)
		// Use fast approximation since we only need rough angle for effectiveness
		relativeAngle := fastAtan2(ly, lx)

		// Distance weight using fast sqrt
		dist := fastSqrt(n.DistSq)
		distWeight := clamp01(1.0 - dist*invVisionRange)

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
// Uses cached vision zones for performance.
func visionEffectiveness(relAngle float32, kind components.Kind) float32 {
	minEff := cachedMinEffectiveness

	// Get zones for this kind (cached)
	var zones []config.VisionZone
	if kind == components.KindPredator {
		zones = cachedPredVisionZones
	} else {
		zones = cachedPreyVisionZones
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
		absAngleDist := absf(angleDist)

		// Smooth falloff within zone width using cosine
		// At center (dist=0): 1.0, at edge (dist=width): 0.0, beyond: 0.0
		zoneWidth := float32(zone.Width)
		if absAngleDist < zoneWidth {
			// Cosine falloff: cos(0) = 1, cos(π/2) = 0
			t := absAngleDist / zoneWidth * (math.Pi / 2)
			zoneEff := float32(zone.Power) * fastCos(t)
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

// Precomputed sector relative angles (constant for all entities)
var sectorRelAngles [NumSectors]float32

func init() {
	sectorWidth := float32(2 * math.Pi / NumSectors)
	for i := 0; i < NumSectors; i++ {
		sectorRelAngles[i] = float32(i)*sectorWidth - math.Pi + sectorWidth/2
	}
}

// computeResourceSensors samples the resource field in each sector (full 360°).
func computeResourceSensors(pos components.Position, heading float32, caps components.Capabilities, field ResourceSampler) [NumSectors]float32 {
	var res [NumSectors]float32

	if field == nil {
		return res
	}

	sampleDist := caps.VisionRange * cachedResourceSampleDist
	fieldW := field.Width()
	fieldH := field.Height()

	for i := 0; i < NumSectors; i++ {
		sectorAngle := heading + sectorRelAngles[i]

		dirX := fastCos(sectorAngle)
		dirY := fastSin(sectorAngle)

		// Sample point in this sector's direction
		sampleX := pos.X + dirX*sampleDist
		sampleY := pos.Y + dirY*sampleDist

		// Wrap toroidally (inline for speed)
		if sampleX < 0 {
			sampleX += fieldW
		} else if sampleX >= fieldW {
			sampleX -= fieldW
		}
		if sampleY < 0 {
			sampleY += fieldH
		} else if sampleY >= fieldH {
			sampleY -= fieldH
		}

		res[i] = field.Sample(sampleX, sampleY)
	}

	return res
}

// wrapMod returns positive modulo for toroidal wrapping.
func wrapMod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
}

// normalizeAngle brings angle to [-π, π] with single-step correction.
// Works correctly when angle drift is bounded (e.g., heading += small_delta).
// For unbounded angles, use normalizeAngleFull.
func normalizeAngle(a float32) float32 {
	if a > math.Pi {
		a -= 2 * math.Pi
	} else if a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// normalizeAngleFull handles arbitrary angles using mod.
func normalizeAngleFull(a float32) float32 {
	const twoPi = 2 * math.Pi
	a = float32(math.Mod(float64(a), twoPi))
	if a > math.Pi {
		a -= twoPi
	} else if a < -math.Pi {
		a += twoPi
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
// Uses a fast approximation that's monotonic and accurate enough for sensor signals.
func smoothSaturate(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// For larger x, 1 - exp(-x) approaches 1, so we can clamp early
	if x > 5 {
		return 1
	}
	// Fast approximation: 1 - 1/(1 + x + 0.5*x*x + x*x*x/6)
	// This is the Taylor series denominator, giving exp(x) ≈ 1 + x + x²/2 + x³/6
	// So exp(-x) ≈ 1 / (1 + x + x²/2 + x³/6)
	x2 := x * x
	return 1.0 - 1.0/(1.0+x+0.5*x2+x*x2/6)
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

// absf returns the absolute value of x (avoids float64 conversion).
func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// sectorFromLocal determines sector index [0, NumSectors) from local frame coordinates.
// lx = forward component, ly = leftward component (positive = left of heading).
// Uses only comparisons - no trig, no division.
// For NumSectors=8: sector 0 = back, sector 4 = front.
func sectorFromLocal(lx, ly float32) int {
	// tan(π/8) ≈ 0.4142 - boundary slope for 8 sectors (45° each, split at 22.5°)
	const tanPi8 = 0.41421356

	// Get absolute values for magnitude comparisons
	alx := absf(lx)
	aly := absf(ly)

	if lx >= 0 {
		// Front half
		if ly >= 0 {
			// Front-left quadrant (sectors 2, 3, 4)
			if aly > alx {
				return 2 // left
			} else if aly > alx*tanPi8 {
				return 3 // front-left
			}
			return 4 // front
		} else {
			// Front-right quadrant (sectors 4, 5, 6)
			if aly > alx {
				return 6 // right
			} else if aly > alx*tanPi8 {
				return 5 // front-right
			}
			return 4 // front
		}
	} else {
		// Back half
		if ly >= 0 {
			// Back-left quadrant (sectors 0, 1, 2)
			if aly > alx {
				return 2 // left
			} else if aly > alx*tanPi8 {
				return 1 // back-left
			}
			return 0 // back
		} else {
			// Back-right quadrant (sectors 0, 6, 7)
			if aly > alx {
				return 6 // right
			} else if aly > alx*tanPi8 {
				return 7 // back-right
			}
			return 0 // back
		}
	}
}

// fastSqrt approximates sqrt(x) using the famous fast inverse sqrt + multiply.
// Uses one Newton-Raphson iteration for decent accuracy (~0.2% error).
func fastSqrt(x float32) float32 {
	if x <= 0 {
		return 0
	}
	// Initial guess using bit manipulation (Quake III style)
	i := math.Float32bits(x)
	i = 0x5f375a86 - (i >> 1) // Magic number for inverse sqrt
	y := math.Float32frombits(i)
	// One Newton-Raphson iteration for inverse sqrt: y = y * (1.5 - 0.5*x*y*y)
	y = y * (1.5 - 0.5*x*y*y)
	// sqrt(x) = x * inverseSqrt(x)
	return x * y
}

// fastSin approximates sin(x) using a polynomial. Accurate to ~0.001 for all x.
func fastSin(x float32) float32 {
	// Normalize to [-π, π]
	x = normalizeAngle(x)
	// Parabola approximation: sin(x) ≈ 4x(π-|x|) / π²
	// With correction factor for accuracy
	const pi = math.Pi
	const pi2 = pi * pi
	ax := absf(x)
	y := 4 * x * (pi - ax) / pi2
	// Correction: y = 0.225*(y*|y| - y) + y
	return 0.225*(y*absf(y)-y) + y
}

// fastCos approximates cos(x) using fastSin.
func fastCos(x float32) float32 {
	return fastSin(x + math.Pi/2)
}

// fastAtan2 approximates atan2(y, x). Accurate to ~0.01 radians.
func fastAtan2(y, x float32) float32 {
	if x == 0 {
		if y > 0 {
			return math.Pi / 2
		}
		if y < 0 {
			return -math.Pi / 2
		}
		return 0
	}

	// Compute atan(y/x)
	z := y / x
	var atan float32

	if absf(z) < 1 {
		// |z| < 1: use polynomial for atan
		atan = z / (1 + 0.28*z*z)
	} else {
		// |z| >= 1: use identity atan(z) = π/2 - atan(1/z)
		atan = math.Pi/2 - z/(z*z+0.28)
		if z < 0 {
			atan = -math.Pi/2 - z/(z*z+0.28)
		}
	}

	// Adjust for quadrant
	if x < 0 {
		if y >= 0 {
			return atan + math.Pi
		}
		return atan - math.Pi
	}
	return atan
}

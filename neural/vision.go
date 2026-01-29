package neural

import (
	"math"
)

// Cone indices for the 4-directional polar vision system.
// Angles are relative to organism heading (0 = forward).
const (
	ConeFront = 0 // [-π/4, π/4] - forward direction
	ConeRight = 1 // [π/4, 3π/4] - right side
	ConeBack  = 2 // [3π/4, -3π/4] or equivalently [3π/4, 5π/4] - behind
	ConeLeft  = 3 // [-3π/4, -π/4] - left side
)

// Vision system constants from spec.
const (
	NumCones         = 4
	NumChannels      = 3 // food, threat, friend
	DistanceFalloff  = 2   // inverse square law exponent
	SensorFocusK     = 4   // cosine power for sensor directional focus
	MinIntensity     = 1e-6
	MaxConeIntensity = 10.0 // cap to prevent extreme values
)

// PolarVision holds the computed cone intensities for each channel.
// Each array has 4 elements: [Front, Right, Back, Left].
type PolarVision struct {
	Food   [NumCones]float32 // Edible entities intensity per cone
	Threat [NumCones]float32 // Threatening entities intensity per cone
	Friend [NumCones]float32 // Genetically similar entities intensity per cone
	Light  [NumCones]float32 // Light level sampled in each cone direction (Phase 3b)
}

// LightSampleDistance is how far from the organism to sample directional light.
// Expressed as a fraction of the effective perception radius.
const LightSampleDistance = 0.5

// LightGradientEpsilon prevents division by zero in gradient calculations.
const LightGradientEpsilon = 1e-6

// SampleDirectionalLight samples light levels in each cone direction.
// sampler is a function that returns light level (0-1) at world coordinates.
// Returns per-cone light values for gradient computation.
func (pv *PolarVision) SampleDirectionalLight(
	posX, posY, heading, sampleRadius float32,
	sampler func(x, y float32) float32,
) {
	if sampler == nil {
		// No shadow map available; assume uniform light
		for i := 0; i < NumCones; i++ {
			pv.Light[i] = 0.5
		}
		return
	}

	// Sample light at a point in each cone direction
	distance := sampleRadius * LightSampleDistance

	for cone := 0; cone < NumCones; cone++ {
		// Get cone center angle (relative to heading)
		coneAngle := ConeCenter(cone)
		// Convert to world angle
		worldAngle := heading + coneAngle

		// Sample point in this direction
		sampleX := posX + distance*float32(math.Cos(float64(worldAngle)))
		sampleY := posY + distance*float32(math.Sin(float64(worldAngle)))

		pv.Light[cone] = sampler(sampleX, sampleY)
	}
}

// LightGradients computes front-back and left-right light gradients.
// Returns (light_fb, light_lr) where:
//   - light_fb = (front - back) / (front + back + epsilon)
//   - light_lr = (right - left) / (right + left + epsilon)
//
// Values range from -1 to +1:
//   - light_fb > 0 means brighter ahead
//   - light_lr > 0 means brighter to the right
func (pv *PolarVision) LightGradients() (lightFB, lightLR float32) {
	front := pv.Light[ConeFront]
	back := pv.Light[ConeBack]
	right := pv.Light[ConeRight]
	left := pv.Light[ConeLeft]

	lightFB = (front - back) / (front + back + LightGradientEpsilon)
	lightLR = (right - left) / (right + left + LightGradientEpsilon)

	return lightFB, lightLR
}

// SensorCell represents minimal sensor data needed for vision weighting.
type SensorCell struct {
	GridX    int8    // Position relative to organism center
	GridY    int8
	Strength float32 // Sensor function strength
}

// EntityInfo holds the data needed to evaluate an entity for vision.
type EntityInfo struct {
	X, Y            float32 // World position
	Composition     float32 // 0=fauna, 1=flora (from Composition())
	DigestiveSpec   float32 // 0=herbivore, 1=carnivore
	StructuralArmor float32
	GeneticDistance float32 // NEAT compatibility distance (lower = more similar)
	IsFlora         bool    // True for flora entities
	EmittedLight    float32 // Bioluminescence intensity (Phase 5b)
}

// VisionParams holds parameters for a vision scan.
type VisionParams struct {
	PosX, PosY       float32   // Observer position
	Heading          float32   // Observer heading in radians
	MyComposition    float32   // Observer's composition
	MyDigestiveSpec  float32   // Observer's digestive spectrum
	MyArmor          float32   // Observer's structural armor
	EffectiveRadius  float32   // Maximum vision range
	LightLevel       float32   // Local illumination (0-1)
	Sensors          []SensorCell // Observer's sensor cells
}

// AngleToCone returns which cone an angle falls into.
// Angle is relative to heading, in radians [-π, π].
func AngleToCone(angle float32) int {
	// Normalize to [-π, π]
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}

	// Map angle to cone
	// Front: [-π/4, π/4]
	// Right: [π/4, 3π/4]
	// Back: [3π/4, π] or [-π, -3π/4]
	// Left: [-3π/4, -π/4]
	absAngle := float32(math.Abs(float64(angle)))

	if absAngle <= math.Pi/4 {
		return ConeFront
	} else if absAngle >= 3*math.Pi/4 {
		return ConeBack
	} else if angle > 0 {
		return ConeRight
	} else {
		return ConeLeft
	}
}

// ConeCenter returns the center angle for a cone (relative to heading).
func ConeCenter(cone int) float32 {
	switch cone {
	case ConeFront:
		return 0
	case ConeRight:
		return math.Pi / 2
	case ConeBack:
		return math.Pi
	case ConeLeft:
		return -math.Pi / 2
	default:
		return 0
	}
}

// ScanEntities populates polar vision from a list of entities.
// This is the main vision computation function.
func (pv *PolarVision) ScanEntities(params VisionParams, entities []EntityInfo) {
	// Reset all cones
	for i := 0; i < NumCones; i++ {
		pv.Food[i] = 0
		pv.Threat[i] = 0
		pv.Friend[i] = 0
	}

	// Precompute sensor weights per cone
	sensorWeights := computeSensorConeWeights(params.Sensors)

	maxDistSq := params.EffectiveRadius * params.EffectiveRadius

	for _, entity := range entities {
		// Calculate distance and direction
		dx := entity.X - params.PosX
		dy := entity.Y - params.PosY
		distSq := dx*dx + dy*dy

		if distSq > maxDistSq || distSq < MinIntensity {
			continue
		}

		// Angle to entity relative to heading
		angleToEntity := float32(math.Atan2(float64(dy), float64(dx))) - params.Heading

		// Determine which cone
		cone := AngleToCone(angleToEntity)

		// Base intensity with inverse square falloff
		// intensity = 1 / distance^2
		intensity := 1.0 / distSq

		// Apply light modifier (darkness attenuates vision)
		// Phase 5b: Bioluminescent entities are visible even in darkness
		// effectiveLight = ambient light + entity's emitted light
		effectiveLight := params.LightLevel + entity.EmittedLight
		if effectiveLight > 1.0 {
			effectiveLight = 1.0 // Cap at full visibility
		}
		intensity *= effectiveLight

		// Apply sensor weighting for this cone
		intensity *= sensorWeights[cone]

		if intensity < MinIntensity {
			continue
		}

		// Calculate food relevance (can I eat this?)
		foodRelevance := float32(0)
		if entity.IsFlora {
			// Flora: composition = 1.0
			foodRelevance = Penetration(
				Edibility(params.MyDigestiveSpec, 1.0),
				entity.StructuralArmor,
			)
		} else {
			// Fauna: use their actual composition
			foodRelevance = Penetration(
				Edibility(params.MyDigestiveSpec, entity.Composition),
				entity.StructuralArmor,
			)
		}

		// Calculate threat relevance (can they eat me?)
		threatRelevance := ThreatLevel(
			entity.DigestiveSpec,
			params.MyComposition,
			params.MyArmor,
		)

		// Calculate friend relevance (genetic similarity)
		// Lower genetic distance = higher similarity = higher friend value
		// Use inverse: friendRelevance = 1 / (1 + distance)
		// Normalized so identical genomes (distance=0) give 1.0
		friendRelevance := float32(0)
		if !entity.IsFlora && entity.GeneticDistance >= 0 {
			// Only fauna can be friends, and only if we have genetic data
			friendRelevance = 1.0 / (1.0 + entity.GeneticDistance)
		}

		// Accumulate into cones
		pv.Food[cone] += foodRelevance * float32(intensity)
		pv.Threat[cone] += threatRelevance * float32(intensity)
		pv.Friend[cone] += friendRelevance * float32(intensity)
	}

	// Clamp all values
	for i := 0; i < NumCones; i++ {
		pv.Food[i] = clampVision(pv.Food[i])
		pv.Threat[i] = clampVision(pv.Threat[i])
		pv.Friend[i] = clampVision(pv.Friend[i])
	}
}

// computeSensorConeWeights calculates how much each cone benefits from sensor coverage.
// Sensors facing a cone direction contribute more to that cone's perception.
func computeSensorConeWeights(sensors []SensorCell) [NumCones]float32 {
	var weights [NumCones]float32

	if len(sensors) == 0 {
		// No sensors: equal weight to all cones (baseline perception)
		for i := 0; i < NumCones; i++ {
			weights[i] = 0.3 // Reduced baseline without sensors
		}
		return weights
	}

	var totalStrength float32
	for _, sensor := range sensors {
		totalStrength += sensor.Strength
	}

	if totalStrength < 0.01 {
		for i := 0; i < NumCones; i++ {
			weights[i] = 0.3
		}
		return weights
	}

	// For each sensor, calculate its contribution to each cone
	for _, sensor := range sensors {
		if sensor.Strength <= 0 {
			continue
		}

		// Sensor's facing direction (outward from center based on grid position)
		sensorFacing := float32(math.Atan2(float64(sensor.GridY), float64(sensor.GridX)))

		// For each cone, calculate how aligned this sensor is
		for cone := 0; cone < NumCones; cone++ {
			coneCenter := ConeCenter(cone)

			// Angular difference between sensor facing and cone center
			angleDiff := math.Abs(float64(normalizeAngleVision(sensorFacing - coneCenter)))

			// Cosine falloff with power k
			// cos(0) = 1 (perfectly aligned), cos(π/2) = 0 (perpendicular)
			alignment := math.Max(0, math.Cos(angleDiff))
			weight := math.Pow(alignment, SensorFocusK)

			weights[cone] += sensor.Strength * float32(weight)
		}
	}

	// Normalize by total sensor strength
	for i := 0; i < NumCones; i++ {
		weights[i] /= totalStrength
		// Ensure minimum perception even with poorly-aimed sensors
		if weights[i] < 0.1 {
			weights[i] = 0.1
		}
	}

	return weights
}

// normalizeAngleVision wraps an angle to [-π, π].
func normalizeAngleVision(angle float32) float32 {
	for angle > math.Pi {
		angle -= 2 * math.Pi
	}
	for angle < -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// clampVision clamps a cone intensity to valid range.
func clampVision(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > MaxConeIntensity {
		return MaxConeIntensity
	}
	return v
}

// NormalizeForBrain normalizes cone values to [0, 1] range suitable for neural input.
// Uses logarithmic scaling to handle the wide intensity range.
func (pv *PolarVision) NormalizeForBrain() PolarVision {
	var normalized PolarVision

	for i := 0; i < NumCones; i++ {
		// Log scale: log(1 + x) / log(1 + max) gives [0, 1]
		// With max of 10, values are nicely distributed
		normalized.Food[i] = logNormalize(pv.Food[i])
		normalized.Threat[i] = logNormalize(pv.Threat[i])
		normalized.Friend[i] = logNormalize(pv.Friend[i])
	}

	return normalized
}

// logNormalize applies logarithmic normalization to map [0, MaxConeIntensity] to [0, 1].
func logNormalize(v float32) float32 {
	if v <= 0 {
		return 0
	}
	// log(1 + v) / log(1 + max)
	maxLog := math.Log(1 + MaxConeIntensity)
	return float32(math.Log(float64(1+v)) / maxLog)
}


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
	cachedPreyVisionWeights  [NumSectors]float32
	cachedPredVisionWeights  [NumSectors]float32
	cachedResourceSampleDist float32
	cachedForageRate         float32
	cachedTransferEfficiency float32
	cachedDetritusFraction   float32
	cachedPreyAccelCost      float32
	cachedPredAccelCost      float32
	// Diet interpolation config
	cachedGrazingDietCap   float32
	cachedHuntingDietFloor float32
	// Pre-computed interpolation values
	cachedPreyBaseCost float32
	cachedPredBaseCost float32
	cachedPreyMoveCost float32
	cachedPredMoveCost float32
	// Bite cost and thrust deadzone for energy calculations
	cachedBiteCost        float32
	cachedThrustDeadzone  float32
	// Diet-relative sensor thresholds
	cachedDietThreshold float32
	cachedKinRange      float32
	cacheInitialized    bool
)

// InitSensorCache caches config values for hot-path access.
// Must be called after config.Init().
func InitSensorCache() {
	InitSensorCacheFrom(config.Cfg())
}

// InitSensorCacheFrom caches config values from the given config.
// Use this when running with a per-game config (e.g., optimizer).
func InitSensorCacheFrom(cfg *config.Config) {
	cachedMinEffectiveness = float32(cfg.Capabilities.MinEffectiveness)
	cachedPreyVisionWeights = loadVisionWeights(cfg.Capabilities.Prey.VisionWeights)
	cachedPredVisionWeights = loadVisionWeights(cfg.Capabilities.Predator.VisionWeights)
	cachedResourceSampleDist = float32(cfg.Sensors.ResourceSampleDistance)
	cachedForageRate = float32(cfg.Energy.Prey.ForageRate)
	cachedTransferEfficiency = float32(cfg.Energy.Predator.TransferEfficiency)
	cachedDetritusFraction = float32(cfg.Energy.Predator.DetritusFraction)
	cachedPreyAccelCost = float32(cfg.Energy.Prey.AccelCost)
	cachedPredAccelCost = float32(cfg.Energy.Predator.AccelCost)
	// Diet interpolation
	cachedGrazingDietCap = float32(cfg.Energy.Interpolation.GrazingDietCap)
	cachedHuntingDietFloor = float32(cfg.Energy.Interpolation.HuntingDietFloor)
	cachedPreyBaseCost = float32(cfg.Energy.Prey.BaseCost)
	cachedPredBaseCost = float32(cfg.Energy.Predator.BaseCost)
	cachedPreyMoveCost = float32(cfg.Energy.Prey.MoveCost)
	cachedPredMoveCost = float32(cfg.Energy.Predator.MoveCost)
	// Bite cost and thrust deadzone
	cachedBiteCost = float32(cfg.Energy.Predator.BiteCost)
	cachedThrustDeadzone = float32(cfg.Capabilities.ThrustDeadzone)
	// Diet-relative sensor thresholds
	cachedDietThreshold = float32(cfg.Sensors.DietThreshold)
	cachedKinRange = float32(cfg.Sensors.KinRange)
	cacheInitialized = true
}

// NumSectors is the number of vision sectors (compile-time constant for array sizing).
// This value must match config.yaml sensors.num_sectors.
const NumSectors = 8

// NumInputs is the total number of neural network inputs: NumSectors*3 + 3.
const NumInputs = NumSectors*3 + 3 // 27 for K=8 (food, threat, kin, energy, speed, diet)

// TopKPerSector is the max neighbors kept per (sector, kind) for bounded sensing.
// Total max neighbors processed = NumSectors * 2 * TopKPerSector = 32 for k=2.
const TopKPerSector = 2

// sectorBin holds top-k nearest neighbors for one (sector, kind) pair.
// Uses inline storage to avoid allocations.
type sectorBin struct {
	distSq [TopKPerSector]float32
	weight [TopKPerSector]float32 // precomputed distance * effectiveness weight
	count  int
}

// insert tries to add a neighbor to the bin, keeping only the k nearest.
func (b *sectorBin) insert(distSq, weight float32) {
	if b.count < TopKPerSector {
		// Room available, just append
		b.distSq[b.count] = distSq
		b.weight[b.count] = weight
		b.count++
		return
	}
	// Find furthest and replace if this is closer
	maxIdx := 0
	maxDist := b.distSq[0]
	for i := 1; i < TopKPerSector; i++ {
		if b.distSq[i] > maxDist {
			maxDist = b.distSq[i]
			maxIdx = i
		}
	}
	if distSq < maxDist {
		b.distSq[maxIdx] = distSq
		b.weight[maxIdx] = weight
	}
}

// sum returns the total weight in this bin.
func (b *sectorBin) sum() float32 {
	var s float32
	for i := 0; i < b.count; i++ {
		s += b.weight[i]
	}
	return s
}

// minDist returns the nearest distance in the bin (0 if empty).
func (b *sectorBin) minDist() float32 {
	if b.count == 0 {
		return 0
	}
	minDistSq := b.distSq[0]
	for i := 1; i < b.count; i++ {
		if b.distSq[i] < minDistSq {
			minDistSq = b.distSq[i]
		}
	}
	return fastSqrt(minDistSq)
}

// SectorBins holds per-sector top-k bins for food organisms, threat, and kin.
// Resource-based food is sampled separately and blended in.
// Reuse across calls to avoid allocations.
type SectorBins struct {
	FoodOrg [NumSectors]sectorBin // edible organisms
	Threat  [NumSectors]sectorBin // organisms that can eat me
	Kin     [NumSectors]sectorBin // similar diet organisms
}

// Clear resets all bins for reuse.
func (s *SectorBins) Clear() {
	for i := 0; i < NumSectors; i++ {
		s.FoodOrg[i].count = 0
		s.Threat[i].count = 0
		s.Kin[i].count = 0
	}
}

// SensorInputs holds the computed sensor values for one entity.
// Total: K*3 + 3 = 27 floats for K=8 sectors.
// Channels are diet-relative: food (can eat), threat (can eat me), kin (similar diet).
type SensorInputs struct {
	Food   [NumSectors]float32 // organisms with much lower diet (edible)
	Threat [NumSectors]float32 // organisms with much higher diet (dangerous)
	Kin    [NumSectors]float32 // organisms with similar diet
	// Nearest distances for inspector (not fed to neural network)
	NearestFood   [NumSectors]float32 // nearest food distance per sector (0 = none)
	NearestThreat [NumSectors]float32 // nearest threat distance per sector (0 = none)
	Energy        float32             // self energy [0,1]
	Speed         float32             // self speed normalized [0,1]
	Diet          float32             // self diet [0,1] (0=herbivore, 1=carnivore)
}

// FillSlice writes sensor inputs into dst (must be len >= NumInputs).
// Returns dst for convenience. Use this to avoid allocations.
// Order: Food[K], Threat[K], Kin[K], Energy, Speed, Diet
func (s *SensorInputs) FillSlice(dst []float32) []float32 {
	idx := 0
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Food[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Threat[i]
		idx++
	}
	for i := 0; i < NumSectors; i++ {
		dst[idx] = s.Kin[i]
		idx++
	}
	dst[idx] = s.Energy
	idx++
	dst[idx] = s.Speed
	idx++
	dst[idx] = s.Diet
	return dst[:NumInputs]
}

// AsSlice returns the sensor inputs as a flat slice for the neural network.
// Deprecated: Use FillSlice to avoid allocations.
func (s *SensorInputs) AsSlice() []float32 {
	result := make([]float32, NumInputs)
	return s.FillSlice(result)
}

// ComputeSensorsBounded calculates sensor inputs with bounded neighbor processing.
// Uses per-sector top-k sampling to cap work at O(NumSectors * TopKPerSector * 3).
// bins must be provided and will be cleared before use.
//
// Channels:
//   - Food: weighted blend of resource field and edible organisms
//     Food[k] = (1-diet)*FoodRes[k] + diet*FoodOrg[k]
//   - Threat: organisms that can eat me (edibility of me to them)
//   - Kin: organisms from same lineage (clade/archetype based)
//
// Edibility function: edibility(pred, prey) = pred.diet * (1 - prey.diet)
//   - Carnivore eating herbivore: 1.0 * 1.0 = 1.0 (high)
//   - Herbivore eating anything: 0.0 * x = 0.0 (can't hunt)
//   - Omnivore eating herbivore: 0.5 * 1.0 = 0.5 (moderate)
//
// Kin signal based on lineage:
//   - Same CladeID: 1.0 (direct relatives)
//   - Same ArchetypeID, different Clade: 0.5 (same species)
//   - Different ArchetypeID: 0.0 (different species)
func ComputeSensorsBounded(
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	selfDiet float32,
	selfCladeID uint64,
	selfArchetypeID uint8,
	neighbors []Neighbor,
	orgMap *ecs.Map1[components.Organism],
	resourceField ResourceSampler,
	selfPos components.Position,
	bins *SectorBins,
) SensorInputs {
	var inputs SensorInputs

	// Self-state
	speedSq := selfVel.X*selfVel.X + selfVel.Y*selfVel.Y
	speed := fastSqrt(speedSq)
	inputs.Speed = clamp01(speed / selfCaps.MaxSpeed)
	inputs.Energy = clamp01(selfEnergy.Value / selfEnergy.Max)
	inputs.Diet = selfDiet

	visionRangeSq := selfCaps.VisionRange * selfCaps.VisionRange
	invVisionRange := 1.0 / selfCaps.VisionRange

	// Precompute heading direction for local frame transformation
	cosH := fastCos(selfRot.Heading)
	sinH := fastSin(selfRot.Heading)

	// Clear bins for this entity
	bins.Clear()

	// Cap candidates processed (safety rail for density spikes)
	maxProcess := len(neighbors)
	if maxProcess > MaxQueryResults {
		maxProcess = MaxQueryResults
	}

	// Pass 1: Classify neighbors using edibility model
	for i := 0; i < maxProcess; i++ {
		n := &neighbors[i]

		// Skip if too close or too far
		if n.DistSq < 0.001 || n.DistSq > visionRangeSq {
			continue
		}

		// Get neighbor organism
		nOrg := orgMap.Get(n.E)
		if nOrg == nil {
			continue
		}

		// Transform to local frame
		lx := n.DX*cosH + n.DY*sinH
		ly := -n.DX*sinH + n.DY*cosH

		// Sector classification
		relativeAngle := fastAtan2(ly, lx)
		sectorIdx := sectorIndexFromAngle(relativeAngle)
		dist := fastSqrt(n.DistSq)
		distWeight := clamp01(1.0 - dist*invVisionRange)
		effWeight := VisionEffectivenessForSector(sectorIdx, selfDiet)
		baseWeight := distWeight * effWeight

		// Edibility model: edibility(pred, prey) = pred.diet * (1 - prey.diet)
		// Can I eat them? (I'm the predator)
		edSelf := selfDiet * (1.0 - nOrg.Diet)
		// Can they eat me? (They're the predator)
		edOther := nOrg.Diet * (1.0 - selfDiet)

		// Kin detection based on lineage (clade/archetype)
		// Same clade = 1.0, same archetype = 0.5, different archetype = 0.0
		var kinSimilarity float32
		if nOrg.CladeID == selfCladeID {
			kinSimilarity = 1.0 // Direct relatives
		} else if nOrg.FounderArchetypeID == selfArchetypeID {
			kinSimilarity = 0.5 // Same species, different lineage
		}

		// Food (edible organisms): weight by how edible they are to me
		if edSelf > 0.05 { // Small threshold to filter noise
			weight := baseWeight * edSelf
			bins.FoodOrg[sectorIdx].insert(n.DistSq, weight)
		}

		// Threat: weight by how edible I am to them
		if edOther > 0.05 {
			weight := baseWeight * edOther
			bins.Threat[sectorIdx].insert(n.DistSq, weight)
		}

		// Kin: same lineage organisms (same clade or archetype)
		if kinSimilarity > 0 {
			weight := baseWeight * kinSimilarity
			bins.Kin[sectorIdx].insert(n.DistSq, weight)
		}
	}

	// Pass 2: Aggregate and blend signals
	// Sample resource field for herbivore food component
	var resourceSignals [NumSectors]float32
	if resourceField != nil {
		resourceSignals = computeResourceSensors(selfPos, selfRot.Heading, selfCaps, resourceField)
	}

	// Blend weights based on diet:
	// - Herbivores (diet=0): Food = 100% resource, 0% organisms
	// - Carnivores (diet=1): Food = 0% resource, 100% organisms
	// - Omnivores (diet=0.5): Food = 50% resource, 50% organisms
	wRes := 1.0 - selfDiet // Resource weight
	wOrg := selfDiet       // Organism weight

	for i := 0; i < NumSectors; i++ {
		// Resource-based food (for herbivores)
		foodRes := resourceSignals[i]

		// Organism-based food (for carnivores)
		foodOrg := smoothSaturate(bins.FoodOrg[i].sum())

		// Blended food signal
		inputs.Food[i] = clamp01(wRes*foodRes + wOrg*foodOrg)

		// Threat and Kin
		inputs.Threat[i] = smoothSaturate(bins.Threat[i].sum())
		inputs.Kin[i] = smoothSaturate(bins.Kin[i].sum())

		// Nearest distances (for visualization)
		inputs.NearestFood[i] = bins.FoodOrg[i].minDist()
		inputs.NearestThreat[i] = bins.Threat[i].minDist()
	}

	return inputs
}

// ComputeSensorsFromNeighbors calculates sensor inputs using precomputed neighbor data.
// Deprecated: Use ComputeSensorsBounded for bounded work per entity.
func ComputeSensorsFromNeighbors(
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	selfDiet float32,
	selfCladeID uint64,
	selfArchetypeID uint8,
	neighbors []Neighbor,
	orgMap *ecs.Map1[components.Organism],
	resourceField ResourceSampler,
	selfPos components.Position,
) SensorInputs {
	var bins SectorBins
	return ComputeSensorsBounded(
		selfVel, selfRot, selfEnergy, selfCaps, selfDiet,
		selfCladeID, selfArchetypeID,
		neighbors, orgMap, resourceField, selfPos, &bins,
	)
}

// ComputeSensors calculates all sensor inputs for an entity.
// Deprecated: Use ComputeSensorsFromNeighbors with QueryRadiusInto for better performance.
func ComputeSensors(
	selfPos components.Position,
	selfVel components.Velocity,
	selfRot components.Rotation,
	selfEnergy components.Energy,
	selfCaps components.Capabilities,
	selfDiet float32,
	neighbors []ecs.Entity,
	posMap *ecs.Map1[components.Position],
	orgMap *ecs.Map1[components.Organism],
	worldWidth, worldHeight float32,
) SensorInputs {
	var inputs SensorInputs

	// Self-state
	speed := float32(math.Sqrt(float64(selfVel.X*selfVel.X + selfVel.Y*selfVel.Y)))
	inputs.Speed = clamp01(speed / selfCaps.MaxSpeed)
	inputs.Energy = clamp01(selfEnergy.Value / selfEnergy.Max)
	inputs.Diet = selfDiet

	// Diet thresholds
	dietThreshold := cachedDietThreshold
	kinRange := cachedKinRange

	// Accumulators for smooth saturation
	var foodAcc, threatAcc, kinAcc [NumSectors]float32

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

		// Map to sector (full 360°)
		sectorIdx := sectorIndexFromAngle(relativeAngle)

		// Distance weight: closer = stronger signal
		distWeight := clamp01(1.0 - dist/selfCaps.VisionRange)

		// Effectiveness weight based on angle and diet
		effWeight := VisionEffectivenessForSector(sectorIdx, selfDiet)

		// Diet-relative classification
		dietDiff := nOrg.Diet - selfDiet

		if dietDiff < -dietThreshold {
			// Food: neighbor has much lower diet
			dietMag := minf(1.0, -dietDiff)
			weight := distWeight * effWeight * dietMag
			foodAcc[sectorIdx] += weight
			if inputs.NearestFood[sectorIdx] == 0 || dist < inputs.NearestFood[sectorIdx] {
				inputs.NearestFood[sectorIdx] = dist
			}
		} else if dietDiff > dietThreshold {
			// Threat: neighbor has much higher diet
			dietMag := minf(1.0, dietDiff)
			weight := distWeight * effWeight * dietMag
			threatAcc[sectorIdx] += weight
			if inputs.NearestThreat[sectorIdx] == 0 || dist < inputs.NearestThreat[sectorIdx] {
				inputs.NearestThreat[sectorIdx] = dist
			}
		} else {
			// Kin: similar diet
			kinWeight := 1.0 - absf(dietDiff)/kinRange
			if kinWeight < 0 {
				kinWeight = 0
			}
			weight := distWeight * effWeight * kinWeight
			kinAcc[sectorIdx] += weight
		}
	}

	// Normalize accumulated values with smooth saturation
	for i := 0; i < NumSectors; i++ {
		inputs.Food[i] = smoothSaturate(foodAcc[i])
		inputs.Threat[i] = smoothSaturate(threatAcc[i])
		inputs.Kin[i] = smoothSaturate(kinAcc[i])
	}

	return inputs
}

// Precomputed sector relative angles (constant for all entities)
var sectorRelAngles [NumSectors]float32

func init() {
	for i := 0; i < NumSectors; i++ {
		sectorRelAngles[i] = sectorCenterAngle(i)
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

func maxf(a, b float32) float32 {
	if a > b {
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

package systems

import (
	"math"
)

// sectorWidth returns the angular width of a sector.
func sectorWidth() float32 {
	return float32(2 * math.Pi / NumSectors)
}

// sectorCenterAngle returns the center angle for a sector index.
// Index order: back, back-right, right, front-right, front, front-left, left, back-left.
func sectorCenterAngle(index int) float32 {
	half := float32(NumSectors) / 2
	return (float32(index) - half) * sectorWidth()
}

// SectorAngles returns the start and end angles (relative to heading) for a sector index.
func SectorAngles(index int) (float32, float32) {
	center := sectorCenterAngle(index)
	halfWidth := sectorWidth() * 0.5
	return center - halfWidth, center + halfWidth
}

// sectorIndexFromAngle maps a relative angle to a sector index.
func sectorIndexFromAngle(angle float32) int {
	angle = normalizeAngleFull(angle)
	width := sectorWidth()
	shifted := angle + math.Pi
	idx := int(math.Floor(float64(shifted/width + 0.5)))
	idx %= NumSectors
	if idx < 0 {
		idx += NumSectors
	}
	return idx
}

// VisionEffectivenessForSector returns the vision effectiveness for a sector.
// Uses diet to interpolate between default herbivore (diet=0) and carnivore (diet=1) weights.
// For archetype-specific weights, use VisionEffectivenessForArchetype.
func VisionEffectivenessForSector(sectorIdx int, diet float32) float32 {
	if sectorIdx < 0 || sectorIdx >= NumSectors {
		return cachedMinEffectiveness
	}
	// Fall back to archetype 0 and 1 weights for interpolation
	if len(cachedArchetypeVisionWeights) < 2 {
		return cachedMinEffectiveness
	}
	preyW := cachedArchetypeVisionWeights[0][sectorIdx]
	predW := cachedArchetypeVisionWeights[1][sectorIdx]
	weight := preyW + (predW-preyW)*diet
	return cachedMinEffectiveness + (1-cachedMinEffectiveness)*weight
}

// VisionEffectivenessForArchetype returns the vision effectiveness for a sector
// using the specific archetype's vision weights.
func VisionEffectivenessForArchetype(sectorIdx int, archetypeID uint8) float32 {
	if sectorIdx < 0 || sectorIdx >= NumSectors {
		return cachedMinEffectiveness
	}
	weights := GetArchetypeVisionWeights(archetypeID)
	if weights == nil {
		return cachedMinEffectiveness
	}
	return cachedMinEffectiveness + (1-cachedMinEffectiveness)*weights[sectorIdx]
}

// loadVisionWeights resolves per-sector weights from config, falling back to zones if provided.
func loadVisionWeights(weights []float64) [NumSectors]float32 {
	var out [NumSectors]float32
	if len(weights) == NumSectors {
		for i := 0; i < NumSectors; i++ {
			out[i] = clamp01(float32(weights[i]))
		}
	}
	return out
}

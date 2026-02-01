package systems

import (
	"math"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
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

func VisionEffectivenessForSector(sectorIdx int, kind components.Kind) float32 {
	if sectorIdx < 0 || sectorIdx >= NumSectors {
		return cachedMinEffectiveness
	}
	var weight float32
	if kind == components.KindPredator {
		weight = cachedPredVisionWeights[sectorIdx]
	} else {
		weight = cachedPreyVisionWeights[sectorIdx]
	}
	return cachedMinEffectiveness + (1-cachedMinEffectiveness)*weight
}

// loadVisionWeights resolves per-sector weights from config, falling back to zones if provided.
func loadVisionWeights(weights []float64, zones []config.VisionZone) [NumSectors]float32 {
	var out [NumSectors]float32
	if len(weights) == NumSectors {
		for i := 0; i < NumSectors; i++ {
			out[i] = clamp01(float32(weights[i]))
		}
		return out
	}
	if len(zones) == 0 {
		return out
	}
	for i := 0; i < NumSectors; i++ {
		relAngle := sectorCenterAngle(i)
		out[i] = clamp01(zoneEffectiveness(relAngle, zones))
	}
	return out
}

// zoneEffectiveness computes effectiveness at an angle based on legacy zones.
func zoneEffectiveness(relAngle float32, zones []config.VisionZone) float32 {
	maxEff := float32(0)
	for _, zone := range zones {
		angleDist := normalizeAngle(relAngle - float32(zone.Angle))
		absAngleDist := absf(angleDist)
		zoneWidth := float32(zone.Width)
		if absAngleDist < zoneWidth {
			t := absAngleDist / zoneWidth * (math.Pi / 2)
			zoneEff := float32(zone.Power) * fastCos(t)
			if zoneEff > maxEff {
				maxEff = zoneEff
			}
		}
	}
	return maxEff
}

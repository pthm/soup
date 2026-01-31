package neural

import (
	"math"
	"sort"

	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"
)

// SpeciesColor represents an RGB color for species visualization.
type SpeciesColor struct {
	R, G, B uint8
}

// Species represents a group of genetically similar organisms.
type Species struct {
	ID                  int
	Representative      *genetics.Genome // Used for compatibility comparisons
	Members             []int            // Entity IDs of members
	BestFitness         float64
	AvgFitness          float64
	TotalFitness        float64
	Age                 int // Generations since species was created
	Staleness           int // Generations without fitness improvement (or offspring in ecology mode)
	Color               SpeciesColor
	OffspringCount      int // Total offspring produced by this species
	GenerationOffspring int // Offspring produced this generation (reset each gen)
}

// SpeciesManager manages speciation for the population.
type SpeciesManager struct {
	Species                []*Species
	opts                   *neat.Options
	nextSpeciesID          int
	generation             int
	speciesColors          []SpeciesColor // Pre-generated distinct colors
	DisableFitnessTracking bool           // When true, skip fitness accumulation (persistent ecology mode)
}

// NewSpeciesManager creates a new species manager.
func NewSpeciesManager(opts *neat.Options) *SpeciesManager {
	sm := &SpeciesManager{
		Species:       make([]*Species, 0),
		opts:          opts,
		nextSpeciesID: 1,
		generation:    0,
		speciesColors: generateDistinctColors(64), // Pre-generate 64 colors
	}
	return sm
}

// generateDistinctColors creates visually distinct colors using golden angle.
func generateDistinctColors(count int) []SpeciesColor {
	colors := make([]SpeciesColor, count)
	goldenAngle := 137.508 // Golden angle in degrees

	for i := 0; i < count; i++ {
		// Use golden angle to spread hues evenly
		hue := float64(i) * goldenAngle
		hue = math.Mod(hue, 360.0)

		// Convert HSV to RGB (saturation=0.7, value=0.9 for pleasant colors)
		r, g, b := hsvToRGB(hue, 0.7, 0.9)
		colors[i] = SpeciesColor{R: r, G: g, B: b}
	}
	return colors
}

// hsvToRGB converts HSV to RGB.
func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	h = math.Mod(h, 360)
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c

	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}

	return uint8((r + m) * 255), uint8((g + m) * 255), uint8((b + m) * 255)
}

// AssignSpecies finds or creates a species for the given genome.
// Returns the species ID.
func (sm *SpeciesManager) AssignSpecies(genome *genetics.Genome) int {
	if genome == nil {
		return 0
	}

	// Try to find a compatible existing species
	for _, sp := range sm.Species {
		if sp.Representative == nil {
			continue
		}

		// Calculate compatibility distance
		dist := GenomeCompatibility(genome, sp.Representative, sm.opts)

		if dist < sm.opts.CompatThreshold {
			return sp.ID
		}
	}

	// No compatible species - create a new one
	colorIdx := sm.nextSpeciesID % len(sm.speciesColors)
	newSpecies := &Species{
		ID:             sm.nextSpeciesID,
		Representative: genome,
		Members:        make([]int, 0),
		Age:            0,
		Staleness:      0,
		Color:          sm.speciesColors[colorIdx],
	}
	sm.nextSpeciesID++
	sm.Species = append(sm.Species, newSpecies)

	return newSpecies.ID
}

// GetSpeciesColor returns the color for a species ID.
// Returns a default gray if species not found.
func (sm *SpeciesManager) GetSpeciesColor(speciesID int) SpeciesColor {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			return sp.Color
		}
	}
	return SpeciesColor{R: 128, G: 128, B: 128} // Gray default
}

// AddMember adds an organism to its species.
func (sm *SpeciesManager) AddMember(speciesID int, entityID int) {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			sp.Members = append(sp.Members, entityID)
			return
		}
	}
}

// RemoveMember removes an organism from its species.
func (sm *SpeciesManager) RemoveMember(speciesID int, entityID int) {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			for i, id := range sp.Members {
				if id == entityID {
					// Remove by swapping with last
					sp.Members[i] = sp.Members[len(sp.Members)-1]
					sp.Members = sp.Members[:len(sp.Members)-1]
					return
				}
			}
			return
		}
	}
}

// EndGeneration processes end-of-generation updates.
// Should be called once per generation after all fitness values are set.
func (sm *SpeciesManager) EndGeneration() {
	sm.generation++

	for _, sp := range sm.Species {
		sp.Age++

		if sm.DisableFitnessTracking {
			// In ecology mode, staleness is based on reproductive success
			// If no offspring this generation, increase staleness
			if sp.GenerationOffspring == 0 {
				sp.Staleness++
			}
			// Staleness reset happens in RecordOffspring when offspring are produced
		} else {
			// Calculate average fitness from total
			if len(sp.Members) > 0 && sp.TotalFitness > 0 {
				sp.AvgFitness = sp.TotalFitness / float64(len(sp.Members))
			}

			// Increase staleness (will be reset by AccumulateFitness if improved)
			sp.Staleness++

			// Reset fitness tracking for next generation
			sp.TotalFitness = 0
		}

		// Reset per-generation offspring counter
		sp.GenerationOffspring = 0
	}

	// Remove empty or stale species
	sm.RemoveStaleSpecies()
}

// RecordOffspring increments the offspring count for a species.
// When fitness tracking is disabled, this also resets staleness (reproductive success = not stale).
func (sm *SpeciesManager) RecordOffspring(speciesID int) {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			sp.OffspringCount++
			sp.GenerationOffspring++
			if sm.DisableFitnessTracking {
				sp.Staleness = 0 // Reproductive success indicates species viability
			}
			return
		}
	}
}

// AccumulateFitness adds to the total fitness for a species member.
// Called when updating an organism's fitness.
// When DisableFitnessTracking is true, this is a no-op (persistent ecology mode).
func (sm *SpeciesManager) AccumulateFitness(speciesID int, fitness float64) {
	if sm.DisableFitnessTracking {
		return // Skip fitness tracking in persistent ecology mode
	}
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			sp.TotalFitness += fitness
			if fitness > sp.BestFitness {
				sp.BestFitness = fitness
				sp.Staleness = 0 // Reset staleness on improvement
			}
			return
		}
	}
}

// RemoveStaleSpecies removes species that have no members or are too stale.
func (sm *SpeciesManager) RemoveStaleSpecies() {
	maxStaleness := sm.opts.DropOffAge
	active := make([]*Species, 0, len(sm.Species))

	for _, sp := range sm.Species {
		// Keep species that have members and aren't too stale
		if len(sp.Members) > 0 && sp.Staleness < maxStaleness {
			active = append(active, sp)
		}
	}

	sm.Species = active
}

// SpeciesStats contains summary statistics about all species.
type SpeciesStats struct {
	Count            int
	TotalMembers     int
	LargestSize      int
	SmallestSize     int
	AverageStaleness float64
	Generation       int
	TotalOffspring   int
	BestFitness      float64
}

// SpeciesInfo contains display information about a single species.
type SpeciesInfo struct {
	ID                  int
	Size                int
	BestFit             float64
	AvgFit              float64
	Age                 int
	Staleness           int
	Color               SpeciesColor
	Offspring           int // Total offspring
	GenerationOffspring int // Offspring this generation
}

// GetStats returns summary statistics about species distribution.
func (sm *SpeciesManager) GetStats() SpeciesStats {
	if len(sm.Species) == 0 {
		return SpeciesStats{Generation: sm.generation}
	}

	stats := SpeciesStats{
		Count:        len(sm.Species),
		LargestSize:  0,
		SmallestSize: int(^uint(0) >> 1), // Max int
		Generation:   sm.generation,
	}

	totalStaleness := 0
	for _, sp := range sm.Species {
		size := len(sp.Members)
		stats.TotalMembers += size
		stats.TotalOffspring += sp.OffspringCount

		if sp.BestFitness > stats.BestFitness {
			stats.BestFitness = sp.BestFitness
		}

		if size > stats.LargestSize {
			stats.LargestSize = size
		}
		if size < stats.SmallestSize && size > 0 {
			stats.SmallestSize = size
		}

		totalStaleness += sp.Staleness
	}

	if stats.Count > 0 {
		stats.AverageStaleness = float64(totalStaleness) / float64(stats.Count)
	}

	if stats.SmallestSize == int(^uint(0)>>1) {
		stats.SmallestSize = 0
	}

	return stats
}

// GetTopSpecies returns info about the top N species by size.
func (sm *SpeciesManager) GetTopSpecies(n int) []SpeciesInfo {
	if len(sm.Species) == 0 {
		return nil
	}

	// Copy species for sorting
	sorted := make([]*Species, len(sm.Species))
	copy(sorted, sm.Species)

	// Sort by member count descending using standard library
	sort.Slice(sorted, func(i, j int) bool {
		return len(sorted[i].Members) > len(sorted[j].Members)
	})

	// Take top N
	if n > len(sorted) {
		n = len(sorted)
	}

	result := make([]SpeciesInfo, n)
	for i := 0; i < n; i++ {
		sp := sorted[i]
		result[i] = SpeciesInfo{
			ID:                  sp.ID,
			Size:                len(sp.Members),
			BestFit:             sp.BestFitness,
			AvgFit:              sp.AvgFitness,
			Age:                 sp.Age,
			Staleness:           sp.Staleness,
			Color:               sp.Color,
			Offspring:           sp.OffspringCount,
			GenerationOffspring: sp.GenerationOffspring,
		}
	}

	return result
}

// FitnessTracker tracks fitness for organisms.
type FitnessTracker struct {
	// Fitness is calculated as: energy * survivalTime * reproductionBonus
	// This provides implicit fitness without explicit evaluation
}

// CalculateFitness calculates fitness for an organism based on its state.
// Higher is better.
func CalculateFitness(energy, maxEnergy float32, survivalTicks int32, offspringCount int) float64 {
	// Base fitness from energy ratio (clamped to avoid 0)
	energyRatio := float64(energy / maxEnergy)
	if energyRatio < 0.1 {
		energyRatio = 0.1 // Minimum to not zero out fitness
	}

	// Survival bonus - linear component rewards longevity
	// Both logarithmic (early survival) and linear (sustained survival) matter
	logSurvival := math.Log1p(float64(survivalTicks) / 500.0)
	linearSurvival := float64(survivalTicks) / 2000.0 // Linear bonus up to ~2.5 at 5000 ticks
	survivalBonus := logSurvival + linearSurvival*0.5

	// Reproduction bonus (reduced - survival should matter more)
	reproBonus := 1.0 + float64(offspringCount)*0.2

	return energyRatio * survivalBonus * reproBonus
}

// CalculateBreedingFitness calculates fitness specifically for breeding selection.
// Used to determine which parent contributes more genes during crossover.
func CalculateBreedingFitness(energy, maxEnergy float32, cellCount int) float64 {
	// For breeding, we care about current health
	energyRatio := float64(energy / maxEnergy)
	sizeBonus := 1.0 + float64(cellCount)*0.1

	return energyRatio * sizeBonus
}

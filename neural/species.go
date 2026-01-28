package neural

import (
	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"
)

// Species represents a group of genetically similar organisms.
type Species struct {
	ID             int
	Representative *genetics.Genome // Used for compatibility comparisons
	Members        []int            // Entity IDs of members
	BestFitness    float64
	AvgFitness     float64
	Age            int // Generations since species was created
	Staleness      int // Generations without fitness improvement
}

// SpeciesManager manages speciation for the population.
type SpeciesManager struct {
	Species       []*Species
	opts          *neat.Options
	nextSpeciesID int
}

// NewSpeciesManager creates a new species manager.
func NewSpeciesManager(opts *neat.Options) *SpeciesManager {
	return &SpeciesManager{
		Species:       make([]*Species, 0),
		opts:          opts,
		nextSpeciesID: 1,
	}
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
	newSpecies := &Species{
		ID:             sm.nextSpeciesID,
		Representative: genome,
		Members:        make([]int, 0),
		Age:            0,
		Staleness:      0,
	}
	sm.nextSpeciesID++
	sm.Species = append(sm.Species, newSpecies)

	return newSpecies.ID
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

// UpdateFitness updates the fitness statistics for a species.
func (sm *SpeciesManager) UpdateFitness(speciesID int, fitness float64) {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			if fitness > sp.BestFitness {
				sp.BestFitness = fitness
				sp.Staleness = 0
			}
			return
		}
	}
}

// GetSpecies returns the species with the given ID.
func (sm *SpeciesManager) GetSpecies(speciesID int) *Species {
	for _, sp := range sm.Species {
		if sp.ID == speciesID {
			return sp
		}
	}
	return nil
}

// GetSpeciesCount returns the number of active species.
func (sm *SpeciesManager) GetSpeciesCount() int {
	return len(sm.Species)
}

// GetMemberCount returns the total number of members across all species.
func (sm *SpeciesManager) GetMemberCount() int {
	count := 0
	for _, sp := range sm.Species {
		count += len(sp.Members)
	}
	return count
}

// EndGeneration processes end-of-generation updates.
// Should be called once per generation after all fitness values are set.
func (sm *SpeciesManager) EndGeneration() {
	for _, sp := range sm.Species {
		sp.Age++
		sp.Staleness++

		// Calculate average fitness
		if len(sp.Members) > 0 {
			// Note: actual fitness calculation would need to iterate over organisms
			// This is a placeholder - real implementation would sum actual fitness values
		}
	}

	// Remove empty or stale species
	sm.RemoveStaleSpecies()
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

// UpdateRepresentatives selects new representative genomes for each species.
// Should be called at the start of a new generation.
func (sm *SpeciesManager) UpdateRepresentatives(getGenome func(entityID int) *genetics.Genome) {
	for _, sp := range sm.Species {
		if len(sp.Members) > 0 {
			// Pick a random member as the new representative
			memberID := sp.Members[0] // Could randomize
			genome := getGenome(memberID)
			if genome != nil {
				sp.Representative = genome
			}
		}
		// Clear members for next generation
		sp.Members = sp.Members[:0]
	}
}

// GetSpeciesForGenome returns the species ID for a compatible genome without adding it.
func (sm *SpeciesManager) GetSpeciesForGenome(genome *genetics.Genome) int {
	if genome == nil {
		return 0
	}

	for _, sp := range sm.Species {
		if sp.Representative == nil {
			continue
		}

		dist := GenomeCompatibility(genome, sp.Representative, sm.opts)
		if dist < sm.opts.CompatThreshold {
			return sp.ID
		}
	}

	return 0 // No compatible species
}

// Stats returns summary statistics about the species.
type SpeciesStats struct {
	Count         int
	TotalMembers  int
	LargestSize   int
	SmallestSize  int
	AverageStaleness float64
}

// GetStats returns summary statistics about species distribution.
func (sm *SpeciesManager) GetStats() SpeciesStats {
	if len(sm.Species) == 0 {
		return SpeciesStats{}
	}

	stats := SpeciesStats{
		Count:        len(sm.Species),
		LargestSize:  0,
		SmallestSize: int(^uint(0) >> 1), // Max int
	}

	totalStaleness := 0
	for _, sp := range sm.Species {
		size := len(sp.Members)
		stats.TotalMembers += size

		if size > stats.LargestSize {
			stats.LargestSize = size
		}
		if size < stats.SmallestSize {
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

// CalculateCompatibility returns the compatibility distance between two genomes.
func (sm *SpeciesManager) CalculateCompatibility(g1, g2 *genetics.Genome) float64 {
	return GenomeCompatibility(g1, g2, sm.opts)
}

// FitnessTracker tracks fitness for organisms.
type FitnessTracker struct {
	// Fitness is calculated as: energy * survivalTime * reproductionBonus
	// This provides implicit fitness without explicit evaluation
}

// CalculateFitness calculates fitness for an organism based on its state.
// Higher is better.
func CalculateFitness(energy, maxEnergy float32, survivalTicks int32, offspringCount int) float64 {
	// Base fitness from energy ratio
	energyRatio := float64(energy / maxEnergy)

	// Survival bonus (more ticks alive = better adapted)
	survivalBonus := float64(survivalTicks) / 1000.0 // Normalize to reasonable range

	// Reproduction bonus (offspring count matters most for evolution)
	reproBonus := 1.0 + float64(offspringCount)*0.5

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

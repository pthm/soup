package telemetry

import (
	"log/slog"
	"math/rand"
	"sort"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/neural"
)

// HallEntry represents a successful organism's brain weights and fitness.
type HallEntry struct {
	Weights  neural.BrainWeights
	Fitness  float32
	Kind     components.Kind
	EntityID uint32
	Children int
	Kills    int
	Survival float32
	Foraging float32
}

// HallOfFame stores proven lineages for reseeding when populations crash.
type HallOfFame struct {
	prey     []HallEntry
	predator []HallEntry
	maxSize  int
	rng      *rand.Rand
}

// NewHallOfFame creates a new hall of fame with the given capacity per kind.
func NewHallOfFame(maxSize int, rng *rand.Rand) *HallOfFame {
	return &HallOfFame{
		prey:     make([]HallEntry, 0, maxSize),
		predator: make([]HallEntry, 0, maxSize),
		maxSize:  maxSize,
		rng:      rng,
	}
}

// Consider evaluates a dead organism for hall of fame entry.
// Returns true if the organism was added to the hall.
func (hof *HallOfFame) Consider(
	kind components.Kind,
	weights neural.BrainWeights,
	stats *LifetimeStats,
	entityID uint32,
) bool {
	cfg := config.Cfg()
	hofCfg := cfg.HallOfFame

	// Check entry criteria
	if !hof.meetsEntryCriteria(kind, stats, hofCfg) {
		return false
	}

	// Calculate fitness
	fitness := hof.calculateFitness(kind, stats, hofCfg)

	entry := HallEntry{
		Weights:  weights,
		Fitness:  fitness,
		Kind:     kind,
		EntityID: entityID,
		Children: stats.Children,
		Kills:    stats.Kills,
		Survival: stats.SurvivalTimeSec,
		Foraging: stats.TotalForaged,
	}

	// Add to appropriate hall
	hall := hof.getHall(kind)
	*hall = hof.insertEntry(*hall, entry)

	return true
}

// meetsEntryCriteria checks if an organism qualifies for the hall.
func (hof *HallOfFame) meetsEntryCriteria(
	kind components.Kind,
	stats *LifetimeStats,
	cfg config.HallOfFameConfig,
) bool {
	// Primary criterion: reproduced at least once
	if stats.Children >= cfg.Entry.MinChildren {
		return true
	}

	// Secondary criterion: survived long enough AND achieved something
	if stats.SurvivalTimeSec >= float32(cfg.Entry.MinSurvivalSec) {
		if kind == components.KindPredator {
			return stats.Kills >= cfg.Entry.MinKills
		}
		return stats.TotalForaged >= float32(cfg.Entry.MinForaging)
	}

	return false
}

// calculateFitness computes the weighted fitness score.
func (hof *HallOfFame) calculateFitness(
	kind components.Kind,
	stats *LifetimeStats,
	cfg config.HallOfFameConfig,
) float32 {
	fitness := float32(stats.Children) * float32(cfg.Fitness.ChildrenWeight)
	fitness += stats.SurvivalTimeSec * float32(cfg.Fitness.SurvivalWeight)

	if kind == components.KindPredator {
		fitness += float32(stats.Kills) * float32(cfg.Fitness.KillsWeight)
	} else {
		fitness += stats.TotalForaged * float32(cfg.Fitness.ForageWeight)
	}

	return fitness
}

// insertEntry adds an entry to the hall, maintaining sorted order by fitness.
// If the hall is full, the lowest-fitness entry is removed.
func (hof *HallOfFame) insertEntry(hall []HallEntry, entry HallEntry) []HallEntry {
	// Find insertion point (sorted descending by fitness)
	idx := sort.Search(len(hall), func(i int) bool {
		return hall[i].Fitness < entry.Fitness
	})

	// If hall is full and entry would be last (lowest), skip it
	if len(hall) >= hof.maxSize && idx >= hof.maxSize {
		return hall
	}

	// Insert at position
	hall = append(hall, HallEntry{})
	copy(hall[idx+1:], hall[idx:])
	hall[idx] = entry

	// Trim if over capacity
	if len(hall) > hof.maxSize {
		hall = hall[:hof.maxSize]
	}

	return hall
}

// Sample selects a brain from the hall using tournament selection.
// Returns nil if the hall is empty.
func (hof *HallOfFame) Sample(kind components.Kind) *neural.BrainWeights {
	hall := *hof.getHall(kind)
	if len(hall) == 0 {
		return nil
	}

	// Tournament selection with k=3
	const tournamentSize = 3
	var best *HallEntry

	for i := 0; i < tournamentSize && i < len(hall); i++ {
		idx := hof.rng.Intn(len(hall))
		candidate := &hall[idx]
		if best == nil || candidate.Fitness > best.Fitness {
			best = candidate
		}
	}

	if best == nil {
		return nil
	}

	// Return a copy of the weights
	weightsCopy := best.Weights
	return &weightsCopy
}

// Size returns the number of entries for a given kind.
func (hof *HallOfFame) Size(kind components.Kind) int {
	return len(*hof.getHall(kind))
}

// getHall returns a pointer to the hall for the given kind.
func (hof *HallOfFame) getHall(kind components.Kind) *[]HallEntry {
	if kind == components.KindPredator {
		return &hof.predator
	}
	return &hof.prey
}

// TopFitness returns the highest fitness in the hall for a given kind.
// Returns 0 if the hall is empty.
func (hof *HallOfFame) TopFitness(kind components.Kind) float32 {
	hall := *hof.getHall(kind)
	if len(hall) == 0 {
		return 0
	}
	return hall[0].Fitness
}

// Stats returns summary statistics for logging.
func (hof *HallOfFame) Stats() (preySize, predSize int, preyTopFit, predTopFit float32) {
	preySize = len(hof.prey)
	predSize = len(hof.predator)
	if preySize > 0 {
		preyTopFit = hof.prey[0].Fitness
	}
	if predSize > 0 {
		predTopFit = hof.predator[0].Fitness
	}
	return
}

// LogHallEntry logs when an entry is added to the hall.
func LogHallEntry(kind components.Kind, entityID uint32, fitness float32, hallSize int) {
	slog.Debug("hall_of_fame_entry",
		"kind", kind.String(),
		"entity_id", entityID,
		"fitness", fitness,
		"hall_size", hallSize,
	)
}

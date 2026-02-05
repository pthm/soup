package telemetry

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"os"
	"sort"

	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/neural"
)

// HallEntry represents a successful organism's brain weights and fitness.
type HallEntry struct {
	Weights            neural.BrainWeights
	Fitness            float32
	EntityID           uint32
	Children           int
	Kills              int
	Survival           float32
	Foraging           float32
	CladeID            uint64
	FounderArchetypeID uint8
	Diet               float32
}

// HallOfFame stores proven lineages for reseeding when populations crash.
// Halls are indexed by archetype ID (one hall per archetype).
type HallOfFame struct {
	halls   [][]HallEntry
	maxSize int
	rng     *rand.Rand
}

// NewHallOfFame creates a new hall of fame with the given capacity per archetype.
func NewHallOfFame(maxSize int, numArchetypes int, rng *rand.Rand) *HallOfFame {
	halls := make([][]HallEntry, numArchetypes)
	for i := range halls {
		halls[i] = make([]HallEntry, 0, maxSize)
	}
	return &HallOfFame{
		halls:   halls,
		maxSize: maxSize,
		rng:     rng,
	}
}

// Consider evaluates a dead organism for hall of fame entry.
// Returns true if the organism was added to the hall.
func (hof *HallOfFame) Consider(
	diet float32,
	weights neural.BrainWeights,
	stats *LifetimeStats,
	entityID uint32,
) bool {
	cfg := config.Cfg()
	hofCfg := cfg.HallOfFame

	// Check entry criteria
	if !hof.meetsEntryCriteria(diet, stats, hofCfg) {
		return false
	}

	// Calculate fitness
	fitness := hof.calculateFitness(diet, stats, hofCfg)

	entry := HallEntry{
		Weights:            weights,
		Fitness:            fitness,
		EntityID:           entityID,
		Children:           stats.Children,
		Kills:              stats.Kills,
		Survival:           stats.SurvivalTimeSec,
		Foraging:           stats.TotalForaged,
		CladeID:            stats.CladeID,
		FounderArchetypeID: stats.FounderArchetypeID,
		Diet:               stats.BirthDiet,
	}

	// Add to appropriate hall (by founder archetype)
	archetypeID := stats.FounderArchetypeID
	hall := hof.getHall(archetypeID)
	*hall = hof.insertEntry(*hall, entry)

	return true
}

// meetsEntryCriteria checks if an organism qualifies for the hall.
func (hof *HallOfFame) meetsEntryCriteria(
	diet float32,
	stats *LifetimeStats,
	cfg config.HallOfFameConfig,
) bool {
	// Primary criterion: reproduced at least once
	if stats.Children >= cfg.Entry.MinChildren {
		return true
	}

	// Secondary criterion: survived long enough AND achieved something
	if stats.SurvivalTimeSec >= float32(cfg.Entry.MinSurvivalSec) {
		if diet >= 0.5 {
			return stats.Kills >= cfg.Entry.MinKills
		}
		return stats.TotalForaged >= float32(cfg.Entry.MinForaging)
	}

	return false
}

// calculateFitness computes the weighted fitness score.
func (hof *HallOfFame) calculateFitness(
	diet float32,
	stats *LifetimeStats,
	cfg config.HallOfFameConfig,
) float32 {
	fitness := float32(stats.Children) * float32(cfg.Fitness.ChildrenWeight)
	fitness += stats.SurvivalTimeSec * float32(cfg.Fitness.SurvivalWeight)

	if diet >= 0.5 {
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
func (hof *HallOfFame) Sample(archetypeID uint8) *neural.BrainWeights {
	hall := *hof.getHall(archetypeID)
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

// Size returns the number of entries for a given archetype.
func (hof *HallOfFame) Size(archetypeID uint8) int {
	return len(*hof.getHall(archetypeID))
}

// getHall returns a pointer to the hall for the given archetype.
func (hof *HallOfFame) getHall(archetypeID uint8) *[]HallEntry {
	if int(archetypeID) >= len(hof.halls) {
		// Safety: return an empty hall for unknown archetypes
		empty := make([]HallEntry, 0)
		return &empty
	}
	return &hof.halls[archetypeID]
}

// TopFitness returns the highest fitness in the hall for a given archetype.
// Returns 0 if the hall is empty.
func (hof *HallOfFame) TopFitness(archetypeID uint8) float32 {
	hall := *hof.getHall(archetypeID)
	if len(hall) == 0 {
		return 0
	}
	return hall[0].Fitness
}

// Stats returns summary statistics for logging.
func (hof *HallOfFame) Stats() (sizes []int, topFitnesses []float32) {
	sizes = make([]int, len(hof.halls))
	topFitnesses = make([]float32, len(hof.halls))
	for i, hall := range hof.halls {
		sizes[i] = len(hall)
		if len(hall) > 0 {
			topFitnesses[i] = hall[0].Fitness
		}
	}
	return
}

// hallEntryJSON is the JSON-serializable representation of a hall entry.
type hallEntryJSON struct {
	EntityID         uint32              `json:"entity_id"`
	Fitness          float32             `json:"fitness"`
	Children         int                 `json:"children"`
	Kills            int                 `json:"kills"`
	Survival         float32             `json:"survival_sec"`
	Foraging         float32             `json:"foraging"`
	CladeID          uint64              `json:"clade_id"`
	FounderArchetype string              `json:"founder_archetype"`
	Diet             float32             `json:"diet"`
	Weights          neural.BrainWeights `json:"brain"`
}

// MarshalJSON serializes the hall of fame to JSON.
// Keys are archetype names (e.g., "grazer", "hunter").
func (hof *HallOfFame) MarshalJSON() ([]byte, error) {
	cfg := config.Cfg()
	export := make(map[string][]hallEntryJSON)

	for archIdx, hall := range hof.halls {
		archetypeName := "unknown"
		if archIdx < len(cfg.Archetypes) {
			archetypeName = cfg.Archetypes[archIdx].Name
		}

		entries := make([]hallEntryJSON, len(hall))
		for i, entry := range hall {
			entryArchName := "unknown"
			if int(entry.FounderArchetypeID) < len(cfg.Archetypes) {
				entryArchName = cfg.Archetypes[entry.FounderArchetypeID].Name
			}
			entries[i] = hallEntryJSON{
				EntityID:         entry.EntityID,
				Fitness:          entry.Fitness,
				Children:         entry.Children,
				Kills:            entry.Kills,
				Survival:         entry.Survival,
				Foraging:         entry.Foraging,
				CladeID:          entry.CladeID,
				FounderArchetype: entryArchName,
				Diet:             entry.Diet,
				Weights:          entry.Weights,
			}
		}
		export[archetypeName] = entries
	}

	return json.MarshalIndent(export, "", "  ")
}

// LoadHallOfFameFromFile reads a hall of fame JSON file and returns a HallOfFame
// populated with the entries. archetypeIndex maps archetype names (e.g. "grazer")
// to their index, and numArchetypes is the total number of archetypes.
func LoadHallOfFameFromFile(path string, numArchetypes int, archetypeIndex map[string]uint8, rng *rand.Rand) (*HallOfFame, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading hall of fame: %w", err)
	}

	var raw map[string][]hallEntryJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing hall of fame JSON: %w", err)
	}

	// Determine max size from the largest hall in the file
	maxSize := 30 // reasonable default
	for _, entries := range raw {
		if len(entries) > maxSize {
			maxSize = len(entries)
		}
	}

	hof := NewHallOfFame(maxSize, numArchetypes, rng)

	for archName, entries := range raw {
		archIdx, ok := archetypeIndex[archName]
		if !ok {
			slog.Warn("hall_of_fame_load: unknown archetype, skipping", "archetype", archName)
			continue
		}

		for _, ej := range entries {
			// Map the founder archetype name back to index
			founderIdx := archIdx // default to the hall's archetype
			if fi, ok := archetypeIndex[ej.FounderArchetype]; ok {
				founderIdx = fi
			}

			entry := HallEntry{
				Weights:            ej.Weights,
				Fitness:            ej.Fitness,
				EntityID:           ej.EntityID,
				Children:           ej.Children,
				Kills:              ej.Kills,
				Survival:           ej.Survival,
				Foraging:           ej.Foraging,
				CladeID:            ej.CladeID,
				FounderArchetypeID: founderIdx,
				Diet:               ej.Diet,
			}

			hall := hof.getHall(archIdx)
			*hall = hof.insertEntry(*hall, entry)
		}
	}

	return hof, nil
}

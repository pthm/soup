package telemetry

// LifetimeStats tracks per-entity statistics over its lifetime.
type LifetimeStats struct {
	BirthTick       int32
	SurvivalTimeSec float32

	// Clade/archetype tracking
	CladeID            uint64
	FounderArchetypeID uint8
	BirthDiet          float32

	// Hunting (predators)
	BitesAttempted int
	BitesHit       int
	Kills          int

	// Reproduction
	Children int

	// Energy
	PeakEnergy   float32
	TotalForaged float32 // prey only: cumulative energy gained from foraging
}

// LifetimeTracker manages per-entity lifetime statistics.
type LifetimeTracker struct {
	stats map[uint32]*LifetimeStats
}

// NewLifetimeTracker creates a new lifetime tracker.
func NewLifetimeTracker() *LifetimeTracker {
	return &LifetimeTracker{
		stats: make(map[uint32]*LifetimeStats),
	}
}

// Register creates lifetime stats for a new entity with clade tracking info.
func (lt *LifetimeTracker) Register(entityID uint32, birthTick int32, cladeID uint64, archetypeID uint8, diet float32) {
	lt.stats[entityID] = &LifetimeStats{
		BirthTick:          birthTick,
		CladeID:            cladeID,
		FounderArchetypeID: archetypeID,
		BirthDiet:          diet,
	}
}

// Get returns the lifetime stats for an entity, or nil if not found.
func (lt *LifetimeTracker) Get(entityID uint32) *LifetimeStats {
	return lt.stats[entityID]
}

// Remove removes an entity's stats and returns them (for snapshot/logging).
func (lt *LifetimeTracker) Remove(entityID uint32) *LifetimeStats {
	stats := lt.stats[entityID]
	delete(lt.stats, entityID)
	return stats
}

// RecordBiteAttempt increments bite attempt count.
func (lt *LifetimeTracker) RecordBiteAttempt(entityID uint32) {
	if s := lt.stats[entityID]; s != nil {
		s.BitesAttempted++
	}
}

// RecordBiteHit increments successful bite count.
func (lt *LifetimeTracker) RecordBiteHit(entityID uint32) {
	if s := lt.stats[entityID]; s != nil {
		s.BitesHit++
	}
}

// RecordKill increments kill count.
func (lt *LifetimeTracker) RecordKill(entityID uint32) {
	if s := lt.stats[entityID]; s != nil {
		s.Kills++
	}
}

// RecordChild increments children count.
func (lt *LifetimeTracker) RecordChild(parentID uint32) {
	if s := lt.stats[parentID]; s != nil {
		s.Children++
	}
}

// RecordForage adds foraging gain to cumulative total.
func (lt *LifetimeTracker) RecordForage(entityID uint32, amount float32) {
	if s := lt.stats[entityID]; s != nil {
		s.TotalForaged += amount
	}
}

// UpdateEnergy tracks peak energy.
func (lt *LifetimeTracker) UpdateEnergy(entityID uint32, energy float32) {
	if s := lt.stats[entityID]; s != nil {
		if energy > s.PeakEnergy {
			s.PeakEnergy = energy
		}
	}
}

// UpdateSurvivalTime updates the survival time based on current tick.
func (lt *LifetimeTracker) UpdateSurvivalTime(entityID uint32, currentTick int32, dt float32) {
	if s := lt.stats[entityID]; s != nil {
		s.SurvivalTimeSec = float32(currentTick-s.BirthTick) * dt
	}
}

// All returns all tracked stats (for snapshots).
func (lt *LifetimeTracker) All() map[uint32]*LifetimeStats {
	return lt.stats
}

// Count returns the number of tracked entities.
func (lt *LifetimeTracker) Count() int {
	return len(lt.stats)
}

// ActiveCladeCount returns the number of unique clades among living entities.
func (lt *LifetimeTracker) ActiveCladeCount() int {
	seen := make(map[uint64]struct{})
	for _, stats := range lt.stats {
		seen[stats.CladeID] = struct{}{}
	}
	return len(seen)
}

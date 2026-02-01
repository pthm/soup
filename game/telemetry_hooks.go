package game

import (
	"log/slog"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/telemetry"
)

// flushTelemetry checks if the stats window should be flushed and handles bookmarks.
func (g *Game) flushTelemetry() {
	if !g.collector.ShouldFlush(g.tick) {
		return
	}

	// Sample energy distributions and resource utilization
	preyEnergies, predEnergies, meanResource := g.sampleEnergyDistributions()

	// Flush the stats window
	stats := g.collector.Flush(g.tick, g.numPrey, g.numPred, preyEnergies, predEnergies, meanResource)

	// Log stats if enabled
	if g.logStats {
		stats.LogStats()
		g.perfCollector.Stats().LogStats()
	}

	// Check for bookmarks
	bookmarks := g.bookmarkDetector.Check(stats)
	for _, bm := range bookmarks {
		if g.logStats {
			bm.LogBookmark()
		}

		// Save snapshot on bookmark
		if g.snapshotDir != "" {
			g.saveSnapshot(&bm)
		}
	}
}

// sampleEnergyDistributions collects energy values for percentile calculation.
func (g *Game) sampleEnergyDistributions() (preyEnergies, predEnergies []float64, meanResource float64) {
	var resourceSum float64
	var preyCount int

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		if org.Kind == components.KindPrey {
			preyEnergies = append(preyEnergies, float64(energy.Value))
			resourceSum += float64(g.resourceField.Sample(pos.X, pos.Y))
			preyCount++
		} else {
			predEnergies = append(predEnergies, float64(energy.Value))
		}

		// Update lifetime peak energy
		g.lifetimeTracker.UpdateEnergy(org.ID, energy.Value)
	}

	if preyCount > 0 {
		meanResource = resourceSum / float64(preyCount)
	}

	return preyEnergies, predEnergies, meanResource
}

// saveSnapshot creates and saves a snapshot to disk.
func (g *Game) saveSnapshot(bookmark *telemetry.Bookmark) {
	snapshot := g.createSnapshot(bookmark)

	path, err := telemetry.SaveSnapshot(snapshot, g.snapshotDir)
	if err != nil {
		slog.Error("failed to save snapshot", "error", err)
		return
	}

	slog.Info("snapshot saved", "path", path, "tick", g.tick)
}

// createSnapshot builds a snapshot from the current state.
func (g *Game) createSnapshot(bookmark *telemetry.Bookmark) *telemetry.Snapshot {
	snapshot := &telemetry.Snapshot{
		Version:     telemetry.SnapshotVersion,
		RNGSeed:     g.rngSeed,
		WorldWidth:  g.width,
		WorldHeight: g.height,
		Tick:        g.tick,
		Bookmark:    bookmark,
	}

	// Serialize CPU resource field if available
	if g.cpuResourceField != nil {
		w, h := g.cpuResourceField.GridSize()
		snapshot.ResourceGridW = w
		snapshot.ResourceGridH = h
		snapshot.ResourceRes = make([]float32, len(g.cpuResourceField.Res))
		copy(snapshot.ResourceRes, g.cpuResourceField.Res)
		snapshot.ResourceCap = make([]float32, len(g.cpuResourceField.Cap))
		copy(snapshot.ResourceCap, g.cpuResourceField.Cap)
		snapshot.ResourceTime = g.cpuResourceField.Time
	}

	// Collect entity states
	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, rot, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Get brain weights
		brain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Get lifetime stats
		var lifetime *telemetry.LifetimeStatsJSON
		if ls := g.lifetimeTracker.Get(org.ID); ls != nil {
			lifetime = ls.ToJSON()
		}

		state := telemetry.EntityState{
			ID:             org.ID,
			Kind:           org.Kind,
			X:              pos.X,
			Y:              pos.Y,
			VelX:           vel.X,
			VelY:           vel.Y,
			Heading:        rot.Heading,
			Energy:         energy.Value,
			Age:            energy.Age,
			ReproCooldown:  org.ReproCooldown,
			DigestCooldown: org.DigestCooldown,
			Brain:          brain.MarshalWeights(),
			Lifetime:       lifetime,
		}

		snapshot.Entities = append(snapshot.Entities, state)
	}

	return snapshot
}

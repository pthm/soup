package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// SnapshotVersion is incremented when the format changes.
const SnapshotVersion = 1

// Snapshot holds the complete simulation state for replay.
type Snapshot struct {
	Version int   `json:"version"`
	RNGSeed int64 `json:"rng_seed"`

	WorldWidth  float32 `json:"world_width"`
	WorldHeight float32 `json:"world_height"`

	ResourceHotspots []systems.HotspotDef `json:"resource_hotspots"`
	ResourceSigma    float32              `json:"resource_sigma"`

	Tick int32 `json:"tick"`

	Entities []EntityState `json:"entities"`

	Bookmark *Bookmark `json:"bookmark,omitempty"`
}

// EntityState holds one entity's complete state.
type EntityState struct {
	ID   uint32          `json:"id"`
	Kind components.Kind `json:"kind"`

	// Position and movement
	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	VelX    float32 `json:"vel_x"`
	VelY    float32 `json:"vel_y"`
	Heading float32 `json:"heading"`

	// Organism state
	Energy         float32 `json:"energy"`
	Age            float32 `json:"age"`
	ReproCooldown  float32 `json:"repro_cooldown"`
	DigestCooldown float32 `json:"digest_cooldown"`

	// Brain weights
	Brain neural.BrainWeights `json:"brain"`

	// Lifetime stats
	Lifetime *LifetimeStatsJSON `json:"lifetime,omitempty"`
}

// LifetimeStatsJSON is the JSON-serializable form of LifetimeStats.
type LifetimeStatsJSON struct {
	BirthTick       int32   `json:"birth_tick"`
	SurvivalTimeSec float32 `json:"survival_time_sec"`
	BitesAttempted  int     `json:"bites_attempted"`
	BitesHit        int     `json:"bites_hit"`
	Kills           int     `json:"kills"`
	Children        int     `json:"children"`
	PeakEnergy      float32 `json:"peak_energy"`
	TotalForaged    float32 `json:"total_foraged"`
}

// ToJSON converts LifetimeStats to its JSON form.
func (ls *LifetimeStats) ToJSON() *LifetimeStatsJSON {
	if ls == nil {
		return nil
	}
	return &LifetimeStatsJSON{
		BirthTick:       ls.BirthTick,
		SurvivalTimeSec: ls.SurvivalTimeSec,
		BitesAttempted:  ls.BitesAttempted,
		BitesHit:        ls.BitesHit,
		Kills:           ls.Kills,
		Children:        ls.Children,
		PeakEnergy:      ls.PeakEnergy,
		TotalForaged:    ls.TotalForaged,
	}
}

// FromJSON converts the JSON form back to LifetimeStats.
func (lsj *LifetimeStatsJSON) FromJSON() *LifetimeStats {
	if lsj == nil {
		return nil
	}
	return &LifetimeStats{
		BirthTick:       lsj.BirthTick,
		SurvivalTimeSec: lsj.SurvivalTimeSec,
		BitesAttempted:  lsj.BitesAttempted,
		BitesHit:        lsj.BitesHit,
		Kills:           lsj.Kills,
		Children:        lsj.Children,
		PeakEnergy:      lsj.PeakEnergy,
		TotalForaged:    lsj.TotalForaged,
	}
}

// SaveSnapshot writes a snapshot to disk.
// Returns the filepath where it was saved.
func SaveSnapshot(snapshot *Snapshot, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create snapshot dir: %w", err)
	}

	// Build filename
	name := fmt.Sprintf("snapshot_%d", snapshot.Tick)
	if snapshot.Bookmark != nil {
		// Sanitize bookmark type for filename
		sanitized := strings.ReplaceAll(string(snapshot.Bookmark.Type), " ", "_")
		name = fmt.Sprintf("snapshot_%d_%s", snapshot.Tick, sanitized)
	}
	name += ".json"

	path := filepath.Join(dir, name)

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write snapshot: %w", err)
	}

	return path, nil
}

// LoadSnapshot reads a snapshot from disk.
func LoadSnapshot(path string) (*Snapshot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	return &snapshot, nil
}

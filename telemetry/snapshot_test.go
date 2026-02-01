package telemetry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

func TestSnapshotSaveLoad(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Create a test snapshot
	snapshot := &Snapshot{
		Version:     SnapshotVersion,
		RNGSeed:     42,
		WorldWidth:  1280,
		WorldHeight: 720,
		ResourceHotspots: []systems.HotspotDef{
			{X: 100, Y: 200},
			{X: 300, Y: 400},
		},
		ResourceSigma: 57.6,
		Tick:          1000,
		Entities: []EntityState{
			{
				ID:            1,
				Kind:          components.KindPrey,
				X:             150,
				Y:             250,
				VelX:          0.5,
				VelY:          -0.3,
				Heading:       1.2,
				Energy:        0.75,
				Age:           30.5,
				ReproCooldown: 2.0,
				Brain: neural.BrainWeights{
					W1: make([]float32, neural.NumHidden*neural.NumInputs),
					B1: make([]float32, neural.NumHidden),
					W2: make([]float32, neural.NumOutputs*neural.NumHidden),
					B2: make([]float32, neural.NumOutputs),
				},
				Lifetime: &LifetimeStatsJSON{
					BirthTick:       100,
					SurvivalTimeSec: 15.0,
					Children:        2,
					PeakEnergy:      0.95,
					TotalForaged:    5.5,
				},
			},
		},
		Bookmark: &Bookmark{
			Type:        BookmarkHuntBreakthrough,
			Tick:        1000,
			Description: "Test bookmark",
		},
	}

	// Save the snapshot
	path, err := SaveSnapshot(snapshot, tmpDir)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Snapshot file not created at %s", path)
	}

	// Load the snapshot
	loaded, err := LoadSnapshot(path)
	if err != nil {
		t.Fatalf("LoadSnapshot failed: %v", err)
	}

	// Verify loaded data matches original
	if loaded.Version != snapshot.Version {
		t.Errorf("Version mismatch: got %d, want %d", loaded.Version, snapshot.Version)
	}
	if loaded.RNGSeed != snapshot.RNGSeed {
		t.Errorf("RNGSeed mismatch: got %d, want %d", loaded.RNGSeed, snapshot.RNGSeed)
	}
	if loaded.Tick != snapshot.Tick {
		t.Errorf("Tick mismatch: got %d, want %d", loaded.Tick, snapshot.Tick)
	}
	if len(loaded.Entities) != len(snapshot.Entities) {
		t.Errorf("Entities count mismatch: got %d, want %d", len(loaded.Entities), len(snapshot.Entities))
	}
	if loaded.Bookmark == nil {
		t.Error("Bookmark not loaded")
	} else if loaded.Bookmark.Type != snapshot.Bookmark.Type {
		t.Errorf("Bookmark type mismatch: got %s, want %s", loaded.Bookmark.Type, snapshot.Bookmark.Type)
	}
}

func TestSnapshotFilename(t *testing.T) {
	tmpDir := t.TempDir()

	// Test with bookmark
	snapshot := &Snapshot{
		Version: SnapshotVersion,
		Tick:    5000,
		Bookmark: &Bookmark{
			Type: BookmarkPreyCrash,
			Tick: 5000,
		},
	}

	path, err := SaveSnapshot(snapshot, tmpDir)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	expected := filepath.Join(tmpDir, "snapshot_5000_prey_crash.json")
	if path != expected {
		t.Errorf("Path mismatch: got %s, want %s", path, expected)
	}

	// Test without bookmark
	snapshotNoBookmark := &Snapshot{
		Version: SnapshotVersion,
		Tick:    3000,
	}

	path, err = SaveSnapshot(snapshotNoBookmark, tmpDir)
	if err != nil {
		t.Fatalf("SaveSnapshot failed: %v", err)
	}

	expected = filepath.Join(tmpDir, "snapshot_3000.json")
	if path != expected {
		t.Errorf("Path mismatch: got %s, want %s", path, expected)
	}
}

func TestBrainWeightsSerialization(t *testing.T) {
	// Create a brain with known weights
	brain := neural.BrainWeights{
		W1: make([]float32, neural.NumHidden*neural.NumInputs),
		B1: make([]float32, neural.NumHidden),
		W2: make([]float32, neural.NumOutputs*neural.NumHidden),
		B2: make([]float32, neural.NumOutputs),
	}

	// Set some test values
	brain.W1[0] = 1.5
	brain.W1[10] = -0.3
	brain.B1[0] = 0.1
	brain.W2[0] = 2.0
	brain.B2[0] = -0.5

	// Serialize to JSON
	data, err := json.Marshal(brain)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Deserialize
	var loaded neural.BrainWeights
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Verify values
	if loaded.W1[0] != brain.W1[0] {
		t.Errorf("W1[0] mismatch: got %f, want %f", loaded.W1[0], brain.W1[0])
	}
	if loaded.W1[10] != brain.W1[10] {
		t.Errorf("W1[10] mismatch: got %f, want %f", loaded.W1[10], brain.W1[10])
	}
	if loaded.B1[0] != brain.B1[0] {
		t.Errorf("B1[0] mismatch: got %f, want %f", loaded.B1[0], brain.B1[0])
	}
}

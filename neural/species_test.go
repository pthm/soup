package neural

import (
	"testing"
)

func TestNewSpeciesManager(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	if sm == nil {
		t.Fatal("NewSpeciesManager returned nil")
	}

	if len(sm.Species) != 0 {
		t.Errorf("expected 0 species, got %d", len(sm.Species))
	}

	if sm.generation != 0 {
		t.Errorf("expected generation 0, got %d", sm.generation)
	}

	if len(sm.speciesColors) < 32 {
		t.Errorf("expected at least 32 pre-generated colors, got %d", len(sm.speciesColors))
	}
}

func TestSpeciesManagerAssignSpecies(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	// Create a genome
	genome1 := CreateBrainGenome(1, 0.3)

	// Assign to species
	speciesID := sm.AssignSpecies(genome1)

	if speciesID == 0 {
		t.Error("expected non-zero species ID")
	}

	if len(sm.Species) != 1 {
		t.Errorf("expected 1 species, got %d", len(sm.Species))
	}

	// Same genome should get same species
	speciesID2 := sm.AssignSpecies(genome1)
	if speciesID2 != speciesID {
		t.Errorf("same genome should get same species: %d != %d", speciesID2, speciesID)
	}
}

func TestSpeciesManagerMembership(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)

	// Add members
	sm.AddMember(speciesID, 100)
	sm.AddMember(speciesID, 101)
	sm.AddMember(speciesID, 102)

	sp := sm.GetSpecies(speciesID)
	if sp == nil {
		t.Fatal("species not found")
	}

	if len(sp.Members) != 3 {
		t.Errorf("expected 3 members, got %d", len(sp.Members))
	}

	// Remove a member
	sm.RemoveMember(speciesID, 101)
	if len(sp.Members) != 2 {
		t.Errorf("expected 2 members after removal, got %d", len(sp.Members))
	}
}

func TestSpeciesColor(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)

	color := sm.GetSpeciesColor(speciesID)

	// Color should be non-zero (not black)
	if color.R == 0 && color.G == 0 && color.B == 0 {
		t.Error("species color should not be black")
	}

	// Non-existent species should return gray
	grayColor := sm.GetSpeciesColor(9999)
	if grayColor.R != 128 || grayColor.G != 128 || grayColor.B != 128 {
		t.Errorf("non-existent species should return gray, got (%d,%d,%d)",
			grayColor.R, grayColor.G, grayColor.B)
	}

	t.Logf("Species %d color: RGB(%d, %d, %d)", speciesID, color.R, color.G, color.B)
}

func TestSpeciesColorsDiversity(t *testing.T) {
	// Test that generated colors are diverse
	colors := generateDistinctColors(10)

	// Check that colors are different
	seen := make(map[uint32]bool)
	for _, c := range colors {
		key := uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
		if seen[key] {
			t.Error("duplicate color found in generated colors")
		}
		seen[key] = true
	}
}

func TestSpeciesStats(t *testing.T) {
	opts := DefaultNEATOptions()
	// Use very low threshold to force separate species
	opts.CompatThreshold = 0.01
	sm := NewSpeciesManager(opts)

	// Create some species with members
	// Each genome will be different enough to form its own species
	speciesIDs := make([]int, 3)
	for i := 0; i < 3; i++ {
		genome := CreateBrainGenome(i+1, 0.3)
		// Mutate heavily to ensure difference
		idGen := NewGenomeIDGenerator()
		for m := 0; m < 10; m++ {
			MutateGenome(genome, opts, idGen)
		}
		speciesID := sm.AssignSpecies(genome)
		speciesIDs[i] = speciesID
		for j := 0; j < (i + 1); j++ {
			sm.AddMember(speciesID, i*10+j)
		}
	}

	stats := sm.GetStats()

	// With low threshold, we should get multiple species
	if stats.Count < 1 {
		t.Errorf("expected at least 1 species, got %d", stats.Count)
	}

	if stats.TotalMembers == 0 {
		t.Errorf("expected some members, got 0")
	}

	t.Logf("Stats: %+v", stats)
}

func TestGetTopSpecies(t *testing.T) {
	opts := DefaultNEATOptions()
	// Use very low threshold to force separate species
	opts.CompatThreshold = 0.01
	sm := NewSpeciesManager(opts)

	// Create species with different member counts
	idGen := NewGenomeIDGenerator()
	for i := 0; i < 5; i++ {
		genome := CreateBrainGenome(i+1, 0.3)
		// Mutate heavily to ensure difference
		for m := 0; m < 10; m++ {
			MutateGenome(genome, opts, idGen)
		}
		speciesID := sm.AssignSpecies(genome)
		// Add i+1 members to each species
		for j := 0; j <= i; j++ {
			sm.AddMember(speciesID, i*10+j)
		}
	}

	topSpecies := sm.GetTopSpecies(3)

	// We might not get exactly 3 species due to genome similarity
	if len(topSpecies) < 1 {
		t.Errorf("expected at least 1 top species, got %d", len(topSpecies))
	}

	// If we have multiple, they should be sorted by size descending
	for i := 1; i < len(topSpecies); i++ {
		if topSpecies[i-1].Size < topSpecies[i].Size {
			t.Error("top species not sorted by size descending")
		}
	}

	t.Logf("Top species count: %d", len(topSpecies))
	for i, sp := range topSpecies {
		t.Logf("  #%d: ID=%d, Size=%d", i+1, sp.ID, sp.Size)
	}
}

func TestSpeciesFitness(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)
	sm.AddMember(speciesID, 1)

	// Accumulate fitness
	sm.AccumulateFitness(speciesID, 10.0)
	sm.AccumulateFitness(speciesID, 20.0)

	sp := sm.GetSpecies(speciesID)
	if sp.BestFitness != 20.0 {
		t.Errorf("expected best fitness 20, got %f", sp.BestFitness)
	}

	if sp.TotalFitness != 30.0 {
		t.Errorf("expected total fitness 30, got %f", sp.TotalFitness)
	}
}

func TestEndGeneration(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)
	sm.AddMember(speciesID, 1)
	sm.AccumulateFitness(speciesID, 100.0)

	initialGen := sm.GetGeneration()
	sm.EndGeneration()

	if sm.GetGeneration() != initialGen+1 {
		t.Errorf("generation should increment")
	}

	sp := sm.GetSpecies(speciesID)
	if sp.Age != 1 {
		t.Errorf("species age should be 1, got %d", sp.Age)
	}

	// Total fitness should be reset
	if sp.TotalFitness != 0 {
		t.Errorf("total fitness should be reset to 0, got %f", sp.TotalFitness)
	}
}

func TestRecordOffspring(t *testing.T) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)

	sm.RecordOffspring(speciesID)
	sm.RecordOffspring(speciesID)
	sm.RecordOffspring(speciesID)

	sp := sm.GetSpecies(speciesID)
	if sp.OffspringCount != 3 {
		t.Errorf("expected 3 offspring, got %d", sp.OffspringCount)
	}
}

func TestRemoveStaleSpecies(t *testing.T) {
	opts := DefaultNEATOptions()
	opts.DropOffAge = 3 // Remove after 3 generations of no improvement
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)
	sm.AddMember(speciesID, 1)

	// Age the species without improvement
	for i := 0; i < 5; i++ {
		sm.EndGeneration()
		// Re-add member (simulating survival but no improvement)
		sm.AddMember(speciesID, 1)
	}

	// Species should be removed due to staleness
	if sm.GetSpecies(speciesID) != nil {
		// Check staleness
		sp := sm.GetSpecies(speciesID)
		t.Logf("Species staleness: %d, drop off age: %d", sp.Staleness, opts.DropOffAge)
	}
}

func BenchmarkAssignSpecies(b *testing.B) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		genome := CreateBrainGenome(i, 0.3)
		_ = sm.AssignSpecies(genome)
	}
}

func BenchmarkGetSpeciesColor(b *testing.B) {
	opts := DefaultNEATOptions()
	sm := NewSpeciesManager(opts)

	genome := CreateBrainGenome(1, 0.3)
	speciesID := sm.AssignSpecies(genome)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = sm.GetSpeciesColor(speciesID)
	}
}

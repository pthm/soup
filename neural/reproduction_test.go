package neural

import (
	"testing"
)

func TestGenomeIDGenerator(t *testing.T) {
	gen := NewGenomeIDGenerator()

	id1 := gen.NextID()
	id2 := gen.NextID()
	id3 := gen.NextID()

	if id1 >= id2 || id2 >= id3 {
		t.Errorf("IDs should be strictly increasing: %d, %d, %d", id1, id2, id3)
	}

	innov1 := gen.NextInnovation()
	innov2 := gen.NextInnovation()

	if innov1 >= innov2 {
		t.Errorf("innovations should be strictly increasing: %d, %d", innov1, innov2)
	}
}

func TestCreateCPPNGenome(t *testing.T) {
	genome := CreateCPPNGenome(1)

	if genome == nil {
		t.Fatal("CreateCPPNGenome returned nil")
	}

	if genome.Id != 1 {
		t.Errorf("expected genome ID 1, got %d", genome.Id)
	}

	// Should have input + output nodes
	expectedNodes := CPPNInputs + CPPNOutputs
	if len(genome.Nodes) != expectedNodes {
		t.Errorf("expected %d nodes, got %d", expectedNodes, len(genome.Nodes))
	}

	// Should have at least CPPNOutputs genes (one per output minimum)
	if len(genome.Genes) < CPPNOutputs {
		t.Errorf("expected at least %d genes, got %d", CPPNOutputs, len(genome.Genes))
	}

	t.Logf("Created CPPN genome with %d nodes and %d genes", len(genome.Nodes), len(genome.Genes))
}

func TestCrossoverGenomes(t *testing.T) {
	parent1 := CreateBrainGenome(1, 0.5)
	parent2 := CreateBrainGenome(2, 0.5)

	child, err := CrossoverGenomes(parent1, parent2, 1.0, 1.0, 3)
	if err != nil {
		t.Fatalf("CrossoverGenomes failed: %v", err)
	}

	if child == nil {
		t.Fatal("CrossoverGenomes returned nil child")
	}

	if child.Id != 3 {
		t.Errorf("expected child ID 3, got %d", child.Id)
	}

	// Child should have nodes and genes
	if len(child.Nodes) == 0 {
		t.Error("child has no nodes")
	}

	if len(child.Genes) == 0 {
		t.Error("child has no genes")
	}

	t.Logf("Created child genome with %d nodes and %d genes", len(child.Nodes), len(child.Genes))
}

func TestCrossoverGenomesWithDifferentFitness(t *testing.T) {
	parent1 := CreateBrainGenome(1, 0.5)
	parent2 := CreateBrainGenome(2, 0.5)

	// Parent1 is more fit
	child1, err := CrossoverGenomes(parent1, parent2, 2.0, 1.0, 3)
	if err != nil {
		t.Fatalf("CrossoverGenomes failed: %v", err)
	}

	// Parent2 is more fit
	child2, err := CrossoverGenomes(parent1, parent2, 1.0, 2.0, 4)
	if err != nil {
		t.Fatalf("CrossoverGenomes failed: %v", err)
	}

	// Both should succeed
	if child1 == nil || child2 == nil {
		t.Error("crossover with different fitness failed")
	}
}

func TestMutateGenome(t *testing.T) {
	genome := CreateBrainGenome(1, 0.5)
	opts := DefaultNEATOptions()
	idGen := NewGenomeIDGenerator()

	// Force mutations to happen
	opts.MutateLinkWeightsProb = 1.0

	originalGenes := len(genome.Genes)
	mutated, err := MutateGenome(genome, opts, idGen)

	if err != nil {
		t.Fatalf("MutateGenome failed: %v", err)
	}

	if !mutated {
		t.Error("expected mutation to occur")
	}

	// Genes count might change due to structural mutations
	t.Logf("Genome went from %d to %d genes", originalGenes, len(genome.Genes))
}

func TestMutateCPPNGenome(t *testing.T) {
	genome := CreateCPPNGenome(1)
	opts := DefaultNEATOptions()
	idGen := NewGenomeIDGenerator()

	opts.MutateLinkWeightsProb = 1.0

	mutated, err := MutateCPPNGenome(genome, opts, idGen)

	if err != nil {
		t.Fatalf("MutateCPPNGenome failed: %v", err)
	}

	if !mutated {
		t.Error("expected mutation to occur")
	}
}

func TestCloneGenome(t *testing.T) {
	original := CreateBrainGenome(1, 0.5)

	clone, err := CloneGenome(original, 2)
	if err != nil {
		t.Fatalf("CloneGenome failed: %v", err)
	}

	if clone.Id != 2 {
		t.Errorf("expected clone ID 2, got %d", clone.Id)
	}

	if len(clone.Nodes) != len(original.Nodes) {
		t.Errorf("node count mismatch: original %d, clone %d", len(original.Nodes), len(clone.Nodes))
	}

	if len(clone.Genes) != len(original.Genes) {
		t.Errorf("gene count mismatch: original %d, clone %d", len(original.Genes), len(clone.Genes))
	}

	// Verify clone is independent (modify original, check clone unchanged)
	originalGeneCount := len(clone.Genes)
	original.Genes = original.Genes[:1]

	if len(clone.Genes) != originalGeneCount {
		t.Error("clone is not independent from original")
	}
}

func TestCreateOffspringGenomes(t *testing.T) {
	idGen := NewGenomeIDGenerator()
	opts := DefaultNEATOptions()

	body1, brain1 := CreateInitialGenomePair(idGen, 0.3)
	body2, brain2 := CreateInitialGenomePair(idGen, 0.3)

	bodyChild, brainChild, err := CreateOffspringGenomes(
		body1, body2, brain1, brain2,
		1.0, 1.0,
		idGen, opts,
	)

	if err != nil {
		t.Fatalf("CreateOffspringGenomes failed: %v", err)
	}

	if bodyChild == nil {
		t.Error("body child is nil")
	}

	if brainChild == nil {
		t.Error("brain child is nil")
	}

	// Build networks to verify they're valid
	_, err = bodyChild.Genesis(bodyChild.Id)
	if err != nil {
		t.Errorf("body child cannot build network: %v", err)
	}

	_, err = brainChild.Genesis(brainChild.Id)
	if err != nil {
		t.Errorf("brain child cannot build network: %v", err)
	}

	t.Logf("Created offspring: body=%d nodes/%d genes, brain=%d nodes/%d genes",
		len(bodyChild.Nodes), len(bodyChild.Genes),
		len(brainChild.Nodes), len(brainChild.Genes))
}

func TestGenomeCompatibility(t *testing.T) {
	opts := DefaultNEATOptions()

	// Same genome should have 0 distance
	genome := CreateBrainGenome(1, 0.5)
	dist := GenomeCompatibility(genome, genome, opts)
	if dist != 0 {
		t.Errorf("same genome should have 0 distance, got %f", dist)
	}

	// Different genomes should have some distance
	genome2 := CreateBrainGenome(2, 0.5)
	dist2 := GenomeCompatibility(genome, genome2, opts)
	if dist2 < 0 {
		t.Error("distance should not be negative")
	}

	t.Logf("Distance between different genomes: %f", dist2)
}

func TestCreateInitialGenomePair(t *testing.T) {
	idGen := NewGenomeIDGenerator()

	body, brain := CreateInitialGenomePair(idGen, 0.3)

	if body == nil {
		t.Error("body genome is nil")
	}

	if brain == nil {
		t.Error("brain genome is nil")
	}

	// Verify they can build networks
	_, err := body.Genesis(body.Id)
	if err != nil {
		t.Errorf("body genome cannot build network: %v", err)
	}

	_, err = brain.Genesis(brain.Id)
	if err != nil {
		t.Errorf("brain genome cannot build network: %v", err)
	}
}

func BenchmarkCrossoverGenomes(b *testing.B) {
	parent1 := CreateBrainGenome(1, 0.5)
	parent2 := CreateBrainGenome(2, 0.5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = CrossoverGenomes(parent1, parent2, 1.0, 1.0, i+3)
	}
}

func BenchmarkMutateGenome(b *testing.B) {
	opts := DefaultNEATOptions()
	idGen := NewGenomeIDGenerator()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		genome := CreateBrainGenome(i, 0.5)
		_, _ = MutateGenome(genome, opts, idGen)
	}
}

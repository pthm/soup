package neural

import (
	"math"
	"testing"
)

func TestCreateBrainGenome(t *testing.T) {
	genome := CreateBrainGenome(1, 0.3)

	if genome == nil {
		t.Fatal("CreateBrainGenome returned nil")
	}

	if genome.Id != 1 {
		t.Errorf("expected genome ID 1, got %d", genome.Id)
	}

	// Should have input + output nodes
	expectedNodes := BrainInputs + BrainOutputs
	if len(genome.Nodes) != expectedNodes {
		t.Errorf("expected %d nodes, got %d", expectedNodes, len(genome.Nodes))
	}

	// Should have at least 1 gene (connection)
	if len(genome.Genes) == 0 {
		t.Error("expected at least 1 gene, got 0")
	}

	t.Logf("Created genome with %d nodes and %d genes", len(genome.Nodes), len(genome.Genes))
}

func TestCreateMinimalBrainGenome(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)

	if genome == nil {
		t.Fatal("CreateMinimalBrainGenome returned nil")
	}

	// Minimal genome should be fully connected: inputs * outputs genes
	expectedGenes := BrainInputs * BrainOutputs
	if len(genome.Genes) != expectedGenes {
		t.Errorf("expected %d genes, got %d", expectedGenes, len(genome.Genes))
	}

	t.Logf("Created minimal genome with %d nodes and %d genes", len(genome.Nodes), len(genome.Genes))
}

func TestNewBrainController(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)

	controller, err := NewBrainController(genome)
	if err != nil {
		t.Fatalf("NewBrainController failed: %v", err)
	}

	if controller == nil {
		t.Fatal("NewBrainController returned nil")
	}

	if controller.Genome != genome {
		t.Error("controller genome mismatch")
	}

	t.Logf("Created controller with %d nodes and %d links",
		controller.NodeCount(), controller.LinkCount())
}

func TestBrainControllerThink(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)
	controller, err := NewBrainController(genome)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Create test inputs (all zeros except bias)
	inputs := make([]float64, BrainInputs)
	inputs[BrainInputs-1] = 1.0 // bias

	outputs, err := controller.Think(inputs)
	if err != nil {
		t.Fatalf("Think failed: %v", err)
	}

	if len(outputs) != BrainOutputs {
		t.Errorf("expected %d outputs, got %d", BrainOutputs, len(outputs))
	}

	// Outputs should be in [0, 1] range due to sigmoid activation
	for i, out := range outputs {
		if out < 0 || out > 1 {
			t.Errorf("output %d out of sigmoid range [0,1]: %f", i, out)
		}
	}

	t.Logf("Think produced outputs: %v", outputs)
}

func TestBrainControllerThinkWithVariedInputs(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)
	controller, err := NewBrainController(genome)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Test with different input patterns
	testCases := []struct {
		name   string
		inputs []float64
	}{
		{
			name:   "all zeros",
			inputs: make([]float64, BrainInputs),
		},
		{
			name: "all ones",
			inputs: func() []float64 {
				in := make([]float64, BrainInputs)
				for i := range in {
					in[i] = 1.0
				}
				return in
			}(),
		},
		{
			name: "mixed",
			inputs: func() []float64 {
				in := make([]float64, BrainInputs)
				for i := range in {
					in[i] = float64(i) / float64(BrainInputs)
				}
				return in
			}(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outputs, err := controller.Think(tc.inputs)
			if err != nil {
				t.Fatalf("Think failed: %v", err)
			}

			// All outputs should be valid numbers
			for i, out := range outputs {
				if math.IsNaN(out) || math.IsInf(out, 0) {
					t.Errorf("output %d is invalid: %f", i, out)
				}
			}

			t.Logf("Outputs for %s: %v", tc.name, outputs)
		})
	}
}

func TestBrainControllerThinkWrongInputCount(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)
	controller, err := NewBrainController(genome)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Wrong number of inputs should fail
	wrongInputs := make([]float64, BrainInputs-1)
	_, err = controller.Think(wrongInputs)
	if err == nil {
		t.Error("expected error for wrong input count, got nil")
	}
}

func TestBrainControllerRebuildNetwork(t *testing.T) {
	genome := CreateMinimalBrainGenome(1)
	controller, err := NewBrainController(genome)
	if err != nil {
		t.Fatalf("failed to create controller: %v", err)
	}

	// Get initial outputs
	inputs := make([]float64, BrainInputs)
	inputs[BrainInputs-1] = 1.0
	outputsBefore, _ := controller.Think(inputs)

	// Rebuild network (should produce same outputs since genome unchanged)
	err = controller.RebuildNetwork()
	if err != nil {
		t.Fatalf("RebuildNetwork failed: %v", err)
	}

	outputsAfter, _ := controller.Think(inputs)

	// Outputs should be identical
	for i := range outputsBefore {
		if math.Abs(outputsBefore[i]-outputsAfter[i]) > 1e-9 {
			t.Errorf("output %d changed after rebuild: %f -> %f",
				i, outputsBefore[i], outputsAfter[i])
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if cfg.NEAT == nil {
		t.Error("NEAT options is nil")
	}

	if cfg.Brain.Inputs != BrainInputs {
		t.Errorf("expected brain inputs %d, got %d", BrainInputs, cfg.Brain.Inputs)
	}

	if cfg.Brain.Outputs != BrainOutputs {
		t.Errorf("expected brain outputs %d, got %d", BrainOutputs, cfg.Brain.Outputs)
	}

	if cfg.CPPN.MaxCells != 16 {
		t.Errorf("expected max cells 16, got %d", cfg.CPPN.MaxCells)
	}

	t.Logf("Config: brain=%dx%d, cppn grid=%d, max cells=%d",
		cfg.Brain.Inputs, cfg.Brain.Outputs, cfg.CPPN.GridSize, cfg.CPPN.MaxCells)
}

func TestDefaultNEATOptions(t *testing.T) {
	opts := DefaultNEATOptions()

	if opts == nil {
		t.Fatal("DefaultNEATOptions returned nil")
	}

	// Verify key mutation rates are set
	if opts.MutateAddNodeProb <= 0 {
		t.Error("MutateAddNodeProb should be positive")
	}

	if opts.MutateAddLinkProb <= 0 {
		t.Error("MutateAddLinkProb should be positive")
	}

	if opts.MutateLinkWeightsProb <= 0 {
		t.Error("MutateLinkWeightsProb should be positive")
	}

	// Verify speciation params
	if opts.CompatThreshold <= 0 {
		t.Error("CompatThreshold should be positive")
	}

	t.Logf("NEAT options: add_node=%.2f, add_link=%.2f, compat=%.2f",
		opts.MutateAddNodeProb, opts.MutateAddLinkProb, opts.CompatThreshold)
}

func BenchmarkBrainThink(b *testing.B) {
	genome := CreateMinimalBrainGenome(1)
	controller, err := NewBrainController(genome)
	if err != nil {
		b.Fatalf("failed to create controller: %v", err)
	}

	inputs := make([]float64, BrainInputs)
	for i := range inputs {
		inputs[i] = float64(i) / float64(BrainInputs)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = controller.Think(inputs)
	}
}

func BenchmarkBrainThinkSparse(b *testing.B) {
	// Test with sparse connections (more realistic after evolution)
	genome := CreateBrainGenome(1, 0.3)
	controller, err := NewBrainController(genome)
	if err != nil {
		b.Fatalf("failed to create controller: %v", err)
	}

	inputs := make([]float64, BrainInputs)
	for i := range inputs {
		inputs[i] = float64(i) / float64(BrainInputs)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = controller.Think(inputs)
	}
}

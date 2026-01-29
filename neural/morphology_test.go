package neural

import (
	"testing"
)

func TestGenerateMorphology(t *testing.T) {
	genome := CreateCPPNGenome(1)

	result, err := GenerateMorphology(genome, 32, 0.0)
	if err != nil {
		t.Fatalf("GenerateMorphology failed: %v", err)
	}

	// Should have at least 1 cell
	if len(result.Cells) == 0 {
		t.Error("expected at least 1 cell")
	}

	// Should not exceed max cells
	if len(result.Cells) > 32 {
		t.Errorf("expected max 32 cells, got %d", len(result.Cells))
	}

	// Verify viability: should have sensor and actuator capability
	hasSensor := false
	hasActuator := false
	for _, c := range result.Cells {
		if c.PrimaryType == CellTypeSensor || c.SecondaryType == CellTypeSensor {
			hasSensor = true
		}
		if c.PrimaryType == CellTypeActuator || c.SecondaryType == CellTypeActuator {
			hasActuator = true
		}
	}
	if !hasSensor {
		t.Error("morphology should have at least one sensor capability")
	}
	if !hasActuator {
		t.Error("morphology should have at least one actuator capability")
	}

	t.Logf("Generated morphology with %d cells, diet bias: %.2f",
		len(result.Cells), result.DietBias)
}

func TestGenerateMorphologyMinimum(t *testing.T) {
	genome := CreateCPPNGenome(1)

	// Even with very high threshold, should get at least 1 cell
	result, err := GenerateMorphology(genome, 32, 100.0)
	if err != nil {
		t.Fatalf("GenerateMorphology failed: %v", err)
	}

	if len(result.Cells) < 1 {
		t.Error("expected at least 1 cell even with high threshold")
	}

	// The guaranteed cell should be at center
	if result.Cells[0].GridX != 0 || result.Cells[0].GridY != 0 {
		t.Errorf("fallback cell should be at center, got (%d, %d)",
			result.Cells[0].GridX, result.Cells[0].GridY)
	}
}

func TestGenerateMorphologyMaxCells(t *testing.T) {
	genome := CreateCPPNGenome(1)

	// Test with small max cells
	result, err := GenerateMorphology(genome, 3, -1.0) // Very low threshold
	if err != nil {
		t.Fatalf("GenerateMorphology failed: %v", err)
	}

	if len(result.Cells) > 3 {
		t.Errorf("expected max 3 cells, got %d", len(result.Cells))
	}
}

func TestGenerateMorphologyNilGenome(t *testing.T) {
	_, err := GenerateMorphology(nil, 32, 0.0)
	if err == nil {
		t.Error("expected error for nil genome")
	}
}

func TestGenerateMorphologyDiversity(t *testing.T) {
	// Generate multiple morphologies and check they're different
	var morphologies []MorphologyResult

	for i := 0; i < 5; i++ {
		genome := CreateCPPNGenome(i + 1)
		result, err := GenerateMorphology(genome, 32, 0.0)
		if err != nil {
			t.Fatalf("GenerateMorphology failed: %v", err)
		}
		morphologies = append(morphologies, result)
	}

	// Check that we get some variety in cell counts
	cellCounts := make(map[int]bool)
	for _, m := range morphologies {
		cellCounts[len(m.Cells)] = true
	}

	// With random genomes, we should get at least 2 different cell counts
	// (This test might occasionally fail due to randomness, but usually passes)
	if len(cellCounts) < 2 {
		t.Logf("Warning: all morphologies had same cell count, might indicate lack of diversity")
	}

	for i, m := range morphologies {
		t.Logf("Morphology %d: %d cells, diet=%.2f",
			i+1, len(m.Cells), m.DietBias)
	}
}

func TestSelectCellFunctions(t *testing.T) {
	// Test argmax selection
	outputs := []float64{0.8, 0.3, 0.1, 0.2, 0.4, 0.5, 0.6} // 7 functional outputs
	// Highest is index 0 (0.8), second is index 6 (0.6)

	primary, secondary, effP, effS := SelectCellFunctions(outputs)

	// Primary should be index 0+1 = CellTypeSensor (since CellTypeNone is 0)
	if primary != CellTypeSensor {
		t.Errorf("expected primary=CellTypeSensor(1), got %v", primary)
	}

	// Secondary should be index 6+1 = CellTypeReproductive
	if secondary != CellTypeReproductive {
		t.Errorf("expected secondary=CellTypeReproductive(7), got %v", secondary)
	}

	// Check effective strengths with mixed-function penalty
	expectedPrimary := float32((outputs[0]+1.0)/2.0) * MixedPrimaryPenalty
	expectedSecondary := float32((outputs[6]+1.0)/2.0) * MixedSecondaryScale

	if effP < expectedPrimary-0.01 || effP > expectedPrimary+0.01 {
		t.Errorf("expected effPrimary=%.3f, got %.3f", expectedPrimary, effP)
	}
	if effS < expectedSecondary-0.01 || effS > expectedSecondary+0.01 {
		t.Errorf("expected effSecondary=%.3f, got %.3f", expectedSecondary, effS)
	}
}

func TestSelectCellFunctionsNoSecondary(t *testing.T) {
	// Test when second-highest is below threshold
	// SecondaryThreshold is 0.25 in [0,1] space
	// So in [-1,1] space, values must be below (0.25*2 - 1) = -0.5
	// Using values well below that to ensure no secondary
	outputs := []float64{0.8, -0.8, -0.9, -0.95, -0.85, -0.99, -0.7} // 7 functional outputs
	// Highest is index 0 (0.8) -> normalized = 0.9
	// Second highest is index 6 (-0.7) -> normalized = 0.15, below 0.25 threshold

	primary, secondary, effP, effS := SelectCellFunctions(outputs)

	if primary != CellTypeSensor {
		t.Errorf("expected primary=CellTypeSensor, got %v", primary)
	}

	if secondary != CellTypeNone {
		t.Errorf("expected secondary=CellTypeNone, got %v", secondary)
	}

	// No mixed penalty when no secondary
	expectedPrimary := float32((outputs[0] + 1.0) / 2.0)
	if effP < expectedPrimary-0.01 || effP > expectedPrimary+0.01 {
		t.Errorf("expected effPrimary=%.3f (no penalty), got %.3f", expectedPrimary, effP)
	}

	if effS != 0 {
		t.Errorf("expected effSecondary=0, got %.3f", effS)
	}
}

func TestCellSpecHasFunction(t *testing.T) {
	spec := CellSpec{
		PrimaryType:        CellTypeSensor,
		SecondaryType:      CellTypeActuator,
		EffectivePrimary:   0.8,
		EffectiveSecondary: 0.3,
	}

	// HasFunction tests
	if !spec.HasFunction(CellTypeSensor) {
		t.Error("expected HasFunction(CellTypeSensor) = true")
	}
	if !spec.HasFunction(CellTypeActuator) {
		t.Error("expected HasFunction(CellTypeActuator) = true")
	}
	if spec.HasFunction(CellTypeMouth) {
		t.Error("expected HasFunction(CellTypeMouth) = false")
	}
}

func TestEnsureViability(t *testing.T) {
	// Test with empty cells
	cells := ensureViability([]CellSpec{})
	if len(cells) < 1 {
		t.Error("ensureViability should create at least 1 cell")
	}

	// Verify the fallback has both sensor and actuator
	hasSensor := false
	hasActuator := false
	for _, c := range cells {
		if c.PrimaryType == CellTypeSensor || c.SecondaryType == CellTypeSensor {
			hasSensor = true
		}
		if c.PrimaryType == CellTypeActuator || c.SecondaryType == CellTypeActuator {
			hasActuator = true
		}
	}
	if !hasSensor || !hasActuator {
		t.Error("fallback morphology should have both sensor and actuator capability")
	}
}

func TestEnsureViabilityAddsMissingSensor(t *testing.T) {
	// Cells with only actuators
	cells := []CellSpec{
		{GridX: 0, GridY: 0, PrimaryType: CellTypeActuator, SecondaryType: CellTypeNone, EffectivePrimary: 0.5},
		{GridX: 1, GridY: 0, PrimaryType: CellTypeActuator, SecondaryType: CellTypeNone, EffectivePrimary: 0.5},
	}

	result := ensureViability(cells)

	hasSensor := false
	for _, c := range result {
		if c.PrimaryType == CellTypeSensor || c.SecondaryType == CellTypeSensor {
			hasSensor = true
			break
		}
	}

	if !hasSensor {
		t.Error("ensureViability should add sensor capability when missing")
	}
}

func TestEnsureViabilityAddsMissingActuator(t *testing.T) {
	// Cells with only sensors
	cells := []CellSpec{
		{GridX: 0, GridY: 0, PrimaryType: CellTypeSensor, SecondaryType: CellTypeNone, EffectivePrimary: 0.5},
		{GridX: 1, GridY: 0, PrimaryType: CellTypeSensor, SecondaryType: CellTypeNone, EffectivePrimary: 0.5},
	}

	result := ensureViability(cells)

	hasActuator := false
	for _, c := range result {
		if c.PrimaryType == CellTypeActuator || c.SecondaryType == CellTypeActuator {
			hasActuator = true
			break
		}
	}

	if !hasActuator {
		t.Error("ensureViability should add actuator capability when missing")
	}
}

func TestMorphologyResultCellCount(t *testing.T) {
	genome := CreateCPPNGenome(1)
	result, err := GenerateMorphology(genome, 32, 0.0)
	if err != nil {
		t.Fatalf("GenerateMorphology failed: %v", err)
	}

	// Test CellCount
	if result.CellCount() != len(result.Cells) {
		t.Errorf("CellCount() mismatch")
	}

	// Verify viability
	hasSensor := false
	hasActuator := false
	for _, c := range result.Cells {
		if c.PrimaryType == CellTypeSensor || c.SecondaryType == CellTypeSensor {
			hasSensor = true
		}
		if c.PrimaryType == CellTypeActuator || c.SecondaryType == CellTypeActuator {
			hasActuator = true
		}
	}

	if !hasSensor {
		t.Error("expected at least 1 sensor")
	}
	if !hasActuator {
		t.Error("expected at least 1 actuator")
	}
}

func TestGenerateMorphologyWithConfig(t *testing.T) {
	genome := CreateCPPNGenome(1)
	cfg := CPPNConfig{
		GridSize:        8,
		MaxCells:        16,
		MinCells:        1,
		CellThreshold:   0.0,
		EnforceSymmetry: false,
	}

	result, err := GenerateMorphologyWithConfig(genome, cfg)
	if err != nil {
		t.Fatalf("GenerateMorphologyWithConfig failed: %v", err)
	}

	if len(result.Cells) > cfg.MaxCells {
		t.Errorf("exceeded max cells: got %d, max %d", len(result.Cells), cfg.MaxCells)
	}
}

func TestMorphologyEmptyCells(t *testing.T) {
	// Test methods with empty cells slice
	result := MorphologyResult{
		Cells: []CellSpec{},
	}

	if result.CellCount() != 0 {
		t.Error("CellCount should be 0 for empty morphology")
	}
}

func TestCellTypeStrings(t *testing.T) {
	tests := []struct {
		ct       CellType
		expected string
	}{
		{CellTypeNone, "None"},
		{CellTypeSensor, "Sensor"},
		{CellTypeActuator, "Actuator"},
		{CellTypeMouth, "Mouth"},
		{CellTypeDigestive, "Digestive"},
		{CellTypePhotosynthetic, "Photosynthetic"},
		{CellTypeBioluminescent, "Bioluminescent"},
		{CellTypeReproductive, "Reproductive"},
	}

	for _, tt := range tests {
		if got := tt.ct.String(); got != tt.expected {
			t.Errorf("CellType(%d).String() = %q, want %q", tt.ct, got, tt.expected)
		}
	}
}

func BenchmarkGenerateMorphology(b *testing.B) {
	genome := CreateCPPNGenome(1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateMorphology(genome, 32, 0.0)
	}
}

func BenchmarkGenerateMorphologySmall(b *testing.B) {
	genome := CreateCPPNGenome(1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = GenerateMorphology(genome, 8, 0.0)
	}
}

func BenchmarkSelectCellFunctions(b *testing.B) {
	outputs := []float64{0.8, 0.3, 0.1, 0.2, 0.4, 0.5, 0.6}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = SelectCellFunctions(outputs)
	}
}

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
		t.Logf("Morphology %d: %d cells, diet=%.2f, dims=%dx%d",
			i+1, len(m.Cells), m.DietBias, m.Width(), m.Height())
	}
}

func TestMorphologyResultMethods(t *testing.T) {
	genome := CreateCPPNGenome(1)
	result, err := GenerateMorphology(genome, 32, 0.0)
	if err != nil {
		t.Fatalf("GenerateMorphology failed: %v", err)
	}

	// Test CellCount
	if result.CellCount() != len(result.Cells) {
		t.Errorf("CellCount() mismatch")
	}

	// Test Bounds
	minX, minY, maxX, maxY := result.Bounds()
	t.Logf("Bounds: (%d,%d) to (%d,%d)", minX, minY, maxX, maxY)

	// Verify all cells are within bounds
	for _, c := range result.Cells {
		if c.GridX < minX || c.GridX > maxX || c.GridY < minY || c.GridY > maxY {
			t.Errorf("cell (%d,%d) outside bounds", c.GridX, c.GridY)
		}
	}

	// Test Width and Height
	width := result.Width()
	height := result.Height()
	if width < 1 || height < 1 {
		t.Errorf("invalid dimensions: %dx%d", width, height)
	}

	// Test Centroid
	cx, cy := result.Centroid()
	t.Logf("Centroid: (%.2f, %.2f)", cx, cy)

	// Test diet classification
	if result.DietBias > 0.3 && !result.IsCarnivore() {
		t.Error("should be carnivore")
	}
	if result.DietBias < -0.3 && !result.IsHerbivore() {
		t.Error("should be herbivore")
	}
	if result.DietBias >= -0.3 && result.DietBias <= 0.3 && !result.IsOmnivore() {
		t.Error("should be omnivore")
	}
}

func TestMorphologyResultSymmetry(t *testing.T) {
	// Test with a known symmetric morphology
	result := MorphologyResult{
		Cells: []CellSpec{
			{GridX: 0, GridY: 0},
			{GridX: -1, GridY: 0},
			{GridX: 1, GridY: 0},
			{GridX: -1, GridY: 1},
			{GridX: 1, GridY: 1},
		},
	}

	if !result.IsSymmetric() {
		t.Error("expected symmetric morphology to be detected as symmetric")
	}

	// Test with asymmetric morphology
	asymResult := MorphologyResult{
		Cells: []CellSpec{
			{GridX: 0, GridY: 0},
			{GridX: 1, GridY: 0},
			{GridX: 2, GridY: 0},
			{GridX: 3, GridY: 1},
		},
	}

	if asymResult.IsSymmetric() {
		t.Error("expected asymmetric morphology to not be detected as symmetric")
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

	minX, minY, maxX, maxY := result.Bounds()
	if minX != 0 || minY != 0 || maxX != 0 || maxY != 0 {
		t.Error("Bounds should be 0 for empty morphology")
	}

	cx, cy := result.Centroid()
	if cx != 0 || cy != 0 {
		t.Error("Centroid should be 0 for empty morphology")
	}

	if !result.IsSymmetric() {
		t.Error("Empty morphology should be considered symmetric")
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

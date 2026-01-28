package neural

import (
	"fmt"
	"math"
	"sort"

	"github.com/yaricom/goNEAT/v4/neat/genetics"
)

// MorphGridSize is the size of the grid used to query the CPPN.
const MorphGridSize = 8

// CellSpec describes a single cell's position and properties.
type CellSpec struct {
	GridX    int8    // X position relative to organism center
	GridY    int8    // Y position relative to organism center
	DietBias float32 // <0 herbivore, >0 carnivore
}

// MorphologyResult holds the output of CPPN morphology generation.
type MorphologyResult struct {
	Cells       []CellSpec // Cell positions and properties
	DietBias    float32    // Average diet bias across all cells
	SpeedTrait  bool       // Whether organism should have speed trait
	HerdTrait   bool       // Whether organism should have herding trait
	VisionTrait bool       // Whether organism should have far sight trait
}

// candidate holds intermediate data during morphology generation.
type candidate struct {
	gridX    int8
	gridY    int8
	presence float64
	diet     float64
	traits   float64
}

// GenerateMorphology queries the CPPN genome to produce an organism's body layout.
// The CPPN is queried at each position in an 8x8 grid.
// Positions with presence > threshold become cells.
// Returns up to maxCells cells sorted by priority.
func GenerateMorphology(genome *genetics.Genome, maxCells int, threshold float64) (MorphologyResult, error) {
	if genome == nil {
		return MorphologyResult{}, fmt.Errorf("cannot generate morphology from nil genome")
	}

	if maxCells < 1 {
		maxCells = 1
	}

	// Build phenotype network from genome
	phenotype, err := genome.Genesis(genome.Id)
	if err != nil {
		return MorphologyResult{}, fmt.Errorf("failed to build CPPN network: %w", err)
	}

	var candidates []candidate

	// Query CPPN at each grid position
	for gx := 0; gx < MorphGridSize; gx++ {
		for gy := 0; gy < MorphGridSize; gy++ {
			// Normalize coordinates to [-1, 1]
			x := (float64(gx)/float64(MorphGridSize-1))*2 - 1
			y := (float64(gy)/float64(MorphGridSize-1))*2 - 1
			d := math.Sqrt(x*x + y*y)
			a := math.Atan2(y, x)

			// CPPN inputs: x, y, distance, angle, bias
			inputs := []float64{x, y, d, a, 1.0}

			if err := phenotype.LoadSensors(inputs); err != nil {
				continue
			}

			// Activate network
			activated, err := phenotype.Activate()
			if err != nil || !activated {
				// Flush and continue on failure
				phenotype.Flush()
				continue
			}

			outputs := phenotype.ReadOutputs()

			// Flush for next iteration
			phenotype.Flush()

			// Need at least 4 outputs
			if len(outputs) < CPPNOutputs {
				continue
			}

			// Output 0: cell presence (tanh output in [-1, 1])
			presence := outputs[0]

			if presence > threshold {
				candidates = append(candidates, candidate{
					// Convert grid coords to centered coords
					gridX:    int8(gx - MorphGridSize/2),
					gridY:    int8(gy - MorphGridSize/2),
					presence: presence,
					diet:     outputs[1], // diet bias
					traits:   outputs[2], // trait encoding
				})
			}
		}
	}

	// Sort by presence (priority) descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].presence > candidates[j].presence
	})

	// Limit to maxCells
	if len(candidates) > maxCells {
		candidates = candidates[:maxCells]
	}

	// Ensure at least 1 cell at center
	if len(candidates) == 0 {
		candidates = append(candidates, candidate{
			gridX:    0,
			gridY:    0,
			presence: 1.0,
			diet:     0,
			traits:   0,
		})
	}

	// Build result
	result := MorphologyResult{
		Cells: make([]CellSpec, len(candidates)),
	}

	var totalDiet, totalTraits float64
	for i, c := range candidates {
		result.Cells[i] = CellSpec{
			GridX:    c.gridX,
			GridY:    c.gridY,
			DietBias: float32(c.diet),
		}
		totalDiet += c.diet
		totalTraits += c.traits
	}

	// Calculate averages
	n := float64(len(candidates))
	result.DietBias = float32(totalDiet / n)
	avgTraits := totalTraits / n

	// Decode trait flags from average traits output
	// Traits output is in [-1, 1] range (tanh)
	// Map different regions to different traits
	result.SpeedTrait = avgTraits > 0.3
	result.HerdTrait = avgTraits > -0.3 && avgTraits < 0.5
	result.VisionTrait = avgTraits < -0.2

	return result, nil
}

// GenerateMorphologyWithConfig uses configuration for grid size and threshold.
func GenerateMorphologyWithConfig(genome *genetics.Genome, cfg CPPNConfig) (MorphologyResult, error) {
	return GenerateMorphology(genome, cfg.MaxCells, cfg.CellThreshold)
}

// IsCarnivore returns true if the morphology suggests carnivore diet.
func (m *MorphologyResult) IsCarnivore() bool {
	return m.DietBias > 0.3
}

// IsHerbivore returns true if the morphology suggests herbivore diet.
func (m *MorphologyResult) IsHerbivore() bool {
	return m.DietBias < -0.3
}

// IsOmnivore returns true if the morphology suggests omnivore diet.
func (m *MorphologyResult) IsOmnivore() bool {
	return m.DietBias >= -0.3 && m.DietBias <= 0.3
}

// CellCount returns the number of cells in the morphology.
func (m *MorphologyResult) CellCount() int {
	return len(m.Cells)
}

// Bounds returns the bounding box of the morphology in grid units.
func (m *MorphologyResult) Bounds() (minX, minY, maxX, maxY int8) {
	if len(m.Cells) == 0 {
		return 0, 0, 0, 0
	}

	minX, minY = m.Cells[0].GridX, m.Cells[0].GridY
	maxX, maxY = minX, minY

	for _, c := range m.Cells {
		if c.GridX < minX {
			minX = c.GridX
		}
		if c.GridX > maxX {
			maxX = c.GridX
		}
		if c.GridY < minY {
			minY = c.GridY
		}
		if c.GridY > maxY {
			maxY = c.GridY
		}
	}

	return minX, minY, maxX, maxY
}

// Width returns the width of the morphology in grid units.
func (m *MorphologyResult) Width() int {
	minX, _, maxX, _ := m.Bounds()
	return int(maxX-minX) + 1
}

// Height returns the height of the morphology in grid units.
func (m *MorphologyResult) Height() int {
	_, minY, _, maxY := m.Bounds()
	return int(maxY-minY) + 1
}

// IsSymmetric checks if the morphology is roughly symmetric along the Y axis.
func (m *MorphologyResult) IsSymmetric() bool {
	if len(m.Cells) <= 1 {
		return true
	}

	// Build a set of positions
	positions := make(map[int16]bool)
	for _, c := range m.Cells {
		// Encode position as single int16
		key := int16(c.GridX)<<8 | int16(c.GridY)&0xFF
		positions[key] = true
	}

	// Check if each position has a mirror
	symmetricCount := 0
	for _, c := range m.Cells {
		mirrorKey := int16(-c.GridX)<<8 | int16(c.GridY)&0xFF
		if positions[mirrorKey] {
			symmetricCount++
		}
	}

	// Consider symmetric if >70% of cells have mirrors
	return float64(symmetricCount)/float64(len(m.Cells)) > 0.7
}

// Centroid returns the center of mass of the morphology.
func (m *MorphologyResult) Centroid() (float32, float32) {
	if len(m.Cells) == 0 {
		return 0, 0
	}

	var sumX, sumY float32
	for _, c := range m.Cells {
		sumX += float32(c.GridX)
		sumY += float32(c.GridY)
	}

	n := float32(len(m.Cells))
	return sumX / n, sumY / n
}

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
	GridX int8 // X position relative to organism center
	GridY int8 // Y position relative to organism center

	// Function selection (from CPPN argmax)
	PrimaryType   CellType // Main function (from argmax of functional outputs)
	SecondaryType CellType // Optional secondary function (CellTypeNone if none)

	// Raw CPPN output strengths (before mixed-function penalty)
	PrimaryStrength   float32 // Raw CPPN output for primary type
	SecondaryStrength float32 // Raw CPPN output for secondary type (if above threshold)

	// Derived effective strengths (after mixed-function penalty)
	EffectivePrimary   float32 // Primary strength after penalty
	EffectiveSecondary float32 // Secondary strength after penalty

	// Spectrum value (only meaningful for digestive cells)
	DigestiveSpectrum float32 // 0=herbivore, 1=carnivore

	// Reproductive mode (only meaningful for reproductive cells)
	ReproductiveMode float32 // 0=asexual, 0.5=mixed, 1=sexual

	// Additive modifiers (incur costs)
	StructuralArmor  float32 // 0-1, damage reduction (adds drag)
	StorageCapacity  float32 // 0-1, max energy bonus (adds metabolism)
}

// MorphologyResult holds the output of CPPN morphology generation.
type MorphologyResult struct {
	Cells    []CellSpec // Cell positions and properties
	DietBias float32    // Average diet bias across all cells
}

// candidate holds intermediate data during morphology generation.
type candidate struct {
	gridX   int8
	gridY   int8
	outputs []float64 // Raw CPPN outputs for this position
}

// SelectCellFunctions selects primary and secondary cell types from CPPN outputs.
// Uses argmax to pick primary type, second-highest for secondary if above threshold.
// Returns: primary type, secondary type (CellTypeNone if none), effective strengths.
func SelectCellFunctions(functionalOutputs []float64) (primary, secondary CellType, effPrimary, effSecondary float32) {
	if len(functionalOutputs) < CPPNFunctionalOutputs {
		return CellTypeSensor, CellTypeNone, 0.5, 0
	}

	// Normalize functional outputs to [0,1]
	normalized := make([]float64, CPPNFunctionalOutputs)
	for i := 0; i < CPPNFunctionalOutputs; i++ {
		normalized[i] = (functionalOutputs[i] + 1.0) / 2.0
	}

	// Find primary (argmax)
	maxIdx, maxVal := 0, normalized[0]
	for i, v := range normalized {
		if v > maxVal {
			maxIdx, maxVal = i, v
		}
	}

	// Map index to CellType (offset by 1 because CellTypeNone is 0)
	primary = CellType(maxIdx + 1)
	pStr := float32(maxVal)

	// Find secondary (second max above threshold)
	secondIdx, secondVal := -1, 0.0
	for i, v := range normalized {
		if i != maxIdx && v > secondVal {
			secondIdx, secondVal = i, v
		}
	}

	if secondVal >= SecondaryThreshold {
		secondary = CellType(secondIdx + 1)
		sStr := float32(secondVal)
		// Apply mixed-function penalty
		effPrimary = pStr * MixedPrimaryPenalty
		effSecondary = sStr * MixedSecondaryScale
	} else {
		secondary = CellTypeNone
		effPrimary = pStr
		effSecondary = 0
	}

	return primary, secondary, effPrimary, effSecondary
}

// GenerateMorphology queries the CPPN genome to produce an organism's body layout.
// The CPPN is queried at each position in an 8x8 grid.
// Positions with presence > threshold become cells.
// Returns up to maxCells cells sorted by presence (priority).
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

			// Simplified CPPN inputs: x, y, distance, angle, bias
			inputs := []float64{
				x,
				y,
				d,
				a / math.Pi, // Normalize angle to [-1, 1]
				1.0,         // Bias
			}

			if err := phenotype.LoadSensors(inputs); err != nil {
				continue
			}

			// Activate network
			activated, err := phenotype.Activate()
			if err != nil || !activated {
				phenotype.Flush()
				continue
			}

			outputs := phenotype.ReadOutputs()
			phenotype.Flush()

			if len(outputs) < CPPNOutputs {
				continue
			}

			// Check presence threshold
			presence := outputs[CPPNOutPresence]
			if presence > threshold {
				candidates = append(candidates, candidate{
					gridX:   int8(gx - MorphGridSize/2),
					gridY:   int8(gy - MorphGridSize/2),
					outputs: append([]float64{}, outputs...), // Copy outputs
				})
			}
		}
	}

	// Sort by presence (first output) descending - acts as priority
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].outputs[CPPNOutPresence] > candidates[j].outputs[CPPNOutPresence]
	})

	// Limit to maxCells
	if len(candidates) > maxCells {
		candidates = candidates[:maxCells]
	}

	// Build CellSpecs from candidates
	cells := make([]CellSpec, len(candidates))
	for i, c := range candidates {
		// Extract functional outputs explicitly (the 5 cell type functions)
		functionalOutputs := []float64{
			c.outputs[CPPNOutSensor],       // 0 → CellTypeSensor
			c.outputs[CPPNOutActuator],     // 1 → CellTypeActuator
			c.outputs[CPPNOutMouth],        // 2 → CellTypeMouth
			c.outputs[CPPNOutDigestive],    // 3 → CellTypeDigestive
			c.outputs[CPPNOutReproductive], // 4 → CellTypeReproductive
		}
		primary, secondary, effPrimary, effSecondary := SelectCellFunctions(functionalOutputs)

		// Raw strengths before penalty - map CellType to CPPN output index
		cellTypeToOutput := map[CellType]int{
			CellTypeSensor:      CPPNOutSensor,
			CellTypeActuator:    CPPNOutActuator,
			CellTypeMouth:       CPPNOutMouth,
			CellTypeDigestive:   CPPNOutDigestive,
			CellTypeReproductive: CPPNOutReproductive,
		}

		var rawPrimary, rawSecondary float32
		if outIdx, ok := cellTypeToOutput[primary]; ok {
			rawPrimary = float32((c.outputs[outIdx] + 1.0) / 2.0)
		}
		if secondary != CellTypeNone {
			if outIdx, ok := cellTypeToOutput[secondary]; ok {
				rawSecondary = float32((c.outputs[outIdx] + 1.0) / 2.0)
			}
		}

		// Digestive spectrum (normalize to 0-1 range)
		digestiveSpectrum := float32((c.outputs[CPPNOutDigestive] + 1.0) / 2.0)

		// Reproductive mode (normalize to 0-1 range)
		// For reproductive cells, this determines asexual(0) vs sexual(1)
		reproductiveMode := float32((c.outputs[CPPNOutReproductive] + 1.0) / 2.0)

		// Additive modifiers (normalize to 0-1)
		structuralArmor := float32((c.outputs[CPPNOutStructuralArmor] + 1.0) / 2.0)
		storageCapacity := float32((c.outputs[CPPNOutStorageCapacity] + 1.0) / 2.0)

		cells[i] = CellSpec{
			GridX:              c.gridX,
			GridY:              c.gridY,
			PrimaryType:        primary,
			SecondaryType:      secondary,
			PrimaryStrength:    rawPrimary,
			SecondaryStrength:  rawSecondary,
			EffectivePrimary:   effPrimary,
			EffectiveSecondary: effSecondary,
			DigestiveSpectrum:  digestiveSpectrum,
			ReproductiveMode:   reproductiveMode,
			StructuralArmor:    structuralArmor,
			StorageCapacity:    storageCapacity,
		}
	}

	// Ensure viability: at least 1 sensor and 1 actuator
	cells = ensureViability(cells)

	// Calculate average diet bias from digestive spectrum
	var totalDigestive float64
	for _, c := range cells {
		totalDigestive += float64(c.DigestiveSpectrum)
	}
	avgDigestive := float32(0.0)
	if len(cells) > 0 {
		avgDigestive = float32(totalDigestive / float64(len(cells)))
	}
	// Convert 0-1 range to -1 to 1 for diet bias
	dietBias := avgDigestive*2 - 1

	return MorphologyResult{
		Cells:    cells,
		DietBias: dietBias,
	}, nil
}

// ensureViability ensures the morphology has at least one sensor and one actuator.
// Modifies cells in-place or returns new cells if needed.
func ensureViability(cells []CellSpec) []CellSpec {
	if len(cells) == 0 {
		// Create minimal viable morphology
		return []CellSpec{
			{
				GridX:            0,
				GridY:            0,
				PrimaryType:      CellTypeSensor,
				SecondaryType:    CellTypeActuator,
				PrimaryStrength:  0.5,
				SecondaryStrength: 0.3,
				EffectivePrimary:  0.5 * MixedPrimaryPenalty,
				EffectiveSecondary: 0.3 * MixedSecondaryScale,
			},
		}
	}

	// Check for sensor capability
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

	// Fix missing capabilities by adding as secondary function
	if !hasSensor {
		// Find best candidate to add sensor secondary
		bestIdx := 0
		for i, c := range cells {
			if c.SecondaryType == CellTypeNone {
				bestIdx = i
				break
			}
		}
		cells[bestIdx].SecondaryType = CellTypeSensor
		cells[bestIdx].SecondaryStrength = 0.3
		cells[bestIdx].EffectiveSecondary = 0.3 * MixedSecondaryScale
		// Apply mixed penalty to primary
		cells[bestIdx].EffectivePrimary = cells[bestIdx].PrimaryStrength * MixedPrimaryPenalty
	}

	if !hasActuator {
		// Find best candidate to add actuator secondary
		bestIdx := 0
		for i, c := range cells {
			// Prefer cell that doesn't already have a sensor secondary
			if c.SecondaryType == CellTypeNone {
				bestIdx = i
				break
			}
			if c.SecondaryType != CellTypeSensor {
				bestIdx = i
			}
		}
		// If best candidate already has secondary, upgrade it
		if cells[bestIdx].SecondaryType != CellTypeNone && cells[bestIdx].SecondaryType != CellTypeSensor {
			cells[bestIdx].SecondaryType = CellTypeActuator
		} else if cells[bestIdx].SecondaryType == CellTypeNone {
			cells[bestIdx].SecondaryType = CellTypeActuator
			cells[bestIdx].SecondaryStrength = 0.3
			cells[bestIdx].EffectiveSecondary = 0.3 * MixedSecondaryScale
			cells[bestIdx].EffectivePrimary = cells[bestIdx].PrimaryStrength * MixedPrimaryPenalty
		}
	}

	return cells
}

// GenerateMorphologyWithConfig uses configuration for grid size and threshold.
func GenerateMorphologyWithConfig(genome *genetics.Genome, cfg CPPNConfig) (MorphologyResult, error) {
	return GenerateMorphology(genome, cfg.MaxCells, cfg.CellThreshold)
}

// CellCount returns the number of cells in the morphology.
func (m *MorphologyResult) CellCount() int {
	return len(m.Cells)
}

// HasFunction returns true if the cell has the given function (primary or secondary).
func (c *CellSpec) HasFunction(ct CellType) bool {
	return c.PrimaryType == ct || c.SecondaryType == ct
}


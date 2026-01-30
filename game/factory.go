package game

import (
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// createFloraLightweight creates a lightweight flora using FloraSystem.
// Returns true if flora was created successfully.
func (g *Game) createFloraLightweight(x, y float32, isRooted bool, energy float32) bool {
	if isRooted {
		return g.floraSystem.AddRooted(x, y, energy)
	}
	return g.floraSystem.AddFloating(x, y, energy)
}

// createNeuralOrganism creates an organism with neural network brain and CPPN-generated morphology.
// If maxCells <= 0, no cell count constraint is applied (uses full CPPN output).
func (g *Game) createNeuralOrganism(x, y float32, energy float32, neuralGenome *components.NeuralGenome, brain *components.Brain, maxCells int) ecs.Entity {
	// Generate morphology from CPPN
	var morph neural.MorphologyResult
	if neuralGenome != nil && neuralGenome.BodyGenome != nil {
		var err error
		if maxCells > 0 {
			// Constrained cell count
			morph, err = neural.GenerateMorphology(neuralGenome.BodyGenome, maxCells, g.neuralConfig.CPPN.CellThreshold)
		} else {
			// Full CPPN output
			morph, err = neural.GenerateMorphologyWithConfig(neuralGenome.BodyGenome, g.neuralConfig.CPPN)
		}
		if err != nil {
			// Fallback to minimal viable morphology
			morph = minimalMorphology()
		}
	} else {
		// No CPPN genome, minimal viable fallback
		morph = minimalMorphology()
	}

	// Calculate max energy based on cell count
	cellCount := morph.CellCount()
	maxEnergy := float32(100 + cellCount*50)

	pos := &components.Position{X: x, Y: y}
	vel := &components.Velocity{X: 0, Y: 0}
	org := &components.Organism{
		Energy:           energy,
		MaxEnergy:        maxEnergy,
		CellSize:         5,
		MaxSpeed:         1.5,
		MaxForce:         0.1,
		PerceptionRadius: 60,
		Heading:          rand.Float32() * 3.14159 * 2,
		GrowthInterval:   200,
		SporeInterval:    400,
		BreedingCooldown: 300,
		TargetCells:      uint8(cellCount),
		EatIntent:        0.5,
		GrowIntent:       0.3,
		BreedIntent:      0.3,
	}

	// Create cells from morphology
	cells := &components.CellBuffer{Count: 0}
	for _, cellSpec := range morph.Cells {
		cell := components.Cell{
			GridX:             cellSpec.GridX,
			GridY:             cellSpec.GridY,
			Age:               0,
			MaxAge:            3000 + rand.Int31n(1000),
			Alive:             true,
			PrimaryType:       cellSpec.PrimaryType,
			SecondaryType:     cellSpec.SecondaryType,
			PrimaryStrength:   cellSpec.EffectivePrimary,
			SecondaryStrength: cellSpec.EffectiveSecondary,
			DigestiveSpectrum: cellSpec.DigestiveSpectrum,
			StructuralArmor:   cellSpec.StructuralArmor,
			StorageCapacity:   cellSpec.StorageCapacity,
			ReproductiveMode:  cellSpec.ReproductiveMode,
		}
		cells.AddCell(cell)
	}

	// Calculate shape metrics and collision OBB from morphology
	org.ShapeMetrics = systems.CalculateShapeMetrics(cells)
	org.OBB = systems.ComputeCollisionOBB(cells, org.CellSize)

	// Create entity - neural organisms are always fauna
	entity := g.faunaMapper.NewEntity(pos, vel, org, cells, &components.Fauna{})

	// Assign species BEFORE adding to map (map stores copy, not pointer)
	if neuralGenome != nil && neuralGenome.BrainGenome != nil && g.speciesManager != nil {
		parentSpeciesID := neuralGenome.SpeciesID
		speciesID := g.speciesManager.AssignSpecies(neuralGenome.BrainGenome)
		neuralGenome.SpeciesID = speciesID
		g.speciesManager.AddMember(speciesID, int(entity.ID()))

		// If this is offspring (had a parent species), record it and log
		if parentSpeciesID > 0 {
			g.speciesManager.RecordOffspring(parentSpeciesID)

			// Log breeding event
			if g.neuralLog && g.neuralLogDetail {
				nodes := len(neuralGenome.BrainGenome.Nodes)
				genes := len(neuralGenome.BrainGenome.Genes)
				g.logBirthEvent(uint64(entity.ID()), x, y, neuralGenome.Generation, nodes, genes, speciesID, parentSpeciesID)
			}
		}
	}

	// Add neural components (after species is set)
	if neuralGenome != nil {
		g.neuralGenomeMap.Add(entity, neuralGenome)
	}
	if brain != nil {
		g.brainMap.Add(entity, brain)
	}

	return entity
}

// createInitialNeuralOrganism creates a new neural organism with fresh genomes.
// This is used during seeding to create the initial neural population.
// Initial organisms are constrained to 1-4 cells for easier evolution.
// Uses HyperNEAT: CPPN generates both morphology and brain weights.
func (g *Game) createInitialNeuralOrganism(x, y float32, energy float32) ecs.Entity {
	// Create CPPN genome (generates both body morphology and brain weights)
	bodyGenome := neural.CreateCPPNGenome(g.genomeIDGen.NextID())

	// Generate morphology from CPPN first (needed for brain building)
	morph, err := neural.GenerateMorphology(bodyGenome, neural.InitialMaxCells, g.neuralConfig.CPPN.CellThreshold)
	if err != nil {
		// Retry with a new genome
		bodyGenome = neural.CreateCPPNGenome(g.genomeIDGen.NextID())
		morph, err = neural.GenerateMorphology(bodyGenome, neural.InitialMaxCells, g.neuralConfig.CPPN.CellThreshold)
		if err != nil {
			// Use minimal morphology as last resort
			morph = minimalMorphology()
		}
	}

	// Build brain from CPPN using HyperNEAT (CPPN determines connection weights)
	brainController, err := neural.SimplifiedHyperNEATBrain(bodyGenome, &morph)
	if err != nil {
		// Fallback to traditional brain if HyperNEAT fails
		brainGenome := neural.CreateBrainGenome(g.genomeIDGen.NextID(), g.neuralConfig.Brain.InitialConnectionProb)
		brainController, err = neural.NewBrainController(brainGenome)
		if err != nil {
			// Last resort - minimal brain
			brainGenome = neural.CreateMinimalBrainGenome(g.genomeIDGen.NextID())
			brainController, _ = neural.NewBrainController(brainGenome)
		}
	}

	// Build neural components (BrainGenome is now derived from CPPN, stored for compatibility)
	neuralGenome := &components.NeuralGenome{
		BodyGenome:  bodyGenome,
		BrainGenome: brainController.Genome, // Store the derived brain genome
		SpeciesID:   0,                      // Will be assigned by species manager
		Generation:  0,
	}

	brain := &components.Brain{
		Controller: brainController,
	}

	return g.createNeuralOrganism(x, y, energy, neuralGenome, brain, neural.InitialMaxCells)
}

// minimalMorphology returns a fallback minimal viable morphology.
func minimalMorphology() neural.MorphologyResult {
	return neural.MorphologyResult{
		Cells: []neural.CellSpec{{
			GridX: 0, GridY: 0,
			PrimaryType: neural.CellTypeSensor, SecondaryType: neural.CellTypeActuator,
			PrimaryStrength: 0.5, SecondaryStrength: 0.3,
			EffectivePrimary:   0.5 * neural.MixedPrimaryPenalty,
			EffectiveSecondary: 0.3 * neural.MixedSecondaryScale,
		}},
	}
}

// CreateNeuralOrganismForBreeding creates a neural organism for the breeding system.
// This matches the NeuralOrganismCreator signature.
func (g *Game) CreateNeuralOrganismForBreeding(x, y, energy float32, neuralGenome *components.NeuralGenome, brain *components.Brain) ecs.Entity {
	return g.createNeuralOrganism(x, y, energy, neuralGenome, brain, 0) // No constraint
}

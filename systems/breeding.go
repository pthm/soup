package systems

import (
	"math/rand"

	"github.com/mlange-42/ark/ecs"
	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

// Breeding constants
const (
	BreedIntentThreshold = 0.5  // Brain output threshold for breeding intent (lowered)
	MinEnergyRatio       = 0.5  // Minimum energy ratio for reproduction (lowered from 0.7)
	AsexualEnergyCost    = 15.0 // Energy cost for asexual reproduction (reduced)
	SexualEnergyCost     = 12.0 // Energy cost for sexual reproduction (per parent, reduced)
	MateProximity        = 100.0 // Maximum distance for finding mates (increased)
)

// BreedingSystem handles fauna reproduction (both asexual and sexual).
type BreedingSystem struct {
	filter      ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	neuralMap   *ecs.Map[components.NeuralGenome]
	brainMap    *ecs.Map[components.Brain]
	neatOpts    *neat.Options
	genomeIDGen *neural.GenomeIDGenerator
	cppnConfig  neural.CPPNConfig
}

// NewBreedingSystem creates a new breeding system.
func NewBreedingSystem(w *ecs.World, opts *neat.Options, idGen *neural.GenomeIDGenerator, cppnCfg neural.CPPNConfig) *BreedingSystem {
	return &BreedingSystem{
		filter:      *ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](w),
		neuralMap:   ecs.NewMap[components.NeuralGenome](w),
		brainMap:    ecs.NewMap[components.Brain](w),
		neatOpts:    opts,
		genomeIDGen: idGen,
		cppnConfig:  cppnCfg,
	}
}

// OrganismCreator is called to create new organisms.
type OrganismCreator func(x, y float32, energy float32) ecs.Entity

// NeuralOrganismCreator is called to create neural organisms with genomes.
type NeuralOrganismCreator func(x, y float32, energy float32, neural *components.NeuralGenome, brain *components.Brain) ecs.Entity

// breeder holds data for a potential breeding organism.
type breeder struct {
	entity    ecs.Entity
	pos       *components.Position
	vel       *components.Velocity
	org       *components.Organism
	cells     *components.CellBuffer
	caps      components.Capabilities
	speciesID int
}

// Update processes breeding for all eligible fauna.
// Organisms can reproduce sexually (with a mate) or asexually (cloning with mutation)
// based on their ReproductiveMode derived from CPPN.
func (s *BreedingSystem) Update(w *ecs.World, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Collect all potential breeders
	var breeders []breeder

	query := s.filter.Query()
	for query.Next() {
		pos, vel, org, cells := query.Get()

		// Skip dead organisms (all ECS organisms are fauna)
		if org.Dead {
			continue
		}

		// Check basic eligibility
		if !s.isEligible(org, cells) {
			continue
		}

		// Compute capabilities (includes reproductive mode)
		caps := cells.ComputeCapabilities()

		// Must have reproductive capability to breed
		if caps.ReproductiveWeight <= 0 {
			continue
		}

		// Get species ID if neural organism
		speciesID := 0
		entity := query.Entity()
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}

		breeders = append(breeders, breeder{
			entity:    entity,
			pos:       pos,
			vel:       vel,
			org:       org,
			cells:     cells,
			caps:      caps,
			speciesID: speciesID,
		})
	}

	// Track which organisms have bred this tick
	bred := make(map[ecs.Entity]bool)

	// Process each eligible breeder
	for i := range breeders {
		if bred[breeders[i].entity] {
			continue
		}

		a := &breeders[i]

		// Get reproductive mode (0=asexual, 0.5=mixed, 1=sexual)
		reproMode := a.caps.ReproductiveMode()

		// Try sexual reproduction based on reproductive mode
		if rand.Float32() < reproMode {
			// Look for a compatible mate
			for j := range breeders {
				if i == j || bred[breeders[j].entity] {
					continue
				}

				b := &breeders[j]

				if s.isCompatibleForSexual(a, b) {
					s.breedSexual(a, b, createOrganism, createNeuralOrganism)
					bred[a.entity] = true
					bred[b.entity] = true
					break
				}
			}
		}

		// If not bred yet, try asexual reproduction
		if !bred[a.entity] && rand.Float32() < (1.0-reproMode) {
			s.breedAsexual(a, createOrganism, createNeuralOrganism)
			bred[a.entity] = true
		}
	}
}

// isEligible checks if an organism meets basic requirements to attempt reproduction.
// Brain output (BreedIntent) directly controls breeding - no allocation mode gating.
func (s *BreedingSystem) isEligible(org *components.Organism, cells *components.CellBuffer) bool {
	// Check breed intent from brain (threshold for wanting to reproduce)
	// This is the PRIMARY control - the brain decides when to breed
	if org.BreedIntent < BreedIntentThreshold {
		return false
	}

	// Minimum energy to attempt reproduction (can't breed at 0 energy)
	// But this is much lower than before - let brains learn the tradeoffs
	if org.Energy < org.MaxEnergy*0.25 {
		return false
	}

	// Must have at least 1 cell
	if cells.Count < 1 {
		return false
	}

	// Cooldown must be 0
	if org.BreedingCooldown > 0 {
		return false
	}

	return true
}

// Mate compatibility constants
const (
	mateProximitySq          = MateProximity * MateProximity // Squared for faster comparison
	minAvgReproModeForSexual = float32(0.3)                  // Minimum average reproductive mode
)

// isCompatibleForSexual checks if two organisms can mate sexually.
// No longer requires opposite genders - any two willing organisms can mate.
func (s *BreedingSystem) isCompatibleForSexual(a, b *breeder) bool {
	// Both must have reproductive capability
	if a.caps.ReproductiveWeight <= 0 || b.caps.ReproductiveWeight <= 0 {
		return false
	}

	// Must be within proximity (use squared distance to avoid sqrt)
	if distanceSq(a.pos.X, a.pos.Y, b.pos.X, b.pos.Y) > mateProximitySq {
		return false
	}

	// Both should prefer sexual reproduction (or at least be willing)
	avgReproMode := (a.caps.ReproductiveMode() + b.caps.ReproductiveMode()) / 2
	return avgReproMode >= minAvgReproModeForSexual
}

// breedSexual performs sexual reproduction between two organisms.
func (s *BreedingSystem) breedSexual(a, b *breeder, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Position: midpoint between parents
	x := (a.pos.X + b.pos.X) / 2
	y := (a.pos.Y + b.pos.Y) / 2

	// Check if both parents have neural genomes
	neuralA := s.neuralMap.Get(a.entity)
	neuralB := s.neuralMap.Get(b.entity)

	if neuralA != nil && neuralB != nil && createNeuralOrganism != nil && s.genomeIDGen != nil && s.neatOpts != nil {
		// Neural breeding with crossover
		s.breedNeuralSexual(x, y, a.org, b.org, a.cells, b.cells, neuralA, neuralB, createNeuralOrganism)
	} else if createOrganism != nil {
		// Fallback breeding
		createOrganism(x, y, 50)
	}

	// Cost to both parents (shared cost)
	a.org.Energy -= SexualEnergyCost
	b.org.Energy -= SexualEnergyCost

	// Set cooldowns
	a.org.BreedingCooldown = 120
	b.org.BreedingCooldown = 120
}

// breedAsexual performs asexual reproduction (budding/cloning with mutation).
func (s *BreedingSystem) breedAsexual(a *breeder, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Position: slight offset from parent
	offsetX := (rand.Float32() - 0.5) * 40
	offsetY := (rand.Float32() - 0.5) * 40
	x := a.pos.X + offsetX
	y := a.pos.Y + offsetY

	// Check if parent has neural genome
	neuralA := s.neuralMap.Get(a.entity)

	if neuralA != nil && createNeuralOrganism != nil && s.genomeIDGen != nil && s.neatOpts != nil {
		// Neural asexual reproduction - clone and mutate
		s.breedNeuralAsexual(x, y, a.org, neuralA, createNeuralOrganism)
	} else if createOrganism != nil {
		// Fallback breeding
		createOrganism(x, y, 50)
	}

	// Higher cost for single parent (solo investment)
	a.org.Energy -= AsexualEnergyCost

	// Set cooldown
	a.org.BreedingCooldown = 150 // Slightly longer cooldown for asexual
}

// breedNeuralSexual performs neural reproduction with genome crossover.
func (s *BreedingSystem) breedNeuralSexual(
	x, y float32,
	orgA, orgB *components.Organism,
	cellsA, cellsB *components.CellBuffer,
	neuralA, neuralB *components.NeuralGenome,
	createNeuralOrganism NeuralOrganismCreator,
) {
	// Calculate fitness for each parent
	fitnessA := neural.CalculateBreedingFitness(orgA.Energy, orgA.MaxEnergy, int(cellsA.Count))
	fitnessB := neural.CalculateBreedingFitness(orgB.Energy, orgB.MaxEnergy, int(cellsB.Count))

	// HyperNEAT: Crossover and mutate only the CPPN genome
	bodyChild, err := neural.CreateOffspringCPPN(
		neuralA.BodyGenome, neuralB.BodyGenome,
		fitnessA, fitnessB,
		s.genomeIDGen,
		s.neatOpts,
	)
	if err != nil {
		// Fall back to cloning parent A's genome
		bodyChild, _ = neural.CloneGenome(neuralA.BodyGenome, s.genomeIDGen.NextID())
		if bodyChild != nil {
			neural.MutateCPPNGenome(bodyChild, s.neatOpts, s.genomeIDGen)
		}
	}

	if bodyChild == nil {
		return
	}

	s.createOffspringFromCPPN(x, y, bodyChild, neuralA, neuralB, createNeuralOrganism)
}

// breedNeuralAsexual performs neural reproduction by cloning and mutating.
func (s *BreedingSystem) breedNeuralAsexual(
	x, y float32,
	org *components.Organism,
	neuralParent *components.NeuralGenome,
	createNeuralOrganism NeuralOrganismCreator,
) {
	// Clone parent's CPPN genome
	bodyChild, err := neural.CloneGenome(neuralParent.BodyGenome, s.genomeIDGen.NextID())
	if err != nil || bodyChild == nil {
		return
	}

	// Apply mutations (slightly higher mutation rate for asexual to maintain diversity)
	neural.MutateCPPNGenome(bodyChild, s.neatOpts, s.genomeIDGen)

	// Create offspring from mutated clone
	s.createOffspringFromCPPN(x, y, bodyChild, neuralParent, neuralParent, createNeuralOrganism)
}

// createOffspringFromCPPN generates morphology and brain from CPPN and creates the organism.
func (s *BreedingSystem) createOffspringFromCPPN(
	x, y float32,
	bodyGenome *genetics.Genome,
	neuralA, neuralB *components.NeuralGenome,
	createNeuralOrganism NeuralOrganismCreator,
) {
	// Generate morphology from child CPPN
	morph, err := neural.GenerateMorphology(bodyGenome, s.cppnConfig.MaxCells, s.cppnConfig.CellThreshold)
	if err != nil {
		return
	}

	// HyperNEAT: Build brain from CPPN + morphology
	brainController, err := neural.SimplifiedHyperNEATBrain(bodyGenome, &morph)
	if err != nil {
		// Fallback to traditional brain creation
		brainGenome := neural.CreateBrainGenome(s.genomeIDGen.NextID(), 0.3)
		brainController, err = neural.NewBrainController(brainGenome)
		if err != nil {
			return
		}
	}

	// Determine species (based on CPPN genome compatibility)
	childSpeciesID := neuralA.SpeciesID // Default to parent's species

	// Create neural genome component
	childNeural := &components.NeuralGenome{
		BodyGenome:  bodyGenome,
		BrainGenome: brainController.Genome,
		SpeciesID:   childSpeciesID,
		Generation:  max(neuralA.Generation, neuralB.Generation) + 1,
	}

	// Create brain component
	childBrain := &components.Brain{
		Controller: brainController,
	}

	// Create the offspring - diet derived from cells
	createNeuralOrganism(x, y, 100, childNeural, childBrain)
}

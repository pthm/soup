package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/traits"
)

// Breeding constants
const (
	BreedIntentThreshold = 0.6  // Brain output threshold for breeding intent
	MinEnergyRatio       = 0.7  // Minimum energy ratio for reproduction
	AsexualEnergyCost    = 20.0 // Energy cost for asexual reproduction (single parent)
	SexualEnergyCost     = 15.0 // Energy cost for sexual reproduction (per parent)
	MateProximity        = 80.0 // Maximum distance for finding mates
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
type OrganismCreator func(x, y float32, t traits.Trait, energy float32) ecs.Entity

// NeuralOrganismCreator is called to create neural organisms with genomes.
type NeuralOrganismCreator func(x, y float32, t traits.Trait, energy float32, neural *components.NeuralGenome, brain *components.Brain) ecs.Entity

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

		// Skip flora and dead organisms
		if traits.IsFlora(org.Traits) || org.Dead {
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
func (s *BreedingSystem) isEligible(org *components.Organism, cells *components.CellBuffer) bool {
	// Check breed intent from brain (threshold for wanting to reproduce)
	if org.BreedIntent < BreedIntentThreshold {
		return false
	}

	// Must be in Breed allocation mode
	if org.AllocationMode != components.ModeBreed {
		return false
	}

	// Energy must be above minimum ratio
	if org.Energy < org.MaxEnergy*MinEnergyRatio {
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

// isCompatibleForSexual checks if two organisms can mate sexually.
// No longer requires opposite genders - any two willing organisms can mate.
func (s *BreedingSystem) isCompatibleForSexual(a, b *breeder) bool {
	// Both must be eligible (already checked) and have reproductive capability
	if a.caps.ReproductiveWeight <= 0 || b.caps.ReproductiveWeight <= 0 {
		return false
	}

	// Must be within proximity
	dx := a.pos.X - b.pos.X
	dy := a.pos.Y - b.pos.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist > MateProximity {
		return false
	}

	// Both should prefer sexual reproduction (or at least be willing)
	// Use the average of their reproductive modes as compatibility
	avgReproMode := (a.caps.ReproductiveMode() + b.caps.ReproductiveMode()) / 2
	if avgReproMode < 0.3 {
		// Both lean heavily asexual, unlikely to mate
		return false
	}

	return true
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
	} else {
		// Traditional trait-based breeding
		offspringTraits := s.inheritTraits(a.org.Traits, b.org.Traits)
		createOrganism(x, y, offspringTraits, 50)
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
	} else {
		// Traditional trait-based - clone parent traits with small variation
		offspringTraits := s.inheritTraitsAsexual(a.org.Traits)
		createOrganism(x, y, offspringTraits, 50)
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

	s.createOffspringFromCPPN(x, y, bodyChild, orgA.Traits, orgB.Traits, neuralA, neuralB, createNeuralOrganism)
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
	s.createOffspringFromCPPN(x, y, bodyChild, org.Traits, org.Traits, neuralParent, neuralParent, createNeuralOrganism)
}

// createOffspringFromCPPN generates morphology and brain from CPPN and creates the organism.
func (s *BreedingSystem) createOffspringFromCPPN(
	x, y float32,
	bodyGenome *genetics.Genome,
	traitsA, traitsB traits.Trait,
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

	// Inherit traits from parents
	offspringTraits := s.inheritTraits(traitsA, traitsB)

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

	// Create the offspring
	createNeuralOrganism(x, y, offspringTraits, 100, childNeural, childBrain)
}

// inheritTraits combines traits from two parents (for sexual reproduction).
// Excludes deprecated Male/Female traits.
func (s *BreedingSystem) inheritTraits(a, b traits.Trait) traits.Trait {
	var result traits.Trait

	// Inheritable diet traits (50% from each parent)
	dietTraits := []traits.Trait{
		traits.Herbivore,
		traits.Carnivore,
		traits.Carrion,
	}

	for _, t := range dietTraits {
		// 50% chance to inherit if either parent has it
		if (a.Has(t) || b.Has(t)) && rand.Float32() < 0.5 {
			result = result.Add(t)
		}
	}

	// Must have at least one diet trait
	hasDiet := result.Has(traits.Herbivore) || result.Has(traits.Carnivore) || result.Has(traits.Carrion)
	if !hasDiet {
		// Pick a diet from parents
		parentDiets := []traits.Trait{}
		if a.Has(traits.Herbivore) || b.Has(traits.Herbivore) {
			parentDiets = append(parentDiets, traits.Herbivore)
		}
		if a.Has(traits.Carnivore) || b.Has(traits.Carnivore) {
			parentDiets = append(parentDiets, traits.Carnivore)
		}
		if a.Has(traits.Carrion) || b.Has(traits.Carrion) {
			parentDiets = append(parentDiets, traits.Carrion)
		}
		if len(parentDiets) > 0 {
			result = result.Add(parentDiets[rand.Intn(len(parentDiets))])
		} else {
			// Fallback to herbivore
			result = result.Add(traits.Herbivore)
		}
	}

	// Note: Male/Female traits are deprecated and not inherited

	return result
}

// inheritTraitsAsexual clones parent traits for asexual reproduction.
// Excludes deprecated Male/Female traits.
func (s *BreedingSystem) inheritTraitsAsexual(parent traits.Trait) traits.Trait {
	var result traits.Trait

	// Copy diet traits from parent
	if parent.Has(traits.Herbivore) {
		result = result.Add(traits.Herbivore)
	}
	if parent.Has(traits.Carnivore) {
		result = result.Add(traits.Carnivore)
	}
	if parent.Has(traits.Carrion) {
		result = result.Add(traits.Carrion)
	}

	// Must have at least one diet trait
	if !result.Has(traits.Herbivore) && !result.Has(traits.Carnivore) && !result.Has(traits.Carrion) {
		result = result.Add(traits.Herbivore)
	}

	// Note: Male/Female traits are deprecated and not inherited

	return result
}

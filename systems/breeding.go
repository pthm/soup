package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
	"github.com/yaricom/goNEAT/v4/neat"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/traits"
)

// BreedingSystem handles fauna sexual reproduction.
type BreedingSystem struct {
	filter       ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	neuralMap    *ecs.Map[components.NeuralGenome]
	brainMap     *ecs.Map[components.Brain]
	neatOpts     *neat.Options
	genomeIDGen  *neural.GenomeIDGenerator
	cppnConfig   neural.CPPNConfig
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

// Update processes breeding for all eligible fauna.
// createOrganism is used for non-neural organisms.
// createNeuralOrganism is used for neural organisms (can be nil to skip neural breeding).
func (s *BreedingSystem) Update(w *ecs.World, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Collect all potential breeders
	type breeder struct {
		entity    ecs.Entity
		pos       *components.Position
		vel       *components.Velocity
		org       *components.Organism
		cells     *components.CellBuffer
		speciesID int // Species ID for assortative mating
	}

	var breeders []breeder

	query := s.filter.Query()
	for query.Next() {
		pos, vel, org, cells := query.Get()

		// Skip flora and dead organisms
		if traits.IsFlora(org.Traits) || org.Dead {
			continue
		}

		// Check eligibility
		if !s.isEligible(org, cells) {
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
			speciesID: speciesID,
		})
	}

	// Try to find compatible pairs
	bred := make(map[ecs.Entity]bool)

	for i := range breeders {
		if bred[breeders[i].entity] {
			continue
		}

		for j := i + 1; j < len(breeders); j++ {
			if bred[breeders[j].entity] {
				continue
			}

			a := &breeders[i]
			b := &breeders[j]

			if s.isCompatible(a.org, b.org, a.pos, b.pos, a.speciesID, b.speciesID) {
				s.breed(a.entity, b.entity, a.pos, b.pos, a.org, b.org, a.cells, b.cells, createOrganism, createNeuralOrganism)
				bred[a.entity] = true
				bred[b.entity] = true
				break
			}
		}
	}
}

func (s *BreedingSystem) isEligible(org *components.Organism, cells *components.CellBuffer) bool {
	// Check mate intent from brain (>0.5 means try to mate)
	if org.MateIntent < 0.5 {
		return false
	}

	// Must be in Breed allocation mode
	if org.AllocationMode != components.ModeBreed {
		return false
	}

	// Energy must be above 35% of max
	if org.Energy < org.MaxEnergy*0.35 {
		return false
	}

	// Only need 1 cell
	if cells.Count < 1 {
		return false
	}

	// Cooldown must be 0
	if org.BreedingCooldown > 0 {
		return false
	}

	return true
}

func (s *BreedingSystem) isCompatible(a, b *components.Organism, posA, posB *components.Position, speciesA, speciesB int) bool {
	// Must have opposite genders
	aIsMale := a.Traits.Has(traits.Male)
	bIsMale := b.Traits.Has(traits.Male)
	if aIsMale == bIsMale {
		return false
	}

	// Must be within proximity (50 units)
	dx := posA.X - posB.X
	dy := posA.Y - posB.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist > 50 {
		return false
	}

	// No breeding preference by species - organisms breed freely like aquatic species
	// Species are tracked for genetic clustering, not reproductive barriers
	// This allows maximum gene flow while still measuring genetic diversity
	_ = speciesA // unused but kept for potential future use
	_ = speciesB

	return true
}

func (s *BreedingSystem) breed(
	entityA, entityB ecs.Entity,
	posA, posB *components.Position,
	orgA, orgB *components.Organism,
	cellsA, cellsB *components.CellBuffer,
	createOrganism OrganismCreator,
	createNeuralOrganism NeuralOrganismCreator,
) {
	// Position: midpoint between parents
	x := (posA.X + posB.X) / 2
	y := (posA.Y + posB.Y) / 2

	// Check if both parents have neural genomes
	neuralA := s.neuralMap.Get(entityA)
	neuralB := s.neuralMap.Get(entityB)

	if neuralA != nil && neuralB != nil && createNeuralOrganism != nil && s.genomeIDGen != nil && s.neatOpts != nil {
		// Neural breeding
		s.breedNeural(x, y, orgA, orgB, cellsA, cellsB, neuralA, neuralB, createNeuralOrganism)
	} else {
		// Traditional trait-based breeding
		offspringTraits := s.inheritTraits(orgA.Traits, orgB.Traits)
		createOrganism(x, y, offspringTraits, 50)
	}

	// Cost to parents
	orgA.Energy -= 20
	orgB.Energy -= 20

	// Set cooldowns
	orgA.BreedingCooldown = 180
	orgB.BreedingCooldown = 180
}

func (s *BreedingSystem) breedNeural(
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
	// The brain will be derived from the CPPN + morphology
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

	// Generate morphology from child CPPN
	morph, err := neural.GenerateMorphology(bodyChild, s.cppnConfig.MaxCells, s.cppnConfig.CellThreshold)
	if err != nil {
		return
	}

	// HyperNEAT: Build brain from CPPN + morphology
	// CPPN determines connection weights based on sensor/actuator positions
	brainController, err := neural.SimplifiedHyperNEATBrain(bodyChild, &morph)
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

	// Inherit traits from parents (still use trait system alongside neural)
	offspringTraits := s.inheritTraits(orgA.Traits, orgB.Traits)

	// Create neural genome component
	// BrainGenome stores the derived brain for compatibility/inspection
	childNeural := &components.NeuralGenome{
		BodyGenome:  bodyChild,
		BrainGenome: brainController.Genome, // Store derived brain genome
		SpeciesID:   childSpeciesID,
		Generation:  max(neuralA.Generation, neuralB.Generation) + 1,
	}

	// Create brain component
	childBrain := &components.Brain{
		Controller: brainController,
	}

	// Create the offspring
	createNeuralOrganism(x, y, offspringTraits, 50, childNeural, childBrain)
}

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

	return result
}

func countSharedTraits(a, b traits.Trait) int {
	// Traits to check (diet only)
	checkTraits := []traits.Trait{
		traits.Herbivore,
		traits.Carnivore,
		traits.Carrion,
	}

	count := 0
	for _, t := range checkTraits {
		if a.Has(t) && b.Has(t) {
			count++
		}
	}
	return count
}

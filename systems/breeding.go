package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"
	"github.com/yaricom/goNEAT/v4/neat"
	"github.com/yaricom/goNEAT/v4/neat/genetics"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

// Breeding constants
const (
	BreedIntentThreshold = 0.5   // Brain output threshold for breeding intent
	MinEnergyRatio       = 0.5   // Minimum energy ratio for reproduction (legacy)
	AsexualEnergyCost    = 15.0  // Energy cost for asexual reproduction
	SexualEnergyCost     = 12.0  // Energy cost for sexual reproduction (per parent)
	MateProximity        = 100.0 // Maximum distance for finding mates (for partner search)

	// Mating handshake parameters (from neural/config.go)
	mateContactMargin = neural.MateContactMargin // Surface-to-surface distance for contact
	mateDwellTime     = neural.MateDwellTime     // Ticks of sustained contact required
	mateEnergyRatio   = neural.MateEnergyRatio   // Minimum energy ratio throughout handshake
)

// BreedingSystem handles fauna reproduction (both asexual and sexual).
type BreedingSystem struct {
	filter      ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	neuralMap   *ecs.Map[components.NeuralGenome]
	brainMap    *ecs.Map[components.Brain]
	neatOpts    *neat.Options
	genomeIDGen *neural.GenomeIDGenerator
	cppnConfig  neural.CPPNConfig
	bounds      Bounds // World bounds for toroidal distance calculations
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

// SetBounds sets the world bounds for toroidal distance calculations.
func (s *BreedingSystem) SetBounds(bounds Bounds) {
	s.bounds = bounds
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
	centerX   float32 // Precomputed OBB center X
	centerY   float32 // Precomputed OBB center Y
	radius    float32 // Precomputed body radius
}

// Update processes breeding for all eligible fauna using a dwell-time handshake.
// Sexual reproduction requires sustained proximity and intent for mateDwellTime ticks.
// Asexual reproduction (cloning with mutation) can occur without a partner.
func (s *BreedingSystem) Update(w *ecs.World, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Collect all organisms that want to mate (have intent and energy)
	var candidates []breeder
	// Map entity ID to index for partner lookup
	entityToIdx := make(map[uint32]int)

	query := s.filter.Query()
	for query.Next() {
		pos, vel, org, cells := query.Get()
		entity := query.Entity()

		// Skip dead organisms
		if org.Dead {
			continue
		}

		// Compute capabilities
		caps := cells.ComputeCapabilities()

		// Must have reproductive capability
		if caps.ReproductiveWeight <= 0 {
			// Reset any stale mating state
			org.MateProgress = 0
			org.MatePartnerID = 0
			continue
		}

		// Precompute center position and radius
		cosH := float32(math.Cos(float64(org.Heading)))
		sinH := float32(math.Sin(float64(org.Heading)))
		centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
		centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH
		radius := computeBodyRadiusBreeding(int(cells.Count), org.CellSize)

		// Get species ID if neural organism
		speciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}

		idx := len(candidates)
		entityToIdx[entity.ID()] = idx
		candidates = append(candidates, breeder{
			entity:    entity,
			pos:       pos,
			vel:       vel,
			org:       org,
			cells:     cells,
			caps:      caps,
			speciesID: speciesID,
			centerX:   centerX,
			centerY:   centerY,
			radius:    radius,
		})
	}

	// Phase 1: Update mating progress for existing handshakes
	for i := range candidates {
		a := &candidates[i]

		// Skip if on cooldown
		if a.org.BreedingCooldown > 0 {
			a.org.MateProgress = 0
			a.org.MatePartnerID = 0
			continue
		}

		// Check if organism has an active handshake
		if a.org.MatePartnerID != 0 {
			// Verify handshake conditions still hold
			partnerIdx, found := entityToIdx[a.org.MatePartnerID]
			if !found {
				// Partner no longer exists
				a.org.MateProgress = 0
				a.org.MatePartnerID = 0
				continue
			}

			b := &candidates[partnerIdx]

			// Both must maintain intent
			if a.org.MateIntent < BreedIntentThreshold || b.org.MateIntent < BreedIntentThreshold {
				a.org.MateProgress = 0
				a.org.MatePartnerID = 0
				continue
			}

			// Both must maintain energy
			if a.org.Energy < a.org.MaxEnergy*mateEnergyRatio ||
				b.org.Energy < b.org.MaxEnergy*mateEnergyRatio {
				a.org.MateProgress = 0
				a.org.MatePartnerID = 0
				continue
			}

			// Must remain in contact
			surfaceDist := computeSurfaceDistance(a.centerX, a.centerY, a.radius, b.centerX, b.centerY, b.radius, s.bounds.Width, s.bounds.Height)
			if surfaceDist > mateContactMargin {
				a.org.MateProgress = 0
				a.org.MatePartnerID = 0
				continue
			}

			// Partner must still be pointing at us
			if b.org.MatePartnerID != a.entity.ID() {
				a.org.MateProgress = 0
				a.org.MatePartnerID = 0
				continue
			}

			// All conditions hold - increment progress
			a.org.MateProgress++
		}
	}

	// Phase 2: Start new handshakes for organisms without partners
	for i := range candidates {
		a := &candidates[i]

		// Skip if already in a handshake or on cooldown
		if a.org.MatePartnerID != 0 || a.org.BreedingCooldown > 0 {
			continue
		}

		// Check if wants to mate sexually
		if a.org.MateIntent < BreedIntentThreshold {
			continue
		}
		if a.org.Energy < a.org.MaxEnergy*mateEnergyRatio {
			continue
		}

		reproMode := a.caps.ReproductiveMode()
		if reproMode < minAvgReproModeForSexual {
			continue
		}

		// Look for a compatible partner
		for j := range candidates {
			if i == j {
				continue
			}
			b := &candidates[j]

			// Skip if partner already has a handshake or is on cooldown
			if b.org.MatePartnerID != 0 || b.org.BreedingCooldown > 0 {
				continue
			}

			// Check compatibility
			if s.canStartHandshake(a, b) {
				// Start mutual handshake
				a.org.MatePartnerID = b.entity.ID()
				a.org.MateProgress = 1
				b.org.MatePartnerID = a.entity.ID()
				b.org.MateProgress = 1
				break
			}
		}
	}

	// Phase 3: Complete handshakes and trigger reproduction
	bred := make(map[ecs.Entity]bool)

	for i := range candidates {
		a := &candidates[i]

		if bred[a.entity] {
			continue
		}

		// Check if handshake is complete
		if a.org.MateProgress >= mateDwellTime && a.org.MatePartnerID != 0 {
			partnerIdx, found := entityToIdx[a.org.MatePartnerID]
			if found {
				b := &candidates[partnerIdx]

				// Verify partner also completed (should be symmetric)
				if b.org.MateProgress >= mateDwellTime && b.org.MatePartnerID == a.entity.ID() {
					// Reproduce!
					s.breedSexual(a, b, createOrganism, createNeuralOrganism)
					bred[a.entity] = true
					bred[b.entity] = true

					// Reset mating state (cooldown set in breedSexual)
					a.org.MateProgress = 0
					a.org.MatePartnerID = 0
					b.org.MateProgress = 0
					b.org.MatePartnerID = 0
				}
			}
		}
	}

	// Phase 4: Asexual reproduction for organisms preferring it
	for i := range candidates {
		a := &candidates[i]

		if bred[a.entity] || a.org.BreedingCooldown > 0 {
			continue
		}

		// Must have intent and energy
		if a.org.MateIntent < BreedIntentThreshold {
			continue
		}
		if a.org.Energy < a.org.MaxEnergy*mateEnergyRatio {
			continue
		}

		// Check reproductive mode - lower values favor asexual
		reproMode := a.caps.ReproductiveMode()
		if rand.Float32() < (1.0 - reproMode) {
			s.breedAsexual(a, createOrganism, createNeuralOrganism)
			bred[a.entity] = true
		}
	}
}

// computeSurfaceDistance returns the surface-to-surface distance between two organisms.
// Uses toroidal geometry for proper wrap-around at world edges.
func computeSurfaceDistance(ax, ay, ar, bx, by, br, worldWidth, worldHeight float32) float32 {
	dx, dy := ToroidalDelta(ax, ay, bx, by, worldWidth, worldHeight)
	centerDist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	return centerDist - ar - br
}

// canStartHandshake checks if two organisms can begin a mating handshake.
func (s *BreedingSystem) canStartHandshake(a, b *breeder) bool {
	// Both must have reproductive capability
	if a.caps.ReproductiveWeight <= 0 || b.caps.ReproductiveWeight <= 0 {
		return false
	}

	// Must be same species (species 0 = non-neural, can't breed sexually)
	if a.speciesID == 0 || b.speciesID == 0 {
		return false
	}
	if a.speciesID != b.speciesID {
		return false
	}

	// Both must have MateIntent above threshold
	if a.org.MateIntent < BreedIntentThreshold || b.org.MateIntent < BreedIntentThreshold {
		return false
	}

	// Both must have sufficient energy
	if a.org.Energy < a.org.MaxEnergy*mateEnergyRatio ||
		b.org.Energy < b.org.MaxEnergy*mateEnergyRatio {
		return false
	}

	// Must be in contact (surface-to-surface distance)
	surfaceDist := computeSurfaceDistance(a.centerX, a.centerY, a.radius, b.centerX, b.centerY, b.radius, s.bounds.Width, s.bounds.Height)
	if surfaceDist > mateContactMargin {
		return false
	}

	// Both should prefer sexual reproduction (or at least be willing)
	avgReproMode := (a.caps.ReproductiveMode() + b.caps.ReproductiveMode()) / 2
	return avgReproMode >= minAvgReproModeForSexual
}

// Mate compatibility constants
const (
	minAvgReproModeForSexual = float32(0.3) // Minimum average reproductive mode for sexual breeding
)

// computeBodyRadius returns sqrt(cellCount) * cellSize for mating proximity.
func computeBodyRadiusBreeding(cellCount int, cellSize float32) float32 {
	return float32(math.Sqrt(float64(cellCount))) * cellSize
}

// breedSexual performs sexual reproduction between two organisms.
func (s *BreedingSystem) breedSexual(a, b *breeder, createOrganism OrganismCreator, createNeuralOrganism NeuralOrganismCreator) {
	// Position: midpoint between parents using toroidal geometry
	// Use ToroidalDelta to find the shortest path between parents
	dx, dy := ToroidalDelta(a.pos.X, a.pos.Y, b.pos.X, b.pos.Y, s.bounds.Width, s.bounds.Height)
	x := a.pos.X + dx/2
	y := a.pos.Y + dy/2
	// Wrap to ensure valid position
	if x < 0 {
		x += s.bounds.Width
	} else if x > s.bounds.Width {
		x -= s.bounds.Width
	}
	if y < 0 {
		y += s.bounds.Height
	} else if y > s.bounds.Height {
		y -= s.bounds.Height
	}

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

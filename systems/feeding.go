package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/traits"
)

const (
	feedingDistance = 8.0  // Distance to consume food
	baseBiteSize    = 0.05 // Fraction of target energy per bite
	feedEfficiency  = 0.8  // Energy transfer efficiency
	floraDamageRate = 0.015 // Decomposition added to flora cell when eaten

	// Default flora armor (flora don't have cells with armor values)
	defaultFloraArmor = 0.1
)

// biteSizeMultiplier returns how much food an organism can consume per tick.
// Based on mouth cell strength and organism size.
func biteSizeMultiplier(org *components.Organism, mouthSize float32) float32 {
	// Derive cell count from MaxEnergy
	cellCount := (org.MaxEnergy - 100) / 50
	if cellCount < 1 {
		cellCount = 1
	}
	// Base bite scales with sqrt of cell count
	sizeMultiplier := float32(math.Sqrt(float64(cellCount)))

	// Mouth size affects bite capability (0 = can't bite, 1 = full bite)
	// Minimum 0.1 to allow some feeding even without dedicated mouth cells
	mouthMultiplier := float32(0.1)
	if mouthSize > 0.1 {
		mouthMultiplier = mouthSize
	}

	return sizeMultiplier * mouthMultiplier
}

// FeedingSystem handles fauna consuming food sources using capability matching.
type FeedingSystem struct {
	faunaFilter ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	neuralMap   *ecs.Map[components.NeuralGenome]
	floraSystem *FloraSystem // Lightweight flora system (set via SetFloraSystem)
}

// NewFeedingSystem creates a new feeding system.
func NewFeedingSystem(w *ecs.World) *FeedingSystem {
	return &FeedingSystem{
		faunaFilter: *ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](w),
		neuralMap:   ecs.NewMap[components.NeuralGenome](w),
	}
}

// SetFloraSystem sets the flora system reference for feeding queries.
func (s *FeedingSystem) SetFloraSystem(fs *FloraSystem) {
	s.floraSystem = fs
}

// entityData holds data needed for capability matching.
type entityData struct {
	pos         *components.Position
	org         *components.Organism
	cells       *components.CellBuffer
	caps        components.Capabilities
	speciesID   int
	isFlora     bool
	floraRef    FloraRef // Reference to lightweight flora (only valid if isFlora=true)
}

// Update processes feeding for all fauna using capability matching.
func (s *FeedingSystem) Update() {
	// Collect all potential food sources with their capabilities
	var targets []entityData

	// Collect flora from FloraSystem
	if s.floraSystem != nil {
		allFlora := s.floraSystem.GetAllFlora()
		for _, ref := range allFlora {
			// Flora have fixed composition (photo=1, actuator=0)
			caps := components.Capabilities{
				PhotoWeight:     1.0,
				ActuatorWeight:  0.0,
				StructuralArmor: DefaultFloraArmor(),
			}
			targets = append(targets, entityData{
				pos:         &components.Position{X: ref.X, Y: ref.Y},
				org:         &components.Organism{Energy: ref.Energy, MaxEnergy: 150, Dead: false},
				cells:       nil, // Flora don't have cells in lightweight system
				caps:        caps,
				isFlora:     true,
				floraRef:    ref,
			})
		}
	}

	// Collect fauna with their computed capabilities
	faunaQuery := s.faunaFilter.Query()
	for faunaQuery.Next() {
		entity := faunaQuery.Entity()
		pos, _, org, cells := faunaQuery.Get()

		caps := cells.ComputeCapabilities()

		speciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}

		targets = append(targets, entityData{
			pos:       pos,
			org:       org,
			cells:     cells,
			caps:      caps,
			speciesID: speciesID,
			isFlora:   false,
		})
	}

	// Process feeding for each fauna
	faunaQuery2 := s.faunaFilter.Query()
	for faunaQuery2.Next() {
		entity := faunaQuery2.Entity()
		pos, _, org, cells := faunaQuery2.Get()

		if org.Dead {
			continue
		}

		// Skip flora (they don't actively feed)
		if traits.IsFlora(org.Traits) {
			continue
		}

		// Check eat intent from brain (>0.5 means try to eat)
		if org.EatIntent < 0.5 {
			continue
		}

		// Compute this organism's capabilities
		myCaps := cells.ComputeCapabilities()
		myDigestive := myCaps.DigestiveSpectrum()

		// Get species ID for kin avoidance
		mySpeciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				mySpeciesID = ng.SpeciesID
			}
		}

		// Try to feed on the best available target
		s.tryFeed(pos, org, cells, myCaps, myDigestive, mySpeciesID, targets)
	}
}

// tryFeed attempts to feed on nearby targets using capability matching.
func (s *FeedingSystem) tryFeed(
	pos *components.Position,
	org *components.Organism,
	cells *components.CellBuffer,
	myCaps components.Capabilities,
	myDigestive float32,
	mySpeciesID int,
	targets []entityData,
) {
	feedDistSq := float32(feedingDistance * feedingDistance)

	var bestTarget *entityData
	var bestPenetration float32
	var bestDistSq float32 = feedDistSq + 1

	for i := range targets {
		target := &targets[i]

		// Skip self
		if target.org == org {
			continue
		}

		// Calculate distance
		dx := pos.X - target.pos.X
		dy := pos.Y - target.pos.Y
		distSq := dx*dx + dy*dy

		if distSq > feedDistSq {
			continue
		}

		// Calculate capability match
		targetComposition := target.caps.Composition()
		targetArmor := target.caps.StructuralArmor

		edibility := neural.Edibility(myDigestive, targetComposition)
		penetration := neural.Penetration(edibility, targetArmor)

		// Can't feed if penetration is 0
		if penetration <= 0 {
			continue
		}

		// For living fauna targets, apply kin avoidance
		if !target.isFlora && !target.org.Dead && mySpeciesID > 0 && target.speciesID == mySpeciesID {
			// 70% chance to avoid hunting your own species
			if rand.Float32() < 0.70 {
				continue
			}
		}

		// Prefer targets with higher penetration, then closer distance
		if penetration > bestPenetration || (penetration == bestPenetration && distSq < bestDistSq) {
			bestTarget = target
			bestPenetration = penetration
			bestDistSq = distSq
		}
	}

	if bestTarget == nil {
		return
	}

	// Execute feeding
	s.executeFeed(org, cells, myCaps, bestTarget, bestPenetration)
}

// executeFeed performs the actual energy transfer.
func (s *FeedingSystem) executeFeed(
	predOrg *components.Organism,
	_ *components.CellBuffer, // predCells - not used in current implementation
	predCaps components.Capabilities,
	target *entityData,
	penetration float32,
) {
	// Calculate bite amount
	biteMultiplier := biteSizeMultiplier(predOrg, predCaps.MouthSize)

	// Base bite is fraction of target's current energy
	baseBite := target.org.Energy * baseBiteSize * biteMultiplier

	// Penetration affects how much we can actually extract
	// Higher penetration = more efficient feeding
	effectiveBite := baseBite * penetration * feedEfficiency

	// Can only eat what target has
	if effectiveBite > target.org.Energy {
		effectiveBite = target.org.Energy
	}

	// Handle lightweight flora differently
	if target.isFlora && s.floraSystem != nil {
		// Apply damage through FloraSystem
		extracted := s.floraSystem.ApplyDamage(target.floraRef.Index, target.floraRef.IsRooted, effectiveBite)
		predOrg.Energy += extracted
		return
	}

	// Transfer energy (for fauna targets)
	predOrg.Energy += effectiveBite
	target.org.Energy -= effectiveBite

	// Damage target cells (fauna only - flora don't have cells)
	if target.cells != nil && target.cells.Count > 0 {
		// Find first alive cell to damage
		for i := uint8(0); i < target.cells.Count; i++ {
			if target.cells.Cells[i].Alive {
				target.cells.Cells[i].Decomposition += floraDamageRate
				break
			}
		}
	}

	// Kill target if energy depleted
	if target.org.Energy <= 0 {
		target.org.Dead = true
	}
}

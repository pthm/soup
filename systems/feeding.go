package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

const (
	feedingDistance = 8.0  // Distance to consume food
	baseBiteSize    = 0.08 // Fraction of target energy per bite (increased for simpler system)
	feedEfficiency  = 0.85 // Energy transfer efficiency

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
	spatialGrid *SpatialGrid // Spatial grid for O(1) lookups (set via SetSpatialGrid)
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

// SetSpatialGrid sets the spatial grid for O(1) neighbor lookups.
func (s *FeedingSystem) SetSpatialGrid(grid *SpatialGrid) {
	s.spatialGrid = grid
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
// Uses spatial grid for O(1) neighbor lookups when available.
func (s *FeedingSystem) Update() {
	// Use optimized spatial version if grid is available
	if s.spatialGrid != nil {
		s.updateWithSpatialGrid()
		return
	}

	// Fallback to O(n²) version
	s.updateLegacy()
}

// updateWithSpatialGrid uses spatial queries for efficient neighbor lookup.
func (s *FeedingSystem) updateWithSpatialGrid() {
	// Pre-collect fauna data for spatial lookups
	var faunaPos []components.Position
	var faunaOrgs []*components.Organism
	var faunaCells []*components.CellBuffer
	var faunaSpecies []int

	faunaQuery := s.faunaFilter.Query()
	for faunaQuery.Next() {
		entity := faunaQuery.Entity()
		pos, _, org, cells := faunaQuery.Get()

		faunaPos = append(faunaPos, *pos)
		faunaOrgs = append(faunaOrgs, org)
		faunaCells = append(faunaCells, cells)

		speciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}
		faunaSpecies = append(faunaSpecies, speciesID)
	}

	// Process each fauna's feeding with spatial lookup
	for i := range faunaOrgs {
		org := faunaOrgs[i]
		if org.Dead || org.EatIntent < 0.5 {
			continue
		}

		pos := &faunaPos[i]
		cells := faunaCells[i]
		myCaps := cells.ComputeCapabilities()
		mySpeciesID := faunaSpecies[i]
		myDigestive := myCaps.DigestiveSpectrum()

		s.tryFeedSpatial(pos, org, myCaps, myDigestive, mySpeciesID, i, faunaPos, faunaOrgs, faunaCells, faunaSpecies)
	}
}

// tryFeedSpatial finds and eats nearby targets using spatial grid.
func (s *FeedingSystem) tryFeedSpatial(
	pos *components.Position,
	org *components.Organism,
	myCaps components.Capabilities,
	myDigestive float32,
	mySpeciesID int,
	myIdx int,
	faunaPos []components.Position,
	faunaOrgs []*components.Organism,
	faunaCells []*components.CellBuffer,
	faunaSpecies []int,
) {
	const feedDistSq = feedingDistance * feedingDistance
	const kinAvoidanceProb = 0.92 // High kin avoidance to reduce cannibalism

	var bestTarget *entityData
	var bestPenetration float32
	var bestDistSq float32 = feedDistSq + 1

	// Check nearby flora using FloraSystem's spatial query
	if s.floraSystem != nil {
		nearbyFlora := s.floraSystem.GetNearbyFlora(pos.X, pos.Y, feedingDistance)
		for _, ref := range nearbyFlora {
			dSq := distanceSq(pos.X, pos.Y, ref.X, ref.Y)
			if dSq > feedDistSq {
				continue
			}

			// Flora edibility
			edibility := neural.Edibility(myDigestive, 1.0) // Flora composition = 1.0
			penetration := neural.Penetration(edibility, DefaultFloraArmor())

			if penetration <= 0 {
				continue
			}

			if penetration > bestPenetration || (penetration == bestPenetration && dSq < bestDistSq) {
				caps := components.Capabilities{
					PhotoWeight:     1.0,
					ActuatorWeight:  0.0,
					StructuralArmor: DefaultFloraArmor(),
				}
				bestTarget = &entityData{
					pos:      &components.Position{X: ref.X, Y: ref.Y},
					org:      &components.Organism{Energy: ref.Energy, MaxEnergy: 150, Dead: false},
					caps:     caps,
					isFlora:  true,
					floraRef: ref,
				}
				bestPenetration = penetration
				bestDistSq = dSq
			}
		}
	}

	// Check nearby fauna using spatial grid
	nearbyFauna := s.spatialGrid.GetNearbyFauna(pos.X, pos.Y, feedingDistance)
	for _, idx := range nearbyFauna {
		if idx == myIdx || faunaOrgs[idx].Dead {
			continue
		}

		targetPos := &faunaPos[idx]
		dSq := distanceSq(pos.X, pos.Y, targetPos.X, targetPos.Y)
		if dSq > feedDistSq {
			continue
		}

		targetOrg := faunaOrgs[idx]
		targetCells := faunaCells[idx]
		targetCaps := targetCells.ComputeCapabilities()

		edibility := neural.Edibility(myDigestive, targetCaps.Composition())
		penetration := neural.Penetration(edibility, targetCaps.StructuralArmor)

		if penetration <= 0 {
			continue
		}

		// Kin avoidance
		targetSpeciesID := faunaSpecies[idx]
		isKin := mySpeciesID > 0 && targetSpeciesID == mySpeciesID
		if isKin && rand.Float32() < kinAvoidanceProb {
			continue
		}

		if penetration > bestPenetration || (penetration == bestPenetration && dSq < bestDistSq) {
			bestTarget = &entityData{
				pos:       targetPos,
				org:       targetOrg,
				cells:     targetCells,
				caps:      targetCaps,
				speciesID: targetSpeciesID,
				isFlora:   false,
			}
			bestPenetration = penetration
			bestDistSq = dSq
		}
	}

	if bestTarget == nil {
		return
	}

	s.executeFeed(org, myCaps, bestTarget, bestPenetration)
}

// updateLegacy is the original O(n²) implementation for when spatial grid is unavailable.
func (s *FeedingSystem) updateLegacy() {
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

		if org.Dead || org.EatIntent < 0.5 {
			continue
		}

		myCaps := cells.ComputeCapabilities()

		mySpeciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				mySpeciesID = ng.SpeciesID
			}
		}

		s.tryFeed(pos, org, myCaps, myCaps.DigestiveSpectrum(), mySpeciesID, targets)
	}
}

// tryFeed attempts to feed on nearby targets using capability matching.
func (s *FeedingSystem) tryFeed(
	pos *components.Position,
	org *components.Organism,
	myCaps components.Capabilities,
	myDigestive float32,
	mySpeciesID int,
	targets []entityData,
) {
	const feedDistSq = feedingDistance * feedingDistance
	const kinAvoidanceProb = 0.92 // High kin avoidance to reduce cannibalism // Probability of avoiding hunting own species

	var bestTarget *entityData
	var bestPenetration float32
	var bestDistSq float32 = feedDistSq + 1

	for i := range targets {
		target := &targets[i]

		// Skip self
		if target.org == org {
			continue
		}

		// Check distance
		dSq := distanceSq(pos.X, pos.Y, target.pos.X, target.pos.Y)
		if dSq > feedDistSq {
			continue
		}

		// Calculate capability match
		edibility := neural.Edibility(myDigestive, target.caps.Composition())
		penetration := neural.Penetration(edibility, target.caps.StructuralArmor)

		if penetration <= 0 {
			continue
		}

		// Kin avoidance: avoid hunting your own species
		isKin := !target.isFlora && !target.org.Dead && mySpeciesID > 0 && target.speciesID == mySpeciesID
		if isKin && rand.Float32() < kinAvoidanceProb {
			continue
		}

		// Prefer targets with higher penetration, then closer distance
		if penetration > bestPenetration || (penetration == bestPenetration && dSq < bestDistSq) {
			bestTarget = target
			bestPenetration = penetration
			bestDistSq = dSq
		}
	}

	if bestTarget == nil {
		return
	}

	s.executeFeed(org, myCaps, bestTarget, bestPenetration)
}

// executeFeed performs the actual energy transfer.
func (s *FeedingSystem) executeFeed(
	predOrg *components.Organism,
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

	// Transfer energy (for fauna targets) - simple energy transfer, no cell damage
	predOrg.Energy += effectiveBite
	target.org.Energy -= effectiveBite

	// Signal to target that it's being eaten (for brain awareness)
	// Scale by how much damage was done relative to max energy
	damageIntensity := effectiveBite / target.org.MaxEnergy
	if damageIntensity > target.org.BeingEaten {
		target.org.BeingEaten = damageIntensity
	}

	// Kill target if energy depleted
	if target.org.Energy <= 0 {
		target.org.Dead = true
	}
}

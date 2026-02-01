package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

const (
	feedingDistance = 8.0  // Distance to consume food (herbivory)
	baseBiteSize    = 0.08 // Fraction of target energy per bite (increased for simpler system)
	feedEfficiency  = 0.85 // Energy transfer efficiency

	// Default flora armor (flora don't have cells with armor values)
	defaultFloraArmor = 0.1

	// Attack parameters (predation requires explicit AttackIntent from brain)
	attackIntentThreshold = 0.5 // Brain output threshold for attacking

	// Predator interference parameters
	crowdPenaltyStart    = 2   // Number of attackers before penalty applies
	crowdPenaltyPerExtra = 0.2 // Penalty per additional attacker beyond threshold
	maxCrowdPenalty      = 0.7 // Maximum penalty (30% minimum reward)
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

// attackRange returns the effective attack range based on body capabilities.
func attackRange(caps components.Capabilities, cellSize float32) float32 {
	return neural.BaseAttackRange * (0.5 + caps.MouthSize)
}

// attackDamage returns the damage multiplier for attacks.
func attackDamage(caps components.Capabilities) float32 {
	return neural.BaseAttackDamage * (0.5 + caps.MouthSize)
}

// attackCost returns the energy cost for attacking.
func attackCost(caps components.Capabilities) float32 {
	return neural.BaseAttackCost * (0.5 + caps.ActuatorWeight*0.1)
}

// canAttack checks if an organism can attack (has intent and no cooldown).
func canAttack(org *components.Organism) bool {
	return org.AttackIntent > attackIntentThreshold && org.AttackCooldown == 0
}

// wantsToEatFlora checks if an organism wants to eat flora (implicit herbivory).
// Herbivory is automatic when near flora and organism is a herbivore.
func wantsToEatFlora(org *components.Organism, digestiveSpectrum float32) bool {
	// Herbivores (low digestive spectrum) automatically try to eat flora
	// This is implicit - no brain output required
	return digestiveSpectrum < 0.7 // Not pure carnivore
}

// FeedingSystem handles fauna consuming food sources using capability matching.
type FeedingSystem struct {
	faunaFilter ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	neuralMap   *ecs.Map[components.NeuralGenome]
	floraSystem *FloraSystem // Lightweight flora system (set via SetFloraSystem)
	spatialGrid *SpatialGrid // Spatial grid for O(1) lookups (set via SetSpatialGrid)
	bounds      Bounds       // World bounds for toroidal distance calculations
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

// SetBounds sets the world bounds for toroidal distance calculations.
func (s *FeedingSystem) SetBounds(bounds Bounds) {
	s.bounds = bounds
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

// pendingAttack represents an attack that will be resolved with interference rules.
type pendingAttack struct {
	predIdx     int                    // Index of predator in fauna arrays
	predOrg     *components.Organism   // Predator organism
	predCaps    components.Capabilities // Predator capabilities
	targetIdx   int                    // Index of target in fauna arrays
	target      *entityData            // Target data
	penetration float32                // Penetration value for this attack
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
// Uses two-pass approach for predation to implement interference rules.
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

	// Collect pending attacks for interference resolution
	var pendingAttacks []pendingAttack

	// Pass 1: Process herbivory (immediate) and collect predation attacks (deferred)
	for i := range faunaOrgs {
		org := faunaOrgs[i]

		// Decay attack cooldown
		if org.AttackCooldown > 0 {
			org.AttackCooldown--
		}

		if org.Dead {
			continue
		}

		pos := &faunaPos[i]
		cells := faunaCells[i]
		myCaps := cells.ComputeCapabilities()
		mySpeciesID := faunaSpecies[i]
		myDigestive := myCaps.DigestiveSpectrum()

		// Collect attacks (predation deferred for interference rules)
		attacks := s.collectFeedingSpatial(pos, org, myCaps, myDigestive, mySpeciesID, i, faunaPos, faunaOrgs, faunaCells, faunaSpecies)
		pendingAttacks = append(pendingAttacks, attacks...)
	}

	// Pass 2: Resolve predation attacks with interference rules
	s.resolveAttacksWithInterference(pendingAttacks)
}

// collectFeedingSpatial finds nearby targets using spatial grid.
// Executes herbivory immediately, returns pending attacks for interference resolution.
func (s *FeedingSystem) collectFeedingSpatial(
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
) []pendingAttack {
	const feedDistSq = feedingDistance * feedingDistance
	const kinAvoidanceProb = 0.92 // High kin avoidance to reduce cannibalism

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	var bestFloraTarget *entityData
	var bestFaunaTarget *entityData
	var bestFloraPenetration float32
	var bestFaunaPenetration float32
	var bestFloraDistSq float32 = feedDistSq + 1
	var bestFaunaDistSq float32
	var bestFaunaIdx int

	// Determine attack range for predation
	atkRange := attackRange(myCaps, org.CellSize)
	atkRangeSq := atkRange * atkRange
	bestFaunaDistSq = atkRangeSq + 1

	// Check nearby flora using FloraSystem's spatial query (implicit herbivory)
	if s.floraSystem != nil && wantsToEatFlora(org, myDigestive) {
		nearbyFlora := s.floraSystem.GetNearbyFlora(centerX, centerY, feedingDistance)
		for _, ref := range nearbyFlora {
			dSq := ToroidalDistanceSq(centerX, centerY, ref.X, ref.Y, s.bounds.Width, s.bounds.Height)
			if dSq > feedDistSq {
				continue
			}

			// Flora edibility
			edibility := neural.Edibility(myDigestive, 1.0) // Flora composition = 1.0
			penetration := neural.Penetration(edibility, DefaultFloraArmor())

			if penetration <= 0 {
				continue
			}

			if penetration > bestFloraPenetration || (penetration == bestFloraPenetration && dSq < bestFloraDistSq) {
				caps := components.Capabilities{
					SensorWeight:    1.0,
					ActuatorWeight:  0.0,
					StructuralArmor: DefaultFloraArmor(),
				}
				bestFloraTarget = &entityData{
					pos:      &components.Position{X: ref.X, Y: ref.Y},
					org:      &components.Organism{Energy: ref.Energy, MaxEnergy: 150, Dead: false},
					caps:     caps,
					isFlora:  true,
					floraRef: ref,
				}
				bestFloraPenetration = penetration
				bestFloraDistSq = dSq
			}
		}
	}

	// Check nearby fauna using spatial grid (explicit predation - requires AttackIntent)
	if canAttack(org) {
		nearbyFauna := s.spatialGrid.GetNearbyFauna(centerX, centerY, atkRange)
		for _, idx := range nearbyFauna {
			if idx == myIdx || faunaOrgs[idx].Dead {
				continue
			}

			targetPos := &faunaPos[idx]
			dSq := ToroidalDistanceSq(centerX, centerY, targetPos.X, targetPos.Y, s.bounds.Width, s.bounds.Height)
			if dSq > atkRangeSq {
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

			if penetration > bestFaunaPenetration || (penetration == bestFaunaPenetration && dSq < bestFaunaDistSq) {
				bestFaunaTarget = &entityData{
					pos:       targetPos,
					org:       targetOrg,
					cells:     targetCells,
					caps:      targetCaps,
					speciesID: targetSpeciesID,
					isFlora:   false,
				}
				bestFaunaPenetration = penetration
				bestFaunaDistSq = dSq
				bestFaunaIdx = idx
			}
		}
	}

	// Execute flora feeding immediately (herbivory - no interference)
	if bestFloraTarget != nil {
		s.executeFeed(org, myCaps, bestFloraTarget, bestFloraPenetration)
	}

	// Return pending attack for interference resolution (predation)
	var attacks []pendingAttack
	if bestFaunaTarget != nil {
		// Check if predator has enough energy for attack cost
		cost := attackCost(myCaps)
		if org.Energy > cost {
			attacks = append(attacks, pendingAttack{
				predIdx:     myIdx,
				predOrg:     org,
				predCaps:    myCaps,
				targetIdx:   bestFaunaIdx,
				target:      bestFaunaTarget,
				penetration: bestFaunaPenetration,
			})
		}
	}

	return attacks
}

// resolveAttacksWithInterference processes all attacks with crowd penalty.
// Multiple predators attacking the same target share rewards and suffer penalties.
func (s *FeedingSystem) resolveAttacksWithInterference(attacks []pendingAttack) {
	if len(attacks) == 0 {
		return
	}

	// Group attacks by target
	attacksByTarget := make(map[int][]pendingAttack)
	for _, atk := range attacks {
		attacksByTarget[atk.targetIdx] = append(attacksByTarget[atk.targetIdx], atk)
	}

	// Process each target's attackers
	for _, targetAttacks := range attacksByTarget {
		if len(targetAttacks) == 0 {
			continue
		}

		target := targetAttacks[0].target
		if target.org.Dead {
			continue
		}

		numAttackers := len(targetAttacks)

		// Calculate crowd penalty
		crowdPenalty := float32(0.0)
		if numAttackers > crowdPenaltyStart {
			extraAttackers := numAttackers - crowdPenaltyStart
			crowdPenalty = float32(extraAttackers) * crowdPenaltyPerExtra
			if crowdPenalty > maxCrowdPenalty {
				crowdPenalty = maxCrowdPenalty
			}
		}
		crowdMultiplier := 1.0 - crowdPenalty

		// Calculate total damage from all attackers
		totalDamage := float32(0.0)
		for _, atk := range targetAttacks {
			damage := attackDamage(atk.predCaps) * atk.penetration * target.org.MaxEnergy
			totalDamage += damage
		}

		// Cap total damage at target's energy
		if totalDamage > target.org.Energy {
			totalDamage = target.org.Energy
		}

		// Distribute rewards proportionally to each attacker's contribution
		for _, atk := range targetAttacks {
			// Apply attack cost and cooldown
			cost := attackCost(atk.predCaps)
			atk.predOrg.Energy -= cost
			atk.predOrg.AttackCooldown = neural.AttackCooldown

			// Calculate this attacker's share of damage
			attackerDamage := attackDamage(atk.predCaps) * atk.penetration * target.org.MaxEnergy
			damageShare := attackerDamage / totalDamage
			actualDamage := totalDamage * damageShare / float32(numAttackers)

			// Apply compat^k power law for nutrition
			nutritionMult := neural.NutritionMultiplier(atk.penetration)

			// Transfer energy with crowd penalty and split
			reward := actualDamage * nutritionMult * feedEfficiency * crowdMultiplier
			atk.predOrg.Energy += reward
		}

		// Apply total damage to target
		target.org.Energy -= totalDamage

		// Signal to target that it's being attacked
		damageIntensity := totalDamage / target.org.MaxEnergy
		if damageIntensity > target.org.BeingEaten {
			target.org.BeingEaten = damageIntensity
		}

		// Kill target if energy depleted
		if target.org.Energy <= 0 {
			target.org.Dead = true
		}
	}
}

// updateLegacy is the original O(n²) implementation for when spatial grid is unavailable.
func (s *FeedingSystem) updateLegacy() {
	// Collect all potential food sources with their capabilities
	var targets []entityData

	// Collect flora from FloraSystem
	if s.floraSystem != nil {
		allFlora := s.floraSystem.GetAllFlora()
		for _, ref := range allFlora {
			// Flora have fixed composition (mostly sensor-like, minimal actuator)
			caps := components.Capabilities{
				SensorWeight:    1.0,
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

	// Calculate organism center using OBB offset (offset is in local space, must be rotated)
	cosH := float32(math.Cos(float64(org.Heading)))
	sinH := float32(math.Sin(float64(org.Heading)))
	centerX := pos.X + org.OBB.OffsetX*cosH - org.OBB.OffsetY*sinH
	centerY := pos.Y + org.OBB.OffsetX*sinH + org.OBB.OffsetY*cosH

	var bestTarget *entityData
	var bestPenetration float32
	var bestDistSq float32 = feedDistSq + 1

	for i := range targets {
		target := &targets[i]

		// Skip self
		if target.org == org {
			continue
		}

		// Check distance from center using toroidal geometry
		dSq := ToroidalDistanceSq(centerX, centerY, target.pos.X, target.pos.Y, s.bounds.Width, s.bounds.Height)
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

// executeFeed performs the actual energy transfer for herbivory.
// Uses compat^k power law for nutrition rewards to create sharper dietary niches.
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

	// Apply compat^k power law for nutrition efficiency
	// This creates sharper dietary niches - specialists get much better returns
	nutritionMult := neural.NutritionMultiplier(penetration)

	// Effective bite uses power-law nutrition multiplier
	effectiveBite := baseBite * nutritionMult * feedEfficiency

	// Can only eat what target has
	if effectiveBite > target.org.Energy {
		effectiveBite = target.org.Energy
	}

	// Handle lightweight flora differently
	if target.isFlora && s.floraSystem != nil {
		// Apply damage through FloraSystem
		extracted := s.floraSystem.ApplyDamage(target.floraRef.Index, effectiveBite)
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

// executeAttack performs predation with body-scaled damage.
// Uses compat^k power law for nutrition rewards to create sharper dietary niches.
func (s *FeedingSystem) executeAttack(
	predOrg *components.Organism,
	predCaps components.Capabilities,
	target *entityData,
	penetration float32,
) {
	// Body-scaled damage (penetration affects how much damage we deal)
	damage := attackDamage(predCaps) * penetration * target.org.MaxEnergy

	// Can only take what target has
	if damage > target.org.Energy {
		damage = target.org.Energy
	}

	// Apply compat^k power law for nutrition efficiency
	// Specialists get much better energy returns from prey
	nutritionMult := neural.NutritionMultiplier(penetration)

	// Transfer energy with power-law nutrition bonus
	predOrg.Energy += damage * nutritionMult * feedEfficiency
	target.org.Energy -= damage

	// Signal to target that it's being attacked
	damageIntensity := damage / target.org.MaxEnergy
	if damageIntensity > target.org.BeingEaten {
		target.org.BeingEaten = damageIntensity
	}

	// Kill target if energy depleted
	if target.org.Energy <= 0 {
		target.org.Dead = true
	}
}

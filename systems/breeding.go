package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// BreedingSystem handles fauna sexual reproduction.
type BreedingSystem struct {
	filter ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
}

// NewBreedingSystem creates a new breeding system.
func NewBreedingSystem(w *ecs.World) *BreedingSystem {
	return &BreedingSystem{
		filter: *ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](w),
	}
}

// Update processes breeding for all eligible fauna.
func (s *BreedingSystem) Update(w *ecs.World, createOrganism func(x, y float32, t traits.Trait, energy float32) ecs.Entity) {
	// Collect all potential breeders
	type breeder struct {
		entity ecs.Entity
		pos    *components.Position
		vel    *components.Velocity
		org    *components.Organism
		cells  *components.CellBuffer
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

		breeders = append(breeders, breeder{
			entity: query.Entity(),
			pos:    pos,
			vel:    vel,
			org:    org,
			cells:  cells,
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

			if s.isCompatible(a.org, b.org, a.pos, b.pos) {
				s.breed(a.pos, b.pos, a.org, b.org, createOrganism)
				bred[a.entity] = true
				bred[b.entity] = true
				break
			}
		}
	}
}

func (s *BreedingSystem) isEligible(org *components.Organism, cells *components.CellBuffer) bool {
	// Must have Breeding trait
	if !org.Traits.Has(traits.Breeding) {
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

func (s *BreedingSystem) isCompatible(a, b *components.Organism, posA, posB *components.Position) bool {
	// Must have opposite genders
	aIsMale := a.Traits.Has(traits.Male)
	bIsMale := b.Traits.Has(traits.Male)
	if aIsMale == bIsMale {
		return false
	}

	// Must be within proximity (50 units, increased from 30)
	dx := posA.X - posB.X
	dy := posA.Y - posB.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist > 50 {
		return false
	}

	// No trait requirement - just need to be same species type
	// (both herbivores, both carnivores, etc.)
	_ = countSharedTraits(a.Traits, b.Traits) // Keep for potential future use

	return true
}

func countSharedTraits(a, b traits.Trait) int {
	// Traits to check (excluding gender and breeding)
	checkTraits := []traits.Trait{
		traits.Herbivore,
		traits.Carnivore,
		traits.Carrion,
		traits.Herding,
		traits.PredatorEyes,
		traits.PreyEyes,
		traits.FarSight,
		traits.Speed,
	}

	count := 0
	for _, t := range checkTraits {
		if a.Has(t) && b.Has(t) {
			count++
		}
	}
	return count
}

func (s *BreedingSystem) breed(posA, posB *components.Position, orgA, orgB *components.Organism, createOrganism func(x, y float32, t traits.Trait, energy float32) ecs.Entity) {
	// Position: midpoint between parents
	x := (posA.X + posB.X) / 2
	y := (posA.Y + posB.Y) / 2

	// Build offspring traits
	offspringTraits := s.inheritTraits(orgA.Traits, orgB.Traits)

	// Create offspring with 50 energy
	createOrganism(x, y, offspringTraits, 50)

	// Cost to parents
	orgA.Energy -= 20
	orgB.Energy -= 20

	// Set cooldowns
	orgA.BreedingCooldown = 180
	orgB.BreedingCooldown = 180
}

func (s *BreedingSystem) inheritTraits(a, b traits.Trait) traits.Trait {
	var result traits.Trait

	// Inheritable traits (50% from each parent)
	inheritableTraits := []traits.Trait{
		traits.Herbivore,
		traits.Carnivore,
		traits.Carrion,
		traits.Herding,
		traits.PredatorEyes,
		traits.PreyEyes,
		traits.FarSight,
		traits.Speed,
	}

	for _, t := range inheritableTraits {
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

	// Always gets Breeding trait (gender will be assigned in createOrganism)
	result = result.Add(traits.Breeding)

	return result
}

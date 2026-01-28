package systems

import (
	"math"
	"math/rand"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

const (
	feedingDistance    = 6.0   // Distance to consume food (scaled for smaller cells)
	herbivoreEatAmount = 1.5   // Base energy gained per tick while eating flora
	carnivoreEatAmount = 2.5   // Base energy gained per tick while eating fauna
	carrionEatAmount   = 4.0   // Base energy gained per tick while eating dead
	floraDamageRate    = 0.008 // Decomposition added to flora cell when eaten

	// Social hunting constants
	herdRadius = 30.0 // How close allies must be to count as herd/pack
)

// biteSizeMultiplier returns how much food an organism can consume per tick.
// Larger organisms take bigger bites - they have bigger mouths/bodies.
// Uses MaxEnergy as proxy for size (MaxEnergy = 100 + cellCount*50).
func biteSizeMultiplier(org *components.Organism) float32 {
	// Derive cell count from MaxEnergy
	cellCount := (org.MaxEnergy - 100) / 50
	if cellCount < 1 {
		cellCount = 1
	}
	// Bite size scales with sqrt of cell count:
	// 1-cell: 1.0x, 4-cell: 2.0x, 9-cell: 3.0x, 16-cell: 4.0x, 25-cell: 5.0x
	return float32(math.Sqrt(float64(cellCount)))
}

// FeedingSystem handles fauna consuming food sources.
type FeedingSystem struct {
	faunaFilter ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	floraFilter ecs.Filter4[components.Position, components.Organism, components.CellBuffer, components.Flora]
	neuralMap   *ecs.Map[components.NeuralGenome]
}

// NewFeedingSystem creates a new feeding system.
func NewFeedingSystem(w *ecs.World) *FeedingSystem {
	return &FeedingSystem{
		faunaFilter: *ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](w),
		floraFilter: *ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Flora](w),
		neuralMap:   ecs.NewMap[components.NeuralGenome](w),
	}
}

// Update processes feeding for all fauna.
func (s *FeedingSystem) Update() {
	// Collect flora (both alive for herbivores and dead for carrion)
	var floraList []floraData

	floraQuery := s.floraFilter.Query()
	for floraQuery.Next() {
		pos, org, cells, _ := floraQuery.Get()
		floraList = append(floraList, floraData{pos, org, cells})
	}

	// Collect fauna for carnivores and carrion eaters
	var faunaList []faunaData

	// First pass: collect all fauna with species info
	faunaQuery := s.faunaFilter.Query()
	for faunaQuery.Next() {
		entity := faunaQuery.Entity()
		pos, _, org, cells := faunaQuery.Get()

		// Get species ID for kin recognition
		speciesID := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				speciesID = ng.SpeciesID
			}
		}

		faunaList = append(faunaList, faunaData{pos, org, cells, speciesID})
	}

	// Second pass: process feeding
	faunaQuery2 := s.faunaFilter.Query()
	for faunaQuery2.Next() {
		entity := faunaQuery2.Entity()
		pos, _, org, _ := faunaQuery2.Get()

		if org.Dead {
			continue
		}

		// Skip flora (they don't eat)
		if traits.IsFlora(org.Traits) {
			continue
		}

		// Get predator's species ID
		predatorSpecies := 0
		if s.neuralMap.Has(entity) {
			if ng := s.neuralMap.Get(entity); ng != nil {
				predatorSpecies = ng.SpeciesID
			}
		}

		// Herbivores eat flora
		if org.Traits.Has(traits.Herbivore) {
			s.tryEatFlora(pos, org, floraList)
		}

		// Carnivores eat other fauna (alive) - with species discrimination
		if org.Traits.Has(traits.Carnivore) {
			s.tryEatFauna(pos, org, faunaList, false, predatorSpecies)
		}

		// Carrion eaters eat dead fauna and dead flora
		// Note: carrion eating ignores species - scavenging dead kin is allowed
		if org.Traits.Has(traits.Carrion) {
			s.tryEatFauna(pos, org, faunaList, true, 0) // 0 = ignore species check
			s.tryEatDeadFlora(pos, org, floraList)
		}
	}
}

type floraData struct {
	pos   *components.Position
	org   *components.Organism
	cells *components.CellBuffer
}

type faunaData struct {
	pos       *components.Position
	org       *components.Organism
	cells     *components.CellBuffer
	speciesID int // For kin recognition (avoid cannibalism)
}

// countHerdDefenders returns how many nearby allies can help defend this prey.
// Only counts living herbivores with the Herding trait within herd radius.
func countHerdDefenders(preyPos *components.Position, preyOrg *components.Organism, faunaList []faunaData) int {
	if !preyOrg.Traits.Has(traits.Herding) {
		return 0 // Non-herding prey get no defense bonus
	}

	count := 0
	for _, other := range faunaList {
		if other.org == preyOrg || other.org.Dead {
			continue
		}
		if !other.org.Traits.Has(traits.Herding) {
			continue
		}
		// Must be same type (herbivore with herbivore)
		if !other.org.Traits.Has(traits.Herbivore) {
			continue
		}

		dx := preyPos.X - other.pos.X
		dy := preyPos.Y - other.pos.Y
		distSq := dx*dx + dy*dy
		if distSq <= herdRadius*herdRadius {
			count++
		}
	}
	return count
}

// countPackHunters returns how many nearby predators are also attacking this prey.
// Only counts living carnivores with the Herding trait within herd radius of the prey.
func countPackHunters(preyPos *components.Position, predOrg *components.Organism, faunaList []faunaData) int {
	if !predOrg.Traits.Has(traits.Herding) {
		return 0 // Lone wolves hunt alone
	}

	count := 0
	for _, other := range faunaList {
		if other.org == predOrg || other.org.Dead {
			continue
		}
		if !other.org.Traits.Has(traits.Herding) || !other.org.Traits.Has(traits.Carnivore) {
			continue
		}

		// Check if this predator is also near the prey (could join the hunt)
		dx := preyPos.X - other.pos.X
		dy := preyPos.Y - other.pos.Y
		distSq := dx*dx + dy*dy
		if distSq <= herdRadius*herdRadius {
			count++
		}
	}
	return count
}

func (s *FeedingSystem) tryEatFlora(predPos *components.Position, predOrg *components.Organism, floraList []floraData) {
	for _, flora := range floraList {
		if flora.org.Dead || flora.cells.Count == 0 {
			continue
		}

		// Check distance
		dx := predPos.X - flora.pos.X
		dy := predPos.Y - flora.pos.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > feedingDistance {
			continue
		}

		// Eat! Transfer energy
		// Larger organisms take bigger bites - but also have higher metabolism (handled in energy.go)
		biteMultiplier := biteSizeMultiplier(predOrg)
		eatAmount := float32(herbivoreEatAmount) * biteMultiplier
		if eatAmount > flora.org.Energy {
			eatAmount = flora.org.Energy
		}

		predOrg.Energy += eatAmount
		flora.org.Energy -= eatAmount * 0.5 // Flora loses half (regenerates)

		// Damage a random cell
		if flora.cells.Count > 0 {
			cellIdx := uint8(0) // Damage first alive cell
			for i := uint8(0); i < flora.cells.Count; i++ {
				if flora.cells.Cells[i].Alive {
					cellIdx = i
					break
				}
			}
			flora.cells.Cells[cellIdx].Decomposition += floraDamageRate
		}

		// Only eat from one flora per tick
		return
	}
}

func (s *FeedingSystem) tryEatDeadFlora(predPos *components.Position, predOrg *components.Organism, floraList []floraData) {
	for _, flora := range floraList {
		// Only eat dead flora
		if !flora.org.Dead {
			continue
		}

		// Check distance
		dx := predPos.X - flora.pos.X
		dy := predPos.Y - flora.pos.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > feedingDistance {
			continue
		}

		// Eat! Transfer energy
		// Larger organisms take bigger bites of carrion
		biteMultiplier := biteSizeMultiplier(predOrg)
		eatAmount := float32(carrionEatAmount) * biteMultiplier
		if eatAmount > flora.org.Energy {
			eatAmount = flora.org.Energy
		}

		predOrg.Energy += eatAmount
		flora.org.Energy -= eatAmount

		// Only eat from one source per tick
		return
	}
}

func (s *FeedingSystem) tryEatFauna(predPos *components.Position, predOrg *components.Organism, faunaList []faunaData, wantDead bool, predatorSpecies int) {
	for _, prey := range faunaList {
		// Skip self
		if prey.org == predOrg {
			continue
		}

		// Carrion wants dead, carnivore wants alive
		if wantDead && !prey.org.Dead {
			continue
		}
		if !wantDead && prey.org.Dead {
			continue
		}

		// Kin recognition: prefer to avoid hunting same species (cannibalism avoidance)
		// Only applies to live prey hunting (predatorSpecies > 0)
		// Carrion eating passes 0 to skip this check
		// 70% chance to avoid same-species prey, 30% will still hunt kin (desperation)
		if predatorSpecies > 0 && prey.speciesID == predatorSpecies {
			if rand.Float32() < 0.70 {
				continue // Avoid hunting your own kind (most of the time)
			}
		}

		// Check distance
		dx := predPos.X - prey.pos.X
		dy := predPos.Y - prey.pos.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > feedingDistance {
			continue
		}

		// === Social hunting dynamics ===
		// Prey in herds are harder to catch (safety in numbers)
		// Predators in packs can overcome herd defense but share food

		herdSize := 0
		packSize := 0
		if !wantDead {
			herdSize = countHerdDefenders(prey.pos, prey.org, faunaList)
			packSize = countPackHunters(prey.pos, predOrg, faunaList)
		}

		// Herd defense: each defender reduces feeding effectiveness
		// 0 defenders = 100%, 1 = 67%, 2 = 50%, 3 = 40%, etc.
		herdDefense := 1.0 / float32(1+herdSize)

		// Pack offense: hunters can overcome herd defense
		// Pack size counters herd size, making hunting more effective
		// But they'll have to share the food
		packOffense := float32(1 + packSize)

		// Net effectiveness: pack hunters vs herd defenders
		// If pack > herd, hunting is easier; if herd > pack, hunting is harder
		effectiveness := herdDefense * packOffense
		if effectiveness > 1.0 {
			effectiveness = 1.0 // Cap at 100% - can't do better than solo vs isolated
		}

		// Calculate base bite
		biteMultiplier := biteSizeMultiplier(predOrg)
		var baseRate float32
		if wantDead {
			baseRate = carrionEatAmount
		} else {
			baseRate = carnivoreEatAmount
		}

		// Apply effectiveness (herd defense reduces bite)
		eatAmount := baseRate * biteMultiplier * effectiveness

		// Can only eat what prey has
		if eatAmount > prey.org.Energy {
			eatAmount = prey.org.Energy
		}

		// Pack hunters share the food
		shareRatio := float32(1.0)
		if packSize > 0 {
			shareRatio = 1.0 / float32(1+packSize) // Split among all hunters
		}

		predOrg.Energy += eatAmount * shareRatio
		prey.org.Energy -= eatAmount // Prey loses full amount regardless of sharing

		// Carnivores deal damage to prey
		if !wantDead && prey.cells.Count > 0 {
			// Pack hunters deal more damage (coordinated attack)
			damageMultiplier := float32(1 + packSize)
			if damageMultiplier > 3 {
				damageMultiplier = 3 // Cap pack damage bonus
			}
			for i := uint8(0); i < prey.cells.Count; i++ {
				if prey.cells.Cells[i].Alive {
					prey.cells.Cells[i].Decomposition += 0.02 * damageMultiplier
					break
				}
			}
		}

		// Kill prey if energy depleted
		if prey.org.Energy <= 0 {
			prey.org.Dead = true
		}

		// Only eat from one prey per tick
		return
	}
}

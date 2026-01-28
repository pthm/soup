package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

const (
	feedingDistance    = 6.0   // Distance to consume food (scaled for smaller cells)
	herbivoreEatAmount = 1.5   // Energy gained per tick while eating flora
	carnivoreEatAmount = 2.5   // Energy gained per tick while eating fauna
	carrionEatAmount   = 4.0   // Energy gained per tick while eating dead
	floraDamageRate    = 0.008 // Decomposition added to flora cell when eaten
)

// FeedingSystem handles fauna consuming food sources.
type FeedingSystem struct {
	faunaFilter ecs.Filter4[components.Position, components.Velocity, components.Organism, components.CellBuffer]
	floraFilter ecs.Filter4[components.Position, components.Organism, components.CellBuffer, components.Flora]
}

// NewFeedingSystem creates a new feeding system.
func NewFeedingSystem(w *ecs.World) *FeedingSystem {
	return &FeedingSystem{
		faunaFilter: *ecs.NewFilter4[components.Position, components.Velocity, components.Organism, components.CellBuffer](w),
		floraFilter: *ecs.NewFilter4[components.Position, components.Organism, components.CellBuffer, components.Flora](w),
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

	// First pass: collect all fauna
	faunaQuery := s.faunaFilter.Query()
	for faunaQuery.Next() {
		pos, _, org, cells := faunaQuery.Get()
		faunaList = append(faunaList, faunaData{pos, org, cells})
	}

	// Second pass: process feeding
	faunaQuery2 := s.faunaFilter.Query()
	for faunaQuery2.Next() {
		pos, _, org, _ := faunaQuery2.Get()

		if org.Dead {
			continue
		}

		// Skip flora (they don't eat)
		if traits.IsFlora(org.Traits) {
			continue
		}

		// Herbivores eat flora
		if org.Traits.Has(traits.Herbivore) {
			s.tryEatFlora(pos, org, floraList)
		}

		// Carnivores eat other fauna (alive)
		if org.Traits.Has(traits.Carnivore) {
			s.tryEatFauna(pos, org, faunaList, false)
		}

		// Carrion eaters eat dead fauna and dead flora
		if org.Traits.Has(traits.Carrion) {
			s.tryEatFauna(pos, org, faunaList, true)
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
	pos   *components.Position
	org   *components.Organism
	cells *components.CellBuffer
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
		eatAmount := float32(herbivoreEatAmount)
		if eatAmount > flora.org.Energy {
			eatAmount = flora.org.Energy
		}

		predOrg.Energy += eatAmount
		flora.org.Energy -= eatAmount * 0.5 // Flora loses less than predator gains

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
		eatAmount := float32(carrionEatAmount)
		if eatAmount > flora.org.Energy {
			eatAmount = flora.org.Energy
		}

		predOrg.Energy += eatAmount
		flora.org.Energy -= eatAmount

		// Only eat from one source per tick
		return
	}
}

func (s *FeedingSystem) tryEatFauna(predPos *components.Position, predOrg *components.Organism, faunaList []faunaData, wantDead bool) {
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

		// Check distance
		dx := predPos.X - prey.pos.X
		dy := predPos.Y - prey.pos.Y
		dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))

		if dist > feedingDistance {
			continue
		}

		// Eat! Transfer energy
		var eatAmount float32
		if wantDead {
			eatAmount = carrionEatAmount
		} else {
			eatAmount = carnivoreEatAmount
		}

		// Can only eat what prey has
		if eatAmount > prey.org.Energy {
			eatAmount = prey.org.Energy
		}

		predOrg.Energy += eatAmount
		prey.org.Energy -= eatAmount

		// Carnivores deal damage to prey (reduced - prey can escape)
		if !wantDead && prey.cells.Count > 0 {
			// Damage a cell
			for i := uint8(0); i < prey.cells.Count; i++ {
				if prey.cells.Cells[i].Alive {
					prey.cells.Cells[i].Decomposition += 0.02 // Moderate damage
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

package game

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
	"github.com/pthm-cable/soup/neural"
)

// spawnInitialPopulation creates the starting entities.
func (g *Game) spawnInitialPopulation() {
	cfg := config.Cfg()
	for i := 0; i < cfg.Population.Initial; i++ {
		x := g.rng.Float32() * g.width
		y := g.rng.Float32() * g.height
		heading := g.rng.Float32() * 2 * math.Pi

		// Alternate between prey and predator
		kind := components.KindPrey
		if i%4 == 0 {
			kind = components.KindPredator
		}

		g.spawnEntity(x, y, heading, kind)
	}
}

// spawnEntity creates a new entity with a fresh brain.
func (g *Game) spawnEntity(x, y, heading float32, kind components.Kind) ecs.Entity {
	cfg := config.Cfg()
	id := g.nextID
	g.nextID++

	pos := components.Position{X: x, Y: y}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: heading, AngVel: 0}
	body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
	energy := components.Energy{Value: float32(cfg.Entity.InitialEnergy), Age: 0, Alive: true}
	caps := components.DefaultCapabilities(kind)
	// Add jitter to desync reproduction across the population
	cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
	org := components.Organism{ID: id, Kind: kind, ReproCooldown: float32(cfg.Reproduction.MaturityAge) + cooldownJitter}

	// Create brain
	brain := neural.NewFFNN(g.rng)
	g.brains[id] = brain

	entity := g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
	g.aliveCount++

	// Register with lifetime tracker
	g.lifetimeTracker.Register(id, g.tick)

	// Track population by kind
	if kind == components.KindPrey {
		g.numPrey++
	} else {
		g.numPred++
	}

	return entity
}

// cleanupDead removes dead entities and their brains.
func (g *Game) cleanupDead() {
	// First pass: collect dead entities (must complete before modifying)
	type deadInfo struct {
		entity ecs.Entity
		id     uint32
		kind   components.Kind
	}
	var toRemove []deadInfo

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			toRemove = append(toRemove, deadInfo{entity: entity, id: org.ID, kind: org.Kind})
		}
	}

	// Second pass: remove entities (query iteration complete)
	for _, dead := range toRemove {
		// Record death in telemetry
		g.collector.RecordDeath(dead.kind)
		g.lifetimeTracker.Remove(dead.id)

		g.entityMapper.Remove(dead.entity)
		delete(g.brains, dead.id)
		g.aliveCount--
		g.deadCount++

		// Track population by kind
		if dead.kind == components.KindPrey {
			g.numPrey--
		} else {
			g.numPred--
		}
	}

	// Respawn if population drops too low
	cfg := config.Cfg()
	if g.aliveCount < cfg.Population.RespawnThreshold && g.tick > 100 {
		for i := 0; i < cfg.Population.RespawnCount; i++ {
			x := g.rng.Float32() * g.width
			y := g.rng.Float32() * g.height
			heading := g.rng.Float32() * 2 * math.Pi
			kind := components.KindPrey
			if g.rng.Float32() < float32(cfg.Population.PredatorSpawnChance) {
				kind = components.KindPredator
			}
			g.spawnEntity(x, y, heading, kind)
		}
	}
}

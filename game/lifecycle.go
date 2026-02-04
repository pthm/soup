package game

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
)

// spawnInitialPopulation creates the starting entities.
func (g *Game) spawnInitialPopulation() {
	cfg := g.config()

	// Get archetype indices for grazer and hunter (default archetypes)
	grazerIdx := cfg.Derived.ArchetypeIndex["grazer"]
	hunterIdx := cfg.Derived.ArchetypeIndex["hunter"]

	for i := 0; i < cfg.Population.Initial; i++ {
		x := g.rng.Float32() * g.worldWidth
		y := g.rng.Float32() * g.worldHeight
		heading := g.rng.Float32() * 2 * math.Pi

		// Use configured spawn chance for initial population
		archetypeID := grazerIdx
		if g.rng.Float32() < float32(cfg.Population.PredatorSpawnChance) {
			archetypeID = hunterIdx
		}

		g.spawnEntity(x, y, heading, archetypeID)
	}
}

// spawnEntity creates a new entity with a fresh brain using the given archetype.
func (g *Game) spawnEntity(x, y, heading float32, archetypeID uint8) ecs.Entity {
	cfg := g.config()
	arch := &cfg.Archetypes[archetypeID]

	id := g.nextID
	g.nextID++

	// Assign a new clade ID for fresh spawns
	cladeID := g.nextCladeID
	g.nextCladeID++

	// Diet comes from archetype
	diet := float32(arch.Diet)

	pos := components.Position{X: x, Y: y}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: heading, AngVel: 0}
	body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
	energy := components.Energy{Value: float32(cfg.Entity.InitialEnergy), Max: float32(cfg.Entity.MaxEnergy), Age: 0, Alive: true}
	caps := components.DefaultCapabilities(diet)
	// Add jitter to desync reproduction across the population
	cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
	// Carnivores can't hunt immediately at birth
	huntCooldown := float32(0)
	if diet >= 0.5 {
		huntCooldown = float32(cfg.Reproduction.NewbornHuntCooldown)
	}
	org := components.Organism{
		ID:                 id,
		FounderArchetypeID: archetypeID,
		Diet:               diet,
		CladeID:            cladeID,
		ReproCooldown:      float32(cfg.Reproduction.MaturityAge) + cooldownJitter,
		HuntCooldown:       huntCooldown,
	}

	// Create brain: use seed hall of fame if available, otherwise random
	var brain *neural.FFNN
	if g.seedHallOfFame != nil {
		if weights := g.seedHallOfFame.Sample(archetypeID); weights != nil {
			brain = neural.NewFFNN(g.rng)
			brain.UnmarshalWeights(*weights)
			// Light mutation for initial population diversity
			brain.MutateSparse(g.rng, 0.10, 0.05, 0.0, 0.0)
		}
	}
	if brain == nil {
		brain = neural.NewFFNN(g.rng)
	}
	g.brains[id] = brain

	entity := g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
	g.aliveCount++

	// Register with lifetime tracker (includes clade info)
	g.lifetimeTracker.Register(id, g.tick, cladeID, archetypeID, diet)

	// Track population by diet bucket
	if diet < 0.5 {
		g.numHerb++
	} else {
		g.numCarn++
	}

	return entity
}

// cleanupDead removes dead entities and their brains.
func (g *Game) cleanupDead() {
	cfg := g.config()

	// First pass: collect dead entities (must complete before modifying)
	type deadInfo struct {
		entity ecs.Entity
		id     uint32
		diet   float32
		x, y   float32 // last position for carcass deposit
		energy float32 // energy at death
	}
	var toRemove []deadInfo

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			toRemove = append(toRemove, deadInfo{
				entity: entity,
				id:     org.ID,
				diet:   org.Diet,
				x:      pos.X,
				y:      pos.Y,
				energy: energy.Value,
			})
		}
	}

	carcassFrac := float32(cfg.Detritus.CarcassFraction)

	// Second pass: remove entities (query iteration complete)
	for _, dead := range toRemove {
		// Deposit carcass to detritus before removal (energy economy step 5)
		if dead.energy > 0 {
			det := carcassFrac * dead.energy
			g.resourceField.DepositDetritus(dead.x, dead.y, det)
			g.heatLossAccum += (1 - carcassFrac) * dead.energy
		}

		// Record death in telemetry
		g.collector.RecordDeath(dead.diet)

		// Evaluate for hall of fame before removing brain
		if g.hallOfFame != nil {
			stats := g.lifetimeTracker.Get(dead.id)
			if stats != nil {
				if brain, ok := g.brains[dead.id]; ok {
					// Update survival time before evaluation
					stats.SurvivalTimeSec = float32(g.tick-stats.BirthTick) * cfg.Derived.DT32
					weights := brain.MarshalWeights()
					g.hallOfFame.Consider(dead.diet, weights, stats, dead.id)
				}
			}
		}

		g.lifetimeTracker.Remove(dead.id)
		g.entityMapper.Remove(dead.entity)
		delete(g.brains, dead.id)
		g.aliveCount--
		g.deadCount++

		// Track population by diet bucket
		if dead.diet < 0.5 {
			g.numHerb--
		} else {
			g.numCarn--
		}
	}

}


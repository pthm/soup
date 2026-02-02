package game

import (
	"log/slog"
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

	// Derive Kind from diet for backwards compatibility
	kind := components.KindPrey
	if diet >= 0.5 {
		kind = components.KindPredator
	}

	pos := components.Position{X: x, Y: y}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: heading, AngVel: 0}
	body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
	energy := components.Energy{Value: float32(cfg.Entity.InitialEnergy), Age: 0, Alive: true}
	caps := components.DefaultCapabilities(kind)
	// Add jitter to desync reproduction across the population
	cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
	// Newborn predators can't hunt immediately
	huntCooldown := float32(0)
	if kind == components.KindPredator {
		huntCooldown = float32(cfg.Reproduction.NewbornHuntCooldown)
	}
	org := components.Organism{
		ID:                 id,
		Kind:               kind,
		FounderArchetypeID: archetypeID,
		Diet:               diet,
		CladeID:            cladeID,
		ReproCooldown:      float32(cfg.Reproduction.MaturityAge) + cooldownJitter,
		HuntCooldown:       huntCooldown,
	}

	// Create brain
	brain := neural.NewFFNN(g.rng)
	g.brains[id] = brain

	entity := g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
	g.aliveCount++

	// Register with lifetime tracker (includes clade info)
	g.lifetimeTracker.Register(id, g.tick, cladeID, archetypeID, diet)

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
	cfg := g.config()

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

		// Evaluate for hall of fame before removing brain
		if g.hallOfFame != nil {
			stats := g.lifetimeTracker.Get(dead.id)
			if stats != nil {
				if brain, ok := g.brains[dead.id]; ok {
					// Update survival time before evaluation
					stats.SurvivalTimeSec = float32(g.tick-stats.BirthTick) * cfg.Derived.DT32
					weights := brain.MarshalWeights()
					g.hallOfFame.Consider(dead.kind, weights, stats, dead.id)
				}
			}
		}

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

	// Get archetype indices for respawn
	grazerIdx := cfg.Derived.ArchetypeIndex["grazer"]
	hunterIdx := cfg.Derived.ArchetypeIndex["hunter"]

	// Respawn if population drops too low (general respawn)
	if g.aliveCount < cfg.Population.RespawnThreshold && g.tick > 100 {
		for i := 0; i < cfg.Population.RespawnCount; i++ {
			x := g.rng.Float32() * g.worldWidth
			y := g.rng.Float32() * g.worldHeight
			heading := g.rng.Float32() * 2 * math.Pi
			archetypeID := grazerIdx
			if g.rng.Float32() < float32(cfg.Population.PredatorSpawnChance) {
				archetypeID = hunterIdx
			}
			g.spawnEntity(x, y, heading, archetypeID)
		}
	}

	// Hall of Fame reseeding (replaces min_predators/min_prey forcing)
	if g.hallOfFame != nil && cfg.HallOfFame.Enabled && g.tick > 100 {
		g.reseedFromHallIfNeeded(components.KindPredator)
		g.reseedFromHallIfNeeded(components.KindPrey)
	} else {
		// Fallback: use legacy min population logic if hall of fame is disabled
		if cfg.Population.MinPredators > 0 && g.numPred < cfg.Population.MinPredators && g.tick > 100 {
			for g.numPred < cfg.Population.MinPredators {
				x := g.rng.Float32() * g.worldWidth
				y := g.rng.Float32() * g.worldHeight
				heading := g.rng.Float32() * 2 * math.Pi
				g.spawnEntity(x, y, heading, hunterIdx)
			}
		}

		if cfg.Population.MinPrey > 0 && g.numPrey < cfg.Population.MinPrey && g.tick > 100 {
			for g.numPrey < cfg.Population.MinPrey {
				x := g.rng.Float32() * g.worldWidth
				y := g.rng.Float32() * g.worldHeight
				heading := g.rng.Float32() * 2 * math.Pi
				g.spawnEntity(x, y, heading, grazerIdx)
			}
		}
	}
}

// reseedFromHallIfNeeded checks if a kind needs reseeding and spawns from the hall.
func (g *Game) reseedFromHallIfNeeded(kind components.Kind) {
	cfg := g.config()
	hofCfg := cfg.HallOfFame

	// Get current population for this kind
	var currentPop int
	if kind == components.KindPrey {
		currentPop = g.numPrey
	} else {
		currentPop = g.numPred
	}

	// Check if below reseed threshold
	if currentPop >= hofCfg.ReseedThreshold {
		return
	}

	// Reseed up to reseed_count entities
	reseeded := 0
	for i := 0; i < hofCfg.ReseedCount && currentPop+reseeded < hofCfg.ReseedThreshold; i++ {
		if g.spawnFromHall(kind) {
			reseeded++
		}
	}

	if reseeded > 0 {
		slog.Info("hall_of_fame_reseed",
			"kind", kind.String(),
			"population_before", currentPop,
			"reseeded_count", reseeded,
			"hall_size", g.hallOfFame.Size(kind),
		)
	}
}

// spawnFromHall creates an entity using a brain from the hall of fame.
// Returns true if an entity was spawned, false if the hall was empty.
func (g *Game) spawnFromHall(kind components.Kind) bool {
	cfg := g.config()
	hofCfg := cfg.HallOfFame

	// Determine archetype from kind
	var archetypeID uint8
	if kind == components.KindPredator {
		archetypeID = cfg.Derived.ArchetypeIndex["hunter"]
	} else {
		archetypeID = cfg.Derived.ArchetypeIndex["grazer"]
	}
	arch := &cfg.Archetypes[archetypeID]
	diet := float32(arch.Diet)

	// Sample a brain from the hall
	weights := g.hallOfFame.Sample(kind)
	if weights == nil {
		// Hall is empty, fall back to random brain with warning
		slog.Warn("hall_of_fame_empty_fallback",
			"kind", kind.String(),
			"message", "no proven lineages yet, spawning random brain",
		)
		x := g.rng.Float32() * g.worldWidth
		y := g.rng.Float32() * g.worldHeight
		heading := g.rng.Float32() * 2 * math.Pi
		g.spawnEntity(x, y, heading, archetypeID)
		return true
	}

	// Create entity with hall brain
	id := g.nextID
	g.nextID++

	// Assign a new clade ID
	cladeID := g.nextCladeID
	g.nextCladeID++

	x := g.rng.Float32() * g.worldWidth
	y := g.rng.Float32() * g.worldHeight
	heading := g.rng.Float32() * 2 * math.Pi

	pos := components.Position{X: x, Y: y}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: heading, AngVel: 0}
	body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
	energy := components.Energy{Value: float32(hofCfg.ReseedEnergy), Age: 0, Alive: true}
	caps := components.DefaultCapabilities(kind)
	cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
	// Newborn predators can't hunt immediately
	huntCooldown := float32(0)
	if kind == components.KindPredator {
		huntCooldown = float32(cfg.Reproduction.NewbornHuntCooldown)
	}
	org := components.Organism{
		ID:                 id,
		Kind:               kind,
		FounderArchetypeID: archetypeID,
		Diet:               diet,
		CladeID:            cladeID,
		ReproCooldown:      float32(cfg.Reproduction.MaturityAge) + cooldownJitter,
		HuntCooldown:       huntCooldown,
	}

	// Create brain from hall weights and mutate
	brain := &neural.FFNN{}
	brain.UnmarshalWeights(*weights)

	// Apply mutation (same as reproduction)
	brain.MutateSparse(
		g.rng,
		float32(cfg.Mutation.Rate),
		float32(cfg.Mutation.Sigma),
		float32(cfg.Mutation.BigRate),
		float32(cfg.Mutation.BigSigma),
	)
	g.brains[id] = brain

	g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
	g.aliveCount++

	// Register with lifetime tracker (includes clade info)
	g.lifetimeTracker.Register(id, g.tick, cladeID, archetypeID, diet)

	// Track population by kind
	if kind == components.KindPrey {
		g.numPrey++
	} else {
		g.numPred++
	}

	return true
}

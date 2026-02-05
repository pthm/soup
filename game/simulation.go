package game

import (
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// updateSpatialGrid rebuilds the spatial index.
func (g *Game) updateSpatialGrid() {
	g.spatialGrid.Clear()

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, _, _ := query.Get()

		if energy.Alive {
			g.spatialGrid.Insert(entity, pos.X, pos.Y)
		}
	}
}

// updateFeeding handles organism attacks (diet-scaled hunting).
// Hunting ability scales linearly with diet: diet=0 can't hunt, diet=1 full hunting.
// Bite damage = feeding_rate × metabolic_rate × diet
// Cooldown = energy_gained × cooldown_factor / metabolic_rate
func (g *Game) updateFeeding() {
	cfg := g.config()
	refugiaStrength := float32(cfg.Refugia.Strength)
	feedingRate := systems.GetCachedFeedingRate()
	cooldownFactor := systems.GetCachedCooldownFactor()
	thrustDeadzone := systems.GetCachedThrustDeadzone()

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Skip if brain isn't signaling bite
		if energy.LastBite <= thrustDeadzone {
			continue
		}

		// Hunting ability scales with diet: diet=0 can't hunt, diet=1 full hunting
		if org.Diet <= 0.05 {
			continue // Pure herbivores can't hunt
		}

		// Skip if still digesting
		if org.DigestCooldown > 0 {
			g.collector.RecordBiteBlockedByDigest()
			continue
		}

		// Skip if newborn still in hunt cooldown
		if org.HuntCooldown > 0 {
			continue
		}

		// Check if brain exists
		_, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Query nearby targets within bite range
		neighbors := g.spatialGrid.QueryRadius(pos.X, pos.Y, caps.BiteRange, entity, g.posMap)

		for _, neighbor := range neighbors {
			// Get neighbor components
			nOrg := g.orgMap.Get(neighbor)
			if nOrg == nil {
				continue
			}

			// Can only eat targets with lower diet
			if nOrg.Diet >= org.Diet-0.2 {
				continue // Target has similar or higher diet
			}

			// Get target position for refugia check
			nPos := g.posMap.Get(neighbor)
			if nPos == nil {
				continue
			}

			// Get target energy directly via mapper
			nEnergy := g.energyMap.Get(neighbor)
			if nEnergy != nil && nEnergy.Alive {
				// Record bite attempt
				g.collector.RecordBiteAttempt()
				g.lifetimeTracker.RecordBiteAttempt(org.ID)

				// Refugia mechanic: high-resource zones protect prey
				resourceHere := g.resourceField.Sample(nPos.X, nPos.Y)
				successProb := 1.0 - refugiaStrength*resourceHere
				if g.rng.Float32() > successProb {
					g.collector.RecordBiteMissedRefugia()
					break // one attempt per tick
				}

				// Bite damage = feeding_rate × metabolic_rate × diet
				biteDamage := feedingRate * org.MetabolicRate * org.Diet
				xfer := systems.TransferEnergy(energy, nEnergy, biteDamage)

				// Conservation accounting
				if xfer.ToDet > 0 {
					g.resourceField.DepositDetritus(nPos.X, nPos.Y, xfer.ToDet)
				}
				if xfer.Overflow > 0 {
					g.resourceField.DepositDetritus(pos.X, pos.Y, xfer.Overflow)
				}
				g.heatLossAccum += xfer.ToHeat

				if xfer.Removed > 0 {
					g.collector.RecordBiteHit()
					g.lifetimeTracker.RecordBiteHit(org.ID)

					// Cooldown = energy_gained × cooldown_factor / metabolic_rate
					digestTime := xfer.ToGainer * cooldownFactor / org.MetabolicRate
					if digestTime > org.DigestCooldown {
						org.DigestCooldown = digestTime
					}

					if xfer.Killed {
						g.collector.RecordKill()
						g.lifetimeTracker.RecordKill(org.ID)
					}
				}

				break // one bite per tick
			}
		}
	}
}

// updateEnergy applies metabolic costs, foraging, and biomass growth.
// Grazing ability scales linearly with (1 - diet): diet=0 full grazing, diet=1 no grazing.
// Graze rate = resource × feeding_rate × metabolic_rate × (1-diet)
func (g *Game) updateEnergy() {
	cfg := g.config()
	dt := cfg.Derived.DT32
	grazeRadius := cfg.Resource.GrazeRadius
	feedingRate := systems.GetCachedFeedingRate()
	feedingEfficiency := systems.GetCachedFeedingEfficiency()
	metPerBio := systems.GetCachedMetPerBio()

	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Grazing ability scales with (1 - diet): diet=0 full grazing, diet=1 no grazing
		grazeAbility := 1.0 - org.Diet
		if grazeAbility > 0.05 {
			// Graze rate = resource × feeding_rate × metabolic_rate × (1-diet)
			resourceHere := g.resourceField.Sample(pos.X, pos.Y)
			grazeRate := resourceHere * feedingRate * org.MetabolicRate * grazeAbility

			// Remove resource and get actual removed amount
			removed := g.resourceField.Graze(pos.X, pos.Y, grazeRate, dt, grazeRadius)

			// Energy gain = removed × feeding_efficiency → Met
			gain := removed * feedingEfficiency
			grazingHeat := removed - gain
			g.heatLossAccum += grazingHeat

			energy.Met += gain

			// Route Met overflow to detritus (MaxMet = Bio * metPerBio)
			maxMet := energy.Bio * metPerBio
			if energy.Met > maxMet {
				overflow := energy.Met - maxMet
				energy.Met = maxMet
				g.resourceField.DepositDetritus(pos.X, pos.Y, overflow)
			}

			// Track foraging for telemetry
			g.lifetimeTracker.RecordForage(org.ID, gain)
		}

		// Grow biomass from surplus Met (no heat loss, just pool transfer)
		systems.GrowBiomass(energy, dt)

		// Apply metabolic costs (scaled by metabolic_rate); track heat for conservation
		metabolicCost := systems.UpdateEnergy(energy, *vel, *caps, org.MetabolicRate, dt)
		g.heatLossAccum += metabolicCost
	}
}

// updateCooldowns decrements reproduction, digestion, and hunt cooldowns.
func (g *Game) updateCooldowns() {
	dt := g.config().Derived.DT32
	query := g.entityFilter.Query()
	for query.Next() {
		_, _, _, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		if org.ReproCooldown > 0 {
			org.ReproCooldown -= dt
			if org.ReproCooldown < 0 {
				org.ReproCooldown = 0
			}
		}

		if org.DigestCooldown > 0 {
			org.DigestCooldown -= dt
			if org.DigestCooldown < 0 {
				org.DigestCooldown = 0
			}
		}

		if org.HuntCooldown > 0 {
			org.HuntCooldown -= dt
			if org.HuntCooldown < 0 {
				org.HuntCooldown = 0
			}
		}
	}
}

// updateReproduction handles asexual reproduction with mutation and clade tracking.
// Parent pays (childBio + childMet) / birthEfficiency from their Met pool.
// Child starts with minBio and minimal Met, must grow to reach full size.
func (g *Game) updateReproduction() {
	cfg := g.config()
	repro := &cfg.Reproduction
	mutation := &cfg.Mutation
	cladeCfg := &cfg.Clades

	// Biomass config
	minBio := systems.GetCachedMinBio()
	birthEfficiency := systems.GetCachedBirthEfficiency()
	metPerBio := systems.GetCachedMetPerBio()

	// Child starts with minBio and minimal Met (enough to survive briefly)
	childBio := minBio
	childMet := minBio * metPerBio * 0.5 // Start at 50% Met capacity
	birthPrice := (childBio + childMet) / birthEfficiency

	// Collect births to spawn after iteration
	type birthInfo struct {
		x, y, heading      float32
		parentBrain        *neural.FFNN
		parentID           uint32
		parentDiet         float32
		parentCladeID      uint64
		founderArchetypeID uint8
		childBio           float32
		childMet           float32
	}
	var births []birthInfo

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Check population caps
		if org.Diet < 0.5 && g.numHerb >= cfg.Population.MaxPrey {
			continue
		}
		if org.Diet >= 0.5 && g.numCarn >= cfg.Population.MaxPred {
			continue
		}

		// Check reproduction thresholds (using Met/MaxMet ratio)
		var threshold, cooldown float32
		if org.Diet >= 0.5 {
			threshold = float32(repro.PredThreshold)
			cooldown = float32(repro.PredCooldown)
		} else {
			threshold = float32(repro.PreyThreshold)
			cooldown = float32(repro.PreyCooldown)
		}

		maturityAge := float32(repro.MaturityAge)
		maxMet := energy.Bio * metPerBio
		var energyRatio float32
		if maxMet > 0 {
			energyRatio = energy.Met / maxMet
		}
		if energyRatio < threshold || energy.Age < maturityAge || org.ReproCooldown > 0 {
			continue
		}

		// Check if parent can afford the birth price
		if energy.Met < birthPrice {
			continue
		}

		// Density-dependent reproduction for carnivores
		// When herbivores are scarce, carnivores breed less (p = herb / (herb + K))
		if org.Diet >= 0.5 && repro.PredDensityK > 0 {
			preyN := float32(g.numHerb)
			k := float32(repro.PredDensityK)
			breedProb := preyN / (preyN + k)
			if g.rng.Float32() > breedProb {
				continue // Skip reproduction this tick, but stay eligible
			}
		}

		// Density-dependent reproduction for herbivores (soft carrying capacity)
		// p = K / (N + K) — at N=K, 50% breed probability
		if org.Diet < 0.5 && repro.PreyDensityK > 0 {
			herbN := float32(g.numHerb)
			k := float32(repro.PreyDensityK)
			breedProb := k / (herbN + k)
			if g.rng.Float32() > breedProb {
				continue
			}
		}

		// Reproduction: parent pays from Met pool
		parentBrain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Parent pays birth price (conservation: parent loses, child gains + heat)
		energy.Met -= birthPrice
		birthHeat := birthPrice - (childBio + childMet) // inefficiency goes to heat
		g.heatLossAccum += birthHeat

		// Set cooldown
		org.ReproCooldown = cooldown

		// Queue child spawn
		spawnOffset := float32(repro.SpawnOffset)
		offset := spawnOffset + g.rng.Float32()*10
		childX := mod(pos.X+(g.rng.Float32()-0.5)*offset*2, g.worldWidth)
		childY := mod(pos.Y+(g.rng.Float32()-0.5)*offset*2, g.worldHeight)
		headingJitter := float32(repro.HeadingJitter)
		childHeading := rot.Heading + (g.rng.Float32()-0.5)*headingJitter*2

		births = append(births, birthInfo{
			x:                  childX,
			y:                  childY,
			heading:            childHeading,
			parentBrain:        parentBrain,
			parentID:           org.ID,
			parentDiet:         org.Diet,
			parentCladeID:      org.CladeID,
			founderArchetypeID: org.FounderArchetypeID,
			childBio:           childBio,
			childMet:           childMet,
		})
	}

	// Spawn children outside query
	for _, b := range births {
		// Clone and mutate brain, capturing avgAbsDelta for clade split decision
		childBrain := b.parentBrain.Clone()
		avgAbsDelta := childBrain.MutateSparse(g.rng,
			float32(mutation.Rate),
			float32(mutation.Sigma),
			float32(mutation.BigRate),
			float32(mutation.BigSigma))

		// Diet mutation: Normal(0, 0.01), 1% chance Normal(0, 0.05)
		var dietMutation float32
		if g.rng.Float32() < 0.01 {
			dietMutation = float32(g.rng.NormFloat64()) * 0.05
		} else {
			dietMutation = float32(g.rng.NormFloat64()) * 0.01
		}
		childDiet := clamp32(b.parentDiet+dietMutation, 0, 1)

		// Clade split logic
		shouldSplit := false
		dietDrift := absf(childDiet - b.parentDiet)

		if g.rng.Float32() < float32(cladeCfg.SplitChance) {
			shouldSplit = true // Random split
		} else if avgAbsDelta > float32(cladeCfg.DeltaThreshold) {
			shouldSplit = true // Large neural mutation
		} else if dietDrift > float32(cladeCfg.DietThreshold) {
			shouldSplit = true // Diet drift
		}

		childCladeID := b.parentCladeID
		if shouldSplit {
			childCladeID = g.nextCladeID
			g.nextCladeID++
		}

		// Create child entity directly (not using spawnEntity since we need custom brain/clade)
		childID := g.nextID
		g.nextID++

		// Get archetype for capabilities
		arch := &cfg.Archetypes[b.founderArchetypeID]

		pos := components.Position{X: b.x, Y: b.y}
		vel := components.Velocity{X: 0, Y: 0}
		rot := components.Rotation{Heading: b.heading, AngVel: 0}
		body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
		childEnergy := components.Energy{
			Met:    b.childMet,
			Bio:    b.childBio,
			BioCap: float32(arch.EnergyCapacity), // BioCap from archetype
			Age:    0,
			Alive:  true,
		}
		caps := components.CapabilitiesFromArchetype(arch)
		cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
		// High-diet organisms can't hunt immediately at birth
		huntCooldown := float32(0)
		if childDiet >= 0.5 {
			huntCooldown = float32(repro.NewbornHuntCooldown)
		}
		childOrg := components.Organism{
			ID:                 childID,
			FounderArchetypeID: b.founderArchetypeID,
			Diet:               childDiet,
			MetabolicRate:      float32(arch.MetabolicRate),
			CladeID:            childCladeID,
			ReproCooldown:      float32(cfg.Reproduction.MaturityAge) + cooldownJitter,
			HuntCooldown:       huntCooldown,
		}

		g.brains[childID] = childBrain
		g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &childEnergy, &caps, &childOrg)
		g.aliveCount++

		// Register with lifetime tracker
		g.lifetimeTracker.Register(childID, g.tick, childCladeID, b.founderArchetypeID, childDiet)

		// Track population by diet bucket
		if childDiet < 0.5 {
			g.numHerb++
		} else {
			g.numCarn++
		}

		// Record birth in telemetry
		g.collector.RecordBirth(childDiet)
		g.lifetimeTracker.RecordChild(b.parentID)
	}
}

// clamp32 clamps x to [min, max].
func clamp32(x, min, max float32) float32 {
	if x < min {
		return min
	}
	if x > max {
		return max
	}
	return x
}

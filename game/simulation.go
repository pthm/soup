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

// updateBehaviorAndPhysics runs brains and applies movement.
func (g *Game) updateBehaviorAndPhysics() {
	cfg := g.config()
	dt := cfg.Derived.DT32

	// Check if we have a selected entity for inspector (headless mode has no inspector)
	var selectedEntity any
	var hasSelection bool
	if g.inspector != nil {
		selectedEntity, hasSelection = g.inspector.Selected()
	}

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, vel, rot, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Get brain
		brain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Query neighbors into reusable buffer (avoids allocation)
		g.neighborBuf = g.spatialGrid.QueryRadiusInto(
			g.neighborBuf[:0], // reset but keep capacity
			pos.X, pos.Y, caps.VisionRange, entity, g.posMap,
		)

		// Compute sensors using precomputed neighbor data (avoids double distance calc)
		sensorInputs := systems.ComputeSensorsFromNeighbors(
			*vel, *rot, *energy, *caps, org.Kind, org.Diet,
			org.CladeID, org.FounderArchetypeID,
			g.neighborBuf,
			g.orgMap,
			g.resourceField,
			*pos,
		)

		// Fill reusable input buffer (avoids allocation)
		inputs := sensorInputs.FillSlice(g.inputBuf[:])

		// Run brain (capture activations if this is the selected entity)
		var turn, thrust float32
		if hasSelection && entity == selectedEntity {
			var act *neural.Activations
			turn, thrust, _, act = brain.ForwardWithCapture(inputs)
			g.inspector.SetSensorData(&sensorInputs)
			g.inspector.SetActivations(act)
		} else {
			turn, thrust, _ = brain.Forward(inputs)
		}

		// Scale outputs by capabilities
		turnRate := turn * caps.MaxTurnRate * dt
		if turnRate > caps.MaxTurnRate*dt {
			turnRate = caps.MaxTurnRate * dt
		} else if turnRate < -caps.MaxTurnRate*dt {
			turnRate = -caps.MaxTurnRate * dt
		}

		// Apply angular velocity to heading (heading-as-state)
		// Minimum turn rate (30%) even at zero throttle enables arrival behavior
		effectiveTurnRate := turnRate * max(thrust, 0.3)
		rot.Heading += effectiveTurnRate
		rot.Heading = normalizeAngle(rot.Heading)

		// Compute desired velocity from heading (use fast trig)
		targetSpeed := thrust * caps.MaxSpeed * dt
		desiredVelX := fastCos(rot.Heading) * targetSpeed
		desiredVelY := fastSin(rot.Heading) * targetSpeed

		// Smooth velocity change
		accelFactor := caps.MaxAccel * dt * 0.01
		vel.X += (desiredVelX - vel.X) * accelFactor
		vel.Y += (desiredVelY - vel.Y) * accelFactor

		// Apply drag (use fast exp)
		dragFactor := fastExp(-caps.Drag * dt)
		vel.X *= dragFactor
		vel.Y *= dragFactor

		// Clamp speed (use fast sqrt)
		speed := fastSqrt(vel.X*vel.X + vel.Y*vel.Y)
		maxSpeed := caps.MaxSpeed * dt
		if speed > maxSpeed {
			scale := maxSpeed / speed
			vel.X *= scale
			vel.Y *= scale
		}

		// Update position
		pos.X += vel.X
		pos.Y += vel.Y

		// Toroidal wrap
		pos.X = mod(pos.X, g.worldWidth)
		pos.Y = mod(pos.Y, g.worldHeight)
	}
}

// updateFeeding handles predator attacks (diet-scaled hunting).
func (g *Game) updateFeeding() {
	cfg := g.config()
	digestTime := float32(cfg.Energy.Predator.DigestTime)
	refugiaStrength := float32(cfg.Refugia.Strength)
	baseBiteReward := float32(cfg.Energy.Predator.BiteReward)

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Hunting efficiency based on diet (0 for diet < hunting_floor)
		huntEff := systems.HuntingEfficiency(org.Diet)
		if huntEff <= 0 {
			continue // Can't hunt with this diet
		}

		// Skip if still digesting from a previous kill
		if org.DigestCooldown > 0 {
			g.collector.RecordBiteBlockedByDigest()
			continue
		}

		// Skip if newborn predator still in hunt cooldown
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

			// Can only eat targets with lower diet (prey-like behavior)
			// Diet threshold: attacker must have significantly higher diet
			if nOrg.Diet >= org.Diet-0.2 {
				continue // Target has similar or higher diet, can't eat
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
					// Bite missed due to refugia protection
					g.collector.RecordBiteMissedRefugia()
					break // one attempt per tick
				}

				targetWasAlive := nEnergy.Alive

				// Bite reward scaled by hunting efficiency
				biteReward := baseBiteReward * huntEff
				xfer := systems.TransferEnergy(energy, nEnergy, biteReward)

				// Conservation accounting: detritus and heat
				if xfer.ToDet > 0 {
					g.resourceField.DepositDetritus(nPos.X, nPos.Y, xfer.ToDet)
				}
				if xfer.Overflow > 0 {
					g.resourceField.DepositDetritus(pos.X, pos.Y, xfer.Overflow)
				}
				g.heatLossAccum += xfer.ToHeat

				if xfer.Removed > 0 {
					// Record successful bite
					g.collector.RecordBiteHit()
					g.lifetimeTracker.RecordBiteHit(org.ID)

					// Check for kill
					if targetWasAlive && !nEnergy.Alive {
						g.collector.RecordKill()
						g.lifetimeTracker.RecordKill(org.ID)
						// Start digestion cooldown after a kill
						org.DigestCooldown = digestTime
					}
				}

				break // one bite per tick
			}
		}
	}
}

// updateEnergy applies metabolic costs and prey foraging with true depletion.
func (g *Game) updateEnergy() {
	cfg := g.config()
	dt := cfg.Derived.DT32
	grazeRadius := cfg.Resource.GrazeRadius
	forageEfficiency := float32(cfg.Resource.ForageEfficiency)
	forageRate := float32(cfg.Energy.Prey.ForageRate)

	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Grazing: diet determines grazing ability (diet=0 full grazer, diet>=cap no grazing)
		dietGrazeEff := systems.GrazingEfficiency(org.Diet)
		if dietGrazeEff > 0 {
			// Compute grazing efficiency based on speed
			speed := fastSqrt(vel.X*vel.X + vel.Y*vel.Y)
			speedRatio := speed / caps.MaxSpeed
			if speedRatio > 1 {
				speedRatio = 1
			}
			grazingPeak := float32(cfg.Energy.Prey.GrazingPeak)
			speedEff := 1.0 - 2.0*absf(speedRatio-grazingPeak)
			if speedEff < 0 {
				speedEff = 0
			}

			// Get resource level at position for grazing rate scaling
			resourceHere := g.resourceField.Sample(pos.X, pos.Y)
			grazeRate := resourceHere * forageRate * speedEff * dietGrazeEff

			// Graze: remove resource and get actual removed amount
			removed := g.resourceField.Graze(pos.X, pos.Y, grazeRate, dt, grazeRadius)

			// Energy gain = removed * eta_graze * diet grazing efficiency
			// Remainder is digestion heat loss
			gain := removed * forageEfficiency * dietGrazeEff
			grazingHeat := removed - gain
			g.heatLossAccum += grazingHeat

			energy.Value += gain

			// Route overflow to detritus (waste, not heat)
			if energy.Value > energy.Max {
				overflow := energy.Value - energy.Max
				energy.Value = energy.Max
				g.resourceField.DepositDetritus(pos.X, pos.Y, overflow)
			}

			// Track foraging for telemetry
			g.lifetimeTracker.RecordForage(org.ID, gain)
		}

		// Apply metabolic costs (diet-interpolated)
		systems.UpdateEnergy(energy, *vel, *caps, org.Diet, false, dt)
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
func (g *Game) updateReproduction() {
	cfg := g.config()
	repro := &cfg.Reproduction
	mutation := &cfg.Mutation
	cladeCfg := &cfg.Clades

	// Collect births to spawn after iteration
	type birthInfo struct {
		x, y, heading      float32
		parentBrain        *neural.FFNN
		parentID           uint32
		parentDiet         float32
		parentCladeID      uint64
		founderArchetypeID uint8
	}
	var births []birthInfo

	query := g.entityFilter.Query()
	for query.Next() {
		pos, _, rot, _, energy, _, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Check population caps
		if org.Kind == components.KindPrey && g.numPrey >= cfg.Population.MaxPrey {
			continue
		}
		if org.Kind == components.KindPredator && g.numPred >= cfg.Population.MaxPred {
			continue
		}

		// Check reproduction thresholds
		var threshold, cooldown float32
		if org.Kind == components.KindPredator {
			threshold = float32(repro.PredThreshold)
			cooldown = float32(repro.PredCooldown)
		} else {
			threshold = float32(repro.PreyThreshold)
			cooldown = float32(repro.PreyCooldown)
		}

		maturityAge := float32(repro.MaturityAge)
		if energy.Value < threshold || energy.Age < maturityAge || org.ReproCooldown > 0 {
			continue
		}

		// Density-dependent reproduction for predators
		// When prey are scarce, predators breed less (p = prey / (prey + K))
		if org.Kind == components.KindPredator && repro.PredDensityK > 0 {
			preyN := float32(g.numPrey)
			k := float32(repro.PredDensityK)
			breedProb := preyN / (preyN + k)
			if g.rng.Float32() > breedProb {
				continue // Skip reproduction this tick, but stay eligible
			}
		}

		// Reproduction: parent pays energy, child spawns nearby
		parentBrain, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Energy split
		energy.Value *= float32(repro.ParentEnergySplit)

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

		// Derive Kind from diet for backwards compatibility
		childKind := components.KindPrey
		if childDiet >= 0.5 {
			childKind = components.KindPredator
		}

		// Create child entity directly (not using spawnEntity since we need custom brain/clade)
		childID := g.nextID
		g.nextID++

		pos := components.Position{X: b.x, Y: b.y}
		vel := components.Velocity{X: 0, Y: 0}
		rot := components.Rotation{Heading: b.heading, AngVel: 0}
		body := components.Body{Radius: float32(cfg.Entity.BodyRadius)}
		childEnergy := components.Energy{Value: float32(repro.ChildEnergy), Max: float32(cfg.Entity.MaxEnergy), Age: 0, Alive: true}
		caps := components.DefaultCapabilities(childKind)
		cooldownJitter := (g.rng.Float32()*2.0 - 1.0) * float32(cfg.Reproduction.CooldownJitter)
		// Newborn predators can't hunt immediately
		huntCooldown := float32(0)
		if childKind == components.KindPredator {
			huntCooldown = float32(repro.NewbornHuntCooldown)
		}
		childOrg := components.Organism{
			ID:                 childID,
			Kind:               childKind,
			FounderArchetypeID: b.founderArchetypeID,
			Diet:               childDiet,
			CladeID:            childCladeID,
			ReproCooldown:      float32(cfg.Reproduction.MaturityAge) + cooldownJitter,
			HuntCooldown:       huntCooldown,
		}

		g.brains[childID] = childBrain
		g.entityMapper.NewEntity(&pos, &vel, &rot, &body, &childEnergy, &caps, &childOrg)
		g.aliveCount++

		// Register with lifetime tracker
		g.lifetimeTracker.Register(childID, g.tick, childCladeID, b.founderArchetypeID, childDiet)

		// Track population by kind
		if childKind == components.KindPrey {
			g.numPrey++
		} else {
			g.numPred++
		}

		// Record birth in telemetry
		g.collector.RecordBirth(childKind)
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

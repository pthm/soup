package game

import (
	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
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
	cfg := config.Cfg()
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
			*vel, *rot, *energy, *caps, org.Kind,
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
		pos.X = mod(pos.X, g.width)
		pos.Y = mod(pos.Y, g.height)
	}
}

// updateFeeding handles predator attacks.
func (g *Game) updateFeeding() {
	cfg := config.Cfg()
	digestTime := float32(cfg.Energy.Predator.DigestTime)
	refugiaStrength := float32(cfg.Refugia.Strength)

	query := g.entityFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, _, _, energy, caps, org := query.Get()

		if !energy.Alive || org.Kind != components.KindPredator {
			continue
		}

		// Skip if still digesting from a previous kill
		if org.DigestCooldown > 0 {
			g.collector.RecordBiteBlockedByDigest()
			continue
		}

		// Check if brain exists
		_, ok := g.brains[org.ID]
		if !ok {
			continue
		}

		// Query nearby prey within bite range
		neighbors := g.spatialGrid.QueryRadius(pos.X, pos.Y, caps.BiteRange, entity, g.posMap)

		for _, neighbor := range neighbors {
			// Get neighbor components
			nOrg := g.orgMap.Get(neighbor)
			if nOrg == nil || nOrg.Kind != components.KindPrey {
				continue
			}

			// Get prey position for refugia check
			nPos := g.posMap.Get(neighbor)
			if nPos == nil {
				continue
			}

			// Get prey energy directly via mapper
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

				preyWasAlive := nEnergy.Alive

				// Simple bite: transfer energy
				transferred := systems.TransferEnergy(energy, nEnergy, 0.1)

				if transferred > 0 {
					// Record successful bite
					g.collector.RecordBiteHit()
					g.lifetimeTracker.RecordBiteHit(org.ID)

					// Check for kill
					if preyWasAlive && !nEnergy.Alive {
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

// updateEnergy applies metabolic costs and prey foraging.
func (g *Game) updateEnergy() {
	dt := config.Cfg().Derived.DT32
	query := g.entityFilter.Query()
	for query.Next() {
		pos, vel, _, _, energy, caps, org := query.Get()

		if !energy.Alive {
			continue
		}

		// Prey gain energy from resource field
		if org.Kind == components.KindPrey {
			r := g.resourceField.Sample(pos.X, pos.Y)
			systems.UpdatePreyForage(energy, *vel, *caps, r, dt)
		}

		// Apply metabolic costs (per-kind)
		systems.UpdateEnergy(energy, *vel, *caps, org.Kind, false, dt)
	}
}

// updateCooldowns decrements reproduction and digestion cooldowns.
func (g *Game) updateCooldowns() {
	dt := config.Cfg().Derived.DT32
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
	}
}

// updateReproduction handles asexual reproduction with mutation.
func (g *Game) updateReproduction() {
	cfg := config.Cfg()
	repro := &cfg.Reproduction
	mutation := &cfg.Mutation

	// Collect births to spawn after iteration
	type birthInfo struct {
		x, y, heading float32
		kind          components.Kind
		parentBrain   *neural.FFNN
		parentID      uint32
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
		childX := mod(pos.X+(g.rng.Float32()-0.5)*offset*2, g.width)
		childY := mod(pos.Y+(g.rng.Float32()-0.5)*offset*2, g.height)
		headingJitter := float32(repro.HeadingJitter)
		childHeading := rot.Heading + (g.rng.Float32()-0.5)*headingJitter*2

		births = append(births, birthInfo{
			x:           childX,
			y:           childY,
			heading:     childHeading,
			kind:        org.Kind,
			parentBrain: parentBrain,
			parentID:    org.ID,
		})
	}

	// Spawn children outside query
	for _, b := range births {
		child := g.spawnEntity(b.x, b.y, b.heading, b.kind)
		childOrg := g.orgMap.Get(child)
		childEnergy := g.energyMap.Get(child)

		if childOrg != nil && childEnergy != nil {
			// Inherit mutated brain
			childBrain := b.parentBrain.Clone()
			childBrain.MutateSparse(g.rng,
				float32(mutation.Rate),
				float32(mutation.Sigma),
				float32(mutation.BigRate),
				float32(mutation.BigSigma))
			g.brains[childOrg.ID] = childBrain

			// Set child energy
			childEnergy.Value = float32(repro.ChildEnergy)
			childEnergy.Age = 0

			// Record birth in telemetry
			g.collector.RecordBirth(b.kind)
			g.lifetimeTracker.RecordChild(b.parentID)
		}
	}
}

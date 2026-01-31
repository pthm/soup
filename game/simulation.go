package game

import (
	"math/rand"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/systems"
)

// Update handles input and runs the simulation for graphics mode.
func (g *Game) Update() {
	// Handle input
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}
	if rl.IsKeyPressed(rl.KeyPeriod) {
		if g.stepsPerFrame < 10 {
			g.stepsPerFrame++
		}
	}
	if rl.IsKeyPressed(rl.KeyComma) {
		if g.stepsPerFrame > 1 {
			g.stepsPerFrame--
		}
	}

	// Left-click: select organism (or Shift+click to spawn fauna)
	if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
		if rl.IsKeyDown(rl.KeyLeftShift) || rl.IsKeyDown(rl.KeyRightShift) {
			// Shift+click: spawn neural fauna (diet derived from cells)
			pos := rl.GetMousePosition()
			g.createInitialNeuralOrganism(pos.X, pos.Y, 100)
		} else {
			// Regular click: select organism
			entity, found := g.findOrganismAtClick()
			if found {
				g.selectedEntity = entity
				g.hasSelection = true
			} else {
				g.hasSelection = false
			}
		}
	}
	if rl.IsKeyPressed(rl.KeyF) {
		g.createFloraLightweight(
			rand.Float32()*(g.bounds.Width-100)+50,
			g.bounds.Height-4,
			true, // isRooted
			80,
		)
	}
	if rl.IsKeyPressed(rl.KeyC) {
		// Add new fauna - diet derived from CPPN-generated cells
		g.createInitialNeuralOrganism(
			rand.Float32()*(g.bounds.Width-100)+50,
			rand.Float32()*(g.bounds.Height-150)+50,
			120,
		)
	}

	// Toggle neural stats panel
	if rl.IsKeyPressed(rl.KeyN) {
		g.showNeuralStats = !g.showNeuralStats
	}

	// Toggle overlay controls panel
	if rl.IsKeyPressed(rl.KeyO) {
		g.uiControlsPanel.Toggle()
	}

	// Handle overlay toggles via registry
	g.handleOverlayKeys()

	if g.paused {
		return
	}

	// Run simulation steps
	for step := 0; step < g.stepsPerFrame; step++ {
		g.runSimulationStep()
	}
}

// UpdateHeadless runs simulation without any input handling or graphics.
func (g *Game) UpdateHeadless() {
	for step := 0; step < g.stepsPerFrame; step++ {
		g.runSimulationStep()
	}
}

// runSimulationStep executes a single simulation tick.
func (g *Game) runSimulationStep() {
	g.tick++

	// Helper to time a function
	measure := func(name string, fn func()) {
		if g.perfLog {
			start := time.Now()
			fn()
			g.perf.Record(name, time.Since(start))
		} else {
			fn()
		}
	}

	// Update day/night cycle
	measure("dayNight", func() { g.updateDayNightCycle() })

	// Update GPU flow field texture (if available)
	if g.gpuFlowField != nil {
		measure("gpuFlow", func() {
			g.gpuFlowField.Update(g.tick, float32(g.tick)*0.016) // ~60fps time
		})
	}

	// Update flow field particles (visual, independent)
	measure("flowField", func() { g.flowField.Update(g.tick) })

	// Update shadow map (foundation for light-based systems)
	var occluders []systems.Occluder
	measure("collectOccluders", func() {
		occluders = g.collectOccluders()
	})
	measure("shadowMap", func() {
		sunX := g.light.PosX * g.bounds.Width
		sunY := g.light.PosY * g.bounds.Height
		g.shadowMap.Update(g.tick, sunX, sunY, occluders)
	})

	// Collect position data for behavior system
	var floraPos, faunaPos []components.Position
	var floraOrgs, faunaOrgs []*components.Organism
	measure("collectPositions", func() {
		floraPos, faunaPos = g.collectPositions()
		floraOrgs, faunaOrgs = g.collectOrganisms()
	})

	// Update spatial grid for O(1) neighbor lookups
	measure("spatialGrid", func() { g.spatialGrid.Update(floraPos, faunaPos) })

	// Update allocation modes (determines how organisms spend energy)
	measure("allocation", func() { g.allocation.Update(floraPos, faunaPos, floraOrgs, faunaOrgs) })

	// Run systems (parallel brain evaluation)
	measure("behavior", func() { g.behavior.UpdateParallel(g.world, g.bounds, floraPos, faunaPos, floraOrgs, faunaOrgs, g.spatialGrid) })
	measure("physics", func() { g.physics.Update(g.world) })
	measure("feeding", func() { g.feeding.Update() })
	measure("floraSystem", func() {
		g.floraSystem.UpdateParallel(g.tick, func(x, y float32, isRooted bool) {
			g.spores.SpawnSpore(x, y, isRooted)
		})
	})
	measure("energy", func() { g.energy.Update(g.world) })

	// Breeding (fauna reproduction - flora don't use breeding system anymore)
	measure("breeding", func() { g.breeding.Update(g.world, nil, g.CreateNeuralOrganismForBreeding) })

	// Spores (germinates into new flora via FloraSystem)
	measure("spores", func() {
		g.spores.Update(g.tick, func(x, y float32, isRooted bool, energy float32) ecs.Entity {
			g.createFloraLightweight(x, y, isRooted, energy)
			return ecs.Entity{} // Return zero entity, not used
		})
	})

	// Death particles (fauna growth removed - evolution via breeding only)
	measure("death_particles", func() { g.updateDeathParticles() })

	// Effect particles
	measure("particles", func() { g.particles.Update() })

	// Cleanup
	measure("cleanup", func() { g.cleanupDead() })

	// Periodic logging
	if g.logInterval > 0 && g.tick%int32(g.logInterval) == 0 {
		g.logWorldState()
	}

	// Performance logging (every 120 ticks = ~2 seconds at 1x speed)
	if g.perfLog && g.tick%120 == 0 {
		g.logPerfStats()
	}

	// Neural evolution logging (every 500 ticks = ~8 seconds at 1x speed)
	if g.neuralLog && g.tick%500 == 0 {
		g.logNeuralStats()
	}
}

// updateDayNightCycle keeps light at constant intensity (day/night cycle disabled).
func (g *Game) updateDayNightCycle() {
	// Static sun at top center with constant full intensity
	g.light.PosX = 0.5
	g.light.Intensity = 1.0
}

// updateDeathParticles emits particles for dead organisms.
// Fauna growth has been removed - organisms are born with their cells and don't grow.
// Evolution happens through breeding, not individual growth.
func (g *Game) updateDeathParticles() {
	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, _ := query.Get()
		if org.Dead {
			g.particles.EmitDeath(pos.X, pos.Y)
		}
	}
}

// cleanupDead removes dead organisms and updates fitness tracking.
func (g *Game) cleanupDead() {
	const maxDeadTime = 600 // Remove after ~10 seconds at normal speed

	// Collect entities to remove (can't modify during query)
	var toRemove []ecs.Entity

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, org, _ := query.Get()

		if org.Dead {
			// On first death tick, remove from species and record fitness
			if org.DeadTime == 0 && g.neuralGenomeMap.Has(entity) {
				neuralGenome := g.neuralGenomeMap.Get(entity)
				if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
					// Calculate final fitness before removal
					fitness := neural.CalculateFitness(org.Energy, org.MaxEnergy, g.tick, 0)
					g.speciesManager.AccumulateFitness(neuralGenome.SpeciesID, fitness)
					g.speciesManager.RemoveMember(neuralGenome.SpeciesID, int(entity.ID()))

					// Log death event
					if g.neuralLog && g.neuralLogDetail {
						g.logDeathEvent(uint64(entity.ID()), neuralGenome.Generation, neuralGenome.SpeciesID, fitness, g.tick)
					}
				}
			}

			org.DeadTime++
			if org.DeadTime > maxDeadTime {
				toRemove = append(toRemove, entity)
			}
		} else if g.neuralGenomeMap.Has(entity) {
			// Periodically update fitness for living organisms (every 100 ticks)
			if g.tick%100 == 0 {
				neuralGenome := g.neuralGenomeMap.Get(entity)
				if neuralGenome != nil && neuralGenome.SpeciesID > 0 {
					fitness := neural.CalculateFitness(org.Energy, org.MaxEnergy, g.tick, 0)
					g.speciesManager.AccumulateFitness(neuralGenome.SpeciesID, fitness)
				}
			}
		}
	}

	// Remove dead entities
	for _, e := range toRemove {
		g.world.RemoveEntity(e)
	}

	// Update generations periodically (every 3000 ticks ≈ 50 seconds at normal speed)
	if g.tick%3000 == 0 && g.tick > 0 {
		g.speciesManager.EndGeneration()
	}
}

// collectPositions collects positions of all flora and fauna.
func (g *Game) collectPositions() ([]components.Position, []components.Position) {
	var floraPos, faunaPos []components.Position

	// Collect flora positions from FloraSystem
	allFlora := g.floraSystem.GetAllFlora()
	for _, ref := range allFlora {
		floraPos = append(floraPos, components.Position{X: ref.X, Y: ref.Y})
	}

	// Collect fauna positions from ECS
	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		pos, _, _ := faunaQuery.Get()
		faunaPos = append(faunaPos, *pos)
	}

	return floraPos, faunaPos
}

// collectOccluders collects shadow-casting occluders.
// Note: Terrain shadows are skipped for performance - terrain is rendered with baked-in depth.
func (g *Game) collectOccluders() []systems.Occluder {
	// Skip terrain occluders for performance - they generate thousands of shadows
	// Terrain already has depth-based darkening baked into its texture
	var occluders []systems.Occluder

	// Add floating flora occluders only (fauna don't cast shadows)
	// Rooted flora don't cast shadows - they're attached to terrain which already casts shadows
	for i := range g.floraSystem.Floating {
		f := &g.floraSystem.Floating[i]
		if f.Dead {
			continue
		}

		// Simple bounding box based on flora size
		size := f.Size * 3
		occluders = append(occluders, systems.Occluder{
			X:       f.X - size/2,
			Y:       f.Y - size/2,
			Width:   size,
			Height:  size,
			Density: 0.08, // Very sparse foliage - minimal shadow
		})
	}

	return occluders
}

// collectOrganisms collects organism pointers for behavior/allocation systems.
func (g *Game) collectOrganisms() ([]*components.Organism, []*components.Organism) {
	// Note: This function is legacy - flora are no longer ECS organisms.
	// We return empty flora slice since FloraSystem handles flora now.
	// Behavior/allocation systems that need flora data should use FloraSystem directly.
	var faunaOrgs []*components.Organism

	faunaQuery := g.faunaFilter.Query()
	for faunaQuery.Next() {
		_, org, _ := faunaQuery.Get()
		faunaOrgs = append(faunaOrgs, org)
	}

	// Return nil for floraOrgs since flora are now managed by FloraSystem
	return nil, faunaOrgs
}

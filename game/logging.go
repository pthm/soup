package game

import (
	"fmt"
	"io"
	"math"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/pthm-cable/soup/components"
)

// logWriter is the destination for log output.
var logWriter io.Writer

// SetLogWriter sets the log output destination.
func SetLogWriter(w io.Writer) {
	logWriter = w
}

// Logf writes a formatted log message.
func Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if logWriter != nil {
		fmt.Fprintln(logWriter, msg)
	} else {
		fmt.Println(msg)
	}
}

// logPerfStats logs performance statistics.
func (g *Game) logPerfStats() {
	total := g.perf.Total()
	fps := rl.GetFPS()
	Logf("=== Perf @ Tick %d (speed %dx) | FPS: %d ===", g.tick, g.stepsPerFrame, fps)
	Logf("Total step time: %s", total.Round(time.Microsecond))

	for _, name := range g.perf.SortedNames() {
		avg := g.perf.Avg(name)
		pct := float64(0)
		if total > 0 {
			pct = float64(avg) / float64(total) * 100
		}
		Logf("  %-18s %10s  %5.1f%%", name, avg.Round(time.Microsecond), pct)
	}

	// Log behavior subsystem breakdown
	behaviorStats := g.behavior.GetPerfStats()
	if behaviorStats.Count > 0 {
		Logf("  --- Behavior Subsystems (per %d organisms) ---", behaviorStats.Count)
		totalBehaviorNs := behaviorStats.EntityListNs + behaviorStats.VisionNs +
			behaviorStats.BrainNs + behaviorStats.PathfindingNs + behaviorStats.ActuatorNs

		logSubsystem := func(name string, ns int64) {
			dur := time.Duration(ns)
			pct := float64(0)
			if totalBehaviorNs > 0 {
				pct = float64(ns) / float64(totalBehaviorNs) * 100
			}
			perOrg := time.Duration(ns / int64(behaviorStats.Count))
			Logf("    %-16s %10s  %5.1f%% (%s/org)", name, dur.Round(time.Microsecond), pct, perOrg.Round(time.Microsecond))
		}

		logSubsystem("entityList", behaviorStats.EntityListNs)
		logSubsystem("vision", behaviorStats.VisionNs)
		logSubsystem("brain", behaviorStats.BrainNs)
		logSubsystem("pathfinding", behaviorStats.PathfindingNs)
		logSubsystem("actuator", behaviorStats.ActuatorNs)
	}
	Logf("")
}

// logWorldState logs the current world state.
func (g *Game) logWorldState() {
	var faunaCount, deadCount int
	var faunaEnergy float32
	var herbivoreCount, carnivoreCount, carrionCount int
	var minFaunaEnergy, maxFaunaEnergy float32 = 9999, 0
	var totalFaunaCells int

	query := g.allOrgFilter.Query()
	for query.Next() {
		_, _, org, cells := query.Get()

		if org.Dead {
			deadCount++
			continue
		}

		// All ECS organisms are fauna (flora are in FloraSystem)
		faunaCount++
		faunaEnergy += org.Energy
		totalFaunaCells += int(cells.Count)
		if org.Energy < minFaunaEnergy {
			minFaunaEnergy = org.Energy
		}
		if org.Energy > maxFaunaEnergy {
			maxFaunaEnergy = org.Energy
		}

		// Count by diet based on cell capabilities
		caps := cells.ComputeCapabilities()
		digestiveSpectrum := caps.DigestiveSpectrum()
		if digestiveSpectrum < 0.35 {
			herbivoreCount++
		} else if digestiveSpectrum > 0.65 {
			carnivoreCount++
		} else {
			carrionCount++ // Using carrion slot for omnivores
		}
	}

	// Get flora stats from FloraSystem
	floraCount := g.floraSystem.TotalCount()
	floraEnergy := g.floraSystem.TotalEnergy()
	avgFloraEnergy := float32(0)
	if floraCount > 0 {
		avgFloraEnergy = floraEnergy / float32(floraCount)
	}

	avgFaunaEnergy := float32(0)
	if faunaCount > 0 {
		avgFaunaEnergy = faunaEnergy / float32(faunaCount)
		if avgFaunaEnergy != avgFaunaEnergy { // NaN check
			avgFaunaEnergy = 0
		}
	}
	if minFaunaEnergy == 9999 {
		minFaunaEnergy = 0
	}

	Logf("=== Tick %d ===", g.tick)
	Logf("Flora: %d (rooted: %d, floating: %d, energy: %.1f avg)",
		floraCount, g.floraSystem.RootedCount(), g.floraSystem.FloatingCount(), avgFloraEnergy)
	Logf("Fauna: %d (cells: %d, energy: %.1f avg, %.1f-%.1f range)",
		faunaCount, totalFaunaCells, avgFaunaEnergy, minFaunaEnergy, maxFaunaEnergy)
	Logf("  Herbivores: %d, Carnivores: %d, Carrion: %d",
		herbivoreCount, carnivoreCount, carrionCount)
	Logf("Dead: %d, Spores: %d, Particles: %d",
		deadCount, g.spores.Count(), g.particles.Count())

	// Count breeding-eligible fauna
	breedingEligible := 0
	var modeBreed, modeSurvive, modeStore int
	var omnivores int
	var drifters, generalists, apex int
	var totalDrag float32
	var shapeCount int
	query2 := g.allOrgFilter.Query()
	for query2.Next() {
		_, _, org, cells := query2.Get()
		// All ECS organisms are fauna (flora are in FloraSystem)
		if org.Dead {
			continue
		}
		// Count allocation modes
		switch org.AllocationMode {
		case components.ModeBreed:
			modeBreed++
		case components.ModeSurvive:
			modeSurvive++
		case components.ModeStore:
			modeStore++
		}
		// Count omnivores based on cell digestive spectrum
		caps := cells.ComputeCapabilities()
		digestiveSpectrum := caps.DigestiveSpectrum()
		if digestiveSpectrum >= 0.35 && digestiveSpectrum <= 0.65 {
			omnivores++
		}
		if org.AllocationMode == components.ModeBreed && org.Energy >= org.MaxEnergy*0.35 && cells.Count >= 1 && org.BreedingCooldown == 0 {
			breedingEligible++
		}
		// Count organism classes by size
		cellCount := int(cells.Count)
		switch {
		case cellCount <= 3:
			drifters++
		case cellCount <= 10:
			generalists++
		default:
			apex++
		}
		// Accumulate shape metrics
		totalDrag += org.ShapeMetrics.Drag
		shapeCount++
	}

	avgDrag := float32(0)
	if shapeCount > 0 {
		avgDrag = totalDrag / float32(shapeCount)
	}

	Logf("Breeding eligible: %d", breedingEligible)
	Logf("Modes: Breed=%d, Survive=%d, Store=%d", modeBreed, modeSurvive, modeStore)
	Logf("Omnivores: %d", omnivores)
	Logf("Classes: Drifters=%d, Generalists=%d, Apex=%d | Avg Drag=%.2f",
		drifters, generalists, apex, avgDrag)
	Logf("")
}

// logNeuralStats logs neural evolution statistics.
func (g *Game) logNeuralStats() {
	stats := g.speciesManager.GetStats()
	topSpecies := g.speciesManager.GetTopSpecies(10)

	Logf("╔══════════════════════════════════════════════════════════════════╗")
	Logf("║ NEURAL EVOLUTION @ Tick %d (Gen %d)                              ", g.tick, stats.Generation)
	Logf("╠══════════════════════════════════════════════════════════════════╣")
	Logf("║ Species: %d | Total Members: %d | Best Fitness: %.2f",
		stats.Count, stats.TotalMembers, stats.BestFitness)
	Logf("║ Total Offspring: %d | Avg Staleness: %.1f",
		stats.TotalOffspring, stats.AverageStaleness)

	// Count neural organisms
	neuralCount := 0
	var totalNodes, totalGenes int
	var minNodes, maxNodes int = 9999, 0
	var minGenes, maxGenes int = 9999, 0

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		_, _, org, _ := query.Get()

		if org.Dead {
			continue
		}

		if g.neuralGenomeMap.Has(entity) {
			neuralCount++
			neuralGenome := g.neuralGenomeMap.Get(entity)
			if neuralGenome != nil && neuralGenome.BrainGenome != nil {
				nodes := len(neuralGenome.BrainGenome.Nodes)
				genes := len(neuralGenome.BrainGenome.Genes)
				totalNodes += nodes
				totalGenes += genes

				if nodes < minNodes {
					minNodes = nodes
				}
				if nodes > maxNodes {
					maxNodes = nodes
				}
				if genes < minGenes {
					minGenes = genes
				}
				if genes > maxGenes {
					maxGenes = genes
				}
			}
		}
	}

	if minNodes == 9999 {
		minNodes = 0
	}
	if minGenes == 9999 {
		minGenes = 0
	}

	avgNodes := 0.0
	avgGenes := 0.0
	if neuralCount > 0 {
		avgNodes = float64(totalNodes) / float64(neuralCount)
		avgGenes = float64(totalGenes) / float64(neuralCount)
	}

	Logf("╠══════════════════════════════════════════════════════════════════╣")
	Logf("║ Neural Organisms: %d", neuralCount)
	Logf("║ Brain Complexity:")
	Logf("║   Nodes: avg=%.1f, min=%d, max=%d", avgNodes, minNodes, maxNodes)
	Logf("║   Genes: avg=%.1f, min=%d, max=%d", avgGenes, minGenes, maxGenes)

	if len(topSpecies) > 0 {
		Logf("╠══════════════════════════════════════════════════════════════════╣")
		Logf("║ TOP SPECIES:")
		for i, sp := range topSpecies {
			Logf("║   #%d: Species %d - %d members, age=%d, stale=%d, fit=%.1f, offspring=%d",
				i+1, sp.ID, sp.Size, sp.Age, sp.Staleness, sp.BestFit, sp.Offspring)
		}
	}

	// Detailed per-organism logging if enabled
	if g.neuralLogDetail {
		Logf("╠══════════════════════════════════════════════════════════════════╣")
		Logf("║ DETAILED ORGANISM DATA (sample of 10):")

		count := 0
		query2 := g.allOrgFilter.Query()
		for query2.Next() {
			if count >= 10 {
				continue // Must consume entire query to release world lock
			}

			entity := query2.Entity()
			pos, vel, org, cells := query2.Get()

			if org.Dead || !g.neuralGenomeMap.Has(entity) {
				continue
			}

			neuralGenome := g.neuralGenomeMap.Get(entity)
			if neuralGenome == nil || neuralGenome.BrainGenome == nil {
				continue
			}

			// Calculate speed from velocity
			speed := float32(0)
			if vel != nil {
				speed = float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
			}

			// Mode name for readability
			modeName := "SURVIVE"
			switch org.AllocationMode {
			case components.ModeGrow:
				modeName = "GROW"
			case components.ModeBreed:
				modeName = "BREED"
			}

			Logf("║   Entity %d @ (%.0f,%.0f) v=(%.1f,%.1f) spd=%.1f mode=%s: species=%d, gen=%d, cells=%d, energy=%.0f/%.0f",
				entity.ID(), pos.X, pos.Y, vel.X, vel.Y, speed, modeName,
				neuralGenome.SpeciesID, neuralGenome.Generation,
				cells.Count, org.Energy, org.MaxEnergy)
			count++
		}
	}

	Logf("╚══════════════════════════════════════════════════════════════════╝")
	Logf("")
}

// logBirthEvent logs a birth event for neural organisms.
func (g *Game) logBirthEvent(entityID uint64, x, y float32, generation int, nodes, genes, speciesID int, parentSpeciesID int) {
	speciated := ""
	if speciesID != parentSpeciesID {
		speciated = fmt.Sprintf(" [SPECIATED: %d->%d]", parentSpeciesID, speciesID)
	}
	Logf("[BIRTH] Entity %d @ (%.0f,%.0f): gen=%d, nodes=%d, genes=%d, species=%d%s",
		entityID, x, y, generation, nodes, genes, speciesID, speciated)
}

// logDeathEvent logs a death event for neural organisms.
func (g *Game) logDeathEvent(entityID uint64, generation, speciesID int, fitness float64, survivalTicks int32) {
	Logf("[DEATH] Entity %d: gen=%d, species=%d, fitness=%.2f, survived=%d ticks",
		entityID, generation, speciesID, fitness, survivalTicks)
}

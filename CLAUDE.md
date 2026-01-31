# CLAUDE.md

Real-time artificial life simulation with NEAT-evolved neural organisms.

## Quick Start

```bash
go build . && ./soup                    # Graphics mode
./soup -headless -neural -max-ticks=50000  # Fast evolution run
go test ./...                           # Run tests
```

## Architecture

```
game/       Main loop, entity factory, UI
components/ ECS components (Position, Organism, Brain, CellBuffer)
systems/    ECS systems (Physics, Behavior, Feeding, Energy, Breeding)
neural/     NEAT brains, CPPN morphology, speciation
renderer/   Shaders (water, sun, terrain)
ui/         HUD, controls, overlays
```

### ECS (mlange-42/ark)

```go
// Create entities with typed mappers
entity := g.faunaMapper.NewEntity(&pos, &vel, &org, &cells, &components.Fauna{})
g.neuralGenomeMap.Add(entity, neuralGenome)

// Query with typed filters
query := g.faunaFilter.Query()
for query.Next() {
    pos, org, cells := query.Get()
}
```

### Neural System

**CPPN** (queried once at birth):
- Inputs (5): x, y, distance, angle, bias
- Outputs (12): presence, sensor, actuator, mouth, digestive, photosynthetic, bioluminescent, armor, storage, reproductive, brain_weight, brain_leo

**Brain** (queried every tick via `BrainController.Think()`):
- Inputs (26): self (2) + body descriptor (6) + boid fields (9) + food fields (6) + threat (2) + bias (1)
- Outputs (4): UFwd, UUp, AttackIntent, MateIntent

**Input Structure (26 inputs)**:
| Index | Name | Range | Description |
|-------|------|-------|-------------|
| 0-1 | Self | [0,1] | speed_norm, energy_norm |
| 2-7 | Body Descriptor | [0,1] | size, speed_capacity, agility, sense, bite, armor |
| 8-16 | Boid Fields | varies | cohesion (3), alignment (2), separation (3), density (1) |
| 17-22 | Food Fields | varies | plant (3), meat (3) |
| 23-24 | Threat | varies | proximity, closing_speed |
| 25 | Bias | 1.0 | constant |

**Output Structure (4 outputs)**:
| Index | Name | Range | Description |
|-------|------|-------|-------------|
| 0 | UFwd | [-1,1] | Desired forward velocity |
| 1 | UUp | [-1,1] | Desired lateral velocity |
| 2 | AttackIntent | [0,1] | Predation gate (>0.5 = attack) |
| 3 | MateIntent | [0,1] | Mating gate (>0.5 = ready) |

### Cell Types

| Type | Function |
|------|----------|
| Sensor | Perception radius/quality |
| Actuator | Movement thrust |
| Mouth | Feeding ability |
| Digestive | Diet spectrum (0=herbivore, 1=carnivore) |
| Photosynthetic | Energy from light |
| Bioluminescent | Light emission |
| Reproductive | Breeding capability |

### Energy Model

Brain outputs drive energy costs:
- **Movement**: UFwd + UUp magnitude + ActiveThrust (main pressure)
- **Attack**: Body-scaled cost when AttackIntent > 0.5
- **Base**: ~0.0005/cell/tick (photosynthesis can offset up to 95%)

Death occurs when energy <= 0.

### Feeding Mechanics

**Herbivory** (implicit):
- Automatic when near flora and digestiveSpectrum < 0.7
- No brain output required

**Predation** (explicit):
- Requires AttackIntent > 0.5 from brain
- Body-scaled range, damage, and cost
- Attack cooldown (30 ticks)

### Mating Mechanics

- Requires MateIntent > 0.5 from brain
- Surface-to-surface proximity check using body radii
- Contact margin of 5 world units

### Key Files

| File | Purpose |
|------|---------|
| `game/factory.go` | Entity creation, `createNeuralOrganism()` |
| `game/simulation.go` | System update order, `runSimulationStep()` |
| `neural/brain.go` | `BrainController.Think()`, genome creation |
| `neural/inputs.go` | `SensoryInputs`, `BehaviorOutputs`, I/O mapping |
| `neural/morphology.go` | CPPN evaluation, cell generation |
| `neural/reproduction.go` | Crossover, mutation |
| `systems/behavior.go` | Polar vision, brain evaluation |
| `systems/energy.go` | Energy costs, photosynthesis |
| `systems/breeding.go` | Mating, offspring creation |
| `components/components.go` | All component definitions |

### System Update Order

1. Day/night, flow field, shadow map (environment)
2. Spatial grid (neighbor lookups)
3. Allocation, behavior (AI)
4. Physics
5. Feeding, energy, breeding
6. Cleanup

## Flags

| Flag | Description |
|------|-------------|
| `-headless` | No graphics |
| `-neural` | Log evolution stats (every 500 ticks) |
| `-neural-detail` | Log birth/death events |
| `-speed=N` | Simulation speed 1-10 |
| `-max-ticks=N` | Auto-stop |
| `-perf` | Performance logging |

## NEAT Tuning

Key parameters in `neural/config.go`:

```go
MutateAddNodeProb: 0.10      // Structural complexity
MutateAddLinkProb: 0.15      // Connection density
CompatThreshold:   1.2       // Lower = more species
```

Target: 3-10 species, brain nodes growing from 25 toward 35+, population 100-300.

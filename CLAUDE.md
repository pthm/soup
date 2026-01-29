# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Primordial Soup is a real-time artificial life simulation written in Go. It features neural network-controlled organisms that evolve using NEAT (NeuroEvolution of Augmenting Topologies). Organisms compete for resources, reproduce with genetic crossover and mutation, and speciate over generations.

**Key Technologies:**
- **Go 1.25** with ECS architecture (mlange-42/ark)
- **raylib-go** for graphics rendering
- **goNEAT** for neural network evolution

## Build and Run Commands

```bash
# Build
go build .

# Run with graphics
./soup

# Run headless for evolution analysis
./soup -headless -neural -max-ticks=10000

# Fast headless run with detailed logging
./soup -headless -neural -neural-detail -speed=10 -max-ticks=50000 -logfile=evolution.log

# Run tests
go test ./...
go test ./neural/...
```

### Command-Line Flags

| Flag | Description |
|------|-------------|
| `-headless` | No graphics (faster simulation) |
| `-neural` | Enable neural evolution logging (every 500 ticks) |
| `-neural-detail` | Log individual birth/death events |
| `-speed=N` | Simulation speed 1-10 |
| `-max-ticks=N` | Stop after N ticks |
| `-logfile=FILE` | Write logs to file |
| `-log=N` | Log world state every N ticks |
| `-perf` | Enable performance logging |

### In-Game Controls

- **Space**: Pause/Resume
- **< >**: Decrease/Increase simulation speed
- **Click**: Add herbivore at cursor
- **F**: Add flora
- **C**: Add carnivore
- **S**: Toggle species coloring
- **N**: Toggle neural stats panel

## Architecture

### Package Structure

```
soup/
├── main.go           # Game loop, entity creation, UI rendering
├── components/       # ECS components (Position, Organism, Brain, etc.)
├── systems/          # ECS systems (Physics, Behavior, Feeding, etc.)
├── neural/           # NEAT implementation (brain, CPPN morphology, speciation)
├── traits/           # Organism traits and mutations
├── renderer/         # Shader-based rendering (water, sun, particles)
└── config/           # YAML configuration for NEAT parameters
```

### Neural Evolution System

Each fauna organism has two neural genomes (goNEAT `genetics.Genome`):

1. **Body Genome (CPPN)**: Queried once at birth to generate cell layout/morphology
   - Inputs: x, y, distance, angle, bias (5 total)
   - Outputs: cell presence, diet bias, traits, priority (4 total)

2. **Brain Genome**: Queried every tick to control behavior
   - Inputs (14): food distance/angle, predator distance/angle, mate distance, herd density, light level, flow field, energy ratio, cell count, bias
   - Outputs (9): seek food, flee, seek mate, herd, wander, grow, breed, conserve, speed

### Key Types

- `components.Organism`: Energy, traits, shape metrics, allocation mode
- `components.NeuralGenome`: Stores body and brain goNEAT genomes
- `components.Brain`: Runtime neural network controller
- `neural.BrainController`: Wraps goNEAT network evaluation
- `neural.SpeciesManager`: NEAT speciation and fitness tracking
- `traits.Trait`: Bitmask for organism characteristics (Herbivore, Carnivore, Speed, etc.)

### ECS Patterns

Uses mlange-42/ark ECS with typed mappers and filters:

```go
// Creating entities with components
faunaMapper := ecs.NewMap5[Position, Velocity, Organism, CellBuffer, Fauna](world)
entity := faunaMapper.NewEntity(&pos, &vel, &org, &cells, &Fauna{})

// Adding optional components after creation
neuralGenomeMap.Add(entity, neuralGenome)
brainMap.Add(entity, brain)

// Querying entities
query := faunaFilter.Query()
for query.Next() {
    pos, org, _ := query.Get()
    // process...
}
```

### System Update Order

1. Day/night cycle
2. Flow field particles
3. Shadow map (for photosynthesis)
4. Spatial grid (O(1) neighbor lookups)
5. Allocation mode (energy priorities)
6. Behavior (neural network steering)
7. Physics
8. Feeding
9. Photosynthesis
10. Disease spread
11. Energy consumption
12. Cell aging
13. Breeding (NEAT crossover/mutation)
14. Spores (flora reproduction)
15. Growth
16. Cleanup dead entities

## Tuning Neural Evolution

Key parameters in `neural/config.go` (`DefaultNEATOptions()`):

```go
MutateAddNodeProb: 0.10      // Structural complexity growth
MutateAddLinkProb: 0.15      // Connection density
CompatThreshold:   2.3       // Lower = more species
```

Fitness is calculated in `neural/species.go` as: `energyRatio * survivalBonus * reproBonus`

Target metrics for healthy evolution:
- 3-10 coexisting species
- Brain nodes growing from 23 (14 inputs + 9 outputs) toward 30+
- Stable population of 100-300

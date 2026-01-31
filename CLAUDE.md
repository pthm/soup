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
- Inputs (30): self (2) + body descriptor (6) + boid fields (9) + food fields (6) + threat (2) + approach (4) + bias (1)
- Outputs (4): UTurn, UThrottle, AttackIntent, MateIntent
- Hidden layer: 16 nodes (4x4 grid), CPPN-determined connectivity

**Input Structure (30 inputs)**:
| Index | Name | Range | Description |
|-------|------|-------|-------------|
| 0-1 | Self | [0,1] | speed_norm, energy_norm |
| 2-7 | Body Descriptor | [0,1] | size, speed_capacity, agility, sense, bite, armor |
| 8-16 | Boid Fields | varies | cohesion (3), alignment (2), separation (3), density (1) |
| 17-22 | Food Fields | varies | plant (3), meat (3) |
| 23-24 | Threat | varies | proximity, closing_speed |
| 25-28 | Approach | varies | nearest_food_dist, nearest_food_bearing, nearest_mate_dist, nearest_mate_bearing |
| 29 | Bias | 1.0 | constant |

**Output Structure (4 outputs)**:
| Index | Name | Range | Description |
|-------|------|-------|-------------|
| 0 | UTurn | [-1,1] | Turn rate (heading-as-state) |
| 1 | UThrottle | [0,1] | Forward throttle (no reverse) |
| 2 | AttackIntent | [0,1] | Predation gate (>0.5 = attack) |
| 3 | MateIntent | [0,1] | Mating gate (>0.5 = ready) |

**Heading-as-State Model**:
- Heading is brain-controlled state, not derived from velocity
- `heading += UTurn * TurnRateMax * max(UThrottle, 0.3)` (TurnRateMax = 0.15 rad/tick)
- Minimum turn rate (30%) even at zero throttle enables arrival behavior
- Desired velocity = `(cos(heading), sin(heading)) * UThrottle * maxSpeed`
- Eliminates velocity-heading feedback loop that caused jittering

**Morphology-Based Movement**:
- Actuator placement affects turning and thrust effectiveness
- **Turn effectiveness**: Asymmetric actuators favor one turn direction
  - Left-side actuators (GridY > 0) → better right turns
  - Right-side actuators (GridY < 0) → better left turns
  - ±50% turn rate bonus/penalty at maximum asymmetry
- **Thrust effectiveness**: Rear actuators provide better forward thrust
  - Rear actuators (GridX < 0) → up to +40% thrust bonus
  - Front actuators (GridX > 0) → reduced thrust
- Fish-like body plans (rear actuators) naturally swim faster forward

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
- **Movement**: UThrottle² × (1 + |UTurn| × 0.3) + ActiveThrust + jitter penalty (main pressure)
- Sharp turns while moving cost 30% extra at max turn rate
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

**Diet Compatibility** (`compat^k` power law):
- Nutrition rewards use `penetration^3` (CompatK=3.0)
- Creates sharper dietary niches: specialists get 8x better returns than generalists
- Penetration = Edibility - Armor (linear), then cubed for rewards

**Predator Interference**:
- Multiple attackers on same target share rewards
- Crowd penalty: 20% reduction per extra attacker beyond 2
- Maximum 70% penalty (30% minimum reward)
- Prevents swarm exploits while allowing pack hunting

### Mating Mechanics (Dwell-Time Handshake)

Sexual reproduction requires sustained contact, not instant mating:

- **Intent gate**: Both organisms must have MateIntent > 0.5
- **Energy threshold**: Both must maintain energy > 35% throughout
- **Contact proximity**: Surface-to-surface distance ≤ 5 world units
- **Dwell time**: Must sustain all conditions for 60 ticks
- **Partner locking**: Organisms track partner ID during handshake
- **Failure reset**: If any condition fails, progress resets to zero
- **Cooldown**: 120 ticks after successful mating (150 for asexual)

### World Space

The simulation uses **toroidal space** - all edges wrap around. Entities leaving one side appear on the opposite side.

**Unified Flow Field**:
- GPU-computed via shader (`shaders/flowfield.fs`)
- Simplex noise → angle → velocity vector
- 128x128 texture, updated every 30 ticks
- Sampled by flora, fauna, and flow particles
- `GPUFlowField.Sample(x, y)` for fast O(1) lookups

### Flora System

Flora are lightweight food sources managed outside the ECS. All flora float and drift with the unified flow field.

**Flora Lifecycle**:
- Energy gain: 0.15/tick (passive)
- Max energy: 150
- Death: Below 10% max energy
- Spore release: Every 400 ticks when energy > 40 (costs 15 energy)

**Movement**:
- Samples unified GPU flow field
- Flow force: 0.04, drag: 0.97, max speed: 0.5
- Wraps all edges (toroidal)

**Spores**:
- Drift with Perlin noise
- Settle randomly when slow enough
- Germinate after 50 ticks of settling
- Create new flora with 60 energy
- Wrap all edges (toroidal)

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
| `systems/flora.go` | Flora management, flow-based drift |
| `systems/spores.go` | Spore lifecycle, germination |
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
| `-fitness` | Enable explicit fitness tracking (default: ecology mode) |

## NEAT Tuning

Key parameters in `neural/config.go`:

```go
MutateAddNodeProb: 0.10      // Structural complexity
MutateAddLinkProb: 0.15      // Connection density
CompatThreshold:   1.2       // Lower = more species
```

Target: 3-10 species, brain nodes growing from 25 toward 35+, population 100-300.

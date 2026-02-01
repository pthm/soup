# CLAUDE.md

Artificial life simulation with neural-controlled organisms.

## Quick Start

```bash
go build . && ./soup    # Run with graphics
go test ./...           # Run tests
```

Controls: `Space` pause, `<`/`>` adjust speed (1-10x), click to inspect entity

## CLI Options

| Flag | Default | Description |
|------|---------|-------------|
| `-headless` | false | Run without graphics (fast evolution) |
| `-seed` | time-based | RNG seed for reproducibility |
| `-max-ticks` | 0 | Stop after N ticks (0 = unlimited) |
| `-log-stats` | false | Output ecosystem/perf stats as JSON |
| `-log-file` | stdout | Write logs to file |
| `-stats-window` | 10.0 | Stats window size in seconds |
| `-snapshot-dir` | "" | Save snapshots on bookmarks |

Common patterns:
```bash
./soup --headless --log-stats                    # Fast evolution with stats
./soup --headless --seed=42 --max-ticks=100000   # Reproducible batch run
./soup --log-stats --snapshot-dir=./snapshots    # Save interesting moments
```

## Project Structure

```
soup/
├── main.go           # Entry point, CLI flags
├── game/             # Simulation orchestration
├── components/       # ECS data (what entities have)
├── systems/          # ECS logic (what happens each tick)
├── neural/           # Brain networks
├── renderer/         # GPU shaders and drawing
├── shaders/          # GLSL fragment shaders
├── telemetry/        # Stats, bookmarks, snapshots, perf
└── inspector/        # Click-to-inspect UI
```

## Architecture: Entity-Component-System (ECS)

This project uses the [mlange-42/ark](https://github.com/mlange-42/ark) ECS library.

### What is ECS?

ECS separates **data** from **logic**:

- **Entities** are just IDs (integers) that group components together
- **Components** are pure data structs attached to entities
- **Systems** are functions that query and update components each tick

This enables cache-friendly iteration and clean separation of concerns.

### Components (`components/`)

Components are **data-only structs** with no methods. Each represents one aspect of an entity:

| Component | File | Purpose |
|-----------|------|---------|
| `Position` | `spatial.go` | World coordinates (X, Y) |
| `Velocity` | `spatial.go` | Movement vector (X, Y) |
| `Rotation` | `spatial.go` | Heading angle and angular velocity |
| `Body` | `body.go` | Physical size (Radius) |
| `Capabilities` | `body.go` | Movement limits, vision, bite range |
| `Energy` | `organism.go` | Metabolic state (0-1), age, alive flag |
| `Organism` | `organism.go` | Identity (ID) and kind (prey/predator) |

Entities are composed by attaching multiple components:
```go
entity := mapper.NewEntity(&pos, &vel, &rot, &body, &energy, &caps, &org)
```

### Systems (`systems/`)

Systems contain **all the logic**. Each system queries entities with specific components and updates them:

| System | File | Purpose |
|--------|------|---------|
| `SpatialGrid` | `spatial.go` | O(1) neighbor lookups via cell grid |
| `ComputeSensors` | `sensors.go` | Vision: detects prey/predators in FOV sectors |
| `UpdateEnergy` | `energy.go` | Metabolism costs, death check |
| `TransferEnergy` | `energy.go` | Predator feeding mechanics |

Systems are called in order each tick (see `game/game.go:simulationStep`):
1. Update spatial grid
2. Compute sensors → run brains → apply physics
3. Handle feeding
4. Update energy / check deaths
5. Cleanup dead entities

### The Game Package (`game/`)

Orchestrates everything:

- **Entity factory**: `spawnEntity()` creates organisms with all components + brain
- **Simulation loop**: `simulationStep()` calls systems in order
- **Rendering**: `Draw()` visualizes entities as oriented triangles
- **Input**: Pause, speed controls

The `Game` struct holds:
- The ECS `World` (entity storage)
- Typed mappers/filters for component access
- Brain storage (map of ID → neural network)
- Spatial index, renderers, simulation state

### Neural Package (`neural/`)

Feedforward neural networks that control entity behavior:

| File | Purpose |
|------|---------|
| `ffnn.go` | 2-layer network: 17 inputs → 12 hidden → 3 outputs |

**Inputs** (17): 5 sectors × 3 signals (prey, predator, wall) + energy + speed

**Outputs** (3):
- `turn` [-1, 1]: Steering direction
- `thrust` [0, 1]: Forward acceleration
- `bite` [0, 1]: Attack intent (predators only)

Networks use tanh activation (hidden) and sigmoid/tanh (output).

### Renderer Package (`renderer/`)

GPU-accelerated visuals using raylib:

| File | Purpose |
|------|---------|
| `water.go` | Animated Perlin noise water background |
| `flowfield_gpu.go` | GPU-computed flow field texture, CPU-sampled |

The flow field:
- Rendered to 128×128 texture via shader
- Read back to CPU for fast O(1) sampling
- Updated every 30 ticks

## World Space

The simulation uses **toroidal geometry**—edges wrap around. An entity leaving the right side appears on the left.

All distance calculations use `ToroidalDelta()` to find the shortest path across boundaries.

## Key Patterns

### Querying Entities
```go
query := g.entityFilter.Query()
for query.Next() {
    pos, vel, rot, body, energy, caps, org := query.Get()
    // process entity...
}
```

### Neighbor Lookups
```go
neighbors := g.spatialGrid.QueryRadius(x, y, radius, exclude, posMap)
```

### Component Access by Entity
```go
pos := g.posMap.Get(entity)  // returns *Position or nil
```

## Energy Model

- **Base cost**: 0.01/sec (metabolism)
- **Movement cost**: 0.03 × (speed/maxSpeed)² per sec
- **Bite cost**: Fixed cost when attacking
- **Death**: When energy ≤ 0

Predators gain 80% of energy taken from prey.

## Telemetry Package (`telemetry/`)

Collects and logs simulation metrics:

| File | Purpose |
|------|---------|
| `collector.go` | Aggregates events (births, deaths, bites) per window |
| `stats.go` | `WindowStats` with population, hunting, energy distributions |
| `perf.go` | `PerfCollector` measures per-phase timing |
| `bookmark.go` | Detects evolutionary breakthroughs |
| `snapshot.go` | Serializes full simulation state to JSON |
| `lifetime.go` | Tracks per-organism lifetime statistics |

### Stats Window

Every `--stats-window` seconds (default 10), two log lines are emitted:

1. **Ecosystem stats**: prey/pred counts, births, deaths, hunt success rates, energy percentiles
2. **Perf stats**: avg/min/max tick duration, ticks/sec, phase breakdown percentages

### Bookmarks

Auto-detected evolutionary events that trigger snapshots:

| Type | Condition |
|------|-----------|
| `hunt_breakthrough` | Kill rate > 2× rolling average |
| `forage_breakthrough` | Resource utilization > 2× average |
| `predator_recovery` | Recovered from ≤3 to ≥9 predators |
| `prey_crash` | Population dropped >30% from peak |
| `stable_ecosystem` | Low variance over 5+ windows |

### Performance Phases

The simulation tick is instrumented into phases for profiling:

| Phase | What it measures |
|-------|-----------------|
| `flow_field` | GPU flow field update |
| `spatial_grid` | Rebuild spatial index |
| `behavior_physics` | Sensors + neural nets + movement |
| `feeding` | Predator bite resolution |
| `energy` | Metabolism and foraging |
| `cooldowns` | Reproduction cooldown ticks |
| `reproduction` | Spawning offspring |
| `cleanup` | Removing dead entities |
| `telemetry` | Stats aggregation |

Typical breakdown: `behavior_physics` ~85%, `energy` ~7%, others <2%.

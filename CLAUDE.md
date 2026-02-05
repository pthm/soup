# CLAUDE.md

Artificial life simulation with neural-controlled organisms.

## Quick Start

```bash
go build . && ./soup    # Run with graphics
go test ./...           # Run tests
```

Controls: `Space` pause, `<`/`>` adjust steps-per-update (1-10x), click to inspect entity

## Code Search with `ck`

Use `ck` for semantic code search instead of grep/ripgrep. It understands meaning, not just keywords.

### Search Modes

| Mode | Command | Use Case |
|------|---------|----------|
| Semantic | `ck --sem "concept"` | Find by meaning ("error handling", "entity spawning") |
| Lexical | `ck --lex "keyword"` | Full-text search for exact terms |
| Hybrid | `ck --hybrid "query"` | Combined regex + semantic |
| Regex | `ck --regex "pattern"` | Traditional pattern matching |

### Examples

```bash
# Find authentication/energy validation logic
ck --sem "energy validation" src/

# Find specific function names
ck --lex "spawnEntity" .

# High-confidence semantic results
ck --sem --threshold 0.8 "neural network forward pass" neural/
```

### When to Fallback to grep/ripgrep

Use grep/ripgrep if:
- `ck` is not installed or not working
- Searching for exact regex patterns with complex syntax
- Need line numbers and file context in standard format

## Library Documentation with Context7

Use the Context7 MCP server to fetch up-to-date documentation for libraries used in this project.

### Key Libraries

| Library | Description |
|---------|-------------|
| `mlange-42/ark` | ECS library for entity-component-system architecture |
| `gen2brain/raylib-go` | Go bindings for raylib graphics library |
| `gopkg.in/yaml.v3` | YAML parsing for configuration |
| `gocarina/gocsv` | CSV marshaling with struct tags |

### Usage

First resolve the library ID, then query documentation:

```
# Step 1: Resolve library ID
mcp__context7__resolve-library-id(libraryName: "raylib-go", query: "how to load shaders")

# Step 2: Query docs with the resolved ID
mcp__context7__query-docs(libraryId: "/gen2brain/raylib-go", query: "LoadShader usage")
```

### Common Queries

- **ark ECS**: "how to create entities", "query filters", "component mappers"
- **raylib-go**: "shader uniforms", "texture creation", "drawing primitives"
- **yaml.v3**: "unmarshal embedded files", "custom unmarshaler"

## CLI Options

| Flag | Default | Description |
|------|---------|-------------|
| `-config` | "" | Path to config.yaml (empty = use embedded defaults) |
| `-headless` | false | Run without graphics (fast evolution) |
| `-seed` | time-based | RNG seed for reproducibility |
| `-max-ticks` | 0 | Stop after N ticks (0 = unlimited) |
| `-steps-per-update` | 1 | Simulation ticks per update call (higher = faster) |
| `-log-stats` | false | Output ecosystem/perf stats as JSON to stdout |
| `-output-dir` | "" | Output directory for CSV logs and config snapshot |
| `-stats-window` | 0 | Stats window size in seconds (0 = use config) |
| `-snapshot-dir` | "" | Save snapshots on bookmarks |

Common patterns:
```bash
./soup --headless --log-stats                    # Fast evolution with JSON stats to stdout
./soup --headless --output-dir=./exp1            # Structured CSV output for analysis
./soup --headless --seed=42 --max-ticks=100000   # Reproducible batch run
./soup --output-dir=./exp1 --snapshot-dir=./exp1/snapshots  # Full experiment output
./soup --config=config.yaml                      # Custom tuning parameters
```

## Headless Mode & Fast-Forward

For evolution experiments, run headless with `--steps-per-update` to simulate faster than real time:

```bash
# Run 1 million ticks at 100x speed, log stats every 10 simulated seconds
./soup --headless --log-stats --max-ticks=1000000 --steps-per-update=100

# Quick stability check: 500k ticks (~2.3 hours sim time) at 50x
./soup --headless --log-stats --max-ticks=500000 --steps-per-update=50 | \
  jq -r '[.sim_time, .prey, .pred] | @csv'
```

**How it works**: The simulation uses a fixed timestep (`dt=1/60`). The `--steps-per-update` flag runs multiple fixed-timestep ticks per update call, increasing throughput without changing physics or evolutionary dynamics. This is safe fast-forward—not variable timestep.

**Typical throughput** (M2 Pro):
- `--steps-per-update=1`: ~60k ticks/sec
- `--steps-per-update=100`: ~45k ticks/sec (batching overhead)

In graphical mode, `<`/`>` keys adjust steps-per-update (1-10x) for live fast-forward.

## Project Structure

```
soup/
├── main.go           # Entry point, CLI flags
├── config/           # YAML configuration loading
├── game/             # Simulation orchestration
├── components/       # ECS data (what entities have)
├── systems/          # ECS logic (what happens each tick)
├── neural/           # Brain networks
├── renderer/         # GPU shaders and drawing
├── shaders/          # GLSL fragment shaders
├── telemetry/        # Stats, bookmarks, snapshots, perf
└── inspector/        # Click-to-inspect UI
```

## Configuration (`config/`)

All simulation parameters are loaded from YAML configuration. The package embeds sensible defaults at compile time, with optional override via `-config` flag.

### Usage

```bash
./soup                           # Uses embedded defaults
./soup -config=my-config.yaml    # Custom values merged with defaults
```

### Creating a Config File

Copy `config/defaults.yaml` as a starting point, then modify only the values you want to change:

```yaml
# my-config.yaml - only override what you need
population:
  initial: 50        # Start with more entities
  max_prey: 600      # Allow larger populations

energy:
  prey:
    forage_rate: 0.08  # Faster foraging
```

Unspecified values use embedded defaults.

### Config Sections

| Section | What it controls |
|---------|-----------------|
| `screen` | Window size, target FPS |
| `physics` | Time step (dt), spatial grid cell size |
| `entity` | Body radius, initial energy |
| `capabilities` | Vision range, FOV, speed, turn rate, drag, bite range, thrust deadzone |
| `population` | Initial count, max prey/pred, respawn rules |
| `reproduction` | Energy thresholds, cooldowns, offspring energy |
| `mutation` | Rates and sigma for neural network mutations |
| `energy.prey` | Base cost, movement cost, accel cost, forage rate, grazing peak |
| `energy.predator` | Base cost, movement cost, accel cost, bite cost/reward, digest time |
| `neural` | Hidden layer size, output count |
| `sensors` | Number of sectors, resource sample distance |
| `gpu` | Texture sizes, update intervals |
| `telemetry` | Stats window, bookmark history |
| `bookmarks` | Detection thresholds for evolutionary events |
| `refugia` | Bite success reduction in resource-rich areas |
| `potential` | FBM noise generation for resource hotspots (scale, octaves, contrast) |
| `resource` | Grazing radius, forage efficiency, regeneration rate |

### Accessing Config in Code

```go
import "github.com/pthm-cable/soup/config"

// After config.Init() is called in main()
cfg := config.Cfg()
dt := cfg.Derived.DT32           // float32 version of physics.dt
maxPrey := cfg.Population.MaxPrey
forageRate := cfg.Energy.Prey.ForageRate
```

### Compile-Time Constants

Some values (NumSectors, NumInputs, NumHidden, NumOutputs) remain compile-time constants for array sizing performance. If you change these in config, you must also update the corresponding constants in `systems/sensors.go` and `neural/ffnn.go`.

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
| `light.go` | Renders potential field as dappled sunlight with caustics |
| `resource_fog.go` | Renders resource field as soft green fog overlay |

### Resource Field (`systems/resource_field.go`)

Simple static resource system with regeneration:

| Layer | Description |
|-------|-------------|
| **Potential field P** | Static FBM generated at startup (determines hotspot distribution) |
| **Resource grid R** | Mass density that organisms consume from |
| **Detritus grid D** | Dead biomass that decays back into resource |

| Feature | Description |
|---------|-------------|
| **Static potential** | FBM-based hotspot distribution generated once at startup |
| **Regeneration** | Resource regenerates towards potential over time |
| **True depletion** | Prey grazing removes resource from grid cells |
| **Detritus decay** | Dead organism energy decays back to resource over time |

Key config parameters (`potential:` section):
- `scale`: Base noise frequency (higher = smaller hotspots)
- `octaves`: FBM detail level (more = finer features)
- `contrast`: FBM contrast exponent (higher = sparser patches)

Key config parameters (`resource:` section):
- `graze_radius`: Kernel size for grazing (1 = 3×3 cells)
- `forage_efficiency`: Fraction of removed resource that becomes energy
- `regen_rate`: Regeneration rate towards potential per second

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

All values configurable via `config.yaml` (defaults shown):

**Prey (with true resource depletion):**
- Base cost: 0.015/sec (metabolism)
- Movement cost: 0.12 × (speed/maxSpeed)² per sec
- Accel cost: 0.03 × thrust² per sec (penalizes constant acceleration)
- Forage rate: 0.045/sec at resource=1.0, peaks at ~15% max speed

**Grazing with depletion**: Prey actually remove resource from the grid:
1. Compute desired graze rate: `resourceHere × forageRate × efficiency`
2. Remove resource from 3×3 kernel centered on prey position
3. Energy gain = actual removed amount × `forage_efficiency`

This creates natural migration pressure—prey must move to find fresh patches.

**Grazing efficiency curve**: `1 - 2×|speedRatio - grazingPeak|`, clamped to [0,1]. At default `grazing_peak=0.15`:
- Stationary: 70% efficiency
- 15% speed: 100% efficiency (optimal grazing)
- 50% speed: 30% efficiency
- 65%+ speed: 0% efficiency

**Predator:**
- Base cost: 0.008/sec (lower to allow survival while learning to hunt)
- Movement cost: 0.025 × (speed/maxSpeed)² per sec
- Accel cost: 0.01 × thrust² per sec
- Bite reward: 0.50 energy per successful bite
- Transfer efficiency: 85% of energy taken from prey
- Digest time: 0.8 sec cooldown after a kill

**Thrust deadzone**: Neural network thrust outputs below 0.1 are treated as zero, making it easier to evolve "stop and graze" behavior.

**Death**: When energy ≤ 0

## Telemetry Package (`telemetry/`)

Collects and logs simulation metrics:

| File | Purpose |
|------|---------|
| `collector.go` | Aggregates events (births, deaths, bites) per window |
| `stats.go` | `WindowStats` with population, hunting, energy distributions |
| `perf.go` | `PerfCollector` measures per-phase timing; `PerfStatsCSV` for export |
| `bookmark.go` | Detects evolutionary breakthroughs |
| `snapshot.go` | Serializes full simulation state to JSON |
| `lifetime.go` | Tracks per-organism lifetime statistics |
| `output.go` | `OutputManager` for CSV file output |
| `halloffame.go` | Stores proven lineages for reseeding |

### Output Modes

Two output modes available (can be used together):

| Mode | Flag | Format | Use Case |
|------|------|--------|----------|
| Console | `--log-stats` | JSON to stdout | Quick monitoring, piping to jq |
| Directory | `--output-dir` | CSV files | Analysis with pandas, spreadsheets |

### Output Directory Structure

When `--output-dir=./exp1` is specified:

```
exp1/
├── config.yaml       # Full config snapshot for reproducibility
├── telemetry.csv     # Population, hunting, energy stats per window
├── perf.csv          # Performance metrics per window
├── bookmarks.csv     # Detected evolutionary events
└── hall_of_fame.json # Best-performing organism brains
```

### CSV Struct Tags

CSV columns are defined via struct tags using [gocsv](https://github.com/gocarina/gocsv). To add/rename a column, update the struct field and its `csv:` tag:

```go
type WindowStats struct {
    WindowEndTick   int32   `csv:"window_end"`
    SimTimeSec      float64 `csv:"sim_time"`
    PreyCount       int     `csv:"prey"`
    // ...
}
```

### Stats Window

Every `--stats-window` seconds (default 10), stats are recorded:

1. **Ecosystem stats**: prey/pred counts, births, deaths, hunt success rates, energy percentiles
2. **Perf stats**: avg/min/max tick duration, ticks/sec, phase breakdown percentages

### Bookmarks

Auto-detected evolutionary events that trigger snapshots. Thresholds configurable via `config.yaml`:

| Type | Default Condition |
|------|-----------|
| `hunt_breakthrough` | Kill rate > 2× rolling average, min 3 kills |
| `forage_breakthrough` | Resource utilization > 2× average, min 0.3 |
| `predator_recovery` | Recovered from ≤3 to 3× minimum, min 6 final |
| `prey_crash` | Population dropped >30% from peak, min 10 drop |
| `stable_ecosystem` | CV² < 0.04 over 5 consecutive windows |

### Performance Phases

The simulation tick is instrumented into phases for profiling:

| Phase | What it measures |
|-------|-----------------|
| `resource_field` | Resource regeneration and detritus decay |
| `spatial_grid` | Rebuild spatial index |
| `behavior_physics` | Sensors + neural nets + movement |
| `feeding` | Predator bite resolution |
| `energy` | Metabolism and foraging |
| `cooldowns` | Reproduction cooldown ticks |
| `reproduction` | Spawning offspring |
| `cleanup` | Removing dead entities |
| `telemetry` | Stats aggregation |

Typical breakdown: `behavior_physics` ~85%, `energy` ~7%, others <2%.

## Debug Tools

### Shader Debug Tool

Renders a fragment shader to a PNG file for offline inspection. Useful for debugging GPU shaders without running the full simulation.

**Location**: `cmd/shaderdebug/main.go`

**Usage**:
```bash
go run ./cmd/shaderdebug -shader shaders/resource.fs -out debug.png -width 512 -height 512
```

**Flags**:
| Flag | Default | Description |
|------|---------|-------------|
| `-shader` | `shaders/resource.fs` | Path to fragment shader |
| `-out` | `debug.png` | Output PNG path |
| `-width` | 512 | Render width in pixels |
| `-height` | 512 | Render height in pixels |

**How it works**:
1. Creates a hidden raylib window (GPU context)
2. Loads the fragment shader
3. Sets `time` (float) and `resolution` (vec2) uniforms
4. Renders a fullscreen quad to a texture
5. Exports the texture as PNG

**Example workflow**:
```bash
# Test a shader
go run ./cmd/shaderdebug -shader shaders/resource.fs -out /tmp/test.png

# View the result
open /tmp/test.png  # macOS
# or: xdg-open /tmp/test.png  # Linux

# Quick iteration: edit shader, re-run, check output
```

**Debug shaders included**:
- `shaders/debug_test.fs` - Outputs UV coordinates as colors (red=X, green=Y)
- `shaders/debug_circle.fs` - Single circle at center
- `shaders/debug_dist.fs` - Distance from center as brightness

### In-Game Debug Mode

Press `D` during graphical mode to toggle debug overlay:
- `[R]` Toggle resource field heatmap
- Shows tick timing and TPS stats

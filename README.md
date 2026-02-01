# Primordial Soup

An artificial life simulation where neural-controlled organisms evolve through predator-prey dynamics.

## Quick Start

```bash
go build .
./soup              # Run with graphics
./soup --headless   # Run without graphics (fast evolution)
```

## Controls (Graphics Mode)

| Key | Action |
|-----|--------|
| `Space` | Pause/resume simulation |
| `<` / `>` | Decrease/increase speed (1-10x) |
| Click | Select entity to inspect |

## Command Line Options

```
Usage: ./soup [options]

Options:
  -config string
        Path to config.yaml (empty = use embedded defaults)
  -headless
        Run without graphics (for fast evolution)
  -seed int
        RNG seed for reproducibility (0 = time-based)
  -max-ticks int
        Stop after N ticks (0 = unlimited)
  -log-stats
        Output ecosystem and performance stats via JSON logs
  -log-file string
        Write logs to file (empty = stdout)
  -stats-window float
        Stats aggregation window in seconds (0 = use config default)
  -snapshot-dir string
        Directory to save snapshots on bookmarks
```

## Configuration

All simulation parameters can be tuned via a YAML config file. The simulation includes sensible defaults embedded at compile time.

### Using Custom Config

```bash
./soup --config=my-config.yaml
```

### Creating a Config File

Start from the embedded defaults in `config/defaults.yaml`. You only need to specify values you want to change:

```yaml
# my-config.yaml
population:
  initial: 50          # More starting entities
  max_prey: 600        # Larger population cap

energy:
  prey:
    forage_rate: 0.08  # Faster resource gathering
  predator:
    transfer_efficiency: 0.9  # More efficient hunting

reproduction:
  prey_threshold: 0.75   # Reproduce at lower energy
```

### Configurable Parameters

| Category | Examples |
|----------|----------|
| Screen | Window size, FPS |
| Physics | Time step, grid size |
| Population | Initial count, caps, respawn rules |
| Energy | Base costs, movement costs, forage rate, transfer efficiency |
| Reproduction | Thresholds, cooldowns, offspring energy |
| Mutation | Rates, sigma values |
| Capabilities | Vision range, FOV, speed, turn rate |
| GPU | Texture sizes, update intervals |
| Bookmarks | Detection thresholds |

See `config/defaults.yaml` for the complete list with default values, or [CLAUDE.md](CLAUDE.md) for detailed documentation.

## Examples

### Watch evolution with graphics
```bash
./soup
```

### Fast headless evolution with logging
```bash
./soup --headless --log-stats --max-ticks=100000
```

### Reproducible run with snapshots
```bash
./soup --headless --seed=42 --log-stats --snapshot-dir=./snapshots
```

### Long overnight run to file
```bash
./soup --headless --log-stats --log-file=run.jsonl &
```

### Experiment with tuned parameters
```bash
./soup --config=aggressive-predators.yaml --headless --log-stats
```

## Telemetry

When `--log-stats` is enabled, the simulation outputs JSON logs every stats window (default 10 seconds).

### Ecosystem Stats

Population dynamics, hunting success, and energy distributions:

```json
{
  "msg": "stats",
  "prey": 45,
  "pred": 12,
  "prey_births": 8,
  "pred_births": 2,
  "prey_deaths": 5,
  "pred_deaths": 1,
  "bites_attempted": 23,
  "bites_hit": 18,
  "kills": 5,
  "hit_rate": 0.78,
  "kill_rate": 0.27,
  "prey_energy_mean": 0.62,
  "pred_energy_mean": 0.71
}
```

### Performance Stats

Per-phase timing breakdown for optimization:

```json
{
  "msg": "perf",
  "avg_tick_us": 32,
  "ticks_per_sec": 30614,
  "behavior_physics_pct": 86,
  "energy_pct": 7,
  "spatial_grid_pct": 1
}
```

In headless mode, expect ~30,000 ticks/sec with a small population. The `behavior_physics` phase (sensors, neural networks, movement) typically dominates.

### Bookmarks

The system automatically detects interesting evolutionary moments:

| Bookmark | Trigger |
|----------|---------|
| `hunt_breakthrough` | Kill rate jumps to 2x the rolling average |
| `forage_breakthrough` | Resource utilization doubles |
| `predator_recovery` | Predators recover from near-extinction |
| `prey_crash` | Prey population drops 30%+ |
| `stable_ecosystem` | Low population variance over 5+ windows |

When `--snapshot-dir` is set, a full simulation snapshot is saved on each bookmark.

### Snapshots

Snapshots capture complete simulation state in JSON:
- World configuration and RNG seed
- All entity positions, velocities, energy
- Complete neural network weights
- Lifetime statistics per organism

These can be used for replay, analysis, or seeding new runs with evolved brains.

## Architecture

See [CLAUDE.md](CLAUDE.md) for detailed architecture documentation including:
- Configuration system and all tunable parameters
- Entity-Component-System design
- Neural network structure
- Energy model
- Code patterns

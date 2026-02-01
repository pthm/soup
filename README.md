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
  -steps-per-update int
        Simulation ticks per update call (higher = faster, default 1)
  -log-stats
        Output ecosystem and performance stats via JSON to stdout
  -output-dir string
        Output directory for CSV logs and config snapshot
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

### Fast headless evolution with console logging
```bash
./soup --headless --log-stats --max-ticks=100000
```

### Structured experiment output (CSV)
```bash
./soup --headless --output-dir=./exp1 --max-ticks=500000
```

This creates a directory with:
- `config.yaml` - Full configuration snapshot
- `telemetry.csv` - Population, hunting, energy stats per window
- `perf.csv` - Performance metrics per window
- `bookmarks.csv` - Detected evolutionary events
- `hall_of_fame.json` - Best-performing organism brains

### Reproducible run with snapshots
```bash
./soup --headless --seed=42 --output-dir=./exp2 --snapshot-dir=./exp2/snapshots
```

### Fast evolution at 100x speed
```bash
./soup --headless --output-dir=./exp3 --max-ticks=1000000 --steps-per-update=100
```

### Experiment with tuned parameters
```bash
./soup --config=aggressive-predators.yaml --headless --output-dir=./exp4
```

## Telemetry

The simulation provides two output modes that can be used independently or together:

| Mode | Flag | Format | Use Case |
|------|------|--------|----------|
| Console | `--log-stats` | JSON to stdout | Quick monitoring, piping to jq |
| Directory | `--output-dir` | CSV files | Analysis with pandas, spreadsheets |

### Output Directory Structure

When `--output-dir=./exp1` is specified:

```
exp1/
├── config.yaml       # Full configuration snapshot for reproducibility
├── telemetry.csv     # Population, hunting, energy stats per window
├── perf.csv          # Performance metrics per window
├── bookmarks.csv     # Detected evolutionary events
└── hall_of_fame.json # Best-performing organism brains with weights
```

### CSV Format

**telemetry.csv** - Ecosystem stats per window:
```csv
window_end,sim_time,prey,pred,prey_births,pred_births,prey_deaths,pred_deaths,bites_attempted,bites_hit,kills,hit_rate,kill_rate,prey_energy_mean,pred_energy_mean,resource_util
599,10.0,45,12,8,2,5,1,23,18,5,0.78,0.27,0.62,0.71,0.42
```

**perf.csv** - Performance per window:
```csv
window_end,avg_tick_us,min_tick_us,max_tick_us,ticks_per_sec,fps,behavior_physics_pct,energy_pct,...
599,32,28,45,30614,60,86.2,7.1,...
```

**bookmarks.csv** - Evolutionary events:
```csv
tick,type,description
50000,hunt_breakthrough,"Kill rate 0.65 is 2.3x average (0.28)"
```

### Console Output (JSON)

When `--log-stats` is enabled, JSON logs are written to stdout every stats window:

```json
{"msg":"stats","prey":45,"pred":12,"prey_births":8,"hit_rate":0.78,...}
{"msg":"perf","avg_tick_us":32,"ticks_per_sec":30614,"behavior_physics_pct":86,...}
```

In headless mode, expect ~30,000 ticks/sec with a small population.

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

### Hall of Fame

The simulation tracks proven lineages—organisms that reproduced or survived long enough while achieving something notable. When the run ends, `hall_of_fame.json` contains the top performers with their complete neural network weights, enabling:

- Seeding new runs with evolved brains
- Analyzing successful strategies
- Comparing runs across different configurations

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

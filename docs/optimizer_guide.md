# Ecosystem Optimizer Guide

The optimizer uses CMA-ES to search for simulation parameters that produce stable predator-prey ecosystems. It runs headless simulations and scores each parameter set with a multi-objective fitness function.

## Fitness Function

### Formula

```
fitness = -(survivalTicks × (1.0 + 0.2 × quality))
```

- **survivalTicks** — how long both species coexist before extinction (or maxTicks if they survive). This is the dominant signal.
- **quality** ∈ [0, 1] — a bonus that differentiates configs with similar survival. At most 20% improvement.
- CMA-ES minimizes fitness, so more negative = better.

| Scenario | Fitness |
|----------|---------|
| Full survival (3M ticks), perfect quality | -3,600,000 |
| Full survival, zero quality | -3,000,000 |
| Half survival, perfect quality | -1,800,000 |
| Early extinction | ~0 |

**Key property:** Survival always dominates. The optimizer first learns to avoid extinction, then improves ecosystem health.

## Quality Components

Quality is computed from `WindowStats` collected every 10 sim-seconds. The first 3 windows are skipped (warmup). Windows where either species has fewer than 3 individuals are excluded.

### 1. Population Ratio (weight: 0.30)

Target: ~10:1 prey-to-predator ratio. Scored with a Gaussian in log-space:

```
logErr = ln(ratio / 10)
score  = exp(-logErr²)
```

| Actual Ratio | Score |
|-------------|-------|
| 10:1 | 1.00 |
| 5:1 or 20:1 | 0.62 |
| 3:1 or 33:1 | 0.24 |
| 1:1 or 100:1 | ~0 |

### 2. Population Stability (weight: 0.25)

Low coefficient of variation (CV) across the full time series of population counts:

```
score = exp(-(cvPrey² + cvPred²))
```

Penalizes boom-bust cycles. Stable populations score ~1.0, high-variance populations score near 0.

### 3. Energy Health (weight: 0.25)

Median energy should be ~40% (not starving, not hoarding):

```
preyH = exp(-((preyP50 - 0.40) / 0.20)²)
predH = exp(-((predP50 - 0.40) / 0.20)²)
score = (preyH + predH) / 2
```

| Median Energy | Score |
|--------------|-------|
| 0.40 | 1.00 |
| 0.20 or 0.60 | 0.37 |
| 0.10 or 0.70 | 0.01 |

### 4. Hunting Activity (weight: 0.20)

Two sub-scores blended 60/40:

- **Hit rate** — ideal ~15% (not trivial, not impossible): `exp(-((hitRate - 0.15) / 0.12)²)`
- **Activity** — predators should attempt bites: `1 - exp(-bitesPerPred / 3.0)`

Only scored in windows where predators exist and bites are attempted.

### Aggregation

```
quality = 0.30×ratio + 0.25×stability + 0.25×energy + 0.20×hunting
```

## Building and Running

```bash
# Build
go build -o optimize ./cmd/optimize

# Basic run (required: --output flag)
./optimize --output=./exp1

# Typical experiment
./optimize \
  --max-ticks=3000000 \
  --seeds=3 \
  --max-evals=200 \
  --output=./results/run1

# Quick smoke test
./optimize \
  --max-ticks=100000 \
  --seeds=1 \
  --max-evals=3 \
  --output=/tmp/fitness-test

# Start from custom base config
./optimize \
  --config=my-config.yaml \
  --output=./results/run2
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | "" | Base config YAML (empty = embedded defaults) |
| `--max-ticks` | 3,000,000 | Max ticks per simulation run |
| `--seeds` | 3 | Seeds per evaluation (averaged) |
| `--max-evals` | 200 | Total CMA-ES evaluations |
| `--population` | auto | CMA-ES population size (0 = auto) |
| `--output` | (required) | Output directory |

## Output Files

```
output/
├── optimize_log.csv    # Per-evaluation: fitness + all parameter values
├── best_config.yaml    # Config from the best evaluation
└── hall_of_fame.json   # Best organism brains from the best run
```

## Interpreting optimize_log.csv

Columns: `eval, fitness, param1, param2, ...`

- **fitness** — more negative = better. Values near 0 mean early extinction.
- Check for convergence: fitness should trend more negative over evaluations.
- If fitness plateaus well above `-maxTicks`, survival is the bottleneck.
- If fitness is near `-maxTicks` but not much lower, quality is low — the ecosystem survives but isn't healthy.

## Verifying Results

Run the best config in headless mode to confirm:

```bash
# Run the best config and inspect stats
./soup --headless --log-stats --config=results/run1/best_config.yaml \
  --max-ticks=3000000 | jq -r '[.sim_time, .prey, .pred, .hit_rate] | @csv'
```

Look for:
- Both species surviving the full run
- Prey:predator ratio near 10:1
- Median energy near 0.40
- Non-zero hit rate

## Troubleshooting

### Everything goes extinct quickly
- Increase `--max-ticks` so the optimizer can distinguish 1000-tick configs from 5000-tick configs
- Check that the parameter search space covers viable ranges
- Try more seeds to reduce noise

### Survival is good but quality stays near 0
- This is expected early in optimization — CMA-ES learns survival first
- If quality stays at 0 after many evaluations, the parameter space may not allow healthy ecosystems
- Check that energy costs and forage rates are in reasonable ranges

### Fitness doesn't converge
- Increase `--max-evals`
- Increase `--seeds` for less noisy evaluations (at cost of wall time)
- Try a different `--population` size

### Runs are too slow
- Reduce `--max-ticks` (e.g., 1M instead of 3M) for faster iteration
- Reduce `--seeds` to 1 for rapid exploration, then increase for final runs

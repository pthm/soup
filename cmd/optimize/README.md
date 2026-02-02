# CMA-ES Parameter Optimizer

Finds simulation parameters that produce stable predator-prey ecosystems using [CMA-ES](https://en.wikipedia.org/wiki/CMA-ES) (Covariance Matrix Adaptation Evolution Strategy).

## Quick Start

```bash
# Build
go build ./cmd/optimize

# Run optimization (outputs to ./exp1/)
./optimize --output=./exp1

# Test the optimized config
./soup --config=./exp1/best_config.yaml --headless --log-stats --max-ticks=500000
```

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--config` | "" | Base config YAML (empty = use defaults) |
| `--max-ticks` | 500000 | Simulation duration per evaluation |
| `--seeds` | 3 | Random seeds per evaluation (for robustness) |
| `--max-evals` | 200 | Total fitness evaluations |
| `--population` | 0 | CMA-ES population size (0 = auto) |
| `--output` | required | Output directory |

## Output Files

| File | Description |
|------|-------------|
| `best_config.yaml` | Optimized parameters as full config |
| `optimize_log.csv` | Evaluation history (fitness + params) |
| `hall_of_fame.json` | Best organisms from the winning run |

## Optimized Parameters

The optimizer tunes 13 parameters:

| Parameter | Config Path | Range |
|-----------|-------------|-------|
| prey_base_cost | energy.prey.base_cost | 0.005 - 0.05 |
| prey_move_cost | energy.prey.move_cost | 0.05 - 0.25 |
| prey_forage_rate | energy.prey.forage_rate | 0.02 - 0.10 |
| pred_base_cost | energy.predator.base_cost | 0.002 - 0.02 |
| pred_move_cost | energy.predator.move_cost | 0.01 - 0.08 |
| pred_bite_reward | energy.predator.bite_reward | 0.2 - 0.8 |
| pred_transfer_eff | energy.predator.transfer_efficiency | 0.6 - 1.0 |
| prey_repro_thresh | reproduction.prey_threshold | 0.7 - 0.95 |
| pred_repro_thresh | reproduction.pred_threshold | 0.7 - 0.95 |
| prey_cooldown | reproduction.prey_cooldown | 4.0 - 15.0 |
| pred_cooldown | reproduction.pred_cooldown | 6.0 - 20.0 |
| max_prey | population.max_prey | 200 - 600 |
| max_pred | population.max_pred | 40 - 200 |

## Fitness Function

Lower fitness = better. Components:

1. **Extinction penalty** (1e6): Either species goes extinct
2. **Coexistence** (100×): Penalize if <90% of windows have both species
3. **Population band** (0.1×): Target 200-500 total population
4. **CV stability** (50×): Target CV 0.1-0.35 (not stagnant, not chaotic)
5. **Activity** (20×): Penalize zero birth/death rates
6. **Hunting success** (30×): Penalize kill rate < 0.1
7. **Diversity** (10×): Penalize if ActiveClades < 3

## Examples

```bash
# Quick test run
./optimize --max-ticks=100000 --seeds=1 --max-evals=20 --output=/tmp/quick

# Full optimization (several hours)
./optimize --max-ticks=1000000 --seeds=5 --max-evals=300 --output=./full-opt

# Start from custom base config
./optimize --config=my-base.yaml --output=./from-custom

# Larger CMA-ES population for better exploration
./optimize --population=50 --max-evals=500 --output=./large-pop
```

## Analyzing Results

```bash
# View optimization progress
cat exp1/optimize_log.csv | column -t -s,

# Plot fitness over time (requires csvkit + gnuplot)
csvcut -c eval,fitness exp1/optimize_log.csv | csvlook

# Compare best vs default
diff <(grep -E "^(energy|reproduction|population):" config/defaults.yaml) \
     <(grep -E "^(energy|reproduction|population):" exp1/best_config.yaml)
```

## How It Works

1. CMA-ES proposes parameter vectors in normalized [0,1] space
2. Each evaluation runs `--seeds` headless simulations with different RNG seeds
3. Fitness is averaged across seeds for robustness
4. CMA-ES updates its covariance matrix based on fitness rankings
5. Best parameters (lowest fitness) are saved with their hall of fame

The hall of fame from the best run can be used to seed future simulations with proven organism brains.

# Neuroevolution Tuning Guide

## Overview

This simulation ("Primordial Soup") implements NEAT (NeuroEvolution of Augmenting Topologies) for evolving neural network-controlled organisms. Each organism has:

1. **Brain genome** - A neural network that controls behavior (seeking food, fleeing predators, etc.)
2. **Body genome (CPPN)** - Generates the organism's cell layout/morphology at birth

The goal is to tune the evolution parameters so that:
- Multiple species emerge and coexist
- Brain complexity increases over time (more nodes/genes)
- Diverse behaviors evolve
- Population remains stable (not exploding or dying out)

## Running the Simulation

### Basic Commands

```bash
# Headless mode (no graphics, fastest)
./soup -headless -neural -max-ticks=10000

# With detailed birth/death logging
./soup -headless -neural -neural-detail -max-ticks=10000

# Faster simulation (10x speed)
./soup -headless -neural -speed=10 -max-ticks=50000

# Log to file for analysis
./soup -headless -neural -neural-detail -speed=10 -max-ticks=100000 -logfile=evolution.log

# With world state logging every 5000 ticks
./soup -headless -neural -log=5000 -max-ticks=50000

# Graphics mode with neural stats panel (press N to toggle, S for species colors)
./soup -neural
```

### Flags Reference

| Flag | Description |
|------|-------------|
| `-headless` | Run without graphics (for fast simulation) |
| `-neural` | Enable neural evolution logging (every 500 ticks) |
| `-neural-detail` | Log individual birth/death events |
| `-speed=N` | Simulation speed 1-10 (steps per frame) |
| `-max-ticks=N` | Stop after N ticks (0 = run forever) |
| `-logfile=FILE` | Write logs to file instead of stdout |
| `-log=N` | Log world state every N ticks |
| `-perf` | Enable performance logging |

### Interpreting the Output

```
╔══════════════════════════════════════════════════════════════════╗
║ NEURAL EVOLUTION @ Tick 10000 (Gen 3)
╠══════════════════════════════════════════════════════════════════╣
║ Species: 1 | Total Members: 297 | Best Fitness: 10.67
║ Total Offspring: 1520 | Avg Staleness: 0.0
╠══════════════════════════════════════════════════════════════════╣
║ Neural Organisms: 297
║ Brain Complexity:
║   Nodes: avg=22.4, min=22, max=24
║   Genes: avg=33.7, min=22, max=54
╠══════════════════════════════════════════════════════════════════╣
║ TOP SPECIES:
║   #1: Species 1 - 297 members, age=3, stale=0, fit=10.7, offspring=1520
╚══════════════════════════════════════════════════════════════════╝
```

Key metrics:
- **Species count**: Should be 3-10 for healthy diversity
- **Brain Complexity (Nodes)**: Starting at 22 (14 inputs + 8 outputs), should grow
- **Brain Complexity (Genes)**: Connections between nodes, should increase
- **Staleness**: Generations without fitness improvement (high = stagnant)
- **Offspring**: Reproductive success of species

## Current Observations (Problems to Fix)

From a 10,000 tick run at 10x speed:

### 1. No Speciation Diversity
- Only 1 species exists throughout the entire run
- Species 2 appeared briefly at tick 8000 but immediately died
- **Cause**: Compatibility threshold (3.0) is too high - all organisms are "compatible"
- **Fix**: Lower `compat_threshold` to 1.0-2.0

### 2. Brain Complexity Not Evolving
- Nodes stuck at 22 (the minimum: 14 inputs + 8 outputs)
- Genes hovering around 33-35 (barely above initial)
- **Cause**: Structural mutation rates too low
- **Fix**: Increase `mutate_add_node_prob` and `mutate_add_link_prob`

### 3. Fitness Formula Issues
- Fitness always increases with tick count (survival time dominates)
- No selection pressure for actual behavioral improvement
- **Cause**: `CalculateFitness = energyRatio * survivalBonus * reproBonus`
- **Fix**: Consider normalizing survival time or weighting reproduction more

### 4. Population Explosion
- Started at 80, grew to 300+ by tick 10000
- May cause performance issues and reduce selection pressure
- **Fix**: Could add carrying capacity or increase energy costs

## Key Files to Modify

### 1. `config/neat.yaml` - NEAT Parameters
```yaml
neat:
  # INCREASE these for more structural evolution
  mutate_add_node_prob: 0.03   # Try 0.05-0.10
  mutate_add_link_prob: 0.05   # Try 0.10-0.15

  # DECREASE this for more species
  compat_threshold: 3.0        # Try 1.0-2.0

  # Speciation coefficients
  disjoint_coeff: 1.0          # Weight of disjoint genes in distance
  excess_coeff: 1.0            # Weight of excess genes in distance
  weight_diff_coeff: 0.4       # Weight of weight differences
```

### 2. `neural/config.go` - Default NEAT Options
The `DefaultNEATOptions()` function sets the actual values used. The YAML file exists but isn't loaded yet - you may need to modify the Go code directly:

```go
// neural/config.go - DefaultNEATOptions()
opts.MutateAddNodeProb = 0.03    // Probability of adding hidden node
opts.MutateAddLinkProb = 0.05    // Probability of adding connection
opts.CompatThreshold = 3.0       // Distance threshold for same species
```

### 3. `neural/species.go` - Fitness Calculation
```go
// CalculateFitness - consider modifying this formula
func CalculateFitness(energy, maxEnergy float32, survivalTicks int32, offspringCount int) float64 {
    energyRatio := float64(energy / maxEnergy)
    survivalBonus := float64(survivalTicks) / 1000.0  // This dominates!
    reproBonus := 1.0 + float64(offspringCount)*0.5
    return energyRatio * survivalBonus * reproBonus
}
```

### 4. `neural/reproduction.go` - Mutation Functions
- `MutateGenome()` - Brain network mutations
- `MutateCPPNGenome()` - Body/morphology mutations

## Tuning Strategy

### Phase 1: Get Speciation Working
1. Lower `compat_threshold` from 3.0 to 1.5
2. Run 20,000 ticks and check if multiple species emerge
3. Adjust threshold until you see 3-5 stable species

### Phase 2: Increase Structural Evolution
1. Increase `mutate_add_node_prob` to 0.08
2. Increase `mutate_add_link_prob` to 0.12
3. Run 50,000 ticks and check if max nodes/genes increase
4. Look for nodes growing from 22 toward 30+

### Phase 3: Balance Population
1. If population explodes (>500), increase energy costs or add limits
2. If population crashes (<50), decrease energy costs or mutation severity
3. Target stable population of 100-300

### Phase 4: Improve Fitness Function
1. Consider normalizing survival time (cap at some maximum)
2. Weight offspring count more heavily (actual evolutionary success)
3. Consider adding behavior-specific bonuses (food eaten, predators avoided)

## Testing a Change

```bash
# Quick test (5000 ticks, ~15 seconds)
./soup -headless -neural -speed=10 -max-ticks=5000

# Medium test (20000 ticks, ~1 minute)
./soup -headless -neural -speed=10 -max-ticks=20000

# Long test with logging (100000 ticks, ~5 minutes)
./soup -headless -neural -speed=10 -max-ticks=100000 -logfile=test.log

# Then analyze:
grep "Species:" test.log | tail -20
grep "Nodes:" test.log | tail -20
```

## Success Criteria

A well-tuned system should show:

1. **3-10 species** coexisting by tick 20,000
2. **Growing complexity**: Nodes increasing from 22 toward 30+, genes from 35 toward 60+
3. **Stable population**: 100-300 organisms, not exploding or crashing
4. **Species turnover**: Some species go extinct, new ones emerge
5. **Fitness variation**: Different species have different fitness levels
6. **Behavioral diversity**: Observable differences in how organisms behave (requires graphics mode)

## Build and Test

After making changes:

```bash
# Rebuild
go build .

# Run tests
go test ./neural/...

# Quick verification
./soup -headless -neural -max-ticks=2000
```

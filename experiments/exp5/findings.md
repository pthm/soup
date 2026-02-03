# Experiment 5: CMA-ES Parameter Optimization — Findings

## Overview

100 evaluations of a 33-dimensional parameter space using CMA-ES, optimizing a
composite fitness function (lower = better) that penalizes extinction, population
instability, low hunting activity, stagnation, and lack of evolutionary diversity.
Each evaluation runs the simulation across multiple random seeds and averages fitness.

## Results Summary

### Fitness Distribution

The results are sharply bimodal:

| Range | Count | Description |
|-------|-------|-------------|
| 0 - 1 | 3 | Near-perfect ecosystems |
| 1 - 25 | 5 | Strong stable ecosystems |
| 25 - 53 | 1 | Intermediate (eval 46 at 49.7) |
| 53 - 56 | ~50 | Typical plateau — partial penalty satisfaction |
| 56 - 60 | ~41 | Poor — multiple penalties active |

There is a hard gap between "good" (< 25) and "typical" (> 49). Configs either
satisfy nearly all fitness criteria or they fail to satisfy several simultaneously.
Partial solutions cluster tightly at 53-60, suggesting the fitness landscape has a
plateau with a few deep basins.

### Convergence Timeline

| Eval | Fitness | Note |
|------|---------|------|
| 16 | 0.605 | First breakthrough |
| 37 | 16.968 | Second basin discovered |
| 38 | 22.223 | Nearby exploration |
| 46 | 49.702 | First crack in the 53+ plateau |
| 54 | 0.331 | Second near-perfect config |
| 62 | **0.000** | **Perfect score** |
| 65 | 9.258 | Good config |
| 74 | 14.279 | Good config |
| 87 | 3.186 | Third near-perfect config |

The optimizer found breakthroughs sporadically rather than converging smoothly,
consistent with a rugged fitness landscape with narrow basins of attraction.

### Top 8 Configurations

| Rank | Eval | Fitness | Strategy |
|------|------|---------|----------|
| 1 | 62 | 0.000 | Slow life history, high bite reward, low transfer efficiency |
| 2 | 54 | 0.331 | Fast life history, moderate bite reward, density-regulated |
| 3 | 16 | 0.605 | Fast prey breeding, efficient predators, full transfer |
| 4 | 87 | 3.186 | Fast maturity, weak predators, max digest time, high resources |
| 5 | 65 | 9.258 | Fast maturity, weak predators, high prey cap, density-regulated |
| 6 | 74 | 14.279 | Fast maturity, weak predators, high prey cap, max newborn cooldown |
| 7 | 37 | 16.968 | Fast maturity, weak predators, long digest, high density-K |
| 8 | 38 | 22.223 | Fast maturity, weak predators, density-regulated, wide dispersal |

---

## Universal Rules

These parameters are consistent across all or nearly all top 8 configurations.
They represent non-negotiable requirements for a healthy ecosystem.

### 1. Maximum offspring investment (`child_energy = 0.500`)

**8/8 top configs.** This is the single most consistent finding. Offspring must
start with as much energy as possible. Underfunded offspring die before they can
learn to forage or hunt, creating wasted reproductive effort that destabilizes
populations.

### 2. Low prey reproduction threshold (`prey_repro_thresh = 0.500`)

**7/8 top configs** (only eval 38 deviates at 0.744). Prey should reproduce as
soon as they reach 50% energy. Combined with max child energy, this means prey
reproduce frequently and invest heavily each time. The prey population acts as a
reliable renewable resource for predators.

### 3. Cheap prey metabolism (`prey_base_cost ≤ 0.012`)

**7/8 top configs** are at or near the minimum (0.005). Prey need low baseline
energy drain to survive periods of low resource availability. This prevents
starvation cascades during resource field transitions.

### 4. Breeding desynchronization (`cooldown_jitter = 3.0`)

**6/8 top configs** at the maximum. High jitter randomizes reproduction timing
across the population, preventing synchronized breeding waves that cause
boom-bust oscillations. This is a stability mechanism.

### 5. Tiny particle mass (`part_initial_mass = 0.002`)

**6/8 top configs** at the minimum. Resources should be distributed as many small
packets rather than few large ones. This creates a more uniform resource field,
reducing spatial inequality and preventing prey from clustering in rich patches.

### 6. Low flow scale (`part_flow_scale = 0.5`)

**7/8 top configs** at the minimum (eval 16 at 5.0 is the sole outlier). Fine-grained
flow patterns distribute resources more evenly than large-scale currents.

---

## Bimodal Parameters: Two Strategies That Both Work

Several parameters split cleanly into two viable strategies across the top configs.

### Predator economy: "Efficient killers" vs "Wasteful scavengers"

| Parameter | Efficient (evals 16, 62) | Wasteful (evals 54, 65, 74, 87, 37, 38) |
|-----------|-------------------------|------------------------------------------|
| `pred_bite_reward` | 0.67 - 0.80 (high) | 0.40 (min) |
| `pred_transfer_eff` | 0.60 - 1.00 (varies) | 0.60 (min) |

The efficient strategy gives predators high reward per kill. The wasteful strategy
gives minimal reward but compensates with other mechanisms (low predator caps,
density regulation, long digest times). Both prevent predator population explosions
through different means.

### Life history speed: "K-strategists" vs "r-strategists"

| Parameter | K-strategy (eval 62) | r-strategy (evals 54, 65, 74, 87) |
|-----------|---------------------|-------------------------------------|
| `maturity_age` | 15.0 (max) | 2.0 (min) |
| `prey_cooldown` | 17.8 | 4.0 - 14.1 |
| `pred_cooldown` | 20.0 (max) | 6.0 (min) |

Eval 62 (the perfect score) uses a slow life history — late maturity, long breeding
cooldowns, but stable populations. Most other top configs use fast life histories
with density regulation. Both achieve stability, but through different ecological
dynamics.

### Population structure: "Equal caps" vs "Prey-heavy"

| Structure | Evals | max_prey | max_pred |
|-----------|-------|----------|----------|
| Equal (200/200) | 62, 54, 16 | 200 | 180-200 |
| Prey-heavy | 65, 74, 87, 37, 38 | 436-600 | 40-188 |

The equal-cap configs limit total population but allow balanced ratios. The
prey-heavy configs allow many prey but cap predators low, creating a classic
pyramid structure.

### Predator density regulation

| Approach | pred_density_k | Evals |
|----------|---------------|-------|
| No regulation | 0 | 62, 87 |
| Moderate | 116 - 189 | 54, 16, 65, 74 |
| Strong | 300 | 37, 38 |

Both zero and high density-K work, but for different reasons. Configs with no
density regulation use other constraints (slow breeding, low transfer efficiency).
Configs with high density-K directly suppress predator reproduction when predators
are abundant.

---

## Surprising Findings

### Prey movement should be expensive

5/8 top configs have `prey_move_cost` at or near the maximum (0.25). This seems
counterintuitive — why penalize prey for moving? The answer is that expensive
movement creates a natural "stop and graze" pressure. Prey that sit still in
resource-rich areas survive better, making them predictable targets for predators.
This stabilizes the predator-prey dynamic by making hunting viable.

### Low foraging rate works

6/8 top configs have `prey_forage_rate` at or near the minimum (0.02). Combined
with low base cost, prey don't need to eat fast — they just need to not starve.
This prevents prey from rapidly converting resources to energy to offspring,
which would cause resource depletion crashes.

### Transfer efficiency often at minimum

5/8 top configs have `pred_transfer_eff = 0.600` (minimum). Predators that waste
40% of their kills are actually better for ecosystem stability. Full efficiency
(eval 16) works but requires other strong constraints. Waste acts as a natural
brake on predator population growth.

### The perfect config (eval 62) is a "slow world"

The only config that achieved zero penalty uses maximum maturity age (15.0),
near-maximum breeding cooldowns, and maximum spawn offset. Everything happens
slowly and offspring are scattered far from parents. This prevents local
overpopulation, synchronized dynamics, and competitive exclusion. It's the
ecological equivalent of "slow and steady wins the race."

---

## The Best Configuration (Eval 62)

```yaml
# Energy - Prey
energy:
  prey:
    base_cost: 0.005          # Minimum — cheap to exist
    move_cost: 0.250          # Maximum — expensive to move
    forage_rate: 0.096        # High — efficient grazing

  predator:
    base_cost: 0.020          # Maximum — expensive to exist
    move_cost: 0.063          # Moderate
    bite_reward: 0.800        # Maximum — big payoff per kill
    transfer_efficiency: 0.60 # Minimum — but lose 40%
    digest_time: 0.715        # Moderate cooldown after kill

# Reproduction
reproduction:
  prey_threshold: 0.500       # Minimum — reproduce early
  pred_threshold: 0.710       # Moderate — predators wait longer
  maturity_age: 15.0          # Maximum — late maturity
  prey_cooldown: 17.8         # Long — slow prey breeding
  pred_cooldown: 20.0         # Maximum — slowest pred breeding
  cooldown_jitter: 3.0        # Maximum — desynchronize breeding
  parent_energy_split: 0.509  # Even split with offspring
  child_energy: 0.500         # Maximum — well-provisioned young
  spawn_offset: 30.0          # Maximum — offspring far from parent
  heading_jitter: 0.381       # Moderate dispersal angle
  pred_density_k: 0.0         # No density regulation needed
  newborn_hunt_cooldown: 0.5  # Minimum — newborns can hunt quickly

# Population
population:
  max_prey: 200               # Minimum cap
  max_pred: 200               # Maximum cap

# Refugia
refugia:
  strength: 1.036             # Slight protection in resource-rich areas

# Particles
particles:
  spawn_rate: 176.3           # Moderate particle generation
  initial_mass: 0.002         # Minimum — many tiny packets
  deposit_rate: 5.0           # Maximum — fast resource delivery
  pickup_rate: 0.1            # Minimum — slow pickup
  cell_capacity: 0.438        # Low — prevents resource hoarding
  flow_strength: 10.0         # Minimum — gentle currents
  flow_scale: 0.5             # Minimum — fine-grained flow
  flow_octaves: 1.87          # Low — simple flow patterns
  flow_evolution: 0.005       # Minimum — stable flow field
```

### Why it works

The eval 62 config creates an ecosystem characterized by:

1. **Cheap, sedentary prey** that graze efficiently but rarely move
2. **Expensive predators** with high kill rewards but 40% waste
3. **Extremely slow reproduction** for both species, preventing booms
4. **Maximum offspring dispersal** preventing local overpopulation
5. **Fine, stable resource distribution** that changes slowly
6. **High deposit / low pickup** particle dynamics, meaning resources
   accumulate in cells but particles don't strip resources back out

The net effect is a slow, steady ecosystem where populations turn over gradually,
resources are distributed evenly, and neither species can grow fast enough to
crash the other.

---

## Recommendations for Future Experiments

1. **Validate eval 62**: Run it with 10+ seeds to confirm the perfect score is
   robust and not a lucky draw.

2. **Narrow the search space**: Several parameters are clearly locked in across
   all good configs. Fix `child_energy=0.5`, `prey_repro_thresh=0.5`,
   `prey_base_cost=0.005`, `cooldown_jitter=3.0`, `part_initial_mass=0.002`,
   and `part_flow_scale=0.5`, then re-run CMA-ES on the remaining ~27 dimensions.

3. **Explore the slow-world basin**: Eval 62's slow life history strategy is
   underexplored. Run a focused search around `maturity_age > 10`,
   `pred_cooldown > 15`, and `spawn_offset > 20`.

4. **Test parameter bounds**: `prey_base_cost`, `child_energy`, `part_flow_scale`,
   and `part_initial_mass` are all pinned to bounds. The true optimum may lie
   outside the current search range. Consider extending bounds for these.

5. **Longer simulations**: If evaluations use a fixed tick count, consider
   running the top configs for 2-5x longer to check whether stability persists
   or whether slow collapses emerge.

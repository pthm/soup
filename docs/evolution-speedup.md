# Evolution Speedup Proposals

This document outlines changes to accelerate evolution toward interesting morphologies and behaviors without breaking stability.

---

## Goals
- Reach diverse morphologies in fewer generations.
- Improve selection signal quality (reduce "random drift").
- Avoid premature convergence or monocultures.

---

## 1) Increase Selective Gradient Quality

### A) Early-Life Fitness Shaping
Reward early survivability and basic competency to guide search.
```
fitness = base_fitness * (1 + early_bonus)
early_bonus = clamp((survival_ticks / early_window), 0.0, 1.0) * early_scale
```

### B) Behavior Milestones
Give small bonuses for first-time events:
- first successful feeding
- first successful reproduction
- first survival past N ticks

Pros: faster signal for useful behaviors.  
Cons: need event tracking per organism.

---

## 2) Reduce Wasted Search

### A) Viability Constraints
Enforce minimum viability:
- At least one sensor and one actuator
- Minimum cell count or minimum body connectivity

### B) Morphology Initialization Bias
Seed initial CPPNs with simple viable structures:
- symmetric 2–6 cell bodies
- one sensor + one actuator by default

---

## 3) Increase Useful Variation

### A) Adaptive Mutation Rates
Increase mutation when species stagnates:
```
if species_staleness > threshold:
  mutation_rate *= 1.2 (cap at max)
else:
  mutation_rate *= 0.95 (floor at min)
```

### B) Targeted Mutations
Bias toward changes that alter morphology rather than weights only:
- CPPN node/link mutations > weight tweaks in early evolution

---

## 4) Shorten Generational Lag

### A) Faster Generation Turnover
End generations based on median age or time-window rather than fixed ticks.

### B) Cull Weak Agents Early
Remove agents that stay below an energy threshold for too long.

Pros: more cycles per wall-clock time.  
Cons: can reduce exploration if too aggressive.

---

## 5) Encourage Diversity to Avoid Stagnation

### A) Niche Balancing (see fitness alternatives)
Promote underrepresented morphologies or diets.

### B) Species Size Cap (soft)
Apply fitness penalties to overrepresented species.

---

## 6) Compute Efficiency (More Iterations)

### A) Headless Batch Runs
Run headless simulations with accelerated tick rates and save snapshots.

### B) Parallel Worlds
Run multiple worlds with different seeds and aggregate fitness stats.

---

## Recommended Incremental Plan

Phase 1 (fast wins):
- Add early-life survival bonus
- Add first-feeding bonus
- Ensure morphology viability constraints

Phase 2 (diversity + speed):
- Niche-balanced fitness
- Adaptive mutation rates

Phase 3 (throughput):
- Headless batch simulation
- Optional multi-world runs


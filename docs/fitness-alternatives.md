# Fitness Alternatives (Diversity-Promoting)

This document proposes fitness functions that encourage diversity while keeping selection pressure aligned with survival and reproduction.

---

## Baseline (current intent)
```
fitness = energy_ratio * log1p(survival_ticks / 500) * (1 + 0.5 * offspring_count)
```

Pros: simple, stable.  
Cons: can converge to a single dominant niche; no explicit novelty or ecological balance.

---

## Option A: Niche-Balanced Fitness

Encourage underrepresented niches by weighting fitness with population share.

```
fitness = base_fitness * niche_weight

niche_weight = clamp( target_share / (observed_share + epsilon), 0.5, 2.0 )
```

Define niches by:
- Composition bucket (photo vs actuator ratio)
- Digestive spectrum bucket
- Body size bucket

Pros: pushes population into multiple niches.  
Cons: requires binning and population statistics.

---

## Option B: Novelty Bonus (Behavioral)

Reward agents that behave differently from recent population averages.

```
fitness = base_fitness * (1 + novelty_scale * novelty_score)

novelty_score = distance(behavior_vector, population_mean)
behavior_vector = [avg_speed, time_in_light, feeding_rate, breeding_rate]
```

Pros: encourages behavioral diversity without explicit niches.  
Cons: can drift away from survival if novelty dominates.

---

## Option C: Diversity Penalty for Dominant Species

Apply a soft penalty to species with too many members.

```
fitness = base_fitness * clamp( target_species_size / (species_size + epsilon), 0.5, 1.5 )
```

Pros: simple and directly controls species dominance.  
Cons: can distort selection if species size lags fitness.

---

## Option D: Multi-Objective Weighted Sum

Score multiple dimensions to avoid single-axis convergence.

```
fitness = w1*energy_ratio + w2*survival_score + w3*offspring_score
        + w4*exploration_score + w5*novelty_score
```

Pros: flexible and tunable.  
Cons: harder to reason about and tune.

---

## Option E: Life-History Tradeoff

Explicitly reward different strategies (fast reproduction vs long survival).

```
fitness = (1 + offspring_count)^a * (1 + survival_ticks)^b * (energy_ratio)^c
```

Use different (a, b, c) per environment or alternate across epochs.

Pros: can create stable coexistence of r/K strategies.  
Cons: requires careful tuning to avoid runaway.

---

## Option F: Capability Coverage Bonus

Reward organisms that occupy rare capability mixes.

```
fitness = base_fitness * (1 + capability_rarity_bonus)

capability_rarity_bonus = mean( rarity(primary_type), rarity(secondary_type) )
```

Pros: directly tied to morphology diversity.  
Cons: may reward weird but unviable morphologies.

---

## Recommendation (Incremental)

1) Start with **Option A (Niche-Balanced)** using coarse bins (3–5 per dimension).  
2) Add **Option C** if species domination remains.  
3) Add **Option B** only if behavior converges too strongly.

Keep base fitness unchanged; apply diversity terms as multipliers capped in [0.5, 2.0] to avoid destabilization.


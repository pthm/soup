# Ecosystem Tuning Log

## Goal
Achieve a sustainable, balanced ecosystem where prey and predators coexist with natural oscillations.

## Starting Point: ecosystem_v3.yaml
Key parameters:
- Population: initial=60, max_prey=300, max_pred=8
- Resource: spawn_rate=800, initial_mass=0.03, cell_capacity=0.8
- Prey energy: base_cost=0.010, move_cost=0.06, forage_rate=0.09
- Predator energy: base_cost=0.012, bite_reward=0.60
- Reproduction: prey_threshold=0.65, prey_cooldown=12.0, pred_threshold=0.85

---

## Experiment 1: Baseline (ecosystem_v3.yaml)
**Date:** 2026-02-01
**Config:** ecosystem_v3.yaml unchanged
**Run:** 100k ticks (~1667 sim seconds) at 100x speed

### Results:
- **Windows:** 166 (~1660 sim seconds)
- **Avg prey:** 408 (oscillates 300-600 due to boom-bust cycles)
- **Avg predators:** 7.5 (near max_pred=8 cap)
- **Total kills:** 1177 (~7 kills/window)
- **Avg resource utilization:** 31.6%
- **Final:** 528 prey, 8 predators

### Analysis:
- Predators are capped at max_pred=8, can't grow to control prey booms
- Prey energy very high (0.99), they're thriving
- Kill rate (~7/window with 8 predators) is insufficient for control
- Boom-bust cycle: prey hit 300 cap, over-reproduce, crash, repeat

### Hypothesis:
Need more predators to control prey. max_pred=8 is the bottleneck.

---

## Experiment 2: Increase max_pred to 20
**Date:** 2026-02-01
**Config:** ecosystem_v3 + `max_pred: 20`
**Rationale:** Allow predator population to grow to control prey booms

### Results:
- **Windows:** 166
- **Avg prey:** 406 (similar to baseline)
- **Avg predators:** 18 (up from 7.5!)
- **Total kills:** 3005 (2.5x more than baseline!)
- **Avg resource utilization:** 31.8%
- **Final:** 501 prey, 21 predators

### Analysis:
- More predators exist but still can't control prey booms
- Early predator die-off still occurs (5 → 2 in first 80 sec)
- Eventually 20+ predators exist but prey still oscillate 300-600
- Problem: refugia strength 0.75 blocks hunting in resource-rich areas

### Hypothesis:
Refugia too strong. Predators can't hunt prey in high-resource zones where prey congregate.

---

## Experiment 3: Lower refugia strength to 0.5
**Date:** 2026-02-01
**Config:** exp2 (max_pred=20) + `refugia.strength: 0.5`
**Rationale:** Make hunting easier in resource areas to improve predator survival

### Results:
- **Windows:** 166
- **Avg prey:** 418 (similar)
- **Avg predators:** 14 (DOWN from 18!)
- **Total kills:** 1013 (DOWN from 3005!)
- **Avg resource utilization:** 31.2%
- **Final:** 450 prey, 20 predators

### Analysis:
- Counter-intuitive: easier hunting led to FEWER kills
- Early predator die-off still occurs (same pattern)
- Predator population ends up lower (14 avg vs 18)
- Possible: with easier hunting, they don't learn evasion as well, die to other causes?

### Hypothesis:
The main issue is early predator survival. They die before learning to hunt, regardless of refugia.

---

## Experiment 4: Lower predator breeding threshold
**Date:** 2026-02-01
**Config:** exp2 + `pred_threshold: 0.70` (from 0.85)
**Rationale:** Let predators breed even when not hunting perfectly

### Results:
- **Windows:** 166
- **Avg prey:** 410
- **Avg predators:** 16 (slightly better)
- **Total kills:** 1010 (still low)
- **Avg resource utilization:** 31.2%
- **Final:** 374 prey, 25 predators

### Analysis:
- More predator births happening (pred_births: 1-2 per window early)
- BUT predators still drop to 2 frequently - they're dying of starvation
- Core issue: random brains can't hunt, predators starve before learning

### Hypothesis:
Predators need to survive longer to learn hunting. Lower base_cost.

---

## Experiment 5: Much lower predator base_cost
**Date:** 2026-02-01
**Config:** exp2 + `predator.base_cost: 0.006` (from 0.012, half)
**Rationale:** Give predators more time to learn hunting

### Results:
- **Windows:** 166
- **Avg prey:** 403
- **Avg predators:** 18 (similar to exp2)
- **Total kills:** 997 (still low)
- **Final:** 534 prey, 24 predators

### Analysis:
- Predators survive slightly longer (5-6 for first 60 sec vs dropping to 2 immediately)
- Still drop to 3-4, then recover
- Same overall pattern - no improvement in hunting effectiveness

### Observation:
The core issue might be neural network hunting behavior, not survival time.
Low refugia (exp3) actually hurt - maybe predators need pressure to learn?

---

## Experiment 6: More starting predators
**Date:** 2026-02-01
**Config:** exp5 (low pred cost) + `predator_spawn_chance: 0.15` (from 0.05, 3x)
**Rationale:** More predators = more chances for one to stumble onto hunting

### Results:
- **Windows:** 166
- **Avg prey:** 408
- **Avg predators:** 21 (up from 18!)
- **Total kills:** 1105 (slightly up)
- **Final:** 475 prey, 23 predators

### Analysis:
- **KEY FINDING:** Starting with ~10 predators prevents early die-off!
- Predators stay at 10-12 early (no crash to 2)
- Kills happen more consistently (5-12 per window)
- BUT still have boom-bust cycles when prey hit 300 cap

### Observation:
Early predator survival is critical. Need enough predators to get lucky kills
that kickstart hunting evolution.

---

## Experiment 7: Lower max_prey to reduce oscillation amplitude
**Date:** 2026-02-01
**Config:** exp6 + `max_prey: 200` (from 300)
**Rationale:** Smaller amplitude boom-bust cycles might be more stable

### Results:
- **Windows:** 166
- **Avg prey:** 277 (down from 408)
- **Avg predators:** 21
- **Total kills:** 1174
- **Final:** 211 prey, 23 predators
- **Prey std:** 362 (still high oscillation)

### Analysis:
- Smaller population overall
- Still overshoots max_prey cap (347 seen at window 129)
- Late game shows gradual decline rather than boom-bust
- More resource utilization (36% vs 30%)

### Observation:
Lower cap helps but doesn't eliminate instability. Prey still reproduce
faster than predators can control.

---

## Experiment 8: Boost predator hunting
**Date:** 2026-02-01
**Config:** exp6 + `bite_reward: 0.75`, `refugia.strength: 0.6`
**Rationale:** Make hunting more rewarding and easier

### Results:
- **Windows:** 166
- **Avg prey:** 402
- **Avg predators:** 16 (DOWN from 21!)
- **Total kills:** 1560 (UP 40%!)
- **Final:** 522 prey, 23 predators

### Analysis:
- Predators still die off early (11 → 2 by window 140)
- Lower refugia doesn't help survival - they can't hunt with random brains
- More kills late game when predators recover

### Key Insight:
The problem isn't hunting conditions - it's that random neural networks
can't hunt at all. Predators need a survival floor or better starting brains.

---

## Experiment 9: Add min_predators floor
**Date:** 2026-02-01
**Config:** exp6 + `min_predators: 5`
**Rationale:** Prevent predator extinction during early random-brain phase

### Results:
- **Windows:** 166
- Same as exp6 - predators never dropped below 5 with this config

### Analysis:
- The 15% predator spawn chance (exp6) already prevents extinction
- min_predators=5 doesn't trigger because population stays above 5
- 15% spawn is the key finding - enough predators survive through chance

---

## Experiment 10: Combined best settings
**Date:** 2026-02-01
**Config:**
- `predator_spawn_chance: 0.15` (from exp6)
- `max_prey: 200` (from exp7)
- `base_cost: 0.006` (from exp5)
- `max_pred: 30` (increased for natural regulation)
**Rationale:** Combine all positive findings

### Results:
(pending...)

---

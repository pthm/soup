# Exp11 Stability Analysis (Energy Economy)

This document summarizes what exp11 actually did, why it likely failed to achieve long‑term stability, and the specific fixes/hypotheses to test next. It is based on:

- `experiments/exp11/eval416/telemetry.csv`
- `experiments/exp11/eval416/config.yaml`
- `experiments/exp11/optimize_log.csv` (587 evaluations)
- Energy economy spec: `docs/energy_economy.md`
- Runtime code paths in `game/` and `systems/`

## 1) What exp11 really did (ground truth)

### 1.1 Exp11 "best" run is short‑lived
The optimizer uses **fitness = ‑(survivalTicks × (1 + 0.2 × quality))** where quality is a weighted combination of population ratio, stability, energy health, and hunting activity (see `cmd/optimize/fitness.go`). The best exp11 eval (per log) is **eval 416** (fitness ‑52187), and it *does not* survive long‑term:

- Predator extinction (pred == 0) becomes persistent at **~539s**.
- Total extinction (prey == 0 and pred == 0) occurs at **~1707s**.

Representative telemetry points:

- ~110s: prey ≈ 342, pred ≈ 21
- ~210s: prey ≈ 291, pred ≈ 181
- ~310s: prey ≈ 31, pred ≈ 128
- ~410s: prey ≈ 54, pred ≈ 6
- ~510s: prey ≈ 136, pred ≈ 1
- ~609s: prey ≈ 423, pred ≈ 0
- ~1507s: prey ≈ 2, pred ≈ 0
- ~1707s: prey ≈ 0, pred ≈ 0

This shows a **predator boom → prey crash → predator extinction → slow prey decline → total extinction**.

### 1.2 Two distinct strategies in the top tier

The top two evals (#416 and #134, fitness ‑52187 and ‑50425) use fundamentally different strategies:

| Parameter | #134 (ambush) | #416 (regulated) |
|-----------|--------------|-------------------|
| bite_reward | 0.80 | 0.56 |
| digest_time | 3.0s | 0.2s |
| pred_density_k | 10 | 600 |
| prey_cooldown | 4s | 20s |
| pred_cooldown | 6s | 20s |
| prey_move_cost | 0.094 | 0.25 |
| parent_energy_split | 0.70 | 0.40 |

**#134** is an ambush predator economy: high bite reward, long digest, fast reproduction, low density dependence. **#416** is regulated hunting: moderate reward, instant digest, slow reproduction, massive density‑dependent breeding suppression.

Both converge to extinction via the same boom‑crash cycle, suggesting the problem is structural rather than parametric.

### 1.3 Energy pools don't buffer crashes
From telemetry:

- **Detritus is near zero most of the run** (mean ≈ 0.77, max ≈ 38 against large resource totals). Recycling is barely contributing.
- Resource utilization drops from ~0.19 early to ~0.09 before prey extinction.
- Prey energy mean falls from ~0.28 to ~0.04 in the last phase.

In short: **the system lacks an energy buffer** that could smooth out population oscillations after the predator crash.

### 1.4 Diet distribution collapses toward herbivory
Diet mean and std drop rapidly after predator extinction:

- Early phase: diet mean ≈ 0.29, std ≈ 0.32
- 500–1000s: diet mean ≈ 0.0085, std ≈ 0.013
- 1000–1500s: diet mean ≈ 0.0023, std ≈ 0.0047

This means **the population collapses into near‑pure herbivory**, which then still cannot sustain itself long‑term.

### 1.5 Optimizer is bound‑saturated

22 of 25 parameters hit their bounds in the top 20 evaluations. The optimizer is pressing against its search space. Key convergence signals:

| Parameter | Bound | Count in top 20 |
|-----------|-------|-----------------|
| pred_repro_thresh | MAX (0.95) | 20/20 |
| part_cell_capacity | MIN (0.3) | 20/20 |
| detritus_decay_eff | MIN (0.3) | 20/20 |
| spawn_offset | MAX (30) | 19/20 |
| part_pickup_rate | MIN (0.1) | 18/20 |
| part_deposit_rate | MAX (5.0) | 17/20 |
| pred_digest_time | MIN (0.2) | 17/20 |
| detritus_fraction | MIN (0.0) | 17/20 |
| prey_move_cost | MAX (0.25) | 16/20 |
| pred_move_cost | MAX (0.08) | 17/20 |

Any next experiment must widen bounds before re‑running CMA‑ES, otherwise the search is re‑exploring an exhausted space. See §3 P6 for recommended bound changes.

## 2) Hypotheses (root‑cause candidates)

### H1) Predation does not gate on neural bite intent

**Evidence in code**: In `game/simulation.go`, `updateFeeding()` (lines 144–253) checks hunting efficiency (diet), digest cooldown, hunt cooldown, and target proximity before attempting a bite. However, it does **not** check whether the neural network's bite output exceeds the deadzone. The brain's bite output is stored as `energy.LastBite` (line ~93), and bite **cost** is charged in `systems/energy.go` (lines 51–58) only when `energy.LastBite > cachedThrustDeadzone`.

**Consequence**: A predator can land bites and receive bite_reward energy without the brain signaling bite intent, and without paying bite cost. The brain's bite output only controls whether the energy cost is charged, not whether the bite happens.

**Hypothesis**: This decoupling means predators receive free energy from bites regardless of learned behavior. This makes predation too easy early on, driving prey crashes before predators have evolved hunting skill.

**Expected signature**: High predator counts, rapid prey crash, then predator extinction as prey vanishes. This matches exp11.

### H2) Reproduction is non‑conservative (energy leak / creation)

**Evidence**: In `game/simulation.go` (line ~430), reproduction reduces parent energy: `energy.Value *= ParentEnergySplit`. The child receives a fixed `ChildEnergy` (line ~500). These are independent values — child energy is not drawn from the parent.

With eval416 values (`parent_energy_split=0.4`, `child_energy=0.5`): a parent at energy 0.95 loses 0.57 but child gets 0.50 — slight net destruction. At lower parent energies (near threshold 0.95), parent loses ~0.57 and child gets 0.50. At higher parent energies, the gap widens. The direction of the leak depends on parent energy at reproduction time.

**Hypothesis**: Non‑conservative reproduction causes long‑term energy drift. This destabilizes populations and undermines energy‑economy invariants.

**Expected signature**: Energy pool drift unrelated to resource input, weak relationship between resource/utilization and population equilibrium.

### H3) Diet dead‑zone acts as implicit speciation pressure

**Evidence**: With `grazing_diet_cap=0.15` and `hunting_diet_floor=0.6007` (eval416 config), any diet in **[0.15, 0.60]** can neither graze nor hunt effectively.

**Nuance**: The optimizer explicitly chose a wide dead zone across the top 50 evals (mean gap = 0.50, vs 0.35 for bottom 50). This suggests the dead zone serves as **implicit speciation pressure** — it forces organisms toward diet extremes, maintaining role separation.

**However**: Mutation can push organisms into the dead zone, where they inevitably starve. This reduces effective population size and amplifies crashes, especially when one diet extreme is already under pressure.

**Trade‑off**: Removing the dead zone risks generalist dominance, which could collapse predator‑prey dynamics entirely. Narrowing it (rather than eliminating it) may be the better approach.

**Expected signature**: Diet distribution collapsing toward 0/1, with poor omnivore survival. Observed: diet mean and std collapse after predator extinction.

### H4) Optimizer actively rejected detritus recycling

**Evidence**: `carcass_fraction=0.3` (MIN), `decay_efficiency=0.3` (MIN), and `detritus_fraction=0.0` (MIN) in 15–20 of the top 20 evals. The optimizer consistently minimized all recycling parameters.

**Interpretation**: This is not "recycling is too weak" — the optimizer found that detritus creates positive feedback (more death → more resources → more growth → more death → crash). Minimizing recycling reduces this destabilizing loop.

**Implication**: Simply increasing detritus parameters would likely make things worse. If recycling is to help, it needs a stabilizing mechanism (e.g., slow decay rate, spatial spreading) rather than just higher values. Alternatively, recycling may only become beneficial once other fixes (H1, H2) reduce crash amplitude.

### H5) Fitness objective rewards delayed collapse, not stability

**Evidence**: Fitness = ‑(survivalTicks × (1 + 0.2 × quality)). The quality bonus includes stability (CV, 25% weight) but survival time dominates. A run that oscillates wildly for 800 ticks then collapses scores better than a stable run that collapses at 700 ticks.

**Hypothesis**: CMA‑ES is optimizing for **runs that delay collapse** rather than runs that achieve true equilibrium. The quality bonus is too weak (20% multiplier) to override survival time.

**Expected signature**: Short‑term "stable ecosystem" bookmarks, followed by delayed collapse. This matches `experiments/exp11/eval416/bookmarks.csv`.

## 3) Proposed fixes / changes

### P1) Gate predation on bite output

**Change**: In `game/simulation.go` `updateFeeding()`, skip the bite attempt when `energy.LastBite <= thrustDeadzone`. This makes the neural network's bite output actually control whether a bite happens, not just whether it costs energy.

**Why**: Aligns predation with neural intent. Predators must evolve to signal bite at the right time. Reduces free energy from accidental proximity.

**Success criteria**:
- Prey crash amplitude decreases.
- Predators don't spike as sharply in early phase.
- Predator brains evolve meaningful bite timing.

### P2) Make reproduction energy‑conservative

**Change**:
- Child energy = parent's lost energy: `childEnergy = parentEnergy × (1 ‑ parentEnergySplit)`.
- Remove the fixed `child_energy` config parameter (or keep it as a cap).
- If the resulting child energy is too low, skip reproduction.

**Why**: Energy economy invariants depend on conservation. Reproduction should redistribute energy, not create or destroy it.

**Success criteria**:
- Total organism energy + resource + detritus + heat remains stable relative to particle input.
- Long‑term energy drift decreases.

### P3) Narrow (don't remove) diet dead‑zone

**Change**: Reduce the gap between `grazing_diet_cap` and `hunting_diet_floor` but maintain some separation. Example: cap=0.35, floor=0.45 (gap=0.10 instead of 0.45).

**Why**: Some dead zone provides speciation pressure that the optimizer found beneficial. But the current 0.45 gap is too wide — it kills too many mutants and collapses diversity after any population shock.

**Success criteria**:
- Diet distribution maintains variance (std > ~0.05) in long runs.
- Both diet extremes remain populated.

### P4) Detritus: defer until P1+P2 stabilize dynamics

**Rationale**: The optimizer rejected recycling because it amplifies boom‑crash cycles. After P1 (gated biting) and P2 (conservative reproduction) reduce crash amplitude, recycling may become beneficial. Re‑evaluate detritus parameters only after verifying P1+P2 improve stability.

### P5) Strengthen fitness quality signal

**Change**: Increase the quality multiplier from 0.2 to 0.5 or higher. Add explicit penalties:
- Windows where either species is at 0: subtract survival bonus for those windows.
- High population CV: reduce quality score.
- Consider minimum coexistence time as a hard floor.

**Why**: The current 20% quality bonus is too weak to steer the optimizer toward true stability. Survival time alone rewards delayed collapse.

**Success criteria**:
- Optimizer finds configs that sustain both species longer with lower oscillation amplitude.

### P6) Widen optimizer bounds for next experiment

Recommended bound changes based on convergence analysis:

| Parameter | Current bounds | Suggested bounds |
|-----------|---------------|-----------------|
| pred_repro_thresh | [0.50, 0.95] | [0.50, 0.99] |
| pred_density_k | [10, 600] | [10, 2000] |
| prey/pred_cooldown | [4/6, 20] | [4/6, 60] |
| prey_move_cost | [0.05, 0.25] | [0.05, 0.50] |
| part_cell_capacity | [0.3, 2.0] | [0.05, 2.0] |
| refugia_strength | [0.5, 1.5] | [0.5, 3.0] |

## 4) Priority order for implementation

1. **P1** (gate bite on neural output) — fix the core predation bug.
2. **P2** (conservative reproduction) — fix energy leak.
3. **P3** (narrow diet dead‑zone) — reduce mutant starvation.
4. **P5** (fitness signal) — align optimizer with stability.
5. **P6** (widen bounds) — give optimizer room to explore.
6. **P4** (detritus tuning) — re‑evaluate after P1+P2.

## 5) Suggested next experiment setup

- Apply P1 + P2 as code changes.
- Start with eval416 config (adjusted for P2's removal of fixed child_energy).
- Run a 1–2 hour headless simulation, log telemetry.
- If predator extinction still occurs within ~1000s, apply P3.
- After system behavior improves, apply P5 + P6 and re‑run CMA‑ES.

## 6) Expected outcomes if fixes work

- Predator population no longer collapses to zero early.
- Prey oscillations dampen rather than explode.
- Energy budget is conservative — total pools track particle input.
- Diet distribution retains variance (not collapsing to pure herbivory).
- Long‑term runs survive significantly beyond 1700s without total extinction.

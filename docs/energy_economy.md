# Energy Economy Spec (Particle Resources + Detritus + Organism Energy)

Status: implementation-ready spec

This document proposes a full energy economy that preserves the current visual/emergent dynamics from the particle resource system while adding explicit energy accounting via detritus, losses, and organism energy stores. It also outlines how to align the system with future CPPN-driven morphology and adaptive neural substrates.

---

## 1) Goals and Constraints

- Keep the **particle resource field** as the sole external energy input (sunlight analog) to preserve emergent spatial dynamics.
- Introduce **explicit energy accounting** (resource, organism, detritus, heat/loss).
- Support **future morphology scaling** (e.g., CPPN-defined capabilities) using **absolute energy units** while providing normalized inputs to brains.
- Avoid double energy injection (no extra regrowth in addition to particle spawning).

### Consistency invariant (source discipline)

Particles are the **only external source** of energy. Detritus decay is recycling, not a new source. There must be **no hidden regrowth** into `Res` beyond Det -> Res conversion and particle deposition.

### Conservation equation

At all times:

```
Total = Res + Det + sum(E) + Heat
```

Where `Heat` is the cumulative loss to the sink. This must be tracked explicitly for debugging—otherwise you cannot distinguish intended inefficiency from bugs or numeric drift.

---

## 2) Conceptual Model: Energy Pools and Flows

### Pools
- **Resource grid** `Res(x,y)` (existing): chemical energy available to herbivores (absolute energy density, not [0..1]).
- **Detritus grid** `Det(x,y)` (new): dead biomass and waste (absolute energy density), decays back to resource.
- **Organism energy** `E_i` (existing): absolute stored energy for each agent.
- **Heat / loss sink** `HeatLossAccum` (new): cumulative energy lost to metabolism, inefficiency, and decay. Tracked explicitly for validation.

Detritus is the intermediate recycling buffer between death/waste and new resource. It provides ecosystem inertia, enables scavenger niches, and creates real nutrient cycling.

### Transfers (high-level)
1. **Sunlight input** (external source): particle spawning -> grid deposition.
2. **Grazing**: Res -> organism with efficiency, remainder lost to heat.
3. **Predation**: prey energy -> predator energy with efficiency; remainder to detritus + heat.
4. **Metabolism/motion**: organism -> heat.
5. **Death**: organism -> detritus (fraction) + heat (remainder). Death is instant; decay is slow.
6. **Detritus decay**: detritus -> resource with efficiency; remainder -> heat.

Energy losses at each transfer are intentional and ecologically grounded (trophic transfer inefficiency).

---

## 3) Key Design Decisions

These decisions must be explicit to avoid tuning ambiguity.

### 3.1 Overflow routing: detritus (not heat)

When `E > MaxEnergy`:
```go
overflow := E - MaxEnergy
E = MaxEnergy
DepositDetritus(pos, overflow)
```

Overflow is "waste" not metabolism—it creates spatial nutrient hotspots and prevents full efficiency hoarding. Heat sink is for respiration/costs only.

### 3.2 Heat tracking: explicit accumulator

Add one scalar to world state:
```go
world.HeatLossAccum += loss
```

This enables validation: `Total = Res + Det + sum(E) + Heat` must remain constant (modulo particle input).

### 3.3 Death conversion: instant carcass, slow decay

Corpses appear instantly as detritus (`Det += eta_carcass * E`). The decay system then slowly converts detritus back to resource. This provides the right timescales: fast death, slow recycling.

### 3.4 Predation: removed is the conserved quantity

```go
removed := min(prey.E, bite_amount)
prey.E -= removed
pred_gain := eta_pred * removed
```

The predator gets only a **fraction** of what the prey loses. Never have predator gain and prey loss be independent values.

---

## 4) Energy Units and Normalization

### Internal representation
- Use **absolute energy units** (not clamped to 0..1).
- Add **MaxEnergy** per organism (derived from morphology).
- Overflow is routed to detritus (see 3.1).

### Brain inputs
Expose **normalized ratios**:
- `energy_ratio = E / MaxEnergy`
- `energy_to_cost = E / (base_cost_per_sec * tau)` (optional, for "time to die" signal)

This keeps networks stable while preserving real energy accounting for the ecology.

### Unit scale invariant (initial tuning target)

Pick a scale so numbers are interpretable:

- `E0 = 1.0` (initial energy in absolute units)
- `MaxEnergy = 1.0` initially (before morphology scaling)
- `base_cost` in **energy/sec**
- `graze_rate` in **Res energy/sec** at full resource signal

Balance target for a typical herbivore on a rich patch at grazing speed:

```
eta_graze * graze_rate ≈ base_cost
```

---

## 5) Detailed Energy Economy Cycle (per tick)

Let `dt = cfg.Derived.DT32`.

### 5.0 Update ordering

1. Particle step (spawn/advect/deposit/pickup) -> updates `Res`
2. Grazing (organisms extract from `Res`)
3. Predation (energy transfer between organisms)
4. Metabolism + action costs
5. Deaths -> detritus deposit
6. Detritus decay -> `Res`

This prevents hidden sources and avoids instant same-tick recycling of full carcasses.

### 5.1 Particle Resource Field (external input)
Particle spawning is the only external energy input. It continues to be driven by the potential field and flow dynamics:
- `spawn_rate`, `initial_mass`, `deposit_rate`, `pickup_rate` remain as global controls.
- No explicit regrowth term is added to avoid double counting.

### 5.2 Grazing (herbivore channel)

```
removed = Resource.Graze(x, y, graze_rate, dt, graze_radius)
gain = eta_graze * removed
loss = (1 - eta_graze) * removed
E += gain
HeatLossAccum += loss
```

- `graze_rate` remains rate-limited by resource availability and morphology.
- If `E > MaxEnergy`: overflow -> detritus (see 3.1).
- Grazing must deplete **Res energy**, not a normalized "resource signal."

### 5.3 Predation (carnivore channel)

```
removed = min(prey.E, bite_amount)
pred_gain = eta_pred * removed * (1 - eta_det)
det_gain = eta_det * removed
loss = removed - pred_gain - det_gain
pred.E += pred_gain
prey.E -= removed
Det += det_gain (at prey position)
HeatLossAccum += loss
```

- `eta_det` is optional (0 by default). It introduces "messy kills" and supports scavengers later.
- Split **attempt**, **success**, and **transfer**:
  - attempt: bite cost always paid when bite output triggers
  - success: gated by geometry, digest, and optional refugia/defense
  - transfer: only on success
- `bite_amount` is a **rate limiter** (max removed per successful bite), not a source.

### 5.4 Metabolism and motion

```
cost := base_cost * dt
cost += move_cost * f(speed) * dt
cost += accel_cost * thrust^2 * dt
cost += bite_cost (when biting)
E -= cost
HeatLossAccum += cost
```

All metabolic drains go to the heat sink.

### 5.5 Death -> detritus

When an organism dies (must happen **before** entity removal):
```
det := eta_carcass * E
Det += det (at entity position)
HeatLossAccum += (1 - eta_carcass) * E
E = 0
```

- Deposit should occur **exactly once** at the entity's last position.
- Prefer a `DeathProcessed` flag or an explicit "death system" step to avoid double deposit.

### 5.6 Detritus decay -> resource

```
decayed = decay_rate * Det * dt
Res += eta_decay * decayed
HeatLossAccum += (1 - eta_decay) * decayed
Det -= decayed
```

---

## 6) Tuning Parameters (by category)

### External input (source)
- `particles.spawn_rate`
- `particles.initial_mass`
- `particles.deposit_rate`
- `particles.pickup_rate`

### Transfer efficiencies (loss control)
- `energy.graze_efficiency` (eta_graze)
- `energy.pred_efficiency` (eta_pred)
- `detritus.carcass_fraction` (eta_carcass)
- `detritus.decay_efficiency` (eta_decay)
- Optional `energy.detritus_from_predation` (eta_det)

### Costs (sinks)
- `energy.prey.base_cost`
- `energy.prey.move_cost`
- `energy.prey.accel_cost`
- `energy.predator.base_cost`
- `energy.predator.move_cost`
- `energy.predator.accel_cost`
- `energy.predator.bite_cost`

### Rate limits (ration inputs)
- `energy.prey.forage_rate` (max grazing rate)
- `energy.predator.bite_reward` or `bite_amount`
- `energy.predator.digest_time`

---

## 7) Implementation Patterns (Go)

### 7.1 Pure energy transfer function

Avoid scattered efficiency logic. Create one authority:

```go
type EnergyTransfer struct {
    Removed   float32
    ToGainer  float32
    ToDet     float32
    ToHeat    float32
}

func Transfer(removed, eta, detFrac float32) EnergyTransfer {
    toGainer := eta * removed * (1 - detFrac)
    toDet := detFrac * removed
    toHeat := removed - toGainer - toDet

    return EnergyTransfer{removed, toGainer, toDet, toHeat}
}
```

Every system uses this for consistency.

### 7.2 Dual-grid resource field

```go
type ParticleResourceField struct {
    Res []float32
    Det []float32
}

func (f *ParticleResourceField) DepositDetritus(x, y int, amount float32) {
    i := f.Index(x, y)
    f.Det[i] += amount
}

func (f *ParticleResourceField) StepDetritus(dt float32, cfg DetritusConfig, heat *float32) {
    for i := range f.Det {
        decayed := cfg.DecayRate * f.Det[i] * dt
        f.Det[i] -= decayed
        f.Res[i] += cfg.DecayEff * decayed
        *heat += (1 - cfg.DecayEff) * decayed
    }
}
```

### 7.3 Absolute energy with overflow

In `components.Energy`:

```go
type Energy struct {
    E   float32
    Max float32
}
```

In update:

```go
e.E -= metabolicCost * dt
if e.E <= 0 {
    // mark dead
}
if e.E > e.Max {
    overflow := e.E - e.Max
    e.E = e.Max
    field.DepositDetritus(pos, overflow)
}
```

No more normalization inside ecology.

### 7.4 Detritus on death in cleanupDead()

Critical: must happen **before** entity removal:

```go
if dead {
    det := cfg.CarcassFrac * energy.E
    field.DepositDetritus(pos, det)
    world.HeatLossAccum += (1 - cfg.CarcassFrac) * energy.E
}
```

---

## 8) Codebase Touch Points

### Resource system
File: `systems/particle_resource.go`
- Existing: `Res[]`, particle spawn/deposit/pickup, `Graze()` removes from `Res`.
- Add: `Det[]`, `DepositDetritus()`, `StepDetritus()`; optional `DetData()` for telemetry.

Interface: `systems/resource.go`
- Extend or add new interface (e.g., `EnergyField`) with detritus accessors.

### Energy / feeding
File: `game/simulation.go`
- `updateFeeding()` uses `systems.TransferEnergy` with `bite_reward`.
- `updateEnergy()` performs grazing and metabolism.
- Update to: apply efficiencies + detritus; handle overflow; use bite cost signal.

File: `systems/energy.go`
- `UpdateEnergy()` drains costs and clamps; add MaxEnergy logic.
- `TransferEnergy()` should return `removed` and accept efficiencies, or be replaced by new `Transfer()` helper.

### Death handling
File: `game/lifecycle.go`
- `cleanupDead()` removes entities. Add detritus deposit before removal (needs position lookup).

### Telemetry / snapshots
File: `game/telemetry_hooks.go`
- Add totals: sum Res, sum Det, sum E, HeatLossAccum.
- Add flows: grazed, predated, metabolic loss, death_to_detritus, decay_to_resource, particle_input.

File: `telemetry/*`
- Extend stats structures and snapshots to store `Det` grid.

### Rendering / inspector (optional)
- Add detritus overlay toggle (similar to resource heatmap).
- Add conservation debug overlay showing `Res + Det + E + Heat = Total`.

---

## 9) Resource Field API Contracts

- `Graze(...) -> removedEnergy`
  - Must subtract exactly `removedEnergy` from `Res` (never negative).
  - Returns **absolute energy** removed (not normalized).
- `DepositDetritus(x,y, amount) -> depositedEnergy`
  - Adds to `Det` with a kernel; return actual deposited (normally = amount).
- `StepDetritus(dt) -> (toRes, loss)`
  - Moves energy `Det -> Res` and accounts for loss (heat).
- `ParticleInputThisTick` counter
  - Tracks actual energy injected by particles per tick for telemetry.

---

## 10) Parameter Defaults (first-pass)

These align with current tuning but enforce loss at each step:

- `eta_graze = 0.75–0.85`
- `eta_pred = 0.65–0.80` (typically lower than grazing)
- `eta_det = 0.00–0.10` (optional mess fraction)
- `eta_carcass = 0.60–0.80`
- `det_decay_rate = 0.02–0.08 / sec`
- `det_decay_eff = 0.40–0.70`

Keep particle input as the primary source:
- `particles.spawn_rate` and `initial_mass` already act as sunlight.

---

## 11) Implementation Sequence

Ordered for debuggability—each step should be stable before the next:

1. **Add `Det[]` + decay step** (no gameplay yet)
   - Add detritus grid to `ParticleResourceField`
   - Implement `StepDetritus()` with decay -> Res
   - Verify grid works in isolation

2. **Add `HeatLossAccum` scalar**
   - Track all losses explicitly
   - Add to telemetry output

3. **Convert organism energy from 0..1 -> absolute + MaxEnergy**
   - Update `components.Energy` struct
   - Update all energy comparisons
   - Normalize only for brain inputs

4. **Grazing efficiency + overflow -> detritus**
   - Apply `eta_graze` to grazing gains
   - Route overflow to detritus instead of clamping

5. **Death deposits carcass**
   - Deposit `eta_carcass * E` to detritus on death
   - Track remainder to heat

6. **Predation transfer with detritus fraction**
   - Use `Transfer()` helper for consistent accounting
   - Optional `eta_det` for messy kills

7. **Telemetry totals + conservation debug overlay**
   - Show `Res + Det + E + Heat = Total`
   - Alert on conservation violations

8. **Species array + diet scaling** (last, once stable)
   - Generalize predator/prey to diet spectrum

---

## 12) Validation Checklist

### Energy conservation
- Total energy (`Res + Det + sum(E) + Heat`) stays constant (modulo particle input).
- `conservation_error = delta_total - particle_input + heat_loss` should be ~0.

### Energy half-life test
If particles are disabled:
- System should decay smoothly to zero
- No energy pool should increase
- This is the best sanity test for leaks

### Ecosystem dynamics
- Increasing `particles.spawn_rate` raises total ecosystem energy; increasing costs reduces it.
- Higher `eta_*` should increase standing biomass but risks runaway if too high.
- Removing predators should increase prey energy and detritus; reintroducing predators should stabilize.
- Mixed diet species should form stable niches (no immediate collapse to one extreme).

---

## 13) Species, Diet, and Softening Predator/Prey Distinctions

To move toward fully generative morphology and remove hard predator/prey categories, introduce a **species-based configuration** and a **continuous diet spectrum**. Start with two species that mimic the current herbivore/carnivore split, but use shared systems and continuous parameters.

### 13.1 Species config (array)

Replace `energy.prey` / `energy.predator` with:

```yaml
species:
  - name: herbivore
    diet: 0.05
    capabilities: ...
    vision_weights: ...
    energy: ...
    movement: ...
  - name: carnivore
    diet: 0.95
    capabilities: ...
    vision_weights: ...
    energy: ...
    movement: ...
```

**Diet spectrum**
- Define `diet` in `[0,1]` (0 = pure herbivore, 1 = pure carnivore).
- For now, constrain to `[0.1, 0.9]` to avoid degenerate extremes.

Derived scalars:
```
herb = 1 - diet
carn = diet
```

Apply these scalars consistently:
- **Grazing rate**: `graze_rate = base_graze * herb * mouth_factor`
- **Bite rate**: `bite_rate = base_bite * carn * jaw_factor`
- **Efficiencies**:
  - `eta_graze = base_eta_graze * herb + min_eta_graze * (1 - herb)`
  - `eta_pred  = base_eta_pred  * carn + min_eta_pred  * (1 - carn)`

### 13.2 Species-driven capabilities

Map `capabilities` and `movement` directly from species:
- `MaxSpeed`, `MaxAccel`, `VisionRange`, `BiteRange`, `Drag`, `MaxEnergy`
- `base_cost`, `move_cost`, `accel_cost`, `bite_cost`

Future CPPN mapping: use CPPN outputs to build these directly, but keep the same fields so behavior stays stable during the transition.

### 13.3 Sensor channels: friend / food / foe

Convert sensors to **3 channels** per sector:
- `friend`: same species (eventually genome similarity / clade)
- `food`: **edibility** (I can eat them)
- `foe`: **inverse edibility** (can eat me)

This is a huge unlock—it replaces all brittle "predator flag" logic. Eventually:
```go
friend = exp(-genome_distance)
```

Proposed edibility model:

```
// a eats b if diet aligns with b energy type and size range
edibility(a, b) = clamp01(
  w_diet * diet_match(a.diet, b.diet) +
  w_size * size_match(a.body, b.body) +
  w_range * range_match(a, b)
)

food = edibility(self, other)
foe = edibility(other, self)
friend = 1 if species == species
```

Initial simplification:
- `food = other.is_organism && other.species != self.species && self.diet > 0.5`
- `foe = other.is_organism && other.species != self.species && other.diet > 0.5`

Then move to continuous edibility based on diet + size.

### 13.4 Behavior changes

Replace predator-only bite with **opportunistic feeding**:
- Any organism can attempt a bite, but its bite rate and efficiency are scaled by `carn`.
- Grazing is scaled by `herb`.
- This removes hard-coded predator/prey branches and creates mixed feeders.

### 13.5 Reproduction tuning

Move to a **single reproduction config** with per-species modifiers:

Global:
```yaml
reproduction:
  maturity_age: ...
  cooldown: ...
  threshold: ...
  parent_energy_split: ...
  child_energy: ...
```

Per species:
```yaml
species[i].repro:
  threshold_mult
  cooldown_mult
  child_energy_mult
```

Final thresholds per organism:
```
threshold = repro.threshold * species.repro.threshold_mult
cooldown  = repro.cooldown  * species.repro.cooldown_mult
child_E   = repro.child_energy * species.repro.child_energy_mult
```

Optional energy scaling:
- If `E/MaxEnergy` is high, reduce cooldown or allow multiple births.
- If `E/MaxEnergy` is low, block reproduction.

### 13.6 Migration path

1) Add `SpeciesID` to `Organism`, and species array to config.
2) Convert sensors to friend/food/foe channels.
3) Replace predator/prey branches with diet-scaled logic.
4) Introduce edibility-based bite selection.
5) Remove predator/prey config fields when stable.

---

## 14) Source of Truth for Particles (decision)

To preserve emergent particle dynamics **and** maintain correct accounting, choose one:

1) **CPU-authoritative particles (recommended)**
   Particle simulation and `Res` updates happen on CPU. GPU renders from `Res` (and optional visual particles).
   - Pros: exact energy accounting, no readback complexity.
   - Cons: CPU cost.

2) **GPU-authoritative particles**
   GPU sim is truth; CPU must read back or mirror particle state.
   - Pros: fast visuals.
   - Cons: heavy readback/latency, hard to guarantee accounting.

If the goal is strict accounting and deterministic telemetry, prefer **CPU-authoritative**.

---

## 15) Notes for CPPN Morphology

When CPPN defines morphology:
- `MaxEnergy` scales with body mass.
- `base_cost`, `move_cost`, `bite_cost`, `accel_cost` scale with mass and capability sizes.
- Neural inputs remain normalized (`E / MaxEnergy`) so learning is stable across morphologies.

---

## Appendix: Current Files and Behavior Summary

- `systems/particle_resource.go`:
  - particle-based resource field with mass transport; `Res[]` grid.
  - `Graze()` removes resource and returns removed amount.
- `game/simulation.go`:
  - `updateFeeding()` performs predator bites (uses `systems.TransferEnergy`).
  - `updateEnergy()` handles grazing + metabolism.
- `systems/energy.go`:
  - `UpdateEnergy()` applies metabolic costs; energy clamped to 0..1.
  - `TransferEnergy()` moves energy prey -> predator with efficiency.
- `game/lifecycle.go`:
  - `cleanupDead()` removes dead entities; no resource recycling.

These are the key touch points for the spec above.

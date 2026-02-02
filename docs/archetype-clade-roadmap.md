# Archetype System + Clade Tracking Roadmap

This document tracks the phased implementation of the archetype/clade system.

## Overview

The goal is to replace hard-coded predator/prey with **archetypes** (config-defined templates) and add **clade tracking** (lineage identifiers). Diet becomes a continuous heritable trait, enabling emergent omnivory and speciation.

**Key principle**: Archetypes are founder templates, not permanent types. Traits (diet) drift through mutation; archetypes don't switch.

## Phase 1: Core Changes ✅ COMPLETED

Implemented in commit `9dc4fa9e`.

### What was done:
- Added `ArchetypeConfig` and `CladeConfig` to config
- Added `Diet`, `CladeID`, `FounderArchetypeID` to `Organism` component
- `MutateSparse` returns `avgAbsDelta` for clade split decisions
- Reproduction includes diet mutation and clade split logic
- Telemetry tracks `active_clades` and clade info in HallOfFame
- `spawnEntity` takes `archetypeID` instead of `Kind`
- Full backwards compatibility via `Kind` derived from `Diet >= 0.5`

---

## Phase 2: Diet-Scaled Energy/Grazing Interpolation

**Goal**: Make energy economics vary smoothly with diet instead of binary prey/predator branching.

### 2.1 Config Changes

Add interpolation parameters to allow smooth transitions:

```yaml
energy:
  # Existing prey/predator sections remain for reference
  # Add interpolation config:
  interpolation:
    enabled: true           # When true, use diet-based interpolation
    grazing_diet_cap: 0.3   # Diet above this = no grazing benefit
    hunting_diet_floor: 0.7 # Diet below this = no hunting benefit
```

### 2.2 Energy System Changes

**File**: `systems/energy.go`

Replace binary Kind checks with diet-based interpolation:

```go
// Current (Phase 1):
if org.Kind == components.KindPrey {
    // prey energy logic
} else {
    // predator energy logic
}

// Phase 2:
diet := org.Diet

// Grazing efficiency: linear falloff from 1.0 at diet=0 to 0 at diet=grazing_cap
grazeEff := max(0, 1.0 - diet/cfg.Energy.Interpolation.GrazingDietCap)

// Hunting efficiency: linear ramp from 0 at diet=hunting_floor to 1.0 at diet=1.0
huntEff := max(0, (diet - cfg.Energy.Interpolation.HuntingDietFloor) /
                  (1.0 - cfg.Energy.Interpolation.HuntingDietFloor))

// Interpolate costs
baseCost := lerp(cfg.Energy.Prey.BaseCost, cfg.Energy.Predator.BaseCost, diet)
moveCost := lerp(cfg.Energy.Prey.MoveCost, cfg.Energy.Predator.MoveCost, diet)
```

### 2.3 Grazing Changes

**File**: `game/simulation.go` (updateEnergy)

Scale foraging by grazing efficiency:

```go
// Only graze if diet supports it
if grazeEff > 0 {
    grazeRate := resourceHere * forageRate * eff * grazeEff
    removed := g.resourceField.Graze(pos.X, pos.Y, grazeRate, dt, grazeRadius)
    gain := removed * forageEfficiency * grazeEff
    energy.Value += gain
}
```

### 2.4 Feeding Changes

**File**: `game/simulation.go` (updateFeeding)

Scale bite effectiveness by hunting efficiency:

```go
// Only hunt if diet supports it
huntEff := calcHuntingEfficiency(org.Diet, cfg)
if huntEff <= 0 {
    continue // Can't hunt with this diet
}

// Scale bite reward by hunting efficiency
biteReward := float32(cfg.Energy.Predator.BiteReward) * huntEff
transferred := systems.TransferEnergy(energy, nEnergy, biteReward)
```

### 2.5 Verification

- Organisms with diet=0.0 behave identically to Phase 1 prey
- Organisms with diet=1.0 behave identically to Phase 1 predators
- Organisms with diet=0.5 can partially graze AND partially hunt (true omnivores)
- Run long simulations to verify diet distributions evolve naturally

---

## Phase 3: Unified Food/Threat/Kin Sensor Channels

**Goal**: Replace prey/predator-specific vision with diet-relative perception.

### 3.1 Sensor Redesign

Current sensors detect "prey signal" and "predator signal" separately. This breaks down when diet is continuous.

New approach: **Food/Threat/Kin** channels based on relative diet:

```go
// For each neighbor:
dietDiff := neighbor.Diet - self.Diet

// Food signal: things I can eat (lower diet than me)
// Stronger signal for larger diet difference
if dietDiff < -0.2 {  // At least 0.2 lower diet
    foodSignal += calcSignal(dist, -dietDiff)
}

// Threat signal: things that can eat me (higher diet than me)
if dietDiff > 0.2 {  // At least 0.2 higher diet
    threatSignal += calcSignal(dist, dietDiff)
}

// Kin signal: similar diet (potential mates, competitors)
if abs(dietDiff) < 0.2 {
    kinSignal += calcSignal(dist, 1.0 - abs(dietDiff)*5)
}
```

### 3.2 Neural Input Changes

**File**: `systems/sensors.go`

Replace current inputs:
```
Current: [prey_sector_0..7, pred_sector_0..7, wall_sector_0..7, energy, speed]
         = 8*3 + 2 = 26 inputs

New:     [food_sector_0..7, threat_sector_0..7, kin_sector_0..7, wall_sector_0..7,
          energy, speed, diet]
         = 8*4 + 3 = 35 inputs
```

Or compressed version keeping 26 inputs:
```
New:     [food_sector_0..7, threat_sector_0..7, wall_sector_0..7, energy, speed]
         = 8*3 + 2 = 26 inputs (drop kin, rely on emergent behavior)
```

### 3.3 Config Changes

```yaml
sensors:
  num_sectors: 8
  # New:
  diet_perception_threshold: 0.2  # Min diet difference to register as food/threat
  kin_diet_range: 0.2             # Diet range for kin detection
```

### 3.4 Migration Strategy

1. Add new sensor computation alongside old
2. Add config flag `sensors.use_diet_relative: false`
3. When enabled, use new food/threat/kin channels
4. Verify behavior is similar before making default

---

## Phase 4: Remove Kind-Based Branching

**Goal**: Eliminate all `org.Kind` checks, making diet the sole determinant of behavior.

### 4.1 Audit All Kind References

Search for all Kind usage:
```bash
grep -r "org\.Kind" --include="*.go"
grep -r "KindPrey\|KindPredator" --include="*.go"
```

Expected locations to update:
- `game/simulation.go`: updateFeeding, updateReproduction
- `game/lifecycle.go`: population counting, respawn logic
- `systems/sensors.go`: sensor computation (handled in Phase 3)
- `telemetry/`: stats collection by kind

### 4.2 Replace Kind-Based Population Tracking

Current:
```go
g.numPrey++  // or g.numPred++
```

New approach - track by diet range:
```go
type PopulationBuckets struct {
    Herbivores int  // diet < 0.3
    Omnivores  int  // 0.3 <= diet < 0.7
    Carnivores int  // diet >= 0.7
}
```

Or just track total population and derive distribution from organisms.

### 4.3 Update Telemetry

Replace prey/pred counts with diet distribution stats:
```go
type WindowStats struct {
    // Replace:
    // PreyCount int
    // PredCount int

    // With:
    TotalPopulation int
    MeanDiet        float64
    DietStdDev      float64
    DietP10         float64  // 10th percentile
    DietP50         float64  // Median
    DietP90         float64  // 90th percentile

    // Or buckets:
    HerbivoreCount int  // diet < 0.3
    OmnivoreCount  int  // 0.3-0.7
    CarnivoreCount int  // diet > 0.7
}
```

### 4.4 Deprecate Kind Field

1. Mark `Kind` field as deprecated in comments
2. Keep for one version for backwards compatibility with snapshots
3. Remove in subsequent version

### 4.5 Config Migration

Remove prey/predator specific configs where possible:
```yaml
# Before:
capabilities:
  prey:
    vision_range: 100
  predator:
    vision_range: 140

# After:
capabilities:
  base_vision_range: 100
  vision_range_diet_bonus: 40  # Added at diet=1.0
```

---

## Implementation Order

1. **Phase 2** (Diet-scaled energy) - Can be done independently
   - Low risk, preserves existing behavior at diet extremes
   - Enables testing omnivore viability

2. **Phase 3** (Unified sensors) - Requires careful testing
   - May need neural network architecture changes
   - Should run parallel with old sensors initially

3. **Phase 4** (Remove Kind) - Final cleanup
   - Only after Phases 2-3 are stable
   - Breaking change for telemetry/snapshots

---

## Testing Strategy

### Phase 2 Tests
- Unit tests: energy interpolation functions
- Integration: verify diet=0 matches pure prey, diet=1 matches pure predator
- Long-run: verify omnivores (diet~0.5) are viable

### Phase 3 Tests
- Unit tests: food/threat/kin signal calculation
- Comparison: old vs new sensor outputs for known scenarios
- Behavioral: predators still chase, prey still flee

### Phase 4 Tests
- Regression: full simulation produces similar dynamics
- Telemetry: new diet stats are meaningful
- Snapshot: old snapshots still load (with migration)

---

## References

- Original spec: Conversation transcript from Phase 1 implementation
- Commit `9dc4fa9e`: Phase 1 implementation
- Config defaults: `config/defaults.yaml`

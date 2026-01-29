# Capability-Driven Architecture (Simplified Model)

This document describes the proposed architecture for capability-driven organisms using a simplified "primary + optional secondary" cell model. The focus is on how systems interact, where capabilities emerge, and how balance is enforced to avoid "goo" generalists.

---

## Goals
- Separate strategy (brain) from execution (pathfinding + physics)
- Make capabilities emerge from body structure rather than trait flags
- Reduce "everything-does-everything" cells via explicit tradeoffs
- Keep evolution open-ended but stable under selection

---

## System Overview

```
CPPN (5 → 10 outputs)
        ↓
Cell Selection + Modifiers
        ↓
Body Layout (capabilities)
        ↓
Sensing (polar cones)
        ↓
Brain (desire + intents)
        ↓
Pathfinding (turn/thrust)
        ↓
Physics + Energy
```

---

## Cells and Capabilities

### Functional Types
Each cell has one **primary** function from:
sensor, actuator, mouth, digestive, photosynthetic, bioluminescent, reproductive.

Optionally, a cell may include **one secondary** function if the CPPN's second-highest output clears a threshold. Secondary functions are scaled down.

### Modifiers
Structural and storage are **modifiers** rather than functions. They do not consume the primary/secondary slot, but they carry explicit costs to avoid becoming universal upgrades.

### Selection Rule (Per Cell)
```
raw = [sensor, actuator, mouth, digestive, photo, bio, repro]
primary = argmax(raw)
secondary = second_max(raw) if second_max(raw) >= secondary_threshold else None

primary_strength = raw[primary]
secondary_strength = raw[secondary] * secondary_scale  (if present)

if secondary exists:
  primary_strength *= mix_primary_scale
  secondary_strength *= mix_secondary_scale
```

Default parameters:
- `secondary_threshold = 0.25`
- `secondary_scale = 0.4`
- `mix_primary_scale = 0.85`
- `mix_secondary_scale = 0.35`

---

## Sensing

### Polar Cones (4 × 3 channels)
Vision is split into front/left/right/back cones, each with:
- food
- threat
- friend

Cone intensities use inverse-distance falloff and relevance from capability matching.

### Capability Matching (Food/Threat)
```
composition = photo_weight / (photo_weight + actuator_weight + epsilon)
edibility = clamp01(1.0 - abs(my_digestive_spectrum - their_composition))
penetration = clamp01(edibility - their_structural_armor)
```

Composition uses **summed primary + secondary strengths** across cells.

---

## Brain

### Inputs (15)
- 12 cone values (food/threat/friend × 4 directions)
- energy ratio, local light level, flow alignment

### Outputs (5)
- desire_angle, desire_distance
- eat_intent, grow_intent, breed_intent

The brain chooses **what** to do; it does not control **how** to execute movement or access capabilities.

---

## Pathfinding

Deterministic layer that converts desire into turn/thrust while handling terrain.
This removes terrain navigation from the brain and stabilizes evolution.

---

## Physics and Energy

### Energy Sources
- Feeding (mouth + digestive)
- Photosynthesis (photosynthetic cells)

### Energy Sinks
- Metabolism (per cell)
- Movement (actuator usage)
- Growth
- Reproduction

### Structural/Storage Costs
Tradeoffs prevent universal armor/storage:
```
metabolic_cost += base_cell_cost * (
  1.0
  + armor_cost_scale * structural_armor
  + storage_cost_scale * storage_capacity
)

drag_multiplier = 1.0 + drag_armor_scale * avg_structural_armor
```

Default parameters:
- `armor_cost_scale = 0.5`
- `storage_cost_scale = 0.35`
- `drag_armor_scale = 0.4`

---

## Balancing Principles

### Prevent "Goo"
- Limit functional mixing: one primary + optional secondary
- Penalize mixing with reduced effectiveness
- Charge explicit costs for armor/storage

### Encourage Diversity
- Sensors compete with actuators, mouths, and photo cells
- Energy capacity competes with speed and maneuverability
- Digestive spectrum interacts with composition, producing ecological niches

### Keep Strategy Emergent
- Brain is agnostic to terrain and body layout
- Capabilities are inferred from body composition, not flags

---

## Expected Dynamics
- Specialist body plans should win when the environment is stable or extreme.
- Generalists should survive in variable conditions but pay efficiency costs.
- Predation and diet niches emerge from digestion vs composition rather than hard-coded roles.

---

## Open Balance Knobs
- `secondary_threshold` (more/less mixing)
- `secondary_scale` and mix penalties (how costly mixing is)
- armor/storage cost scales (defensive vs metabolic tradeoff)
- cone count and distance falloff (perception sharpness)


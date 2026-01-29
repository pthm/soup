# Capability-Driven Architecture

A redesign of the neural evolution system to separate strategic decision-making from motor control, with capabilities emerging from body structure rather than trait flags.

## Design Philosophy

**Current problems:**
- Brain must learn both WHAT to do (strategy) and HOW to move (navigation)
- Terrain navigation overwhelms the network with motor control learning
- Traits are boolean flags rather than emergent properties
- Predator/prey distinction is hardcoded

**New approach:**
- Capabilities emerge from cell types (body IS the genome)
- Brain outputs desire/intent, pathfinding handles execution
- No hardcoded categories - "food" and "threat" computed from capability matching
- Reproduction strategy evolves (sexual/asexual spectrum)

---

## Cell Types

The CPPN queries each grid position and outputs continuous values. Each cell has one primary functional type, and can optionally include one secondary functional type at reduced effectiveness. Structural and storage remain modifiers, but carry a small cost.

| Cell Type | Property | Effect |
|-----------|----------|--------|
| **Sensor** | gain | Vision range, directional sensitivity |
| **Actuator** | strength | Movement force, thrust capability |
| **Mouth** | size | Can attempt consumption, bite size |
| **Digestive** | spectrum | What can be processed (0=flora, 1=fauna, 0.5=omnivore) |
| **Photosynthetic** | efficiency | Energy generation from light exposure |
| **Bioluminescent** | intensity | Light emission (illuminates surroundings) |
| **Structural** | armor | Rigidity, damage resistance, harder to digest |
| **Storage** | capacity | Energy holding (fat), increases max energy |
| **Reproductive** | spectrum | Reproduction mode (0=asexual, 1=sexual, 0.5=both) |

### Cell Composition and Selection

Functional outputs (sensor, actuator, mouth, digestive, photosynthetic, bioluminescent, reproductive) are treated as raw weights. Each cell chooses a single **primary** function via argmax, and optionally a **secondary** function if the second-highest weight clears a threshold. Structural and storage are treated as modifiers (not part of the functional budget), but add a small cost.

Example:
```
raw = [sensor, actuator, mouth, digestive, photo, bio, repro]
primary = argmax(raw)
secondary = second_max(raw) if second_max(raw) >= secondary_threshold else None

primary_strength = raw[primary]
secondary_strength = raw[secondary] * secondary_scale  (if present)
```

Optional mixing penalty (if secondary exists):
```
primary_strength *= mix_primary_scale
secondary_strength *= mix_secondary_scale
```

Parameter defaults:
- `secondary_threshold = 0.25`
- `secondary_scale = 0.4`
- `mix_primary_scale = 0.85`
- `mix_secondary_scale = 0.35`
- `epsilon = 1e-6`

### CPPN Outputs (per cell query)

```
Inputs:  x, y, distance_from_center, angle, bias  (5)

Outputs:
  cell_present        0/1 threshold
  sensor_gain         0-1
  actuator_strength   0-1
  mouth_size          0-1
  digestive_spectrum  0-1 (flora↔fauna)
  photosynthetic      0-1
  bioluminescent      0-1
  structural_armor    0-1
  storage_capacity    0-1
  reproductive_mode   0-1 (asexual↔sexual)
```

Total: 5 inputs, 10 outputs

---

## Vision System

### Polar Cone Architecture

Field of view divided into 4 directional cones (front, left, right, back), each reporting intensity for 3 channels:

```
        Front
          │
    ┌─────┼─────┐
    │  F  │  F  │
    │  r  │  o  │
Left├─────┼─────┤ Right
    │  o  │  o  │
    │  n  │  d  │
    └─────┼─────┘
          │
        Back
```

### Cone Channels

| Channel | What it detects | Calculation |
|---------|-----------------|-------------|
| **food** | Things I can eat | edibility vs their_structural armor |
| **threat** | Things that can eat me | their_edibility vs my_structural armor |
| **friend** | Similar organisms | Genetic similarity (NEAT compatibility distance) |

### Capability Matching

Use the same capability match function for both perception and feeding.

```
composition = photo_weight / (photo_weight + actuator_weight + epsilon)
edibility = clamp01(1.0 - abs(my_digestive_spectrum - their_composition))
penetration = clamp01(edibility - their_structural_armor)
```

Notes:
- `composition` is continuous; organisms with no photo/actuator use `composition = 0.5` (neutral).
- `photo_weight` / `actuator_weight` come from the sum of primary + (optional) secondary strengths across all cells.
- `penetration` is the final capability match strength (0-1).
- For threat perception, swap perspectives: use their_digestive_spectrum vs my_composition.

### Intensity Calculation

For each entity in a cone:
```
intensity = (1 / distance²) × relevance × light_modifier

where:
  relevance = penetration (food/threat) or normalized similarity (friend)
  light_modifier = local_light_level (darkness attenuates vision)
```

Parameter defaults:
- `distance_falloff = 2`

Cone value = sum of intensities for all entities in that direction.

### Sensor Cell Influence

- Total sensor gain affects detection range
- Sensor cell positions affect directional sensitivity
- Sensors facing a cone contribute more to that cone's perception

Concrete weighting:
```
weight = max(0, cos(theta)) ^ k
cone_value = sum(weight * sensor_gain)
normalize across cones per organism
```

Parameter defaults:
- `k = 4`

---

## Brain Network

### Inputs (15)

```
Polar vision (12):
  cone_food[4]        food intensity per direction (front/left/right/back)
  cone_threat[4]      threat intensity per direction
  cone_friend[4]      friend intensity per direction

Environment (3):
  energy_ratio        current_energy / max_energy
  light_level         local illumination (0-1)
  flow_alignment      dot(flow_direction, heading) - current helping or hindering
```

### Outputs (5)

```
Movement intent:
  desire_angle        where to go, relative to heading (-π to π)
  desire_distance     how urgently (0 = stay put, 1 = max pursuit)

Action intents:
  eat_intent          attempt to consume (0-1, threshold at 0.5)
  grow_intent         allocate energy to new cells (0-1)
  breed_intent        attempt reproduction (0-1)
```

### What the Brain Does NOT Control

- Navigation around terrain (pathfinding layer)
- Motor coordination (actuator physics)
- What it can eat (digestive cells)
- How fast it can move (actuator cells)
- How far it can see (sensor cells)

The brain learns WHAT to pursue. The body determines HOW.

---

## Pathfinding Layer

Sits between brain output and actuator execution. Not evolved - deterministic.

### Input
- desire_angle, desire_distance from brain
- Current position, heading
- Terrain data

### Process
```
1. Project target point from desire vector
2. Compute potential field:
   - Attractive force toward target
   - Repulsive force from nearby terrain
   - Flow field influence
3. Follow potential gradient
4. Convert to turn/thrust commands
```

### Output
- turn: heading adjustment for actuators
- thrust: forward force magnitude

### Terrain Handling

Brain never sees terrain. Pathfinding routes around obstacles automatically.

Benefits:
- No stupid deaths while learning to navigate
- Brain focuses on strategy, not motor control
- Terrain complexity doesn't slow evolution

Tradeoff:
- Can't evolve terrain-strategic behaviors (ambush, hiding)
- Could add later if behaviors feel too simple

---

## Reproduction

### Sexual vs Asexual Spectrum

Reproductive cells have a mode value:
```
0.0 = pure asexual (budding/cloning)
0.5 = mixed (opportunistic)
1.0 = pure sexual (requires mate)
```

### Reproduction Logic

```python
if breed_intent > threshold and energy > min_required:

    if random() < reproductive_mode and mate_nearby and mate_willing:
        # Sexual reproduction
        offspring = crossover(self.genome, mate.genome)
        offspring = mutate(offspring)

    elif random() < (1.0 - reproductive_mode):
        # Asexual reproduction (budding)
        offspring = clone(self.genome)
        offspring = mutate(offspring)

    else:
        # Can't reproduce (no capability or no mate)
        pass
```

Parameter defaults:
- `threshold = 0.6`
- `min_required = 0.7 * max_energy`

### Mate Compatibility

Two organisms can mate if:
- Both have sexual reproductive capability
- Both have high breed_intent
- Both have sufficient energy
- Genetic similarity in valid range:
  - Not too similar (prevent selfing/inbreeding)
  - Not too different (species barrier)

### Evolutionary Pressure

| Environment | Favored Strategy |
|-------------|------------------|
| Sparse population | Asexual (no mate-finding needed) |
| Dense/competitive | Sexual (diversity enables adaptation) |
| Unstable | Mixed (hedge bets) |

---

## Feeding

### Consumption Mechanics

```python
if eat_intent > threshold and mouth_cells > 0:
    for nearby_entity in contact_range:

        edibility = my_digestive_spectrum vs their_composition
        armor_factor = their_structural_armor

        if edibility > armor_factor:
            energy_gained = bite_size × entity_energy × efficiency
            transfer_energy(entity → self)
```

Parameter defaults:
- `bite_size = 0.05` (fraction of target energy per bite)
- `efficiency = 0.8`

### Digestive Spectrum

```
0.0 = pure herbivore (can only digest flora)
0.5 = omnivore (can digest both, neither optimally)
1.0 = pure carnivore (can only digest fauna)
```

Organisms are "flora-like" or "fauna-like" based on:
- Presence of photosynthetic cells (flora)
- Presence of actuator cells (fauna)
- Absence of both (detritus)

Replaced by continuous composition:
```
composition = photo_weight / (photo_weight + actuator_weight + epsilon)
```

---

## Flora Model (Separate System)

Flora are simulated as lightweight entities, not full organisms. They do not run brains, pathfinding, or CPPNs. They still participate in capability matching using the same composition/edibility rules.

### Representation

- **Rooted flora**: fixed position, occupy a small footprint, grow into adjacent free space.
- **Floating flora**: lightweight particles that drift with the flow field.

Each flora entity exposes:
```
photo_weight = 1.0
actuator_weight = 0.0
structural_armor = armor
energy = current energy store
```

### Growth

Rooted flora:
```
energy += light_level * photosynthesis_rate
if energy > growth_cost and space_free:
    spawn new flora cell (adjacent)
    energy -= growth_cost
```

Floating flora:
```
energy += light_level * photosynthesis_rate
if energy > bud_cost:
    spawn drifting seed
    energy -= bud_cost
```

### Consumption

Fauna use the same feeding rule; flora are simply entities with high `photo_weight` and no `actuator_weight`.

### Parameter defaults

- `photosynthesis_rate = 0.02 * max_energy / tick`
- `growth_cost = 0.3 * max_energy`
- `bud_cost = 0.2 * max_energy`
- `armor = 0.1`

---

## Energy System

### Sources
- **Feeding**: Consuming other organisms (mouth + digestive cells)
- **Photosynthesis**: Light exposure (photosynthetic cells)

### Sinks
- **Metabolism**: Base cost per cell per tick
- **Movement**: Thrust × actuator activity
- **Growth**: Creating new cells (grow_intent)
- **Reproduction**: Energy transferred to offspring

### Storage
- Base energy capacity from organism size
- Storage cells increase max capacity (fat reserves)
- Energy ratio (current/max) is brain input

### Structural/Storage Costs
To keep armor and storage from being universal "free" upgrades, add explicit tradeoffs:
```
# Per-tick metabolic cost (applies per cell)
metabolic_cost += base_cell_cost * (
  1.0
  + armor_cost_scale * structural_armor
  + storage_cost_scale * storage_capacity
)

# Movement drag penalty (applies to organism-level drag)
drag_multiplier = 1.0 + drag_armor_scale * avg_structural_armor
```

Parameter defaults:
- `armor_cost_scale = 0.5`
- `storage_cost_scale = 0.35`
- `drag_armor_scale = 0.4`

---

## Physics

### Movement
- Actuator cells provide thrust
- Actuator positions affect turn capability
- Total actuator strength affects max speed

### Drag
- Larger organisms have more drag
- Shape affects drag (streamlined vs bulky)
- Already implemented via ShapeMetrics

### Flow Field
- Environmental currents affect all organisms
- Organisms can exploit or fight current
- Flow alignment input lets brain learn to use currents strategically; pathfinding still applies flow as physics

---

## What to Remove

### From traits/
- [ ] All trait flags (Herbivore, Carnivore, Speed, etc.)
- [ ] Gender (Male/Female)
- [ ] Mutation system (replaced by CPPN cell variation)
- [ ] Keep Disease? (could be interesting, discuss later)

### From components/
- [ ] Trait field in Organism
- [ ] Gender-related fields
- [ ] Old brain input/output structures

### From systems/
- [ ] Trait-based behavior branching
- [ ] Gender checks in mating
- [ ] Old findFood/findPredator (replace with capability-based)

### From neural/
- [ ] Old BrainInputs (17 inputs)
- [ ] Old BrainOutputs (4 outputs)
- [ ] Predator/prey specific logic

---

## Migration Path

### Phase 1: Cell Type Expansion
- Expand CPPN outputs for new cell types and multi-functional cell weighting
- Add new cell properties (mouth/digestive/photosynthetic/structural/storage/reproductive)
- Update energy capacity and actuation/sensing to use new cell weights

Deprecate/remove:
- `neural/morphology.go` diet bias and single cell_type mapping
- `traits/` diet trait logic as a source of capability (keep only for temporary visualization if needed)

Test (programmatic):
- Add unit tests for CPPN output mapping and normalization (sums to 1.0, min_component behavior)
- Validate new cell fields populate in morphology generation (non-zero for expected CPPN outputs)
- Snapshot test for morphology viability rules (at least one sensor/actuator)

### Phase 2: Capability Matching + Feeding
- Implement composition/edibility/penetration functions
- Replace trait-based feeding and threat checks
- Align flora entities with composition (photo=1, actuator=0)

Deprecate/remove:
- `systems/feeding.go` trait-based branching (herbivore/carnivore/carrion)
- `systems/behavior.go` predator/food checks based on `traits.Carnivore/Herbivore`

Test (programmatic):
- Unit test capability functions with edge cases (armor > edibility, neutral composition)
- Feeding integration test: mock predator/prey with known composition/armor and assert energy transfer
- Threat detection test: verify food/threat classification flips when swapping digestive/composition

### Phase 3: Polar Vision
- Implement 4-cone × 3-channel perception (food/threat/friend)
- Replace old nearest-target sensory queries
- Use capability matching for cone relevance

Deprecate/remove:
- `systems/behavior.go` nearest-target sensing (`findFoodWeighted`, `findPredatorWeighted`, `findMateWeighted`)
- `neural/inputs.go` food/predator/mate distance+angle fields

Test (programmatic):
- Unit test cone binning: entities in each quadrant contribute to correct cone only
- Unit test intensity falloff vs distance and light modifier
- Sensor weighting test: sensors aligned to a cone increase that cone’s output

### Phase 4: New Brain I/O
- Switch to 15 inputs, 5 outputs (desire angle/distance + eat/grow/breed)
- Update NEAT configuration and HyperNEAT substrate
- Retrain from scratch

Deprecate/remove:
- `neural/config.go` current `BrainInputs=17`, `BrainOutputs=4`
- `neural/inputs.go` `BehaviorOutputs` with Turn/Thrust outputs
- `neural/hyperneat.go` standard brain substrate layout (14/4)

Test (programmatic):
- Validate input/output sizes in config and neural construction
- Decode test for desire angle/distance normalization
- End-to-end brain tick test with fixed inputs produces 5 outputs in range

### Phase 5: Pathfinding Layer
- Implement potential-field navigation from desire vector
- Insert between brain and actuators
- Remove terrain inputs from brain

Deprecate/remove:
- `neural/inputs.go` terrain inputs (distance/gradient)
- Direct Turn/Thrust control in `systems/behavior.go`

Test (programmatic):
- Pathfinding unit tests on synthetic terrain (attraction vs repulsion)
- Regression test: with no terrain, pathfinding follows desire vector
- Collision avoidance test: agent does not intersect solid cells under repeated updates

### Phase 6: Reproduction Spectrum
- Replace gender and mating checks with reproductive_mode spectrum
- Add asexual fallback when mates are absent

Deprecate/remove:
- `traits/traits.go` `Male/Female` traits
- `systems/breeding.go` opposite-gender checks and mate-only reproduction

Test (programmatic):
- Asexual reproduction test: high reproductive_mode=0 produces offspring without mate
- Sexual reproduction test: high reproductive_mode=1 requires mate and uses crossover
- Probabilistic mix test: reproductive_mode=0.5 yields both strategies over many trials

### Phase 7: Flora Model Split
- Replace flora organisms with a lightweight flora system (rooted grid + floating particles)
- Remove flora brains/CPPN/cell buffers
- Keep capability matching for fauna feeding

Deprecate/remove:
- `systems/photosynthesis.go` (organism-based)
- `systems/spores.go` (organism-based)
- `components.Flora` as an organism tag (replace with new flora system types)

Test (programmatic):
- Rooted flora growth test: grows when light and space available
- Floating flora drift test: position changes with flow; bounded by world
- Feeding integration test: fauna can consume flora entities via capability matching

### Phase 8: Cleanup
- Remove trait system
- Remove old brain code and inputs
- Update docs and tests

Deprecate/remove:
- `traits/` package (diet flags, mutation hooks if not retained)
- `systems/allocation.go` trait-based mode logic

Test (programmatic):
- Run full simulation step integration tests without trait system
- Ensure no references to removed traits/packages in build

---

## Open Questions

1. **Disease**: Keep as a mechanic? Could spread between organisms in contact.

2. **Terrain awareness**: Add back as simple scalar if behaviors too dumb?
   - `openness`: free space around organism
   - `nearest_cover`: distance to hiding spots

3. **Memory/state**: Should brain have recurrent connections? Or keep feedforward?

4. **Cone count**: 4 cones enough? Or 6/8 for finer directional resolution?

5. **Flora evolution**: Do photosynthetic organisms evolve too? Or static?

6. **Bioluminescence interaction**: How does emitted light affect nearby organisms' vision?

7. **Structural armor**: Does it also slow movement? Energy cost to maintain?

---

## Summary

```
┌─────────────────────────────────────────────────────────────┐
│                          CPPN                               │
│   5 inputs → 10 outputs (cell present + 9 properties)       │
└─────────────────────────────────────────────────────────────┘
                              ↓
                     Body (cell layout)
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                    Polar Vision System                      │
│   Scans environment, computes cone intensities              │
│   food/threat/friend based on capability matching           │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                      Brain Network                          │
│   15 inputs (cones + energy + light + flow)                 │
│   5 outputs (desire + eat + grow + breed)                   │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                   Pathfinding Layer                         │
│   Converts desire to navigation                             │
│   Handles terrain avoidance                                 │
└─────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────┐
│                   Actuator Physics                          │
│   Applies turn/thrust based on actuator cell layout         │
│   Drag from size/shape                                      │
└─────────────────────────────────────────────────────────────┘
```

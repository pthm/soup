# Simplified Brain Architecture Proposal

## Current Problems

### 1. Zero Offspring - Organisms Can't Breed
From the simulation run:
- **0 offspring** produced in 20,000 ticks
- Population dying out from 85 → 18 organisms
- All organisms are generation 0

**Root cause**: CPPN generates organisms with up to 32 cells → MaxEnergy = 1700, but they start with 100 energy. Energy ratio is 6%, far below the 35% breeding threshold.

### 2. Brain Outputs Are "Tuning Knobs", Not Behaviors
Current 9 outputs:
```
SeekFoodWeight, FleeWeight, SeekMateWeight, HerdWeight, WanderWeight,
GrowDrive, BreedDrive, ConserveDrive, SpeedMultiplier
```

These are *weights for predefined behaviors*, not actual decisions. The brain can't learn novel behaviors - it can only adjust the strength of hardcoded ones.

### 3. Behavior System Has Too Many Rules
The behavior system (lines 70-156) contains:
- If herbivore → seek flora
- If carnivore → seek fauna
- If herbivore and not carnivore → flee carnivores
- If carrion → seek dead
- If herding → flock with same type
- If photophilic → move toward light
- If photophobic → move toward shadow
- Plus vision cone calculations, urgency scaling, stealth mechanics...

**The brain doesn't decide behavior - it just tunes preset behaviors.**

### 4. Allocation System Controls Breeding, Not Brain
The `AllocationSystem` deterministically decides when organisms can breed:
- Energy > 35% AND cells >= target AND mode == Breed
- The "BreedDrive" brain output barely matters

---

## Proposed Architecture: "Let the Brain Drive"

### Design Principles

1. **Brain outputs = primitive motor commands** (direction, speed, actions)
2. **Environment defines rules** (physics, collision, consumption)
3. **No behavioral prescriptions** (brain learns seek/flee/herd)
4. **Traits affect capabilities, not behaviors** (speed trait = faster, not "runs away")

---

## New Brain Inputs (12 total)

Replace the current 14 complex inputs with simpler, more "perceptual" inputs:

```
SENSORY INPUTS (what the organism perceives):
[0] energy_ratio       - 0 to 1 (hunger signal)
[1] velocity_x         - current movement X (-1 to 1)
[2] velocity_y         - current movement Y (-1 to 1)
[3] light_level        - ambient light (0 to 1)
[4] wall_proximity     - distance to nearest boundary (0 = touching, 1 = far)

VISION INPUTS (nearest entity in perception):
[5] nearest_food_dist  - 0 (touching) to 1 (max range), or 1 if none
[6] nearest_food_dir   - angle to food (-1 to 1, normalized)
[7] nearest_threat_dist - distance to larger organism
[8] nearest_threat_dir - angle to threat
[9] nearest_same_dist  - distance to same-diet organism
[10] nearest_same_dir  - angle to same-diet

INTERNAL:
[11] bias              - constant 1.0
```

**Key changes:**
- Removed mate detection (mating is just proximity + signal)
- Removed herd count (organism can sense individuals)
- Removed flow field (environmental, not perceptual)
- Simplified to what organism can actually "see"

---

## New Brain Outputs (4 total)

Replace the 9 tuning-knob outputs with primitive actions:

```
MOTOR OUTPUTS:
[0] turn_direction    - -1 (left) to +1 (right), adjust heading
[1] thrust            - 0 (stop) to 1 (full speed forward)

ACTION OUTPUTS:
[2] eat_signal        - 0 to 1 (attempt to consume if > 0.5 and touching food)
[3] mate_signal       - 0 to 1 (ready to mate if > 0.5 and touching compatible)
```

**Why this works:**
- Turn + thrust = full movement control
- Eat = discrete action when touching potential food
- Mate = broadcast readiness, happens on contact

**What the brain now controls:**
- Where to go (turn)
- How fast (thrust)
- When to eat (eat_signal)
- When to reproduce (mate_signal)

---

## Environmental Rules (Physics, Not Behavior)

### Movement
```go
// Apply brain outputs directly to movement
heading += outputs.TurnDirection * maxTurnRate
speed := outputs.Thrust * maxSpeed
vel.X = cos(heading) * speed
vel.Y = sin(heading) * speed

// Energy cost proportional to thrust (movement costs energy)
energyCost := baseMetabolism + outputs.Thrust * thrustCost
```

### Feeding (Collision-Based)
```go
// On collision with potential food:
if touching(organism, target) && outputs.EatSignal > 0.5 {
    if canEat(organism, target) { // herbivore→flora, carnivore→fauna
        energy := min(biteSize, target.Energy)
        organism.Energy += energy * efficiency
        target.Energy -= energy
    }
}
```

### Mating (Contact + Mutual Signal)
```go
// On collision with compatible organism:
if touching(a, b) && compatible(a, b) {
    if a.outputs.MateSignal > 0.5 && b.outputs.MateSignal > 0.5 {
        if a.Energy > minBreedEnergy && b.Energy > minBreedEnergy {
            produceOffspring(a, b)
        }
    }
}
```

### Death
```go
if organism.Energy <= 0 {
    organism.Dead = true
}
```

---

## What Gets Removed

### From Behavior System
- `findFood()` - brain learns to turn toward food
- `findPredator()` - brain learns to turn away from threats
- `findMate()` - mating is just contact + signal
- `flockWithHerd()` - brain might evolve this emergently
- `getLightPreferenceForce()` - brain controls movement directly
- All steering calculations - brain outputs direction directly

### From Allocation System
- All mode logic - energy allocation is implicit in brain outputs
- Growth decisions - cells could grow based on energy surplus

### From Traits
- Herding → emergent from brain (or remove)
- PredatorEyes/PreyEyes → affects input range, not behavior
- Photophilic/Photophobic → brain can learn light preference
- Speed → affects thrust multiplier, not behavior

### Keep
- Herbivore/Carnivore/Carrion → defines what organism *can* eat
- Male/Female → for sexual reproduction compatibility
- Physical traits (cell count, size) → affects capabilities

---

## What Traits Should Do (Capabilities, Not Behaviors)

```
Herbivore: canEat(flora) = true
Carnivore: canEat(fauna) = true
Carrion:   canEat(dead) = true

Speed:     maxSpeed *= 1.5
FarSight:  perceptionRadius *= 1.5
```

Traits define **what the organism CAN do**, not **what it WILL do**.

---

## Expected Emergent Behaviors

With simpler brain and environmental rules, these should evolve:

| Behavior | How it emerges |
|----------|----------------|
| Food seeking | Turn toward food_dir when hungry (low energy_ratio) |
| Predator avoidance | Turn away from threat_dir |
| Herding | Turn toward same_dir → organisms cluster |
| Ambush hunting | Low thrust until food_dist < threshold |
| Fleeing | High thrust when threat_dist is low |
| Mate finding | Turn toward same_dir, high mate_signal |

---

## Implementation Plan

### Phase 1: Simplify Brain I/O
1. Reduce inputs from 14 → 12
2. Reduce outputs from 9 → 4
3. Update `neural/inputs.go` and `neural/brain.go`

### Phase 2: Rebuild Behavior System
1. Remove all steering logic
2. Apply brain outputs directly to heading/velocity
3. Implement contact-based eating
4. Implement contact-based mating

### Phase 3: Fix CPPN Morphology
1. Start organisms with fewer cells (1-4)
2. Or scale starting energy with cell count
3. Let growth be gradual, not instant

### Phase 4: Remove Allocation System
1. Energy flows naturally: eat → gain, move → lose
2. Growth when energy surplus
3. Mating when both signal and have energy

### Phase 5: Test & Tune
1. Run headless simulations
2. Check for breeding
3. Check for diversity
4. Check for species emergence

---

## Alternative: Even Simpler (Braitenberg-Style)

For maximum evolutionary potential, consider 2 outputs:

```
[0] left_motor   - 0 to 1 (speed of left side)
[1] right_motor  - 0 to 1 (speed of right side)
```

This creates differential steering (like Braitenberg vehicles).
- Both high → forward
- Left high, right low → turn right
- Difference → turn rate

Eating/mating could be automatic on contact based on simple rules.

---

## Summary

**Current**: Brain tunes 9 weights for hardcoded behaviors
**Proposed**: Brain directly controls 4 primitive actions

**Current**: Behavior system has 500+ lines of rules
**Proposed**: Environment has simple physics rules

**Current**: Traits define behaviors (herding, photophilic)
**Proposed**: Traits define capabilities (speed, perception)

**Current**: Allocation system decides breeding
**Proposed**: Brain signals mate, physics handles contact

The brain should **decide what to do**, not **how much to do predefined things**.

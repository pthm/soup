# Controller Design Implementation Plan

Implementation plan to align the current codebase with the consolidated design spec: feed-forward NEAT brains + CPPN morphologies, morphology-aware boid fields, diet ecology, and gated predation/mating.

This document is implementation-oriented and maps directly to the current project structure.

---

## 0. Scope and Non-Goals

**In scope**
- Replace cone vision inputs with morphology-normalized aggregated fields.
- Replace movement outputs with local desired velocity (or gait channels) routed through actuators.
- Add body-descriptor inputs to keep behavior stable under morphology drift.
- Make feeding, predation, and mating gates morphology-aware.
- Remove (or optionalize) explicit fitness scoring to favor persistent ecology.

**Out of scope**
- Rendering changes beyond HUD/input-output labels.
- CPPN architecture changes (unless needed to expose body descriptors).
- GPU/terrain/pathfinding refactors unrelated to control.

---

## 1. Current-State Mapping (Key Files)

- Neural I/O: `neural/inputs.go`, `neural/config.go`, `neural/io_metadata.go`
- Perception: `systems/behavior.go`, `neural/vision.go`
- Actuation: `systems/behavior.go` (`calculateActuatorForces`), `components/components.go`
- Feeding: `systems/feeding.go`
- Breeding: `systems/breeding.go`
- Energy: `systems/energy.go`
- Fitness: `game/simulation.go`, `neural/species.go`
- Spatial queries: `systems/spatial.go`

---

## 2. Target Interface Summary

### Inputs (~24–28 floats)
- Self: `speed_norm`, `energy_norm`
- Body descriptor: `r_body_norm`, `v_max_norm`, `a_max_norm`, `sense_radius_norm`, `attack_range_norm`, `attack_power_norm`, `metabolic_rate_norm`
- Same-species boid fields: cohesion (fwd/up/mag), alignment (fwd/up), separation (fwd/up/mag), density
- Food fields: plant (fwd/up/mag), meat (fwd/up/mag)
- Threat/target: nearest predator/prey (fwd/up/dist) + closing speed
- Optional: `mate_ready_norm`

### Outputs
- Continuous: `u_fwd`, `u_up` (local desired velocity)
- Discrete: `attack_intent`, `mate_intent`

---

## 3. Key Invariants

- **Rotation invariance:** All directional inputs are agent-local (fwd/up). No world-frame angles.
- **Morphology normalization:** All distances scaled by body radius or body length.
- **Fixed I/O size:** No variable-length sensor lists to the brain.
- **Action gating:** Predation and mating must be explicitly intent-gated and cooldown-limited.
- **Feeding:** Plant grazing is implicit, predation explicit.
- **Diet ecology:** Compatibility uses `compat^k` (k ~= 2–4) and is applied consistently.
- **Persistent ecology:** Selection emerges from energy economics, not explicit fitness.

---

## 4. Implementation Phases

### Phase 1 — Define new neural I/O schema

**Steps**
- [ ] Update `neural/config.go` to new `BrainInputs` and `BrainOutputs`.
- [ ] Replace `SensoryInputs` struct in `neural/inputs.go` with new fields.
- [ ] Replace `BehaviorOutputs` to `{UForward, UUp, AttackIntent, MateIntent}`.
- [ ] Update `DecodeOutputs` for new outputs (sigmoid -> [-1,1] for u_fwd/u_up).
- [ ] Update `neural/io_metadata.go` with new descriptors and groups.
- [ ] Update `neural/inputs_test.go` to match new layout.

**Verification criteria**
- Brain input/output counts match config constants.
- Inputs are normalized to intended ranges.
- Tests compile and pass for I/O mapping.

**Key invariants**
- Output `u_fwd` and `u_up` are centered around 0 and clamped to [-1,1].
- Any added bias input is explicit and documented.

---

### Phase 2 — Compute body descriptors

**Steps**
- [ ] Add helper to compute `r_body` (from OBB extents or bounding radius).
- [ ] Add helpers for `v_max_norm`, `a_max_norm`, `sense_radius_norm` using existing caps.
- [ ] Add `attack_range_norm`, `attack_power_norm`, `metabolic_rate_norm` computed from cell capabilities (mouth size, mass/energy, armor/drag).
- [ ] Add these values to `SensoryInputs` assembly in `systems/behavior.go`.

**Files**
- `systems/behavior.go`
- `components/components.go` (if new cached fields needed)
- `components/capabilities.go` (if new helper methods are needed)

**Verification criteria**
- Descriptor values are stable across ticks and bounded [0,1].
- Values scale logically with morphology (larger bodies -> larger r_body_norm, etc.).

---

### Phase 3 — Replace cone vision with aggregated fields

**Steps**
- [ ] Create a new field aggregation path in `systems/behavior.go` (replace `PolarVision`).
- [ ] Use COM position and velocity as the agent frame.
- [ ] Scale all distances by `r_body` or `R * r_body`.
- [ ] Implement same-species boid fields (cohesion/alignment/separation/density).
- [ ] Implement food attraction fields (plant + meat) with diet compatibility `compat^k`.
- [ ] Implement nearest predator/prey vectors + distance + closing speed.
- [ ] Pool sensor contributions: max or weighted average by sensor strength.
- [ ] Remove or deprecate `neural/vision.go` usage (keep for debug if needed).

**Files**
- `systems/behavior.go` (new aggregation)
- `systems/spatial.go` (reuse, no change)
- `neural/vision.go` (likely unused)

**Verification criteria**
- Inputs are bounded and rotation-invariant.
- Field magnitudes are stable under body scale changes.
- Small and large bodies perceive similarly when normalized.

**Key invariants**
- No neighbor list is passed to NEAT; everything is aggregated.
- Same-species definition uses `NeuralGenome.SpeciesID` (not genetic-distance cone).

---

### Phase 4 — Update locomotion to desired-velocity steering

**Steps**
- [ ] Replace DesireAngle/DesireDistance with `u_fwd/u_up` handling.
- [ ] Remove or bypass `Pathfinder.Navigate` (or adapt it to desired velocity).
- [ ] Implement `v_des` logic and acceleration clamp with drag (per spec).
- [ ] Route desired movement through actuator mapping (`calculateActuatorForces`) or adapt to “gait channels”.
- [ ] Update energy costs to use actual thrust/accel rather than DesireDistance.

**Files**
- `systems/behavior.go`
- `systems/pathfinding.go` (remove or keep minimal)
- `systems/energy.go`
- `components/components.go` (remove DesireAngle/DesireDistance if unused)

**Verification criteria**
- Movement is smooth (no jitter under zero intent).
- Turn/accel caps respected per morphology.
- Actuators remain influential (not bypassed by direct physics).

---

### Phase 5 — Feeding and predation rules

**Steps**
- [ ] Make plant feeding implicit within eat radius (no intent gating).
- [ ] Add `attack_intent` gate for predation.
- [ ] Implement attack radius and cooldown based on mouth placement/strength.
- [ ] Add energy cost to attacks; apply `compat^k` for rewards.
- [ ] Add predator interference rules (single latch, reward split, crowd penalty).

**Files**
- `systems/feeding.go`
- `systems/energy.go`
- `components/components.go`

**Verification criteria**
- Herbivores graze without intent output.
- Predators must signal attack intent to kill prey.
- Predator clustering reduces net gain (no swarm exploit).

**Key invariants**
- No predation without explicit intent.
- Nutrition scaling uses the same compat logic as perception.

---

### Phase 6 — Mating handshake rules

**Steps**
- [ ] Replace current intent+proximity logic with a dwell-time handshake.
- [ ] Use body-normalized mate radius.
- [ ] Require both parents to maintain mate intent and energy threshold for `T_mate`.
- [ ] Apply cooldowns and gestation delay.

**Files**
- `systems/breeding.go`
- `components/components.go`

**Verification criteria**
- Mating does not trigger on brief contacts.
- Mating requires sustained proximity + intent.
- Offspring inheritance unchanged (brain+body pair).

---

### Phase 7 — Remove or optionalize explicit fitness

**Steps**
- [ ] Remove fitness accumulation on death (or gate with a flag).
- [ ] Update stats UI if it depends on fitness fields.

**Files**
- `game/simulation.go`
- `neural/species.go`
- `game/render.go`

**Verification criteria**
- Simulation runs without fitness accumulation.
- Species stats still render or are updated to reflect new metrics.

---

## 5. Testing Strategy

### Unit tests
- `neural/inputs_test.go`: input normalization ranges and output decoding.
- New tests for field aggregation (cohesion/alignment/separation, prey/pred fields). Suggested file: `systems/behavior_fields_test.go`.
- `neural/capability_test.go`: update for compat^k.

### Integration tests
- Headless run with a fixed seed (`./soup -headless -max-ticks=...`).
- Assert population stays within bounds (no full collapse, no infinite explosion).
- Track mean energy, mean speed, and distribution of diet.

### Behavior checks (manual)
- Prey should disperse when predators are close.
- Predators should approach prey only when intent is active.
- Schooling should emerge in some lineages but not be universal.

---

## 6. Performance Constraints

- Keep aggregation O(k) per agent with spatial grid lookups.
- Avoid per-agent allocations in hot path; reuse buffers.
- Ensure any new pooling uses simple loops and small fixed vectors.

---

## 7. Rollout Plan / Risk Mitigation

- Land Phases 1–2 first (I/O + descriptors) with old perception path still in place.
- Introduce new aggregation under a feature flag if needed.
- Replace locomotion after inputs are stable to avoid compounding failures.
- Add a fallback mode for organisms missing cells (e.g., early seeding).

---

## 8. Open Decisions

- Whether to keep `Pathfinder` for terrain avoidance or remove entirely.
- Whether output channels are `u_fwd/u_up` or gait-style (`drive_forward`, `drive_turn`, `drive_strafe`).
- How to compute `attack_power_norm` (mouth strength vs total mass).
- Whether to keep glow/light-related inputs/outputs (not in the new spec).

---

## 9. Checklist Summary

- [ ] Update brain I/O schema and metadata.
- [ ] Add body descriptor inputs.
- [ ] Implement morphology-normalized aggregated fields.
- [ ] Replace locomotion with desired-velocity steering.
- [ ] Make feeding implicit, predation intent-gated.
- [ ] Add mating handshake and dwell time.
- [ ] Remove or optionalize explicit fitness.
- [ ] Update tests and validation runs.

---

## 10. Success Criteria

- Controllers remain stable across CPPN morphology mutations.
- No hard coupling to world orientation.
- Persistent ecology shows multiple niches without explicit fitness.
- Predators and prey exhibit distinct strategies with emergent flocking.


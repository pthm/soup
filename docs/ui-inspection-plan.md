# UI Inspection Plan (Capability-Driven)

This document proposes a UI update plan aligned with the capability-driven spec. It focuses on inspection and observation tools for debugging perception, behavior, and balance.

---

## Goals
- Replace trait-based labels with capability-based info.
- Make perception and light mechanics visible.
- Make brain inputs/outputs inspectable and explainable.
- Enable quick, toggleable overlays for diagnostics.
- Keep HUD readable during high-entity counts.

---

## Proposed Layout (ASCII)

```
┌────────────────────────────────────────────────────────────────────────────┐
│ TOP HUD                                                                     │
│ [Sim: RUN/PAUSE | Speed xN | Tick | FPS]   [Flora | Fauna | Cells | Spores] │
│ [Energy: feed/photon/metab/move]          [Light: ambient | sun intensity]  │
└────────────────────────────────────────────────────────────────────────────┘
┌─────────────┐                                                        ┌─────┐
│ LEFT PANEL  │                                                        │     │
│ (Systems)   │                                                        │     │
│ - Perf      │                                                        │     │
│ - Toggles   │                                                        │     │
│ - Overlays  │                                                        │     │
│             │                                                        │     │
│             │                                                        │     │
└─────────────┘                                                        │     │
            WORLD VIEW (organisms + terrain + overlays)               │RIGHT│
                                                                      │PANEL│
                                                                      │(Inspect)
                                                                      │     │
                                                                      │     │
                                                                      └─────┘
└────────────────────────────────────────────────────────────────────────────┐
│ BOTTOM HUD                                                                  │
│ Controls: Pause | Step | Speed | Select | Overlay toggles | Spawn tools      │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Panel Content

### Top HUD
- Simulation state: paused/running, speed, tick, FPS.
- Population counts: flora/fauna/cells/spores.
- Energy summary (aggregate per tick): feeding, photosynthesis, metabolism, movement.
- Light context: ambient light level + sun intensity.

### Left Panel (Systems + Diagnostics)
- Performance list (top N systems).
- Overlay toggles (perception cones, light map, cell types, pathfinding).
- Debug toggles (species colors, capability colors, show vectors).
- Quick stats: average energy, average size, death rate.

### Right Panel (Selected Organism Inspector)

**Header**
- Organism type (Flora/Fauna).
- Species color or capability color (toggle).
- Alive/Dead status.

**Body & Capabilities**
- Cell count.
- Primary cell type counts (by category).
- Secondary cell type counts (by category).
- Composition (photo vs actuator ratio).
- Digestive spectrum (0..1).
- Mouth size, sensor gain, actuator strength totals.
- Structural armor, storage capacity totals.

**Energy**
- Current energy / max.
- Recent energy change (delta over N ticks).
- Sources/sinks summary (feeding vs photosynth vs metabolism).

**Perception**
- Cone values: food/threat/friend (4 directions).
- Light cones: ambient + gradients (if enabled).
- Openness scalar (terrain awareness).

**Brain I/O**
- Inputs (same as Perception + energy + flow + openness).
- Outputs: desire angle, desire distance, eat/grow/breed.

**Motion**
- Heading, velocity, flow alignment.
- Pathfinding target vector vs actual movement vector.

---

## Overlays

### 1) Cell Types
- Primary cell type color per cell.
- Secondary type shown as a corner dot or outline.
- Structural/storage shown as border tint.

### 2) Perception Cones
- Draw 4 cones around selected organism.
- Color by channel: food (green), threat (red), friend (blue).
- Intensity shown as opacity or arc thickness.

### 3) Light Visualization
- Ambient light overlay (blue/gray gradient).
- Light cones from organism’s perspective (white/yellow).
- Bioluminescent emitters shown as glow halos.

### 4) Pathfinding + Intent
- Desired vector (thin white line).
- Actual movement vector (green line).
- Obstacle repulsion vectors (optional debug).

### 5) Capability Heatmaps (Optional)
- Global map of edible targets vs threat zones.
- Useful for validating capability matching and diet spectrum.

---

## Controls (proposed)

```
P: Pause/Resume
.: Step frame
< >: Speed down/up
I: Toggle inspector panel
O: Overlay menu
L: Light overlays
V: Vision cones
C: Capability colors
S: Species colors
```

---

## Phased Implementation

### Phase A: Inspection Refresh
- Replace trait-based labels in tooltips/inspector.
- Add capability breakdown to inspector.
- Update brain I/O labels to spec.

### Phase B: Perception + Light Overlays
- Vision cone overlay on selected organism.
- Light cone overlay + ambient light map toggle.
- Bioluminescent emitters highlighted.

### Phase C: Motion + Pathfinding Overlays
- Draw desire vector vs actual movement.
- Show openness scalar as ring intensity or text.

### Phase D: Global Stats + Perf
- Aggregate energy sources/sinks panel.
- Population histogram by composition/digestive spectrum.

---

## Notes on Data Sources
- Capabilities: `components.CellBuffer.ComputeCapabilities()`
- Composition/Digestive: `components.Capabilities` + `DigestiveSpectrum()`
- Light sampling: `shadowMap.SampleLight(x, y)` + global light intensity
- Pathfinding vectors: hook into behavior/pathfinding output structs
- Energy deltas: store per-entity energy history or rolling window


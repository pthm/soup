# Body-Brain Coupling Implementation Plan

## Overview

This document outlines the remaining stages to fully couple the body (CPPN-generated morphology) with the brain (NEAT controller). Currently, Stage A is complete:

- **Stage A (DONE)**: Cell types added to CPPN outputs (sensor/actuator/passive)
- **Stage B (DONE)**: Brain inputs aggregate from sensor cells
- **Stage C (DONE)**: Brain outputs drive actuator cells
- **Stage D (DONE)**: HyperNEAT-style brain wiring from CPPN

The goal is to make body shape evolutionarily meaningful: organisms with more/better sensors perceive more, organisms with more/better actuators move faster.

---

## Stage B: Sensor-Aggregated Brain Inputs

### Goal
Replace fixed 14-input brain with dynamic inputs that aggregate sensory data weighted by each sensor cell's gain and position.

### Current State
- `SensoryInputs` struct has 14 fixed fields (food distance, predator angle, etc.)
- `ToInputs()` converts to `[]float64` of length 14
- Brain always receives same input dimensionality regardless of body

### Target State
- Each sensor cell contributes to perception based on its position and gain
- Sensors facing a direction are more sensitive to stimuli from that direction
- Total sensor gain affects perception radius/quality
- Brain input count remains fixed (for NEAT compatibility) but values are body-dependent

### Implementation

#### 1. Add sensor geometry to perception calculation

**File: `systems/behavior.go`**

Create a new function that weights sensory inputs by sensor cell positions:

```go
// SensorWeightedPerception calculates perception weighted by sensor cells.
// Sensors facing the stimulus direction contribute more.
func (s *BehaviorSystem) SensorWeightedPerception(
    cells *components.CellBuffer,
    heading float32,
    stimulusAngle float32,  // Angle to stimulus relative to organism
    rawDistance float32,
    rawIntensity float32,
) (distance float32, intensity float32) {
    var totalWeight float32
    var weightedIntensity float32

    for i := uint8(0); i < cells.Count; i++ {
        cell := &cells.Cells[i]
        if cell.Type != neural.CellTypeSensor || !cell.Alive {
            continue
        }

        // Calculate sensor's facing direction from its grid position
        sensorAngle := float32(math.Atan2(float64(cell.GridY), float64(cell.GridX)))

        // Angular difference between sensor facing and stimulus direction
        angleDiff := math.Abs(float64(normalizeAngle(sensorAngle - stimulusAngle)))

        // Sensors facing the stimulus contribute more (cosine falloff)
        directionalWeight := float32(math.Max(0, math.Cos(angleDiff)))

        weight := cell.SensorGain * directionalWeight
        totalWeight += weight
        weightedIntensity += weight * rawIntensity
    }

    if totalWeight > 0 {
        intensity = weightedIntensity / totalWeight
    }

    // Distance perception scales with total sensor gain
    totalGain := s.getTotalSensorGain(cells)
    distanceScale := 0.5 + 0.5*math.Min(1.0, totalGain/2.0)
    distance = rawDistance * float32(distanceScale)

    return distance, intensity
}
```

#### 2. Update `findNearestFood`, `findNearestPredator`, etc.

**File: `systems/behavior.go`**

Modify perception functions to use sensor weighting:

```go
func (s *BehaviorSystem) findNearestFood(
    pos *components.Position,
    org *components.Organism,
    cells *components.CellBuffer,
    entity ark.Entity,
) (distance, angle float32, found bool) {
    // ... existing spatial query logic ...

    if found {
        // Weight perception by sensor geometry
        rawAngle := calculateAngle(pos, targetPos)
        relativeAngle := rawAngle - org.Heading

        distance, _ = s.SensorWeightedPerception(
            cells, org.Heading, relativeAngle, rawDistance, 1.0)
    }

    return distance, angle, found
}
```

#### 3. Scale perception radius by total sensor gain

**File: `systems/behavior.go`**

```go
func (s *BehaviorSystem) getEffectivePerceptionRadius(
    org *components.Organism,
    cells *components.CellBuffer,
) float32 {
    totalGain := s.getTotalSensorGain(cells)
    // Base radius scaled by sensor capability (0.5x to 1.5x)
    scale := 0.5 + math.Min(1.0, totalGain/2.0)
    return org.PerceptionRadius * float32(scale)
}

func (s *BehaviorSystem) getTotalSensorGain(cells *components.CellBuffer) float32 {
    var total float32
    for i := uint8(0); i < cells.Count; i++ {
        if cells.Cells[i].Type == neural.CellTypeSensor && cells.Cells[i].Alive {
            total += cells.Cells[i].SensorGain
        }
    }
    return total
}
```

#### 4. Add sensor count to brain inputs

**File: `neural/inputs.go`**

Add sensor-related inputs to give the brain awareness of its sensory capability:

```go
type SensoryInputs struct {
    // ... existing fields ...

    // New: Body awareness
    SensorCount    int     // Number of active sensor cells
    TotalSensorGain float32 // Sum of sensor gains
}

func (s *SensoryInputs) ToInputs() []float64 {
    inputs := make([]float64, BrainInputs)

    // ... existing input mapping ...

    // Input 12: Sensor capability (normalized)
    inputs[12] = math.Min(1.0, float64(s.TotalSensorGain)/4.0)

    // ... rest of inputs ...
}
```

### Testing

```bash
# Verify sensor weighting affects perception
go test ./systems/... -run TestSensorWeightedPerception

# Run simulation and check organisms with more sensors perceive better
./soup -headless -neural -neural-detail -max-ticks=10000
```

### Success Criteria
- Organisms with sensors facing forward detect food ahead better
- Total sensor gain affects effective perception radius
- Brain receives body-aware inputs

---

## Stage C: Actuator-Driven Movement

### Goal
Replace fixed thrust/turn mechanics with actuator-cell-driven movement where each actuator contributes force based on its position and strength.

### Current State
- `BehaviorOutputs.Thrust` directly sets velocity magnitude
- `BehaviorOutputs.Turn` directly adjusts heading
- All organisms have identical movement capability regardless of body

### Target State
- Each actuator cell generates thrust in a direction based on its position
- Asymmetric actuator placement enables turning
- Total actuator strength determines max speed
- Brain outputs modulate actuator activation, not direct movement

### Implementation

#### 1. Calculate thrust vectors from actuator positions

**File: `systems/behavior.go`**

```go
// ActuatorForces calculates net force from actuator cells.
// Returns (forwardThrust, turnTorque) based on actuator geometry.
func (s *BehaviorSystem) ActuatorForces(
    cells *components.CellBuffer,
    heading float32,
    thrustOutput float32,  // 0-1 from brain
    turnOutput float32,    // -1 to +1 from brain
) (thrust float32, torque float32) {
    var totalStrength float32
    var netTorque float32

    for i := uint8(0); i < cells.Count; i++ {
        cell := &cells.Cells[i]
        if cell.Type != neural.CellTypeActuator || !cell.Alive {
            continue
        }

        // Actuator position relative to center
        dx, dy := float32(cell.GridX), float32(cell.GridY)

        // Distance from center (lever arm for torque)
        dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
        if dist < 0.1 {
            dist = 0.1 // Minimum lever arm
        }

        // Actuator facing angle (points outward from center)
        actuatorAngle := float32(math.Atan2(float64(dy), float64(dx)))

        // Lateral offset determines turn contribution
        // Actuators on the right (positive X) turning left when activated
        lateralOffset := dx * float32(math.Cos(float64(heading))) +
                        dy * float32(math.Sin(float64(heading)))

        // Calculate this actuator's contribution
        strength := cell.ActuatorStrength
        totalStrength += strength

        // Turn output biases left/right actuators differently
        // turnOutput > 0 means turn right, so activate left actuators more
        turnBias := 1.0 + turnOutput*lateralOffset*0.5
        if turnBias < 0 {
            turnBias = 0
        }

        netTorque += lateralOffset * strength * float32(turnBias)
    }

    // Forward thrust scaled by total actuator strength and brain output
    thrust = thrustOutput * totalStrength * 0.5

    // Torque for turning (normalized by total strength to prevent runaway)
    if totalStrength > 0 {
        torque = netTorque / totalStrength * turnOutput * 0.15
    }

    return thrust, torque
}
```

#### 2. Update movement application

**File: `systems/behavior.go`**

Replace direct velocity setting with actuator-computed forces:

```go
func (s *BehaviorSystem) applyBrainOutputs(
    org *components.Organism,
    vel *components.Velocity,
    cells *components.CellBuffer,
    outputs neural.BehaviorOutputs,
) {
    // Calculate forces from actuator geometry
    thrust, torque := s.ActuatorForces(
        cells, org.Heading, outputs.Thrust, outputs.Turn)

    // Apply torque to heading
    org.Heading += torque
    org.Heading = normalizeAngle(org.Heading)

    // Apply thrust in heading direction
    vx := float32(math.Cos(float64(org.Heading))) * thrust
    vy := float32(math.Sin(float64(org.Heading))) * thrust

    // Blend with existing velocity (momentum)
    vel.X = vel.X*0.9 + vx*0.1
    vel.Y = vel.Y*0.9 + vy*0.1

    // Clamp to max speed (still determined by organism stats)
    speed := float32(math.Sqrt(float64(vel.X*vel.X + vel.Y*vel.Y)))
    if speed > org.MaxSpeed {
        scale := org.MaxSpeed / speed
        vel.X *= scale
        vel.Y *= scale
    }

    // Store for energy calculation
    org.ActiveThrust = thrust
    org.TurnOutput = outputs.Turn
    org.ThrustOutput = outputs.Thrust
}
```

#### 3. Scale max speed by actuator capability

**File: `main.go` or `systems/behavior.go`**

```go
func calculateMaxSpeed(cells *components.CellBuffer, baseSpeed float32) float32 {
    var totalStrength float32
    for i := uint8(0); i < cells.Count; i++ {
        if cells.Cells[i].Type == neural.CellTypeActuator && cells.Cells[i].Alive {
            totalStrength += cells.Cells[i].ActuatorStrength
        }
    }
    // Scale: 0.5x to 1.5x based on actuator capability
    scale := 0.5 + math.Min(1.0, float64(totalStrength)/2.0)
    return baseSpeed * float32(scale)
}
```

#### 4. Add actuator count to brain inputs

**File: `neural/inputs.go`**

```go
type SensoryInputs struct {
    // ... existing fields ...

    // Body awareness
    SensorCount       int
    TotalSensorGain   float32
    ActuatorCount     int      // NEW
    TotalActuatorStr  float32  // NEW
}
```

### Testing

```bash
# Test asymmetric actuator placement enables turning
go test ./systems/... -run TestActuatorForces

# Visual test: observe organism movement patterns
./soup -neural
```

### Success Criteria
- Organisms with asymmetric actuators can turn
- Total actuator strength affects max speed
- Thrust energy cost scales with actuator usage

---

## Stage D: HyperNEAT Brain Wiring

### Goal
Use the CPPN to generate brain connection weights, creating body-brain geometric correspondence. Sensors at position (x,y) connect to hidden nodes based on CPPN queries.

### Current State
- Brain genome is a separate NEAT network
- Connection weights are evolved independently of body
- No geometric correspondence between body and brain

### Target State
- CPPN queries generate brain connection weights
- Sensor cell positions map to input node positions
- Actuator cell positions map to output node positions
- Hidden layer weights determined by CPPN substrate queries

### Implementation

This is the most complex stage and may require significant refactoring.

#### 1. Extend CPPN outputs for brain weights

**File: `neural/config.go`**

```go
const CPPNOutputs = 8  // Was 6

// CPPN outputs:
// 0: cell presence
// 1: cell type
// 2: sensor gain
// 3: actuator strength
// 4: diet bias
// 5: priority
// 6: brain_weight (NEW) - connection weight for HyperNEAT
// 7: brain_leo (NEW) - link expression output (connection on/off)
```

#### 2. Define substrate geometry

**File: `neural/hyperneat.go` (NEW)**

```go
package neural

// SubstrateNode represents a node position in the geometric substrate.
type SubstrateNode struct {
    X, Y  float64  // Position in normalized space [-1, 1]
    Type  string   // "sensor", "hidden", "actuator"
    Index int      // Index in the layer
}

// Substrate defines the geometric layout of the brain.
type Substrate struct {
    SensorNodes   []SubstrateNode
    HiddenNodes   []SubstrateNode
    ActuatorNodes []SubstrateNode
}

// BuildSubstrateFromMorphology creates a substrate where sensor/actuator
// nodes are positioned based on cell locations.
func BuildSubstrateFromMorphology(morph *MorphologyResult) *Substrate {
    s := &Substrate{}

    // Map sensor cells to input nodes
    for i, cell := range morph.Cells {
        if cell.Type == CellTypeSensor {
            s.SensorNodes = append(s.SensorNodes, SubstrateNode{
                X:     float64(cell.GridX) / 4.0,  // Normalize to [-1, 1]
                Y:     float64(cell.GridY) / 4.0,
                Type:  "sensor",
                Index: i,
            })
        }
    }

    // Map actuator cells to output nodes
    for i, cell := range morph.Cells {
        if cell.Type == CellTypeActuator {
            s.ActuatorNodes = append(s.ActuatorNodes, SubstrateNode{
                X:     float64(cell.GridX) / 4.0,
                Y:     float64(cell.GridY) / 4.0,
                Type:  "actuator",
                Index: i,
            })
        }
    }

    // Fixed hidden layer (can be evolved later)
    hiddenPositions := []struct{ x, y float64 }{
        {-0.5, 0}, {0, 0}, {0.5, 0},  // Middle row
        {-0.25, 0.5}, {0.25, 0.5},     // Upper row
        {-0.25, -0.5}, {0.25, -0.5},   // Lower row
    }
    for i, pos := range hiddenPositions {
        s.HiddenNodes = append(s.HiddenNodes, SubstrateNode{
            X: pos.x, Y: pos.y, Type: "hidden", Index: i,
        })
    }

    return s
}
```

#### 3. Query CPPN for connection weights

**File: `neural/hyperneat.go`**

```go
// QueryConnectionWeight uses the CPPN to determine the weight between
// two substrate nodes based on their geometric positions.
func QueryConnectionWeight(
    cppn *genetics.Network,
    x1, y1, x2, y2 float64,
) (weight float64, expressed bool) {
    // Calculate geometric features
    dx := x2 - x1
    dy := y2 - y1
    dist := math.Sqrt(dx*dx + dy*dy)
    angle := math.Atan2(dy, dx)

    // CPPN inputs for connection query
    inputs := []float64{
        x1, y1,           // Source position
        x2, y2,           // Target position
        dist,             // Distance
        angle,            // Angle
        1.0,              // Bias
    }

    cppn.LoadSensors(inputs)
    cppn.Activate()
    outputs := cppn.ReadOutputs()
    cppn.Flush()

    // Output 6: connection weight
    weight = outputs[6]

    // Output 7: link expression (LEO) - threshold determines if connection exists
    expressed = outputs[7] > 0.0

    return weight, expressed
}

// BuildBrainFromCPPN constructs a brain network using HyperNEAT substrate queries.
func BuildBrainFromCPPN(
    cppn *genetics.Network,
    substrate *Substrate,
) (*BrainController, error) {
    // Create a new network with substrate-defined topology
    // ... implementation details ...

    // For each potential connection, query CPPN
    for _, sensor := range substrate.SensorNodes {
        for _, hidden := range substrate.HiddenNodes {
            weight, expressed := QueryConnectionWeight(
                cppn, sensor.X, sensor.Y, hidden.X, hidden.Y)
            if expressed {
                // Add connection with weight
            }
        }
    }

    for _, hidden := range substrate.HiddenNodes {
        for _, actuator := range substrate.ActuatorNodes {
            weight, expressed := QueryConnectionWeight(
                cppn, hidden.X, hidden.Y, actuator.X, actuator.Y)
            if expressed {
                // Add connection with weight
            }
        }
    }

    // ... build network from connections ...
}
```

#### 4. Remove separate brain genome

**File: `components/components.go`**

```go
// NeuralGenome stores the genetic blueprint.
type NeuralGenome struct {
    BodyGenome  *genetics.Genome // CPPN for morphology AND brain wiring
    // BrainGenome removed - brain is now derived from CPPN
    SpeciesID   int
    Generation  int
}
```

#### 5. Update brain instantiation

**File: `main.go` or `neural/brain.go`**

```go
func createBrainFromCPPN(bodyGenome *genetics.Genome, morph *MorphologyResult) (*BrainController, error) {
    // Build substrate from morphology
    substrate := neural.BuildSubstrateFromMorphology(morph)

    // Build CPPN network
    cppn, err := bodyGenome.Genesis(bodyGenome.Id)
    if err != nil {
        return nil, err
    }

    // Generate brain using HyperNEAT
    return neural.BuildBrainFromCPPN(cppn, substrate)
}
```

### Considerations

1. **Input/Output Mapping**: With variable sensor/actuator counts, need to aggregate:
   - Multiple sensors → single input channels (e.g., "food direction")
   - Single output channels → multiple actuators

2. **Backward Compatibility**: May need migration path for existing genomes

3. **Performance**: CPPN queries at birth are acceptable; avoid per-tick queries

4. **Evolution**: Single genome simplifies crossover but changes selection pressure

### Alternative: Incremental Approach

If full HyperNEAT is too complex, a simpler version:

1. Keep separate brain genome
2. Use CPPN only to scale connection weights by geometric distance
3. Brain topology still evolves via NEAT, but weights are modulated

```go
// Simpler: modulate existing brain weights by body geometry
func ModulateBrainWeights(brain *BrainController, morph *MorphologyResult) {
    sensorPositions := morph.GetSensorPositions()
    actuatorPositions := morph.GetActuatorPositions()

    // Scale input weights by sensor proximity
    // Scale output weights by actuator proximity
}
```

### Testing

```bash
# Test substrate building
go test ./neural/... -run TestBuildSubstrate

# Test CPPN weight queries
go test ./neural/... -run TestQueryConnectionWeight

# Long evolution run to see if body-brain coupling improves fitness
./soup -headless -neural -neural-detail -max-ticks=100000
```

### Success Criteria
- Brain topology reflects body geometry
- Organisms with sensors in front have stronger food-seeking behavior
- Organisms with asymmetric actuators evolve turning behaviors
- Single genome simplifies evolution

---

## Implementation Order

1. **Stage B** (1-2 days): Sensor weighting
   - Minimal risk, additive changes
   - Immediate visible impact on behavior

2. **Stage C** (1-2 days): Actuator forces
   - Builds on Stage B patterns
   - Makes body shape affect movement

3. **Stage D** (3-5 days): HyperNEAT
   - Most complex, requires careful design
   - Consider incremental approach first

## File Change Summary

| File | Stage B | Stage C | Stage D |
|------|---------|---------|---------|
| `neural/config.go` | - | - | CPPNOutputs=8 |
| `neural/inputs.go` | Add sensor fields | Add actuator fields | Refactor for substrate |
| `neural/morphology.go` | - | - | Add position helpers |
| `neural/hyperneat.go` | - | - | NEW: substrate, queries |
| `neural/brain.go` | - | - | Refactor for HyperNEAT |
| `components/components.go` | - | - | Remove BrainGenome |
| `systems/behavior.go` | Sensor weighting | Actuator forces | Use new brain |
| `main.go` | Pass cells to perception | Pass cells to movement | Use single genome |

---

## Verification Checklist

After each stage:

- [ ] `go build .` succeeds
- [ ] `go test ./...` passes
- [ ] Headless simulation runs 10k ticks without crash
- [ ] Population remains stable (50-200 organisms)
- [ ] Species diversity maintained (3-10 species)
- [ ] Observable behavioral differences based on body shape

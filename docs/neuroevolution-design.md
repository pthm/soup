# Neuroevolution System Design

A unified neural network architecture combining NEAT topology evolution, CPPN morphology generation, and evolved brain controllers for organism behavior.

**Implementation**: Uses [goNEAT](https://github.com/yaricom/goNEAT) library for core NEAT algorithms.

## Overview

```
                         GENOME (goNEAT)
                    ┌─────────────────────────────┐
                    │  genetics.Genome            │
                    │  Nodes + Genes (connections)│
                    └──────────────┬──────────────┘
                                   │
                    ┌──────────────┴──────────────┐
                    │                             │
                    ▼                             ▼
            ┌───────────────┐             ┌───────────────┐
            │  BODY NETWORK │             │ BRAIN NETWORK │
            │    (CPPN)     │             │  (Controller) │
            │ network.Network             │ network.Network
            └───────┬───────┘             └───────┬───────┘
                    │                             │
                    │ Queried ONCE                │ Queried EVERY
                    │ at birth                    │ tick
                    ▼                             ▼
            ┌───────────────┐             ┌───────────────┐
            │  MORPHOLOGY   │             │   BEHAVIOR    │
            │  Cell layout, │             │   Steering,   │
            │  cell types   │             │   allocation  │
            └───────────────┘             └───────────────┘
```

### Design Goals

- Replace fixed steering weights with evolved neural networks
- Generate diverse body morphologies procedurally
- Allow topology and weights to evolve together
- Maintain compatibility with existing ECS architecture
- Keep performance acceptable for real-time simulation
- Leverage goNEAT for battle-tested NEAT implementation

---

## Part 1: goNEAT Integration Architecture

### Dependencies

```go
import (
    "github.com/yaricom/goNEAT/v4/neat"
    "github.com/yaricom/goNEAT/v4/neat/genetics"
    "github.com/yaricom/goNEAT/v4/neat/network"
)
```

### Key goNEAT Types We Use

| goNEAT Type | Purpose in Soup |
|-------------|-----------------|
| `genetics.Genome` | Genetic blueprint stored per organism |
| `network.Network` | Runtime phenotype for brain evaluation |
| `genetics.MIMOControlGene` | Not used (we use simple feedforward) |
| `neat.Options` | Configuration for mutation rates, etc. |

### Dual-Genome Architecture

Each organism has **two separate genomes**:

```go
// neural/organism.go

type NeuralOrganism struct {
    BodyGenome  *genetics.Genome   // CPPN for morphology
    BrainGenome *genetics.Genome   // Controller for behavior
    Brain       *network.Network   // Instantiated brain (rebuilt rarely)
}
```

Why separate genomes?
- Body only queried at birth (CPPN)
- Brain queried every tick (needs to be fast)
- Different input/output counts
- Can evolve at different rates
- CPPN needs special activation functions

---

## Part 2: Body Network (CPPN)

The Compositional Pattern Producing Network generates organism morphology at birth.

### CPPN Genome Configuration

```go
// neural/cppn.go

const (
    CPPNInputs  = 5  // x, y, d, a, bias
    CPPNOutputs = 4  // presence, diet, traits, priority
)

// CreateCPPNGenome creates a minimal CPPN genome
func CreateCPPNGenome(id int) *genetics.Genome {
    // Create nodes
    nodes := make([]*network.NNode, 0, CPPNInputs+CPPNOutputs)

    // Input nodes (IDs 1-5)
    for i := 1; i <= CPPNInputs; i++ {
        node := network.NewNNode(i, network.InputNeuron)
        node.ActivationType = network.LinearActivation
        nodes = append(nodes, node)
    }

    // Output nodes (IDs 6-9)
    for i := 1; i <= CPPNOutputs; i++ {
        node := network.NewNNode(CPPNInputs+i, network.OutputNeuron)
        // CPPN outputs use tanh for [-1, 1] range
        node.ActivationType = network.TanhActivation
        nodes = append(nodes, node)
    }

    // Create genes (connections) - start fully connected
    genes := make([]*genetics.Gene, 0)
    innovNum := int64(1)
    for i := 1; i <= CPPNInputs; i++ {
        for j := 1; j <= CPPNOutputs; j++ {
            outIdx := CPPNInputs + j
            weight := rand.Float64()*2 - 1  // [-1, 1]
            gene := genetics.NewGeneWithTrait(nil, weight, nodes[i-1], nodes[outIdx-1], false, innovNum, 0)
            genes = append(genes, gene)
            innovNum++
        }
    }

    return genetics.NewGenome(id, nil, nodes, genes)
}
```

### CPPN Activation Functions

goNEAT supports multiple activation functions. For CPPN pattern generation, we need variety:

```go
// Register additional activations for CPPN nodes
// goNEAT's network.NNode.ActivationType can be:
//   - network.SigmoidActivation
//   - network.TanhActivation
//   - network.LinearActivation
//   - network.GaussianActivation (need to verify availability)
//   - network.SineActivation (need to verify availability)

// For hidden nodes added via mutation, randomize activation:
func randomCPPNActivation() network.NodeActivationType {
    activations := []network.NodeActivationType{
        network.SigmoidActivation,
        network.TanhActivation,
        network.GaussianActivation,
        network.SineActivation,
        network.LinearActivation,
    }
    return activations[rand.Intn(len(activations))]
}
```

### Input/Output Specification

```
INPUTS (5 neurons):
├── [0] x:    Normalized grid X position (-1 to 1)
├── [1] y:    Normalized grid Y position (-1 to 1)
├── [2] d:    Distance from center (0 to √2)
├── [3] a:    Angle from center (-π to π)
└── [4] bias: Constant 1.0

OUTPUTS (4 neurons):
├── [0] cell_presence:  >0 means cell exists at this position
├── [1] diet_bias:      <0 herbivore, >0 carnivore tendency
├── [2] trait_flags:    Encoded special traits (speed, vision, etc.)
└── [3] cell_priority:  Tiebreaker for max cell count
```

### Morphology Generation

```go
// neural/morphology.go

const MorphGridSize = 8  // 8x8 query grid

type MorphologyResult struct {
    Cells      []CellSpec
    DietBias   float32
    TraitFlags traits.Trait
}

type CellSpec struct {
    GridX, GridY int8
    Trait        traits.Trait
}

func GenerateMorphology(genome *genetics.Genome, maxCells int) (MorphologyResult, error) {
    // Build phenotype network from genome
    phenotype, err := genome.Genesis(genome.Id)
    if err != nil {
        return MorphologyResult{}, err
    }

    type candidate struct {
        x, y     int8
        presence float64
        diet     float64
        traits   float64
    }
    var candidates []candidate

    // Query CPPN at each grid position
    for gx := 0; gx < MorphGridSize; gx++ {
        for gy := 0; gy < MorphGridSize; gy++ {
            // Normalize coordinates to [-1, 1]
            x := (float64(gx)/float64(MorphGridSize-1))*2 - 1
            y := (float64(gy)/float64(MorphGridSize-1))*2 - 1
            d := math.Sqrt(x*x + y*y)
            a := math.Atan2(y, x)

            // Load inputs and activate
            inputs := []float64{x, y, d, a, 1.0}
            if err := phenotype.LoadSensors(inputs); err != nil {
                continue
            }

            if res, err := phenotype.Activate(); err != nil || !res {
                continue
            }

            outputs := phenotype.ReadOutputs()

            if outputs[0] > 0 {  // Cell presence threshold
                candidates = append(candidates, candidate{
                    x:        int8(gx - MorphGridSize/2),
                    y:        int8(gy - MorphGridSize/2),
                    presence: outputs[0],
                    diet:     outputs[1],
                    traits:   outputs[2],
                })
            }

            // Flush for next activation
            phenotype.Flush()
        }
    }

    // Sort by priority (presence value), take up to maxCells
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].presence > candidates[j].presence
    })

    if len(candidates) > maxCells {
        candidates = candidates[:maxCells]
    }

    // Ensure at least 1 cell
    if len(candidates) == 0 {
        candidates = append(candidates, candidate{x: 0, y: 0, presence: 1})
    }

    // Build result
    result := MorphologyResult{}
    for _, c := range candidates {
        cell := CellSpec{GridX: c.x, GridY: c.y}

        // Determine cell traits from diet output
        if c.diet < -0.3 {
            cell.Trait = traits.Herbivore
        } else if c.diet > 0.3 {
            cell.Trait = traits.Carnivore
        }

        result.Cells = append(result.Cells, cell)
        result.DietBias += float32(c.diet)
    }
    result.DietBias /= float32(len(candidates))

    // Decode trait flags from average traits output
    result.TraitFlags = decodeTraits(candidates)

    return result, nil
}

func decodeTraits(candidates []candidate) traits.Trait {
    if len(candidates) == 0 {
        return 0
    }

    avgTraits := 0.0
    for _, c := range candidates {
        avgTraits += c.traits
    }
    avgTraits /= float64(len(candidates))

    var t traits.Trait
    // Map continuous value to discrete traits
    if avgTraits > 0.5 {
        t |= traits.Speed
    }
    if avgTraits > 0.3 && avgTraits < 0.7 {
        t |= traits.Herding
    }
    if avgTraits < -0.3 {
        t |= traits.FarSight
    }

    return t
}
```

### Example Patterns CPPN Can Produce

```
Radial (d-focused):      Bilateral (x-focused):    Segmented (sin on y):
      ●●●                     ●●●●●                    ●●●●●
    ●●●●●●●                   ●●●●●
    ●●●●●●●                   ●●●●●                    ●●●●●
      ●●●                     ●●●●●
                                                       ●●●●●

Diagonal (x+y):          Ring (d threshold):       Asymmetric (complex):
●                            ●●●                       ●●●
 ●●                        ●●   ●●                      ●●●●
  ●●●                      ●     ●                       ●●
   ●●●●                    ●●   ●●                        ●
    ●●●●●                    ●●●
```

---

## Part 3: Brain Network (Controller)

The brain network runs every tick to determine organism behavior.

### Brain Genome Configuration

```go
// neural/brain.go

const (
    BrainInputs  = 14
    BrainOutputs = 8
)

// CreateBrainGenome creates a minimal brain genome
func CreateBrainGenome(id int) *genetics.Genome {
    nodes := make([]*network.NNode, 0, BrainInputs+BrainOutputs)

    // Input nodes
    for i := 1; i <= BrainInputs; i++ {
        node := network.NewNNode(i, network.InputNeuron)
        node.ActivationType = network.LinearActivation
        nodes = append(nodes, node)
    }

    // Output nodes
    for i := 1; i <= BrainOutputs; i++ {
        node := network.NewNNode(BrainInputs+i, network.OutputNeuron)
        node.ActivationType = network.SigmoidActivation
        nodes = append(nodes, node)
    }

    // Start with sparse random connections (not fully connected)
    genes := make([]*genetics.Gene, 0)
    innovNum := int64(1)
    connectionProb := 0.3  // 30% initial connectivity

    for i := 1; i <= BrainInputs; i++ {
        for j := 1; j <= BrainOutputs; j++ {
            if rand.Float64() < connectionProb {
                outIdx := BrainInputs + j
                weight := rand.Float64()*2 - 1
                gene := genetics.NewGeneWithTrait(nil, weight, nodes[i-1], nodes[outIdx-1], false, innovNum, 0)
                genes = append(genes, gene)
            }
            innovNum++  // Always increment for consistent innovation numbers
        }
    }

    return genetics.NewGenome(id, nil, nodes, genes)
}
```

### Input Specification

```
INPUTS (14 neurons):

Sensory - Food:
├── [0] food_distance:     Normalized (0=at food, 1=none visible)
├── [1] food_angle_sin:    sin(angle to food)
├── [2] food_angle_cos:    cos(angle to food)

Sensory - Threat:
├── [3] predator_distance: Normalized (0=touching, 1=none visible)
├── [4] predator_angle_sin
├── [5] predator_angle_cos

Sensory - Social:
├── [6] mate_distance:     Normalized distance to compatible mate
├── [7] herd_density:      Count of nearby same-species (normalized)

Sensory - Environment:
├── [8] light_level:       From shadowmap (0-1)
├── [9] flow_x:            Local flow field X component
├── [10] flow_y:           Local flow field Y component

Internal State:
├── [11] energy_ratio:     current / max energy
├── [12] cell_count:       Normalized (cells / max_cells)
└── [13] bias:             Constant 1.0
```

### Output Specification

```
OUTPUTS (8 neurons):

Steering Weights (fed to existing behavior system):
├── [0] seek_food:    Weight for food-seeking vector (sigmoid: 0-1)
├── [1] flee_threat:  Weight for predator-fleeing vector
├── [2] seek_mate:    Weight for approaching mates
├── [3] herd:         Weight for flocking behavior
├── [4] wander:       Weight for random exploration

Allocation (softmax for mode selection):
├── [5] grow_drive:   Preference for ModeGrow
├── [6] breed_drive:  Preference for ModeBreed
└── [7] conserve:     Preference for ModeSurvive/ModeStore
```

### Brain Evaluation

```go
// neural/brain.go

type BrainController struct {
    genome  *genetics.Genome
    network *network.Network
}

func NewBrainController(genome *genetics.Genome) (*BrainController, error) {
    phenotype, err := genome.Genesis(genome.Id)
    if err != nil {
        return nil, err
    }

    return &BrainController{
        genome:  genome,
        network: phenotype,
    }, nil
}

func (b *BrainController) Think(inputs []float64) ([]float64, error) {
    if err := b.network.LoadSensors(inputs); err != nil {
        return nil, err
    }

    // Activate with depth-based steps
    depth, err := b.network.MaxActivationDepth()
    if err != nil {
        depth = 5  // Fallback
    }

    for i := 0; i < depth; i++ {
        if _, err := b.network.Activate(); err != nil {
            return nil, err
        }
    }

    outputs := b.network.ReadOutputs()

    // Prepare for next tick
    b.network.Flush()

    return outputs, nil
}

// RebuildNetwork recreates the phenotype (call after genome mutation)
func (b *BrainController) RebuildNetwork() error {
    phenotype, err := b.genome.Genesis(b.genome.Id)
    if err != nil {
        return err
    }
    b.network = phenotype
    return nil
}
```

### Integration with Behavior System

```go
// systems/behavior.go modifications

func (s *BehaviorSystem) Update() {
    query := s.world.Query(&behaviorFilter)
    for query.Next() {
        pos, vel, org, neural := query.Get()

        if org.Dead {
            continue
        }

        // Gather sensory inputs
        inputs := s.gatherInputs(pos, vel, org)

        // Run brain forward pass
        outputs, err := neural.Brain.Think(inputs)
        if err != nil {
            // Fallback to default behavior
            outputs = []float64{1, 1, 0.5, 0.5, 0.3, 0.3, 0.3, 0.3}
        }

        // Scale outputs to useful ranges
        seekFoodWeight := float32(outputs[0]) * 2.0    // 0-2
        fleeWeight := float32(outputs[1]) * 4.0        // 0-4
        seekMateWeight := float32(outputs[2]) * 1.5    // 0-1.5
        herdWeight := float32(outputs[3]) * 1.5        // 0-1.5
        wanderWeight := float32(outputs[4]) * 0.5      // 0-0.5

        // Apply weights to steering vectors
        steerX, steerY := float32(0), float32(0)

        if food := s.findFood(pos, org); food != nil {
            fx, fy := seek(pos, food)
            steerX += fx * seekFoodWeight
            steerY += fy * seekFoodWeight
        }

        if predator := s.findPredator(pos, org); predator != nil {
            fx, fy := flee(pos, predator)
            steerX += fx * fleeWeight
            steerY += fy * fleeWeight
        }

        if mate := s.findMate(pos, org); mate != nil {
            mx, my := seek(pos, mate)
            steerX += mx * seekMateWeight
            steerY += my * seekMateWeight
        }

        hx, hy := s.flockWithHerd(pos, org)
        steerX += hx * herdWeight
        steerY += hy * herdWeight

        // Wander if low activity
        if steerMag(steerX, steerY) < 0.1 {
            wx, wy := wander(org.Heading)
            steerX += wx * wanderWeight
            steerY += wy * wanderWeight
        }

        // Update allocation mode from brain outputs
        org.AllocationMode = decodeAllocation(outputs[5], outputs[6], outputs[7])

        // Apply steering
        vel.X += clamp(steerX, -org.MaxForce, org.MaxForce)
        vel.Y += clamp(steerY, -org.MaxForce, org.MaxForce)
    }
}

func (s *BehaviorSystem) gatherInputs(pos *Position, vel *Velocity, org *Organism) []float64 {
    inputs := make([]float64, BrainInputs)

    // Food detection
    if food := s.findFood(pos, org); food != nil {
        dist := distance(pos, food) / org.PerceptionRadius
        inputs[0] = math.Min(1.0, dist)
        angle := angleToTarget(pos, food)
        inputs[1] = math.Sin(float64(angle))
        inputs[2] = math.Cos(float64(angle))
    } else {
        inputs[0] = 1.0  // No food visible
    }

    // Predator detection
    if predator := s.findPredator(pos, org); predator != nil {
        dist := distance(pos, predator) / (org.PerceptionRadius * 1.5)
        inputs[3] = math.Min(1.0, dist)
        angle := angleToTarget(pos, predator)
        inputs[4] = math.Sin(float64(angle))
        inputs[5] = math.Cos(float64(angle))
    } else {
        inputs[3] = 1.0  // No predator visible
    }

    // Mate detection
    if mate := s.findMate(pos, org); mate != nil {
        dist := distance(pos, mate) / org.PerceptionRadius
        inputs[6] = math.Min(1.0, dist)
    } else {
        inputs[6] = 1.0
    }

    // Herd density
    inputs[7] = float64(s.countNearbyHerd(pos, org)) / 10.0

    // Environment
    inputs[8] = float64(s.shadowMap.GetLight(pos.X, pos.Y))
    fx, fy := s.flowField.GetFlow(pos.X, pos.Y)
    inputs[9] = float64(fx)
    inputs[10] = float64(fy)

    // Internal state
    inputs[11] = float64(org.Energy / org.MaxEnergy)
    inputs[12] = float64(org.CellCount) / 32.0  // Normalized by max cells
    inputs[13] = 1.0  // Bias

    return inputs
}

func decodeAllocation(grow, breed, conserve float64) AllocationMode {
    max := grow
    mode := ModeGrow
    if breed > max {
        max = breed
        mode = ModeBreed
    }
    if conserve > max {
        mode = ModeSurvive
    }
    return mode
}
```

---

## Part 4: Evolution Mechanics (goNEAT)

### goNEAT Configuration

```yaml
# config/neat.yaml

# Overall parameters
trait_param_mut_prob: 0.5
trait_mutation_power: 1.0
weight_mut_power: 2.5

# Structural mutation
mutate_add_node_prob: 0.03
mutate_add_link_prob: 0.05
mutate_toggle_enable_prob: 0.01

# Weight mutation
mutate_link_weights_prob: 0.8
mutate_only_prob: 0.25
mutate_random_trait_prob: 0.1

# Mating
mate_multipoint_prob: 0.6
mate_multipoint_avg_prob: 0.4
mate_singlepoint_prob: 0.0
mate_only_prob: 0.2
recur_only_prob: 0.0

# Speciation
compat_threshold: 3.0
disjoint_coeff: 1.0
excess_coeff: 1.0
weight_diff_coeff: 0.4

# Population (not directly used - we manage population via simulation)
pop_size: 100
dropoff_age: 15
survival_thresh: 0.2
age_significance: 1.0
```

### Loading Configuration

```go
// neural/config.go

func LoadNEATOptions(configPath string) (*neat.Options, error) {
    file, err := os.Open(configPath)
    if err != nil {
        return nil, err
    }
    defer file.Close()

    opts, err := neat.LoadYAMLOptions(file)
    if err != nil {
        return nil, err
    }

    return opts, nil
}

// Alternatively, create programmatically
func DefaultNEATOptions() *neat.Options {
    return &neat.Options{
        TraitParamMutProb:     0.5,
        TraitMutationPower:    1.0,
        WeightMutPower:        2.5,

        MutateAddNodeProb:     0.03,
        MutateAddLinkProb:     0.05,
        MutateToggleEnableProb: 0.01,

        MutateLinkWeightsProb: 0.8,
        MutateOnlyProb:        0.25,
        MutateRandomTraitProb: 0.1,

        MateMultipointProb:    0.6,
        MateMultipointAvgProb: 0.4,
        MateSinglepointProb:   0.0,
        MateOnlyProb:          0.2,
        RecurOnlyProb:         0.0,

        CompatThreshold:       3.0,
        DisjointCoeff:         1.0,
        ExcessCoeff:           1.0,
        WeightDiffCoeff:       0.4,
    }
}
```

### Crossover Using goNEAT

```go
// neural/reproduction.go

// CrossoverGenomes performs NEAT mating between two genomes
func CrossoverGenomes(parent1, parent2 *genetics.Genome, fitness1, fitness2 float64,
                      childID int, opts *neat.Options) (*genetics.Genome, error) {

    // Create organism wrappers for goNEAT mating
    org1 := &genetics.Organism{
        Genotype: parent1,
        Fitness:  fitness1,
    }
    org2 := &genetics.Organism{
        Genotype: parent2,
        Fitness:  fitness2,
    }

    // Use goNEAT's mating logic
    // genetics.Genome has Mate method
    child, err := parent1.Mate(parent2, childID, fitness1, fitness2)
    if err != nil {
        return nil, err
    }

    return child, nil
}

// MutateGenome applies NEAT mutations to a genome
func MutateGenome(genome *genetics.Genome, opts *neat.Options,
                  innovationNum *int64) (bool, error) {

    // goNEAT mutation methods
    mutated := false

    // Weight mutation
    if rand.Float64() < opts.MutateLinkWeightsProb {
        genome.MutateLinkWeights(opts.WeightMutPower, 1.0,
            genetics.GaussianMutator)
        mutated = true
    }

    // Add node mutation
    if rand.Float64() < opts.MutateAddNodeProb {
        if _, err := genome.MutateAddNode(nil, opts); err == nil {
            mutated = true
        }
    }

    // Add link mutation
    if rand.Float64() < opts.MutateAddLinkProb {
        if _, err := genome.MutateAddLink(opts, 5); err == nil {
            mutated = true
        }
    }

    // Toggle enable mutation
    if rand.Float64() < opts.MutateToggleEnableProb {
        genome.MutateToggleEnable(1)
        mutated = true
    }

    return mutated, nil
}
```

### Speciation Using goNEAT

```go
// neural/species.go

type SpeciesManager struct {
    species     []*genetics.Species
    opts        *neat.Options
    nextSpeciesID int
}

func NewSpeciesManager(opts *neat.Options) *SpeciesManager {
    return &SpeciesManager{
        species:       make([]*genetics.Species, 0),
        opts:          opts,
        nextSpeciesID: 1,
    }
}

// AssignSpecies finds or creates a species for the organism
func (sm *SpeciesManager) AssignSpecies(genome *genetics.Genome,
                                         fitness float64) *genetics.Species {

    org := &genetics.Organism{
        Genotype: genome,
        Fitness:  fitness,
    }

    // Try to find compatible species
    for _, sp := range sm.species {
        if sp.Representative == nil {
            continue
        }

        // Calculate compatibility distance
        dist := genome.Compatibility(sp.Representative.Genotype, sm.opts)

        if dist < sm.opts.CompatThreshold {
            sp.AddOrganism(org)
            org.Species = sp
            return sp
        }
    }

    // No compatible species - create new one
    newSpecies := genetics.NewSpecies(sm.nextSpeciesID)
    sm.nextSpeciesID++
    newSpecies.AddOrganism(org)
    org.Species = newSpecies
    sm.species = append(sm.species, newSpecies)

    return newSpecies
}

// RemoveStaleSpecies removes species that haven't improved
func (sm *SpeciesManager) RemoveStaleSpecies(maxStaleness int) {
    active := make([]*genetics.Species, 0, len(sm.species))
    for _, sp := range sm.species {
        if sp.Age - sp.AgeOfLastImprovement < maxStaleness {
            active = append(active, sp)
        }
    }
    sm.species = active
}

// UpdateSpeciesRepresentatives prepares for next generation
func (sm *SpeciesManager) UpdateSpeciesRepresentatives() {
    for _, sp := range sm.species {
        if len(sp.Organisms) > 0 {
            // Random representative from current members
            idx := rand.Intn(len(sp.Organisms))
            sp.Representative = sp.Organisms[idx]
        }
        sp.Organisms = nil  // Clear for next generation
        sp.Age++
    }
}
```

---

## Part 5: ECS Integration

### New Components

```go
// components/neural.go

// NeuralGenome stores the genetic blueprints
type NeuralGenome struct {
    BodyGenome  *genetics.Genome  // CPPN for morphology
    BrainGenome *genetics.Genome  // Controller for behavior
    SpeciesID   int               // Species assignment
    Generation  int               // Birth generation
}

// Brain stores the instantiated runtime network
type Brain struct {
    Controller *BrainController  // Wraps network + evaluation logic
}
```

### Neural System (manages evolution tracking)

```go
// systems/neural.go

type NeuralSystem struct {
    world          *ecs.World
    speciesManager *SpeciesManager
    opts           *neat.Options
    generation     int
    nextGenomeID   int
}

func NewNeuralSystem(world *ecs.World, opts *neat.Options) *NeuralSystem {
    return &NeuralSystem{
        world:          world,
        speciesManager: NewSpeciesManager(opts),
        opts:           opts,
        generation:     0,
        nextGenomeID:   1,
    }
}

// CreateInitialPopulation creates the starting population with random genomes
func (s *NeuralSystem) CreateInitialPopulation(count int) {
    for i := 0; i < count; i++ {
        bodyGenome := CreateCPPNGenome(s.nextGenomeID)
        s.nextGenomeID++

        brainGenome := CreateBrainGenome(s.nextGenomeID)
        s.nextGenomeID++

        // Generate morphology
        morph, _ := GenerateMorphology(bodyGenome, MaxCells)

        // Create organism entity
        pos := Position{
            X: rand.Float32() * WorldWidth,
            Y: rand.Float32() * WorldHeight,
        }

        s.createOrganism(bodyGenome, brainGenome, morph, pos)
    }
}

func (s *NeuralSystem) createOrganism(bodyGenome, brainGenome *genetics.Genome,
                                       morph MorphologyResult, pos Position) ecs.Entity {

    // Build brain controller
    brain, err := NewBrainController(brainGenome)
    if err != nil {
        // Fallback to minimal brain
        brainGenome = CreateBrainGenome(s.nextGenomeID)
        s.nextGenomeID++
        brain, _ = NewBrainController(brainGenome)
    }

    // Determine traits from morphology
    var orgTraits traits.Trait = morph.TraitFlags
    if morph.DietBias < -0.2 {
        orgTraits |= traits.Herbivore
    } else if morph.DietBias > 0.2 {
        orgTraits |= traits.Carnivore
    } else {
        orgTraits |= traits.Herbivore | traits.Carnivore
    }

    // Create entity
    entity := s.world.Create(
        pos,
        Velocity{},
        Organism{
            Traits:     orgTraits,
            Energy:     50,
            MaxEnergy:  100 + float32(len(morph.Cells))*50,
            CellSize:   2.5,
            MaxSpeed:   2.0,
            MaxForce:   0.03,
            PerceptionRadius: 100,
        },
        NeuralGenome{
            BodyGenome:  bodyGenome,
            BrainGenome: brainGenome,
            SpeciesID:   0,  // Assigned below
            Generation:  s.generation,
        },
        Brain{
            Controller: brain,
        },
    )

    // Create cells based on CPPN output
    for _, cellSpec := range morph.Cells {
        s.world.Create(
            Cell{
                GridX:   cellSpec.GridX,
                GridY:   cellSpec.GridY,
                Trait:   cellSpec.Trait,
                Alive:   true,
                MaxAge:  3000 + rand.Int31n(2000),
            },
            CellOwner{Entity: entity},
        )
    }

    // Assign to species (using brain genome for compatibility)
    // Fitness is 0 for new organisms - increases with survival
    sp := s.speciesManager.AssignSpecies(brainGenome, 0)
    // Update entity's species ID...

    return entity
}
```

### Modified Breeding System

```go
// systems/breeding.go modifications

type BreedingSystem struct {
    world        *ecs.World
    neuralSystem *NeuralSystem
    opts         *neat.Options
}

func (s *BreedingSystem) breed(parent1Ent, parent2Ent ecs.Entity) {
    // Get parent components
    parent1Org := s.world.Get(parent1Ent, &Organism{}).(*Organism)
    parent2Org := s.world.Get(parent2Ent, &Organism{}).(*Organism)
    parent1Neural := s.world.Get(parent1Ent, &NeuralGenome{}).(*NeuralGenome)
    parent2Neural := s.world.Get(parent2Ent, &NeuralGenome{}).(*NeuralGenome)
    parent1Pos := s.world.Get(parent1Ent, &Position{}).(*Position)
    parent2Pos := s.world.Get(parent2Ent, &Position{}).(*Position)

    // Fitness proxy: energy * survival_time
    fitness1 := float64(parent1Org.Energy)
    fitness2 := float64(parent2Org.Energy)

    // Crossover body genomes
    childBodyGenome, err := CrossoverGenomes(
        parent1Neural.BodyGenome,
        parent2Neural.BodyGenome,
        fitness1, fitness2,
        s.neuralSystem.nextGenomeID,
        s.opts,
    )
    if err != nil {
        return
    }
    s.neuralSystem.nextGenomeID++

    // Crossover brain genomes
    childBrainGenome, err := CrossoverGenomes(
        parent1Neural.BrainGenome,
        parent2Neural.BrainGenome,
        fitness1, fitness2,
        s.neuralSystem.nextGenomeID,
        s.opts,
    )
    if err != nil {
        return
    }
    s.neuralSystem.nextGenomeID++

    // Mutate child genomes
    MutateGenome(childBodyGenome, s.opts, nil)
    MutateGenome(childBrainGenome, s.opts, nil)

    // Generate child morphology from CPPN
    morph, err := GenerateMorphology(childBodyGenome, MaxCells)
    if err != nil {
        return
    }

    // Child position at midpoint
    childPos := Position{
        X: (parent1Pos.X + parent2Pos.X) / 2,
        Y: (parent1Pos.Y + parent2Pos.Y) / 2,
    }

    // Create child entity
    s.neuralSystem.createOrganism(childBodyGenome, childBrainGenome, morph, childPos)

    // Deduct energy from parents
    parent1Org.Energy -= 20
    parent2Org.Energy -= 20
    parent1Org.BreedingCooldown = 180
    parent2Org.BreedingCooldown = 180
}
```

---

## Part 6: Implementation Phases

### Phase 1: goNEAT Setup

**Goal**: Get goNEAT integrated and verify basic functionality

Tasks:
- [ ] Add goNEAT dependency: `go get github.com/yaricom/goNEAT/v4`
- [ ] Create `neural/` package structure
- [ ] Create `neural/config.go` with NEAT options
- [ ] Create `neural/brain.go` with BrainController
- [ ] Test genome creation and network evaluation
- [ ] Verify network activation produces valid outputs

Files:
- `neural/config.go`
- `neural/brain.go`
- `neural/brain_test.go`

### Phase 2: Brain Integration

**Goal**: Replace fixed behavior weights with brain network

Tasks:
- [ ] Add `NeuralGenome` and `Brain` components
- [ ] Create `gatherInputs()` function for sensory data
- [ ] Modify `BehaviorSystem` to use brain outputs
- [ ] Create initial brain genome factory
- [ ] Test with hand-tuned weights to verify integration
- [ ] Add fallback behavior for failed activations

Files:
- `components/neural.go`
- `systems/behavior.go` (modified)
- `neural/inputs.go`

### Phase 3: Evolution

**Goal**: Implement reproduction with genetic crossover and mutation

Tasks:
- [ ] Create `neural/reproduction.go` with crossover/mutation
- [ ] Create `neural/species.go` with species management
- [ ] Modify breeding system to use goNEAT mating
- [ ] Track organism fitness (energy × survival time)
- [ ] Test evolution across generations

Files:
- `neural/reproduction.go`
- `neural/species.go`
- `systems/breeding.go` (modified)

### Phase 4: CPPN Morphology

**Goal**: Generate body shapes from CPPN

Tasks:
- [ ] Create `neural/cppn.go` with CPPN genome factory
- [ ] Create `neural/morphology.go` with generation logic
- [ ] Modify organism creation to use CPPN output
- [ ] Add CPPN-specific activation function handling
- [ ] Test morphology diversity
- [ ] Tune cell threshold and priority sorting

Files:
- `neural/cppn.go`
- `neural/morphology.go`
- `systems/neural.go`

### Phase 5: Speciation & Tuning

**Goal**: Protect innovation with species, tune parameters

Tasks:
- [ ] Implement full speciation lifecycle
- [ ] Add species visualization (color by species)
- [ ] Remove stale species
- [ ] Tune compatibility threshold
- [ ] Tune mutation rates for good evolution
- [ ] Add runtime statistics

Files:
- `neural/species.go` (enhanced)
- `renderer/species.go`
- `config/neat.yaml`

### Phase 6: Optimization

**Goal**: Performance optimization for real-time simulation

Tasks:
- [ ] Profile network evaluation performance
- [ ] Cache network topology sort
- [ ] Pool network activation buffers
- [ ] Consider batch evaluation
- [ ] Benchmark with 500+ organisms

Files:
- Various optimization passes
- `neural/pool.go` (optional)

---

## Part 7: Configuration

### YAML Configuration File

```yaml
# config/neat.yaml

neat:
  # Structural mutation rates
  mutate_add_node_prob: 0.03
  mutate_add_link_prob: 0.05
  mutate_toggle_enable_prob: 0.01

  # Weight mutation
  mutate_link_weights_prob: 0.8
  weight_mut_power: 2.5

  # Mating probabilities
  mate_multipoint_prob: 0.6
  mate_multipoint_avg_prob: 0.4

  # Speciation
  compat_threshold: 3.0
  disjoint_coeff: 1.0
  excess_coeff: 1.0
  weight_diff_coeff: 0.4

  # Species management
  dropoff_age: 15
  survival_thresh: 0.2

cppn:
  grid_size: 8
  max_cells: 32
  cell_threshold: 0.0

brain:
  inputs: 14
  outputs: 8
  initial_connection_prob: 0.3
```

### Go Configuration Struct

```go
// neural/config.go

type Config struct {
    NEAT neat.Options
    CPPN CPPNConfig
    Brain BrainConfig
}

type CPPNConfig struct {
    GridSize      int     `yaml:"grid_size"`
    MaxCells      int     `yaml:"max_cells"`
    CellThreshold float64 `yaml:"cell_threshold"`
}

type BrainConfig struct {
    Inputs                int     `yaml:"inputs"`
    Outputs               int     `yaml:"outputs"`
    InitialConnectionProb float64 `yaml:"initial_connection_prob"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }

    var cfg Config
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return nil, err
    }

    return &cfg, nil
}
```

---

## Part 8: Expected Emergent Behaviors

### Body-Brain Coevolution

Over generations, expect to see:

| Body Type | Brain Adaptation | Ecological Role |
|-----------|------------------|-----------------|
| Elongated, few cells | High seek weight, low herd | Fast solitary predator |
| Compact, many cells | High flee weight, high herd | Defensive schooling prey |
| Asymmetric | Turning bias to compensate | Specialized niche |
| Large radial | Low wander, high conserve | Ambush predator |

### Speciation Events

- Initial random genomes differentiate
- Herbivore vs carnivore species emerge
- Sub-species form (fast vs armored herbivores)
- Rare crossover between species creates hybrids

### Arms Races

- Predator vision range vs prey flee sensitivity
- Predator speed vs prey herd tightness
- Body size vs metabolism efficiency

---

## Part 9: Visualization Ideas

### Debug Overlays

```
[B] - Show brain activity (input/output values as bars)
[G] - Show genome complexity (node/connection count)
[S] - Color by species
[M] - Show morphology grid
[F] - Show fitness/energy gradient
```

### Statistics Panel

```
Population: 247
Species: 12
Generation: 156
Avg Brain Size: 18 nodes, 45 connections
Top Species:
  #1: Herbivore-school (89 members, fitness: 2340)
  #2: Carnivore-ambush (34 members, fitness: 1890)
  #3: Omnivore-wander (28 members, fitness: 1245)
```

---

## Appendix: Reference Links

- [goNEAT Repository](https://github.com/yaricom/goNEAT) - Go NEAT implementation
- [goNEAT Documentation](https://pkg.go.dev/github.com/yaricom/goNEAT/v4) - API docs
- [NEAT Paper](http://nn.cs.utexas.edu/downloads/papers/stanley.ec02.pdf) - Original Stanley & Miikkulainen
- [CPPN Paper](http://eplex.cs.ucf.edu/papers/stanley_gpem07.pdf) - Compositional Pattern Producing Networks

### goNEAT Key Types Reference

```go
// Genome - genetic blueprint
genome := genetics.NewGenome(id, traits, nodes, genes)
network, _ := genome.Genesis(id)  // Build phenotype

// Network - runtime evaluation
network.LoadSensors(inputs)
network.Activate()
outputs := network.ReadOutputs()
network.Flush()

// Mating
child, _ := parent1.Mate(parent2, childID, fitness1, fitness2)

// Mutation
genome.MutateLinkWeights(power, rate, mutator)
genome.MutateAddNode(traits, opts)
genome.MutateAddLink(opts, tries)

// Compatibility
dist := genome1.Compatibility(genome2, opts)
```

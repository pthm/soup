# Design Options: Traits, Speciation, and Morphology

## 1. Trait System Options

Currently traits mix **capabilities** (Speed, FarSight), **behaviors** (Herding, Photophilic), and **categories** (Herbivore, Carnivore).

### Option A: Traits as Capabilities Only
Keep traits but they only affect **what organism CAN do**, not what it WILL do.

```
DIET TRAITS (what can be eaten):
  Herbivore  → can eat flora
  Carnivore  → can eat fauna
  Carrion    → can eat dead organisms

PHYSICAL TRAITS (stat modifiers):
  Speed      → maxSpeed *= 1.5
  FarSight   → perceptionRadius *= 1.5
  Armored    → takes less damage from attacks (new)
  Efficient  → lower metabolism cost (new)

SENSORY TRAITS (affects brain inputs):
  WideVision   → sees 270° but shorter range
  NarrowVision → sees 90° but longer range

GENDER (for reproduction):
  Male / Female
```

**Pros**: Clean separation, traits still visible in UI, meaningful for evolution
**Cons**: Still need to decide which traits to include

### Option B: Traits Derived from Body/Brain
No explicit trait genes. Traits are **computed** from organism properties.

```
// Derived at runtime:
isHerbivore = brain prefers eating flora (learned behavior)
isFast = body shape is streamlined + few cells
hasWideVision = cell arrangement is wide
```

UI shows derived properties:
- "Fast" if speed > threshold
- "Predator" if observed eating fauna
- "Social" if often near others

**Pros**: Emergent, no predefined categories, truly evolved
**Cons**: Harder to track lineages, UI less predictable

### Option C: Minimal Essential Traits
Only keep what's **mechanically necessary**:

```
DIET (required for eating rules):
  Herbivore | Carnivore | Carrion | Omnivore

GENDER (required for sexual reproduction):
  Male | Female

That's it. Everything else emerges from brain/body.
```

UI can still show:
- Cell count (derived from body)
- Speed (derived from shape + size)
- Behavior labels (derived from brain output patterns)

**Pros**: Simplest, forces emergence
**Cons**: Less explicit evolution of "traits"

### Option D: Trait Spectrum (Continuous)
Instead of binary traits, use **continuous values**:

```
diet_preference:    -1 (herbivore) to +1 (carnivore)
speed_investment:   0 to 1 (affects metabolism tradeoff)
perception_range:   0.5 to 2.0 (multiplier)
social_tendency:    -1 (solitary) to +1 (gregarious)
```

These are genes that:
- Affect capabilities (speed_investment → actual speed)
- Are inherited and mutated
- Shown as spectrums in UI

**Pros**: More nuanced evolution, gradual changes
**Cons**: More complex genome, harder to categorize

### Recommendation
**Option A (Capabilities Only)** with a subset of traits:

```go
type Trait uint16

const (
    // Diet (at least one required)
    Herbivore Trait = 1 << iota
    Carnivore
    Carrion

    // Physical (optional, affect stats)
    Fast        // +50% speed, +30% metabolism
    Efficient   // -30% metabolism
    Tough       // harder to damage

    // Gender
    Male
    Female
)
```

Remove: Herding, Photophilic, Photophobic, PredatorEyes, PreyEyes, FarSight, Rooted, Floating

---

## 2. Speciation: Pros and Cons

### What Speciation Does
NEAT speciation groups organisms by **genome similarity**. Organisms compete primarily within their species, protecting new innovations from being outcompeted immediately.

### Pros of Speciation

| Benefit | Explanation |
|---------|-------------|
| **Protects innovation** | A new mutation (e.g., first hidden node) gets time to optimize before competing with refined genomes |
| **Maintains diversity** | Prevents single "super-genome" from dominating |
| **Enables niches** | Different species can specialize (fast small vs slow large) |
| **Interesting to observe** | Watch species emerge, compete, go extinct |
| **Proven effective** | NEAT paper shows it's crucial for complex networks |

### Cons of Speciation

| Drawback | Explanation |
|----------|-------------|
| **Complexity** | More code, more parameters to tune |
| **Currently broken** | Only 1 species forming (threshold too high) |
| **May not be necessary** | With spatial separation, speciation might emerge naturally |
| **Parameter sensitivity** | Wrong threshold → too few or too many species |

### Options

**Option 1: Fix Current Speciation**
- Lower `compat_threshold` from 2.3 to 0.8-1.2
- Speciation based on brain genome distance
- Keep fitness sharing within species

**Option 2: Remove Speciation**
- Let all genomes compete freely
- Rely on spatial/ecological separation for diversity
- Simpler code, may work with large populations

**Option 3: Ecological Speciation**
- Species based on **diet and morphology**, not genome
- Herbivores compete with herbivores
- Large organisms compete with large organisms
- More intuitive, less parameter-sensitive

**Option 4: Implicit Speciation (Fitness Sharing)**
- No explicit species labels
- Fitness reduced based on genome similarity to neighbors
- Similar effect to speciation but softer boundaries

### Recommendation
**Option 3 (Ecological Speciation)** for simplicity:

```go
func getEcologicalNiche(org *Organism) int {
    niche := 0
    if org.Traits.Has(Carnivore) { niche |= 1 }
    if org.Traits.Has(Herbivore) { niche |= 2 }
    if org.CellCount > 5 { niche |= 4 }  // Large
    return niche
}
// Organisms in same niche compete more directly
```

Or **Option 1** if you want true NEAT-style speciation (fix the threshold).

---

## 3. CPPN Morphology Options

### Current Problems
- Generates 1-32 cells at birth
- Large organisms starve (100 energy, 1700 max)
- Shapes aren't very diverse (mostly blobs)
- No growth dynamics

### Option A: Constrained Initial Size
Keep CPPN but limit initial generation:

```go
const (
    InitialMaxCells = 4    // Start small
    GrowthMaxCells  = 16   // Can grow larger
)

// At birth: generate up to 4 cells from CPPN
// During life: can grow more cells using same CPPN pattern
```

**Pros**: Simple fix, keeps CPPN diversity
**Cons**: Still somewhat random shapes

### Option B: Growth-Based Morphology
CPPN defines **growth rules**, not initial shape:

```go
// Start with 1 cell always
// When organism has energy surplus, query CPPN:
func (cppn) ShouldGrowAt(x, y int, neighbors int) float64

// CPPN inputs: position, neighbor count, energy level
// CPPN output: probability of growing cell here
```

**Pros**: Dynamic body, responsive to environment
**Cons**: More complex, body changes during life

### Option C: Symmetry-Aware CPPN
Add symmetry to CPPN queries for more biological shapes:

```go
// Query with symmetry
outputs := []float64{}
for _, (x, y) := range gridPositions {
    // Bilateral symmetry: query both sides
    left := cppn.Query(x, y)
    right := cppn.Query(-x, y)  // Mirror
    avg := (left + right) / 2
    outputs = append(outputs, avg)
}
```

Produces symmetric creatures (like real animals).

**Pros**: More "creature-like" shapes
**Cons**: Reduces shape diversity

### Option D: Body Plan Templates
Define archetypes, CPPN adds variation:

```
TEMPLATES:
  Snake:  1xN elongated
  Blob:   NxN compact
  Star:   central with extensions
  Ring:   hollow center

CPPN modifies:
  - Which cells are present/absent
  - Cell properties (diet bias)
  - Asymmetries
```

**Pros**: Predictable base shapes, still varied
**Cons**: Less emergent, predefined categories

### Option E: Positional Encoding CPPN
Better CPPN inputs for more interesting patterns:

```go
// Current inputs: x, y, distance, angle, bias (5)
// Enhanced inputs:
inputs := []float64{
    x, y,                           // Position
    x*x + y*y,                      // Distance squared
    math.Sin(x * math.Pi),          // Periodic X
    math.Sin(y * math.Pi),          // Periodic Y
    math.Sin((x+y) * math.Pi),      // Diagonal wave
    float64(rand.Intn(2)),          // Noise seed
    1.0,                            // Bias
}
```

Creates more varied patterns: stripes, spots, segments.

**Pros**: Richer patterns from same CPPN structure
**Cons**: More inputs = larger network

### Option F: Multi-Scale CPPN
Query at multiple resolutions:

```go
// Coarse query: overall body plan (4x4 grid)
// Fine query: detail within cells (8x8 grid)
// Combine for final shape
```

**Pros**: Hierarchical structure like real development
**Cons**: More complex

### Recommendation
**Option A + E**: Constrained initial size with enhanced positional encoding.

```go
// Start with max 4 cells
// Enhanced CPPN inputs for better patterns
// Can grow to 12 cells during life

CPPNInputs := []float64{
    x, y,                      // Position (-1 to 1)
    math.Sqrt(x*x + y*y),      // Distance from center
    math.Atan2(y, x) / math.Pi, // Angle (-1 to 1)
    math.Sin(x * 2 * math.Pi), // Horizontal wave
    math.Sin(y * 2 * math.Pi), // Vertical wave
    1.0,                       // Bias
}

// CPPN outputs:
// [0] cell_presence (>0 = cell exists)
// [1] cell_type (-1 = herbivore cell, +1 = carnivore cell)
```

---

## Summary of Recommendations

| System | Recommendation |
|--------|---------------|
| **Brain I/O** | 4 outputs (turn, thrust, eat, mate) |
| **Traits** | Capabilities only (Diet, Fast, Efficient, Gender) |
| **Speciation** | Ecological (by diet + size) or fix NEAT threshold |
| **CPPN** | Constrained initial size (1-4), enhanced inputs, growth to 12 |

### Quick Wins to Try First
1. Reduce CPPN max cells from 32 → 4
2. Lower speciation threshold from 2.3 → 1.0
3. Simplify brain to 4 outputs
4. Remove behavior system steering logic

Want me to implement any of these changes?

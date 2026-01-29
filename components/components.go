// Package components defines ECS components for the simulation.
package components

import (
	"github.com/pthm-cable/soup/neural"
	"github.com/yaricom/goNEAT/v4/neat/genetics"
)

// AllocationMode determines how an organism prioritizes energy use.
type AllocationMode uint8

const (
	ModeSurvive AllocationMode = iota // Conserve energy, no growth/breeding
	ModeGrow                          // Prioritize cell growth
	ModeBreed                         // Prioritize reproduction
	ModeStore                         // Build energy reserves
)

// Position represents an entity's world position.
type Position struct {
	X, Y float32
}

// Velocity represents an entity's velocity.
type Velocity struct {
	X, Y float32
}

// ShapeMetrics describes the physical shape characteristics of an organism.
type ShapeMetrics struct {
	AspectRatio     float32 // Length/Width (higher = streamlined)
	CrossSection    float32 // Max width perpendicular to heading
	Streamlining    float32 // 0-1 (1 = streamlined)
	DragCoefficient float32 // 0.3 (streamlined) to 1.0 (blunt)
}

// CollisionOBB defines an oriented bounding box for terrain collision.
// The OBB is computed from the organism's cell layout and rotates with heading.
type CollisionOBB struct {
	HalfWidth  float32 // Half-extent along local X axis
	HalfHeight float32 // Half-extent along local Y axis
	OffsetX    float32 // Center offset from organism position (local X)
	OffsetY    float32 // Center offset from organism position (local Y)
}

// Organism holds organism-specific data.
// Note: Diet and other capabilities are now derived from cells, not stored as traits.
// All ECS organisms are fauna - flora are managed separately by FloraSystem.
type Organism struct {
	Energy float32
	MaxEnergy         float32
	CellSize          float32
	MaxSpeed          float32
	MaxForce          float32
	PerceptionRadius  float32
	Dead              bool
	Heading           float32
	GrowthTimer       int32
	GrowthInterval    int32
	SporeTimer        int32
	SporeInterval     int32
	BreedingCooldown  int32
	AllocationMode    AllocationMode
	TargetCells       uint8        // Desired cell count based on conditions
	DeadTime          int32        // Ticks since death (for decomposition/removal)
	ShapeMetrics      ShapeMetrics // Physical shape characteristics
	ActiveThrust      float32      // Thrust magnitude this tick (for energy cost)
	OBB               CollisionOBB // Collision bounding box computed from cells

	// Brain outputs (Phase 5: intent-based)
	DesireAngle    float32 // Brain output: -π to +π, where to go relative to heading
	DesireDistance float32 // Brain output: 0-1, movement urgency
	EatIntent      float32 // Brain output: 0-1, >0.5 means try to eat
	GrowIntent     float32 // Brain output: 0-1, allocate energy to growth
	BreedIntent    float32 // Brain output: 0-1, >0.5 means try to reproduce
	GlowIntent     float32 // Brain output: 0-1, bioluminescence intensity (Phase 5b)

	// Derived motor outputs (computed by pathfinding layer in Phase 5)
	TurnOutput   float32 // -1 to +1, current turn output
	ThrustOutput float32 // 0 to 1, current thrust output

	// Bioluminescence state (Phase 5b)
	EmittedLight float32 // Current light emission (GlowIntent × BioluminescentCap)
}

// Cell represents a single cell within an organism.
// Cell function and behavior are determined by PrimaryType/SecondaryType from CPPN.
type Cell struct {
	GridX int8
	GridY int8
	Age   int32
	MaxAge int32
	Alive         bool
	Decomposition float32

	// Function selection (from CPPN)
	PrimaryType       neural.CellType // Main function
	SecondaryType     neural.CellType // Optional secondary function (CellTypeNone if none)
	PrimaryStrength   float32         // Effective primary strength
	SecondaryStrength float32         // Effective secondary strength

	// Digestive spectrum (for digestive cells)
	DigestiveSpectrum float32 // 0=herbivore, 1=carnivore

	// Modifiers
	StructuralArmor float32 // 0-1, damage reduction (adds drag)
	StorageCapacity float32 // 0-1, max energy bonus (adds metabolism)

	// Reproduction spectrum (from CPPN)
	ReproductiveMode float32 // 0=asexual, 0.5=mixed, 1=sexual
}

// GetSensorStrength returns the effective sensor capability of this cell.
func (c *Cell) GetSensorStrength() float32 {
	if !c.Alive {
		return 0
	}
	if c.PrimaryType == neural.CellTypeSensor {
		return c.PrimaryStrength
	}
	if c.SecondaryType == neural.CellTypeSensor {
		return c.SecondaryStrength
	}
	return 0
}

// GetActuatorStrength returns the effective actuator capability of this cell.
func (c *Cell) GetActuatorStrength() float32 {
	if !c.Alive {
		return 0
	}
	if c.PrimaryType == neural.CellTypeActuator {
		return c.PrimaryStrength
	}
	if c.SecondaryType == neural.CellTypeActuator {
		return c.SecondaryStrength
	}
	return 0
}

// HasFunction returns true if this cell has the given function (primary or secondary).
func (c *Cell) HasFunction(ct neural.CellType) bool {
	return c.PrimaryType == ct || c.SecondaryType == ct
}

// GetFunctionStrength returns the effective strength of a function for this cell.
func (c *Cell) GetFunctionStrength(ct neural.CellType) float32 {
	if !c.Alive {
		return 0
	}
	if c.PrimaryType == ct {
		return c.PrimaryStrength
	}
	if c.SecondaryType == ct {
		return c.SecondaryStrength
	}
	return 0
}

// CellBuffer holds the cells of an organism.
// Using a fixed-size array for better cache locality.
type CellBuffer struct {
	Cells [16]Cell
	Count uint8
}

// AddCell adds a cell to the buffer.
func (cb *CellBuffer) AddCell(c Cell) bool {
	if cb.Count >= 16 {
		return false
	}
	cb.Cells[cb.Count] = c
	cb.Count++
	return true
}

// RemoveCell removes a cell at index by swapping with last.
func (cb *CellBuffer) RemoveCell(idx uint8) {
	if idx >= cb.Count {
		return
	}
	cb.Count--
	cb.Cells[idx] = cb.Cells[cb.Count]
}

// Capabilities holds computed capability values from cells.
type Capabilities struct {
	PhotoWeight        float32 // Total photosynthetic strength
	ActuatorWeight     float32 // Total actuator strength
	SensorWeight       float32 // Total sensor strength
	MouthSize          float32 // Total mouth strength (for feeding)
	DigestiveSum       float32 // Sum of digestive spectrum weighted by strength
	DigestiveCount     int     // Number of digestive cells
	StructuralArmor    float32 // Average structural armor
	StorageCapacity    float32 // Average storage capacity
	BioluminescentCap  float32 // Total bioluminescent capability (Phase 5b)
	ReproductiveWeight float32 // Total reproductive capability
	ReproductiveSum    float32 // Sum of reproductive mode values from reproductive cells
	ReproductiveCount  int     // Number of reproductive cells
}

// Composition returns the flora/fauna composition ratio.
// 1.0 = pure photosynthetic (flora-like), 0.0 = pure actuator (fauna-like)
func (c *Capabilities) Composition() float32 {
	const epsilon = 1e-6
	total := c.PhotoWeight + c.ActuatorWeight
	if total < epsilon {
		return 0.5 // Neutral for organisms with neither
	}
	return c.PhotoWeight / total
}

// DigestiveSpectrum returns the average digestive spectrum.
// 0.0 = herbivore, 0.5 = omnivore, 1.0 = carnivore
func (c *Capabilities) DigestiveSpectrum() float32 {
	if c.DigestiveCount == 0 {
		return 0.5 // Neutral if no digestive cells
	}
	return c.DigestiveSum / float32(c.DigestiveCount)
}

// ReproductiveMode returns the average reproductive mode across reproductive cells.
// 0.0 = pure asexual (budding/cloning)
// 0.5 = mixed (opportunistic)
// 1.0 = pure sexual (requires mate)
func (c *Capabilities) ReproductiveMode() float32 {
	if c.ReproductiveCount == 0 {
		return 0.5 // Default to mixed if no reproductive cells
	}
	return c.ReproductiveSum / float32(c.ReproductiveCount)
}

// ComputeCapabilities calculates capability totals from all alive cells.
func (cb *CellBuffer) ComputeCapabilities() Capabilities {
	var caps Capabilities
	var armorSum, storageSum float32
	aliveCount := 0

	for i := uint8(0); i < cb.Count; i++ {
		cell := &cb.Cells[i]
		if !cell.Alive {
			continue
		}
		aliveCount++

		// Accumulate function strengths
		caps.PhotoWeight += cell.GetFunctionStrength(neural.CellTypePhotosynthetic)
		caps.ActuatorWeight += cell.GetFunctionStrength(neural.CellTypeActuator)
		caps.SensorWeight += cell.GetFunctionStrength(neural.CellTypeSensor)
		caps.MouthSize += cell.GetFunctionStrength(neural.CellTypeMouth)
		caps.BioluminescentCap += cell.GetFunctionStrength(neural.CellTypeBioluminescent)
		caps.ReproductiveWeight += cell.GetFunctionStrength(neural.CellTypeReproductive)

		// Digestive cells contribute to spectrum
		digestiveStr := cell.GetFunctionStrength(neural.CellTypeDigestive)
		if digestiveStr > 0 {
			caps.DigestiveSum += cell.DigestiveSpectrum * digestiveStr
			caps.DigestiveCount++
		}

		// Reproductive cells contribute to mode
		reproStr := cell.GetFunctionStrength(neural.CellTypeReproductive)
		if reproStr > 0 {
			caps.ReproductiveSum += cell.ReproductiveMode
			caps.ReproductiveCount++
		}

		// Modifiers
		armorSum += cell.StructuralArmor
		storageSum += cell.StorageCapacity
	}

	// Average modifiers
	if aliveCount > 0 {
		caps.StructuralArmor = armorSum / float32(aliveCount)
		caps.StorageCapacity = storageSum / float32(aliveCount)
	}

	return caps
}

// HasMouth returns true if any alive cell has mouth capability.
func (cb *CellBuffer) HasMouth() bool {
	for i := uint8(0); i < cb.Count; i++ {
		cell := &cb.Cells[i]
		if cell.Alive && cell.HasFunction(neural.CellTypeMouth) {
			return true
		}
	}
	return false
}

// FlowParticle represents a flow visualization particle.
type FlowParticle struct {
	Size        float32
	Opacity     float32
	Lifespan    int32
	MaxLifespan int32
}

// Trail holds position history for flow particles.
type Trail struct {
	Points [4]Position
	Count  uint8
}

// Flora tag component for efficient querying.
type Flora struct{}

// Fauna tag component for efficient querying.
type Fauna struct{}

// Dead tag component for dead organisms.
type Dead struct{}

// NeuralGenome stores the genetic blueprints for neural networks.
type NeuralGenome struct {
	BodyGenome  *genetics.Genome // CPPN for morphology (used at birth)
	BrainGenome *genetics.Genome // Controller for behavior
	SpeciesID   int              // Species assignment for speciation
	Generation  int              // Birth generation
}

// Brain stores the instantiated runtime neural network controller.
type Brain struct {
	Controller *neural.BrainController
}

// HasBrain tag component to mark organisms with neural control.
type HasBrain struct{}

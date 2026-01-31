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
// Simplified to just Drag - computed from actual frontal profile.
type ShapeMetrics struct {
	Drag float32 // 0.2 (streamlined fish) to 1.0+ (blunt plate)
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
	// Core state
	Energy           float32
	MaxEnergy        float32
	CellSize         float32
	MaxSpeed         float32
	MaxForce         float32
	PerceptionRadius float32
	Dead             bool
	Heading          float32

	// Timers
	SporeTimer       int32
	SporeInterval    int32
	BreedingCooldown int32

	// Allocation and shape
	AllocationMode AllocationMode
	DeadTime       int32 // Ticks since death (for removal)
	ShapeMetrics   ShapeMetrics // Physical shape characteristics
	ActiveThrust   float32      // Thrust magnitude this tick (for energy cost)
	OBB            CollisionOBB // Collision bounding box computed from cells

	// Body geometry (computed from cells at birth)
	BodyRadius float32 // sqrt(cellCount) * cellSize

	// Brain outputs (heading-as-state control)
	UTurn        float32 // Brain output: -1 to +1, turn rate
	UThrottle    float32 // Brain output: 0 to 1, forward throttle
	AttackIntent float32 // Brain output: 0-1, >0.5 means attack
	MateIntent   float32 // Brain output: 0-1, >0.5 means ready to mate

	// Previous outputs for jitter detection (energy cost)
	PrevUTurn     float32
	PrevUThrottle float32

	// Last brain inputs (for debugging/inspection)
	LastInputs [30]float32 // Last sensory inputs fed to brain

	// Legacy brain outputs (for compatibility with systems that haven't been updated)
	EatIntent   float32 // Derived: implicit from mouth proximity
	BreedIntent float32 // Alias for MateIntent

	// Attack state
	AttackCooldown int32  // Ticks until can attack again
	MateProgress   int32  // Ticks in mating contact
	MatePartnerID  uint32 // Entity ID of current mating partner (0 = none)

	// Damage awareness (set by feeding system, decays each tick)
	BeingEaten float32 // 0-1, how intensely this organism is being eaten
}

// Cell represents a single cell within an organism.
// Cell function and behavior are determined by PrimaryType/SecondaryType from CPPN.
// Note: Decomposition removed - feeding is pure energy transfer for simplicity.
type Cell struct {
	// Position and state
	GridX  int8
	GridY  int8
	Age    int32
	MaxAge int32
	Alive  bool

	// Function selection (from CPPN)
	PrimaryType   neural.CellType // Main function
	SecondaryType neural.CellType // Optional secondary function (CellTypeNone if none)
	PrimaryStrength   float32         // Effective primary strength
	SecondaryStrength float32         // Effective secondary strength

	// Cell specialization (from CPPN)
	DigestiveSpectrum float32 // 0=herbivore, 1=carnivore
	ReproductiveMode  float32 // 0=asexual, 0.5=mixed, 1=sexual

	// Modifiers
	StructuralArmor float32 // 0-1, damage reduction (adds drag)
	StorageCapacity float32 // 0-1, max energy bonus (adds metabolism)
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
	ActuatorWeight     float32 // Total actuator strength
	SensorWeight       float32 // Total sensor strength
	MouthSize          float32 // Total mouth strength (for feeding)
	DigestiveSum       float32 // Sum of digestive spectrum weighted by strength
	DigestiveCount     int     // Number of digestive cells
	StructuralArmor    float32 // Average structural armor
	StorageCapacity    float32 // Average storage capacity
	ReproductiveWeight float32 // Total reproductive capability
	ReproductiveSum    float32 // Sum of reproductive mode values from reproductive cells
	ReproductiveCount  int     // Number of reproductive cells
}

// Composition returns the flora/fauna composition ratio.
// 1.0 = pure sensor (flora-like), 0.0 = pure actuator (fauna-like)
func (c *Capabilities) Composition() float32 {
	const epsilon = 1e-6
	total := c.SensorWeight + c.ActuatorWeight
	if total < epsilon {
		return 0.5 // Neutral for organisms with neither
	}
	return c.SensorWeight / total
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
		caps.ActuatorWeight += cell.GetFunctionStrength(neural.CellTypeActuator)
		caps.SensorWeight += cell.GetFunctionStrength(neural.CellTypeSensor)
		caps.MouthSize += cell.GetFunctionStrength(neural.CellTypeMouth)
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

// ActuatorMetrics holds position-weighted actuator statistics for movement.
// These determine how morphology affects turning and thrust effectiveness.
type ActuatorMetrics struct {
	// ThrustBias: How much actuator strength is at the rear vs front.
	// Positive = more rear thrust (forward propulsion), Negative = more front (braking).
	// Range roughly [-1, 1] when normalized by TotalStrength.
	ThrustBias float32

	// TurnBias: Asymmetry between left and right actuators.
	// Positive = more right-side actuators (better at turning left).
	// Negative = more left-side actuators (better at turning right).
	// Range roughly [-1, 1] when normalized.
	TurnBias float32

	// TotalStrength: Sum of all actuator strengths (for normalization).
	TotalStrength float32
}

// ComputeActuatorMetrics calculates position-weighted actuator statistics.
// Used to determine how body shape affects turning and thrust.
func (cb *CellBuffer) ComputeActuatorMetrics() ActuatorMetrics {
	var metrics ActuatorMetrics

	for i := uint8(0); i < cb.Count; i++ {
		cell := &cb.Cells[i]
		if !cell.Alive {
			continue
		}

		strength := cell.GetFunctionStrength(neural.CellTypeActuator)
		if strength <= 0 {
			continue
		}

		metrics.TotalStrength += strength

		// GridX: positive = front, negative = rear
		// Rear actuators provide forward thrust
		metrics.ThrustBias -= float32(cell.GridX) * strength

		// GridY: positive = left, negative = right
		// Right-side actuators (GridY < 0) help turn left (positive turn)
		// Left-side actuators (GridY > 0) help turn right (negative turn)
		metrics.TurnBias -= float32(cell.GridY) * strength
	}

	return metrics
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

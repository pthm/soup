// Package components defines ECS components for the simulation.
package components

import (
	"github.com/pthm-cable/soup/neural"
	"github.com/pthm-cable/soup/traits"
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

// Organism holds organism-specific data.
type Organism struct {
	Traits            traits.Trait
	Energy            float32
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
	EatIntent         float32      // Brain output: 0-1, >0.5 means try to eat
	MateIntent        float32      // Brain output: 0-1, >0.5 means try to mate
	TurnOutput        float32      // Brain output: -1 to +1, current turn output
	ThrustOutput      float32      // Brain output: 0 to 1, current thrust output
}

// Cell represents a single cell within an organism.
type Cell struct {
	GridX         int8
	GridY         int8
	Age           int32
	MaxAge        int32
	Trait         traits.Trait
	Mutation      traits.Mutation
	Alive         bool
	Decomposition float32
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

// Spore represents a spore entity.
type Spore struct {
	ParentTraits traits.Trait
	Size         float32
	Landed       bool
	Rooted       bool
	Lifespan     int32
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

// AddPoint adds a point to the trail (most recent first).
func (t *Trail) AddPoint(p Position) {
	// Shift existing points
	for i := len(t.Points) - 1; i > 0; i-- {
		t.Points[i] = t.Points[i-1]
	}
	t.Points[0] = p
	if t.Count < uint8(len(t.Points)) {
		t.Count++
	}
}

// Clear clears the trail.
func (t *Trail) Clear() {
	t.Count = 0
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

// Package components defines ECS components for the simulation.
package components

import (
	"github.com/pthm-cable/soup/traits"
)

// Position represents an entity's world position.
type Position struct {
	X, Y float32
}

// Velocity represents an entity's velocity.
type Velocity struct {
	X, Y float32
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
	Cells [32]Cell
	Count uint8
}

// AddCell adds a cell to the buffer.
func (cb *CellBuffer) AddCell(c Cell) bool {
	if cb.Count >= 32 {
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

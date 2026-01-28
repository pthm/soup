package systems

import (
	"math"

	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/traits"
)

// SplittingSystem handles organism splitting mutation.
type SplittingSystem struct{}

// NewSplittingSystem creates a new splitting system.
func NewSplittingSystem() *SplittingSystem {
	return &SplittingSystem{}
}

// TrySplit attempts to split an organism if it has the Splitting mutation and enough cells.
// Returns true if a split occurred, along with the position for the new organism.
func (s *SplittingSystem) TrySplit(
	pos *components.Position,
	vel *components.Velocity,
	org *components.Organism,
	cells *components.CellBuffer,
	createOrganism func(x, y float32, t traits.Trait, energy float32) ecs.Entity,
	particles *ParticleSystem,
) bool {
	// Need at least 4 cells to split (2 each after split)
	if cells.Count < 4 {
		return false
	}

	// Check if any cell has Splitting mutation
	hasSplitting := false
	for i := uint8(0); i < cells.Count; i++ {
		if cells.Cells[i].Mutation == traits.Splitting {
			hasSplitting = true
			break
		}
	}

	if !hasSplitting {
		return false
	}

	// Calculate split point (divide cells at midpoint)
	splitPoint := cells.Count / 2

	// Calculate centroids for each half
	var centroid1X, centroid1Y float32
	var centroid2X, centroid2Y float32
	count1 := float32(0)
	count2 := float32(0)

	for i := uint8(0); i < cells.Count; i++ {
		cell := &cells.Cells[i]
		cellX := pos.X + float32(cell.GridX)*org.CellSize
		cellY := pos.Y + float32(cell.GridY)*org.CellSize

		if i < splitPoint {
			centroid1X += cellX
			centroid1Y += cellY
			count1++
		} else {
			centroid2X += cellX
			centroid2Y += cellY
			count2++
		}
	}

	if count1 > 0 {
		centroid1X /= count1
		centroid1Y /= count1
	}
	if count2 > 0 {
		centroid2X /= count2
		centroid2Y /= count2
	}

	// Direction between centroids
	dx := centroid2X - centroid1X
	dy := centroid2Y - centroid1Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist < 0.001 {
		dx = 1
		dy = 0
		dist = 1
	}
	dx /= dist
	dy /= dist

	// New organism position: offset from centroid2
	newX := centroid2X + dx*org.CellSize*3
	newY := centroid2Y + dy*org.CellSize*3

	// Split energy 50/50
	newEnergy := org.Energy / 2
	org.Energy = org.Energy / 2

	// Create new organism with inherited traits
	newEntity := createOrganism(newX, newY, org.Traits, newEnergy)

	// Transfer second half of cells to new organism
	// This is a simplified version - we just initialize the new organism
	// with cells starting from the split point
	// The actual cell transfer would require accessing the new entity's CellBuffer
	_ = newEntity // New organism is created with a single cell, will grow over time

	// Remove second half of cells from original
	cells.Count = splitPoint

	// Apply opposite momentum push
	pushForce := float32(0.5)
	vel.X -= dx * pushForce
	vel.Y -= dy * pushForce

	// Emit split particles
	if particles != nil {
		particles.EmitSplit(centroid1X+(centroid2X-centroid1X)/2, centroid1Y+(centroid2Y-centroid1Y)/2)
	}

	return true
}

package game

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/mlange-42/ark/ecs"

	"github.com/pthm-cable/soup/components"
)

// HoveredOrganism holds data about the organism under the cursor.
type HoveredOrganism struct {
	Pos   *components.Position
	Org   *components.Organism
	Cells *components.CellBuffer
}

// findOrganismAtMouse returns the organism under the mouse cursor, if any.
func (g *Game) findOrganismAtMouse() *HoveredOrganism {
	mousePos := rl.GetMousePosition()
	mouseX, mouseY := mousePos.X, mousePos.Y

	var closest *HoveredOrganism
	closestDist := float32(20.0) // Max hover distance

	query := g.allOrgFilter.Query()
	for query.Next() {
		pos, _, org, cells := query.Get()

		// Calculate organism bounds
		minX, minY := pos.X, pos.Y
		maxX, maxY := pos.X, pos.Y

		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			cellX := pos.X + float32(cell.GridX)*org.CellSize
			cellY := pos.Y + float32(cell.GridY)*org.CellSize
			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}

		// Expand bounds by cell size
		minX -= org.CellSize
		minY -= org.CellSize
		maxX += org.CellSize
		maxY += org.CellSize

		// Check if mouse is within bounds
		if mouseX >= minX && mouseX <= maxX && mouseY >= minY && mouseY <= maxY {
			// Calculate distance to center
			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2
			dist := float32(math.Sqrt(float64((mouseX-centerX)*(mouseX-centerX) + (mouseY-centerY)*(mouseY-centerY))))

			if dist < closestDist {
				closestDist = dist
				closest = &HoveredOrganism{Pos: pos, Org: org, Cells: cells}
			}
		}
	}

	return closest
}

// findOrganismAtClick returns the entity under the mouse cursor, if any.
func (g *Game) findOrganismAtClick() (ecs.Entity, bool) {
	mousePos := rl.GetMousePosition()
	mouseX, mouseY := mousePos.X, mousePos.Y

	var closestEntity ecs.Entity
	closestDist := float32(20.0) // Max click distance
	found := false

	query := g.allOrgFilter.Query()
	for query.Next() {
		entity := query.Entity()
		pos, _, org, cells := query.Get()

		// Calculate organism bounds
		minX, minY := pos.X, pos.Y
		maxX, maxY := pos.X, pos.Y

		for i := uint8(0); i < cells.Count; i++ {
			cell := &cells.Cells[i]
			if !cell.Alive {
				continue
			}
			cellX := pos.X + float32(cell.GridX)*org.CellSize
			cellY := pos.Y + float32(cell.GridY)*org.CellSize
			if cellX < minX {
				minX = cellX
			}
			if cellX > maxX {
				maxX = cellX
			}
			if cellY < minY {
				minY = cellY
			}
			if cellY > maxY {
				maxY = cellY
			}
		}

		// Expand bounds by cell size
		minX -= org.CellSize
		minY -= org.CellSize
		maxX += org.CellSize
		maxY += org.CellSize

		// Check if mouse is within bounds
		if mouseX >= minX && mouseX <= maxX && mouseY >= minY && mouseY <= maxY {
			// Calculate distance to center
			centerX := (minX + maxX) / 2
			centerY := (minY + maxY) / 2
			dist := float32(math.Sqrt(float64((mouseX-centerX)*(mouseX-centerX) + (mouseY-centerY)*(mouseY-centerY))))

			if dist < closestDist {
				closestDist = dist
				closestEntity = entity
				found = true
			}
		}
	}

	return closestEntity, found
}

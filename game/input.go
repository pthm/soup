package game

import rl "github.com/gen2brain/raylib-go/raylib"

// handleInput processes keyboard input.
func (g *Game) handleInput() {
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}

	// Steps-per-update control with < > keys (comma and period)
	if rl.IsKeyPressed(rl.KeyComma) && g.stepsPerUpdate > 1 {
		g.stepsPerUpdate--
	}
	if rl.IsKeyPressed(rl.KeyPeriod) && g.stepsPerUpdate < 10 {
		g.stepsPerUpdate++
	}

	// Debug mode toggle
	if rl.IsKeyPressed(rl.KeyD) {
		g.debugMode = !g.debugMode
		if g.debugMode {
			g.debugShowResource = true // Default to showing resource overlay
		}
	}

	// Debug sub-options (only when debug mode is active)
	if g.debugMode {
		if rl.IsKeyPressed(rl.KeyR) {
			g.debugShowResource = !g.debugShowResource
		}
	}

	// Inspector input
	if g.inspector != nil {
		mousePos := rl.GetMousePosition()
		g.inspector.HandleInput(mousePos.X, mousePos.Y, g.posMap, g.bodyMap, g.orgMap, g.entityFilter)
	}
}

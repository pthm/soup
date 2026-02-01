package game

import rl "github.com/gen2brain/raylib-go/raylib"

// handleInput processes keyboard input.
func (g *Game) handleInput() {
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}

	// Speed control with < > keys (comma and period)
	if rl.IsKeyPressed(rl.KeyComma) && g.speed > 1 {
		g.speed--
	}
	if rl.IsKeyPressed(rl.KeyPeriod) && g.speed < 10 {
		g.speed++
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

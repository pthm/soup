package game

import rl "github.com/gen2brain/raylib-go/raylib"

// handleInput processes keyboard input.
func (g *Game) handleInput() {
	// Window resize propagation
	g.handleResize()

	// Fullscreen toggle
	if rl.IsKeyPressed(rl.KeyF11) {
		rl.ToggleFullscreen()
	}

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
		if rl.IsKeyPressed(rl.KeyP) {
			g.debugShowPotential = !g.debugShowPotential
		}
		if rl.IsKeyPressed(rl.KeyF) {
			g.debugShowFlow = !g.debugShowFlow
		}
	}

	// Camera controls
	g.handleCameraInput()

	// Inspector input
	if g.inspector != nil {
		mousePos := rl.GetMousePosition()
		g.inspector.HandleInput(mousePos.X, mousePos.Y, g.posMap, g.bodyMap, g.orgMap, g.entityFilter, g.camera)
	}
}

// handleResize checks for window resize and propagates new dimensions.
func (g *Game) handleResize() {
	if !rl.IsWindowResized() {
		return
	}
	w := float32(rl.GetScreenWidth())
	h := float32(rl.GetScreenHeight())
	if w == g.screenWidth && h == g.screenHeight {
		return
	}
	g.screenWidth = w
	g.screenHeight = h

	if g.camera != nil {
		g.camera.Resize(w, h)
	}
	if g.backgroundRenderer != nil {
		g.backgroundRenderer.Resize(w, h)
	}
	if g.lightRenderer != nil {
		g.lightRenderer.Resize(w, h)
	}
	if g.resourceFogRenderer != nil {
		g.resourceFogRenderer.Resize(w, h)
	}
	if g.particleRenderer != nil {
		g.particleRenderer.Resize(int32(w), int32(h))
	}
	if g.inspector != nil {
		g.inspector.Resize(int32(w), int32(h))
	}
}

// handleCameraInput processes camera pan/zoom controls.
func (g *Game) handleCameraInput() {
	if g.camera == nil {
		return
	}

	// Pan speed scales inversely with zoom for natural feel
	panSpeed := float32(8.0) / g.camera.Zoom

	// Arrow key panning
	if rl.IsKeyDown(rl.KeyRight) {
		g.camera.Pan(panSpeed, 0)
	}
	if rl.IsKeyDown(rl.KeyLeft) {
		g.camera.Pan(-panSpeed, 0)
	}
	if rl.IsKeyDown(rl.KeyDown) {
		g.camera.Pan(0, panSpeed)
	}
	if rl.IsKeyDown(rl.KeyUp) {
		g.camera.Pan(0, -panSpeed)
	}

	// Zoom controls: mouse wheel or +/- keys
	wheelMove := rl.GetMouseWheelMove()
	if wheelMove != 0 {
		// Zoom toward/away from cursor position
		zoomFactor := float32(1.0) + wheelMove*0.1
		g.camera.ZoomBy(zoomFactor)
	}

	// Keyboard zoom with +/- (= and - keys)
	if rl.IsKeyPressed(rl.KeyEqual) || rl.IsKeyPressed(rl.KeyKpAdd) {
		g.camera.ZoomBy(1.25)
	}
	if rl.IsKeyPressed(rl.KeyMinus) || rl.IsKeyPressed(rl.KeyKpSubtract) {
		g.camera.ZoomBy(0.8)
	}

	// Home key to reset camera
	if rl.IsKeyPressed(rl.KeyHome) {
		g.camera.Reset()
	}
}

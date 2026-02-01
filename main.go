package main

import (
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/game"
)

func main() {
	rl.InitWindow(int32(game.ScreenWidth), int32(game.ScreenHeight), "Primordial Soup")
	defer rl.CloseWindow()

	rl.SetTargetFPS(60)

	g := game.NewGame()
	defer g.Unload()

	for !rl.WindowShouldClose() {
		g.Update()
		g.Draw()
	}
}

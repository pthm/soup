package game

// Screen dimensions
const (
	ScreenWidth  = 1280
	ScreenHeight = 800
)

// Note: Fauna growth has been removed. Organisms are born with their cells from CPPN.
// Evolution happens through breeding/reproduction, not individual organism growth.

// GameConfig holds configuration for game initialization.
type GameConfig struct {
	Headless bool
	Width    int
	Height   int
}

// DefaultConfig returns the default game configuration.
func DefaultConfig() GameConfig {
	return GameConfig{
		Headless: false,
		Width:    ScreenWidth,
		Height:   ScreenHeight,
	}
}

package game

// Screen dimensions
const (
	ScreenWidth  = 1600
	ScreenHeight = 1000
)

// Note: Fauna growth has been removed. Organisms are born with their cells from CPPN.
// Evolution happens through breeding/reproduction, not individual organism growth.

// GameConfig holds configuration for game initialization.
type GameConfig struct {
	Headless          bool
	Width             int
	Height            int
	PersistentEcology bool // When true, disable explicit fitness tracking (selection via energy economics)
}

// DefaultConfig returns the default game configuration.
func DefaultConfig() GameConfig {
	return GameConfig{
		Headless:          false,
		Width:             ScreenWidth,
		Height:            ScreenHeight,
		PersistentEcology: true, // Default: ecology mode (no explicit fitness)
	}
}

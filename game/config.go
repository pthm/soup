package game

// Screen dimensions
const (
	ScreenWidth  = 1280
	ScreenHeight = 800
)

// GrowIntent constants for neural-controlled growth
const (
	GrowIntentThreshold = 0.4  // Minimum intent to trigger growth
	MinGrowthInterval   = 60   // Fastest growth (high intent)
	MaxGrowthInterval   = 300  // Slowest growth (low intent)
)

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

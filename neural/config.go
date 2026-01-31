package neural

import (
	"github.com/yaricom/goNEAT/v4/neat"
)

// BrainInputs is the number of sensory inputs to the brain network.
// Layout: self (2) + body (6) + boid (9) + food (6) + threat (2) + approach (4) + bias (1) = 30 total
const BrainInputs = 30

// BrainOutputs is the number of outputs from the brain network.
// Layout: UTurn, UThrottle, AttackIntent, MateIntent
const BrainOutputs = 4

// Movement control parameters
const (
	TurnRateMax = 0.15 // Maximum turn rate in radians per tick (~8.6 degrees)
)

// Body descriptor normalization constants
const (
	MaxBodySize     = 6.0 // sqrt(16 cells) * typical cellSize
	MaxSensorWeight = 4.0 // ~4 strong sensor cells
	MaxMouthSize    = 2.0 // ~2 strong mouth cells
)

// Boid field parameters
const (
	ExpectedNeighbors  = 10.0 // Expected number of neighbors for density normalization
	SeparationRadius   = 3.0  // Separation radius in body lengths
	BaseCellSize       = 6.0  // Reference cell size for normalization
)

// Attack parameters
const (
	BaseAttackRange  = 12.0 // Base attack range in world units
	BaseAttackCost   = 5.0  // Base energy cost for attacking
	BaseAttackDamage = 0.08 // Base damage fraction per attack
	AttackCooldown   = 30   // Ticks between attacks
)

// Mating parameters
const (
	MateContactMargin = 5.0  // Extra margin for mating proximity
	MateDwellTime     = 60   // Ticks required in contact to mate
	MateEnergyRatio   = 0.35 // Minimum energy ratio for mating
)

// CPPNInputs is the number of inputs to the CPPN.
// Inputs: x, y, distance, angle, bias
const CPPNInputs = 5

// InitialMaxCells constrains generation-0 organisms to small sizes.
const InitialMaxCells = 4

// CPPNOutputs is the number of outputs from the CPPN.
// Outputs: presence, sensor, actuator, mouth, digestive_spectrum,
// structural_armor, storage_capacity, reproductive, brain_weight, brain_leo
const CPPNOutputs = 10

// CPPN output indices for clarity
const (
	CPPNOutPresence        = 0 // Cell presence threshold
	CPPNOutSensor          = 1 // Sensor function strength
	CPPNOutActuator        = 2 // Actuator function strength
	CPPNOutMouth           = 3 // Mouth function strength
	CPPNOutDigestive       = 4 // Digestive spectrum (0=herbivore, 1=carnivore)
	CPPNOutStructuralArmor = 5 // Structural armor modifier
	CPPNOutStorageCapacity = 6 // Storage capacity modifier
	CPPNOutReproductive    = 7 // Reproductive function strength
	CPPNOutBrainWeight     = 8 // HyperNEAT: connection weight
	CPPNOutBrainLEO        = 9 // HyperNEAT: link expression output
)

// Number of functional cell type outputs (for argmax selection)
const CPPNFunctionalOutputs = 5 // sensor, actuator, mouth, digestive, reproductive

// Cell function selection constants
const (
	SecondaryThreshold    = 0.25 // Minimum value for secondary function
	MixedPrimaryPenalty   = 0.85 // Primary strength multiplier when secondary exists
	MixedSecondaryScale   = 0.35 // Secondary strength multiplier
	StructuralDragCost    = 0.1  // Drag increase per unit structural armor
	StorageMetabolicCost  = 0.05 // Metabolic increase per unit storage capacity
)

// CellType represents the functional type of a cell.
type CellType uint8

const (
	CellTypeNone        CellType = iota // No function (sentinel for no secondary)
	CellTypeSensor                      // Sensing cell, contributes to perception
	CellTypeActuator                    // Motor cell, contributes to thrust
	CellTypeMouth                       // Mouth cell, for feeding
	CellTypeDigestive                   // Digestive cell, determines diet efficiency
	CellTypeReproductive                // Reproductive cell, for breeding
)

// CellTypeName returns a human-readable name for the cell type.
func (ct CellType) String() string {
	switch ct {
	case CellTypeNone:
		return "None"
	case CellTypeSensor:
		return "Sensor"
	case CellTypeActuator:
		return "Actuator"
	case CellTypeMouth:
		return "Mouth"
	case CellTypeDigestive:
		return "Digestive"
	case CellTypeReproductive:
		return "Reproductive"
	default:
		return "Unknown"
	}
}

// IsSensor returns true if this cell type provides sensing capability.
func (ct CellType) IsSensor() bool {
	return ct == CellTypeSensor
}

// IsActuator returns true if this cell type provides movement capability.
func (ct CellType) IsActuator() bool {
	return ct == CellTypeActuator
}

// Color returns RGB values for this cell type for visualization.
func (ct CellType) Color() (r, g, b uint8) {
	switch ct {
	case CellTypeSensor:
		return 100, 180, 255 // Light blue - sensing
	case CellTypeActuator:
		return 255, 150, 100 // Orange - motor
	case CellTypeMouth:
		return 255, 100, 100 // Red - feeding
	case CellTypeDigestive:
		return 200, 150, 100 // Tan - digestion
	case CellTypeReproductive:
		return 255, 150, 200 // Pink - reproduction
	default:
		return 150, 150, 150 // Gray - unknown/none
	}
}

// GetCapabilityColor returns RGB color based on cell DigestiveSpectrum.
// This is used when diet is derived from cell capabilities.
func GetCapabilityColor(digestiveSpectrum float32) (r, g, b uint8) {
	if digestiveSpectrum < 0.35 {
		return 80, 150, 200 // Blue - herbivore
	} else if digestiveSpectrum > 0.65 {
		return 200, 80, 80 // Red - carnivore
	}
	return 180, 100, 180 // Purple - omnivore
}

// GetDietName returns a human-readable diet name based on DigestiveSpectrum.
func GetDietName(digestiveSpectrum float32) string {
	if digestiveSpectrum < 0.35 {
		return "HERBIVORE"
	}
	if digestiveSpectrum > 0.65 {
		return "CARNIVORE"
	}
	return "OMNIVORE"
}

// Config holds all neural network configuration.
type Config struct {
	NEAT  *neat.Options
	CPPN  CPPNConfig
	Brain BrainConfig
}

// CPPNConfig holds CPPN-specific settings.
type CPPNConfig struct {
	GridSize        int     `yaml:"grid_size"`
	MaxCells        int     `yaml:"max_cells"`
	MinCells        int     `yaml:"min_cells"`
	CellThreshold   float64 `yaml:"cell_threshold"`
	EnforceSymmetry bool    `yaml:"enforce_symmetry"`
}

// BrainConfig holds brain network settings.
type BrainConfig struct {
	Inputs                int     `yaml:"inputs"`
	Outputs               int     `yaml:"outputs"`
	InitialConnectionProb float64 `yaml:"initial_connection_prob"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		NEAT: DefaultNEATOptions(),
		CPPN: CPPNConfig{
			GridSize:        10,
			MaxCells:        24,
			MinCells:        1,
			CellThreshold:   0.3,
			EnforceSymmetry: false,
		},
		Brain: BrainConfig{
			Inputs:                BrainInputs,
			Outputs:               BrainOutputs,
			InitialConnectionProb: 0.3,
		},
	}
}

// DefaultNEATOptions returns NEAT options tuned for the soup simulation.
func DefaultNEATOptions() *neat.Options {
	return &neat.Options{
		// Trait mutation
		TraitParamMutProb:  0.5,
		TraitMutationPower: 1.0,

		// Weight mutation
		WeightMutPower: 2.5,

		// Structural mutation rates
		MutateAddNodeProb:      0.10,
		MutateAddLinkProb:      0.15,
		MutateToggleEnableProb: 0.01,

		// Weight mutation probability
		MutateLinkWeightsProb: 0.8,
		MutateOnlyProb:        0.25,
		MutateRandomTraitProb: 0.1,

		// Mating probabilities
		MateMultipointProb:    0.6,
		MateMultipointAvgProb: 0.4,
		MateSinglepointProb:   0.0,
		MateOnlyProb:          0.2,
		RecurOnlyProb:         0.0,

		// Speciation - Lower threshold to capture initial population diversity
		// Initial organisms have varied CPPN weights, need sensitive threshold
		CompatThreshold: 1.2,
		DisjointCoeff:   1.0,
		ExcessCoeff:     1.0,
		MutdiffCoeff:    0.4, // Reduce weight difference sensitivity

		// Species management
		DropOffAge:      25,  // Give species more time to improve (was 15)
		SurvivalThresh:  0.3, // Top 30% survive to reproduce (was 20%)
		AgeSignificance: 1.0,

		// Population (used as reference, actual pop managed by simulation)
		PopSize: 150,
	}
}


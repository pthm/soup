package neural

import (
	"os"

	"github.com/yaricom/goNEAT/v4/neat"
	"gopkg.in/yaml.v3"
)

// BrainInputs is the number of sensory inputs to the brain network.
// Phase 4b: 12 cone inputs + 5 environment + 2 light gradients = 19 total
const BrainInputs = 19

// BrainOutputs is the number of outputs from the brain network.
// Phase 5: DesireAngle, DesireDistance, Eat, Grow, Breed, Glow (intent-based)
const BrainOutputs = 6

// CPPNInputs is the number of inputs to the CPPN.
// Inputs: x, y, distance, angle, bias
const CPPNInputs = 5

// InitialMaxCells constrains generation-0 organisms to small sizes.
const InitialMaxCells = 4

// CPPNOutputs is the number of outputs from the CPPN.
// Outputs: presence, sensor, actuator, mouth, digestive_spectrum,
// photosynthetic, bioluminescent, structural_armor, storage_capacity,
// reproductive, brain_weight, brain_leo
const CPPNOutputs = 12

// CPPN output indices for clarity
const (
	CPPNOutPresence        = 0  // Cell presence threshold
	CPPNOutSensor          = 1  // Sensor function strength
	CPPNOutActuator        = 2  // Actuator function strength
	CPPNOutMouth           = 3  // Mouth function strength
	CPPNOutDigestive       = 4  // Digestive spectrum (0=herbivore, 1=carnivore)
	CPPNOutPhotosynthetic  = 5  // Photosynthetic function strength
	CPPNOutBioluminescent  = 6  // Bioluminescent function strength
	CPPNOutStructuralArmor = 7  // Structural armor modifier
	CPPNOutStorageCapacity = 8  // Storage capacity modifier
	CPPNOutReproductive    = 9  // Reproductive function strength
	CPPNOutBrainWeight     = 10 // HyperNEAT: connection weight
	CPPNOutBrainLEO        = 11 // HyperNEAT: link expression output
)

// Number of functional cell type outputs (for argmax selection)
const CPPNFunctionalOutputs = 7 // sensor, actuator, mouth, digestive, photosynthetic, bioluminescent, reproductive

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
	CellTypeNone           CellType = iota // No function (sentinel for no secondary)
	CellTypeSensor                         // Sensing cell, contributes to perception
	CellTypeActuator                       // Motor cell, contributes to thrust
	CellTypeMouth                          // Mouth cell, for feeding
	CellTypeDigestive                      // Digestive cell, determines diet efficiency
	CellTypePhotosynthetic                 // Photosynthetic cell, produces energy from light
	CellTypeBioluminescent                 // Bioluminescent cell, emits light
	CellTypeReproductive                   // Reproductive cell, for breeding
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
	case CellTypePhotosynthetic:
		return "Photosynthetic"
	case CellTypeBioluminescent:
		return "Bioluminescent"
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
			GridSize:        8,
			MaxCells:        16,
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

		// Speciation - Lower threshold for more species diversity
		CompatThreshold: 1.0,
		DisjointCoeff:   1.0,
		ExcessCoeff:     1.0,
		MutdiffCoeff:    0.6,

		// Species management
		DropOffAge:      15,
		SurvivalThresh:  0.2,
		AgeSignificance: 1.0,

		// Population (used as reference, actual pop managed by simulation)
		PopSize: 150,
	}
}

// LoadConfig loads configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Start with defaults
	cfg := DefaultConfig()

	// Override with file values
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

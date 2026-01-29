package neural

import (
	"os"

	"github.com/yaricom/goNEAT/v4/neat"
	"gopkg.in/yaml.v3"
)

// BrainInputs is the number of sensory inputs to the brain network.
const BrainInputs = 14

// BrainOutputs is the number of outputs from the brain network.
// Outputs: Turn, Thrust, Eat, Mate (direct control)
const BrainOutputs = 4

// CPPNInputs is the number of inputs to the CPPN.
// Inputs: x, y, d, a, sin(d*Pi), cos(d*Pi), sin(a*2), bias
const CPPNInputs = 8

// InitialMaxCells constrains generation-0 organisms to small sizes.
const InitialMaxCells = 4

// CPPNOutputs is the number of outputs from the CPPN.
// Outputs: presence, cell_type, sensor_gain, actuator_strength, diet_bias, priority, brain_weight, brain_leo
const CPPNOutputs = 8

// CPPN output indices for clarity
const (
	CPPNOutPresence     = 0 // Cell presence threshold
	CPPNOutCellType     = 1 // Sensor/Actuator/Passive
	CPPNOutSensorGain   = 2 // Sensor sensitivity
	CPPNOutActuatorStr  = 3 // Actuator strength
	CPPNOutDietBias     = 4 // Herbivore/Carnivore bias
	CPPNOutPriority     = 5 // Cell selection priority
	CPPNOutBrainWeight  = 6 // HyperNEAT: connection weight
	CPPNOutBrainLEO     = 7 // HyperNEAT: link expression output
)

// CellType represents the functional type of a cell.
type CellType uint8

const (
	CellTypePassive  CellType = iota // Structural cell, no special function
	CellTypeSensor                   // Sensing cell, contributes to perception
	CellTypeActuator                 // Motor cell, contributes to thrust
)

// CellTypeFromOutput converts a CPPN output value to a cell type.
// output in [-1, 1] range from tanh activation.
func CellTypeFromOutput(output float64) CellType {
	if output < -0.3 {
		return CellTypeSensor
	} else if output > 0.3 {
		return CellTypeActuator
	}
	return CellTypePassive
}

// CellTypeName returns a human-readable name for the cell type.
func (ct CellType) String() string {
	switch ct {
	case CellTypeSensor:
		return "Sensor"
	case CellTypeActuator:
		return "Actuator"
	default:
		return "Passive"
	}
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

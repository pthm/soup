package neural

import (
	"os"

	"github.com/yaricom/goNEAT/v4/neat"
	"gopkg.in/yaml.v3"
)

// BrainInputs is the number of sensory inputs to the brain network.
const BrainInputs = 14

// BrainOutputs is the number of outputs from the brain network.
const BrainOutputs = 8

// CPPNInputs is the number of inputs to the CPPN (x, y, d, a, bias).
const CPPNInputs = 5

// CPPNOutputs is the number of outputs from the CPPN (presence, diet, traits, priority).
const CPPNOutputs = 4

// Config holds all neural network configuration.
type Config struct {
	NEAT  *neat.Options
	CPPN  CPPNConfig
	Brain BrainConfig
}

// CPPNConfig holds CPPN-specific settings.
type CPPNConfig struct {
	GridSize      int     `yaml:"grid_size"`
	MaxCells      int     `yaml:"max_cells"`
	CellThreshold float64 `yaml:"cell_threshold"`
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
			GridSize:      8,
			MaxCells:      32,
			CellThreshold: 0.0,
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

		// Speciation
		CompatThreshold: 2.3,
		DisjointCoeff:   1.0,
		ExcessCoeff:     1.0,
		MutdiffCoeff:    0.4,

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

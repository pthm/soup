// Package config provides configuration loading and access for the simulation.
package config

import (
	_ "embed"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

//go:embed defaults.yaml
var defaultsYAML []byte

// Config holds all simulation configuration parameters.
type Config struct {
	Screen       ScreenConfig       `yaml:"screen"`
	Physics      PhysicsConfig      `yaml:"physics"`
	Entity       EntityConfig       `yaml:"entity"`
	Capabilities CapabilitiesConfig `yaml:"capabilities"`
	Population   PopulationConfig   `yaml:"population"`
	Reproduction ReproductionConfig `yaml:"reproduction"`
	Mutation     MutationConfig     `yaml:"mutation"`
	Energy       EnergyConfig       `yaml:"energy"`
	Neural       NeuralConfig       `yaml:"neural"`
	Sensors      SensorsConfig      `yaml:"sensors"`
	GPU          GPUConfig          `yaml:"gpu"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	Bookmarks    BookmarksConfig    `yaml:"bookmarks"`
	Refugia      RefugiaConfig      `yaml:"refugia"`

	// Derived values computed after loading
	Derived DerivedConfig `yaml:"-"`
}

// ScreenConfig holds display settings.
type ScreenConfig struct {
	Width     int `yaml:"width"`
	Height    int `yaml:"height"`
	TargetFPS int `yaml:"target_fps"`
}

// PhysicsConfig holds simulation physics parameters.
type PhysicsConfig struct {
	DT           float64 `yaml:"dt"`
	GridCellSize float64 `yaml:"grid_cell_size"`
}

// EntityConfig holds entity creation parameters.
type EntityConfig struct {
	BodyRadius    float64 `yaml:"body_radius"`
	InitialEnergy float64 `yaml:"initial_energy"`
}

// VisionZone defines a zone of enhanced vision effectiveness.
// Effectiveness is calculated as: power * smooth_falloff(angle_distance / width)
type VisionZone struct {
	Angle float64 `yaml:"angle"` // center angle in radians (0 = front, ±π/2 = sides)
	Width float64 `yaml:"width"` // half-width in radians
	Power float64 `yaml:"power"` // effectiveness multiplier (0-1)
}

// PreyCapabilitiesConfig holds prey-specific capability parameters.
type PreyCapabilitiesConfig struct {
	VisionRange float64      `yaml:"vision_range"`
	VisionZones []VisionZone `yaml:"vision_zones"`
}

// PredatorCapabilitiesConfig holds predator-specific capability parameters.
type PredatorCapabilitiesConfig struct {
	VisionRange float64      `yaml:"vision_range"`
	VisionZones []VisionZone `yaml:"vision_zones"`
}

// CapabilitiesConfig holds entity capability defaults.
// All entities have 360° vision; effectiveness varies by angle and kind.
type CapabilitiesConfig struct {
	Prey             PreyCapabilitiesConfig     `yaml:"prey"`
	Predator         PredatorCapabilitiesConfig `yaml:"predator"`
	MinEffectiveness float64                    `yaml:"min_effectiveness"`
	MaxSpeed         float64                    `yaml:"max_speed"`
	MaxAccel         float64                    `yaml:"max_accel"`
	MaxTurnRate      float64                    `yaml:"max_turn_rate"`
	Drag             float64                    `yaml:"drag"`
	BiteRange        float64                    `yaml:"bite_range"`
	BiteCost         float64                    `yaml:"bite_cost"`
	ThrustDeadzone   float64                    `yaml:"thrust_deadzone"` // Thrust below this = 0
}

// PopulationConfig holds population management parameters.
type PopulationConfig struct {
	Initial            int     `yaml:"initial"`
	MaxPrey            int     `yaml:"max_prey"`
	MaxPred            int     `yaml:"max_pred"`
	RespawnThreshold   int     `yaml:"respawn_threshold"`
	RespawnCount       int     `yaml:"respawn_count"`
	PredatorSpawnChance float64 `yaml:"predator_spawn_chance"`
}

// ReproductionConfig holds reproduction parameters.
type ReproductionConfig struct {
	PreyThreshold    float64 `yaml:"prey_threshold"`
	PredThreshold    float64 `yaml:"pred_threshold"`
	MaturityAge      float64 `yaml:"maturity_age"`
	PreyCooldown     float64 `yaml:"prey_cooldown"`
	PredCooldown     float64 `yaml:"pred_cooldown"`
	CooldownJitter   float64 `yaml:"cooldown_jitter"`
	ParentEnergySplit float64 `yaml:"parent_energy_split"`
	ChildEnergy      float64 `yaml:"child_energy"`
	SpawnOffset      float64 `yaml:"spawn_offset"`
	HeadingJitter    float64 `yaml:"heading_jitter"`
}

// MutationConfig holds mutation parameters.
type MutationConfig struct {
	Rate              float64 `yaml:"rate"`
	Sigma             float64 `yaml:"sigma"`
	BigRate           float64 `yaml:"big_rate"`
	BigSigma          float64 `yaml:"big_sigma"`
	BiasRateMultiplier float64 `yaml:"bias_rate_multiplier"`
}

// EnergyConfig holds energy economics parameters.
type EnergyConfig struct {
	Prey     PreyEnergyConfig     `yaml:"prey"`
	Predator PredatorEnergyConfig `yaml:"predator"`
}

// PreyEnergyConfig holds prey energy parameters.
type PreyEnergyConfig struct {
	BaseCost    float64 `yaml:"base_cost"`
	MoveCost    float64 `yaml:"move_cost"`
	ForageRate  float64 `yaml:"forage_rate"`
	GrazingPeak float64 `yaml:"grazing_peak"` // Speed ratio for optimal foraging
	AccelCost   float64 `yaml:"accel_cost"`   // Energy penalty for thrust
}

// PredatorEnergyConfig holds predator energy parameters.
type PredatorEnergyConfig struct {
	BaseCost           float64 `yaml:"base_cost"`
	MoveCost           float64 `yaml:"move_cost"`
	BiteCost           float64 `yaml:"bite_cost"`
	BiteReward         float64 `yaml:"bite_reward"`
	TransferEfficiency float64 `yaml:"transfer_efficiency"`
	DigestTime         float64 `yaml:"digest_time"`
	AccelCost          float64 `yaml:"accel_cost"` // Energy penalty for thrust
}

// RefugiaConfig holds refugia mechanics parameters.
type RefugiaConfig struct {
	Strength float64 `yaml:"strength"`
}

// NeuralConfig holds neural network parameters.
type NeuralConfig struct {
	NumHidden  int `yaml:"num_hidden"`
	NumOutputs int `yaml:"num_outputs"`
}

// SensorsConfig holds sensor parameters.
type SensorsConfig struct {
	NumSectors             int     `yaml:"num_sectors"`
	ResourceSampleDistance float64 `yaml:"resource_sample_distance"`
}

// GPUConfig holds GPU rendering parameters.
type GPUConfig struct {
	FlowTextureSize     int `yaml:"flow_texture_size"`
	FlowUpdateInterval  int `yaml:"flow_update_interval"`
	ResourceTextureSize int `yaml:"resource_texture_size"`
}

// TelemetryConfig holds telemetry parameters.
type TelemetryConfig struct {
	StatsWindow         float64 `yaml:"stats_window"`
	BookmarkHistorySize int     `yaml:"bookmark_history_size"`
	PerfCollectorWindow int     `yaml:"perf_collector_window"`
}

// BookmarksConfig holds bookmark detection thresholds.
type BookmarksConfig struct {
	HuntBreakthrough   HuntBreakthroughConfig   `yaml:"hunt_breakthrough"`
	ForageBreakthrough ForageBreakthroughConfig `yaml:"forage_breakthrough"`
	PredatorRecovery   PredatorRecoveryConfig   `yaml:"predator_recovery"`
	PreyCrash          PreyCrashConfig          `yaml:"prey_crash"`
	StableEcosystem    StableEcosystemConfig    `yaml:"stable_ecosystem"`
}

// HuntBreakthroughConfig holds hunt breakthrough detection parameters.
type HuntBreakthroughConfig struct {
	Multiplier float64 `yaml:"multiplier"`
	MinKills   int     `yaml:"min_kills"`
}

// ForageBreakthroughConfig holds forage breakthrough detection parameters.
type ForageBreakthroughConfig struct {
	Multiplier  float64 `yaml:"multiplier"`
	MinResource float64 `yaml:"min_resource"`
}

// PredatorRecoveryConfig holds predator recovery detection parameters.
type PredatorRecoveryConfig struct {
	MinPopulation      int `yaml:"min_population"`
	RecoveryMultiplier int `yaml:"recovery_multiplier"`
	MinFinal           int `yaml:"min_final"`
}

// PreyCrashConfig holds prey crash detection parameters.
type PreyCrashConfig struct {
	DropPercent float64 `yaml:"drop_percent"`
	MinDrop     int     `yaml:"min_drop"`
}

// StableEcosystemConfig holds stable ecosystem detection parameters.
type StableEcosystemConfig struct {
	MinPrey       int     `yaml:"min_prey"`
	MinPred       int     `yaml:"min_pred"`
	CVThreshold   float64 `yaml:"cv_threshold"`
	StableWindows int     `yaml:"stable_windows"`
}

// DerivedConfig holds computed values derived from the loaded config.
type DerivedConfig struct {
	DT32      float32 // Physics.DT as float32
	NumInputs int     // Sensors.NumSectors*3 + 2
}

// global holds the loaded configuration.
var global *Config

// Init loads configuration from the given path, or uses embedded defaults if path is empty.
// Must be called before Cfg().
func Init(path string) error {
	cfg, err := Load(path)
	if err != nil {
		return err
	}
	global = cfg
	return nil
}

// MustInit is like Init but panics on error.
func MustInit(path string) {
	if err := Init(path); err != nil {
		panic(fmt.Sprintf("config: failed to initialize: %v", err))
	}
}

// Cfg returns the global configuration. Panics if Init was not called.
func Cfg() *Config {
	if global == nil {
		panic("config: Cfg() called before Init()")
	}
	return global
}

// Load loads configuration from a YAML file, merging with embedded defaults.
// If path is empty, only embedded defaults are used.
func Load(path string) (*Config, error) {
	// Start with embedded defaults
	cfg := &Config{}
	if err := yaml.Unmarshal(defaultsYAML, cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded defaults: %w", err)
	}

	// Load user config if provided
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Unmarshal into same struct - only overwrites fields present in file
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Compute derived values
	cfg.computeDerived()

	return cfg, nil
}

// computeDerived calculates values derived from loaded config.
func (c *Config) computeDerived() {
	c.Derived.DT32 = float32(c.Physics.DT)
	c.Derived.NumInputs = c.Sensors.NumSectors*3 + 2
}

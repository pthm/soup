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
	World        WorldConfig        `yaml:"world"`
	Physics      PhysicsConfig      `yaml:"physics"`
	Entity       EntityConfig       `yaml:"entity"`
	Capabilities CapabilitiesConfig `yaml:"capabilities"`
	Population   PopulationConfig   `yaml:"population"`
	Reproduction ReproductionConfig `yaml:"reproduction"`
	Mutation     MutationConfig     `yaml:"mutation"`
	Energy       EnergyConfig       `yaml:"energy"`
	Resource     ResourceConfig     `yaml:"resource"`
	Potential    PotentialConfig    `yaml:"potential"`
	Neural       NeuralConfig       `yaml:"neural"`
	Sensors      SensorsConfig      `yaml:"sensors"`
	GPU          GPUConfig          `yaml:"gpu"`
	Telemetry    TelemetryConfig    `yaml:"telemetry"`
	Bookmarks    BookmarksConfig    `yaml:"bookmarks"`
	Refugia      RefugiaConfig      `yaml:"refugia"`
	HallOfFame   HallOfFameConfig   `yaml:"hall_of_fame"`
	Archetypes   []ArchetypeConfig  `yaml:"archetypes"`
	Clades       CladeConfig        `yaml:"clades"`
	Detritus     DetritusConfig     `yaml:"detritus"`
	Biomass      BiomassConfig      `yaml:"biomass"`

	// Derived values computed after loading
	Derived DerivedConfig `yaml:"-"`
}

// BiomassConfig holds biomass/growth parameters for the two-pool energy model.
type BiomassConfig struct {
	MetPerBio       float64 `yaml:"met_per_bio"`       // MaxMet = Bio * this
	GrowthRate      float64 `yaml:"growth_rate"`       // Bio growth per second from surplus Met
	GrowthThreshold float64 `yaml:"growth_threshold"`  // Grow when Met > MaxMet * this
	MinBio          float64 `yaml:"min_bio"`           // Child starts with this biomass
	BirthEfficiency float64 `yaml:"birth_efficiency"`  // Parent pays (childBio + childMet) / this
	BioCost         float64 `yaml:"bio_cost"`          // Metabolic drain per Bio per second
}

// ArchetypeConfig defines a founder template for organisms.
// Each archetype specifies the full set of capabilities that define organism behavior.
type ArchetypeConfig struct {
	Name           string    `yaml:"name"`
	Diet           float64   `yaml:"diet"`            // 0=herbivore, 1=carnivore (determines food sources)
	MetabolicRate  float64   `yaml:"metabolic_rate"`  // Scales all energy costs and intake rates (default 1.0)
	EnergyCapacity float64   `yaml:"energy_capacity"` // Maximum energy storage (default from entity.max_energy)
	VisionRange    float64   `yaml:"vision_range"`    // Perception distance
	VisionWeights  []float64 `yaml:"vision_weights"`  // Per-sector sensitivity (NumSectors elements)
}

// CladeConfig holds clade split parameters.
type CladeConfig struct {
	SplitChance    float64 `yaml:"split_chance"`    // Random split probability per birth (e.g., 0.005)
	DeltaThreshold float64 `yaml:"delta_threshold"` // avgAbsDelta threshold for forced split
	DietThreshold  float64 `yaml:"diet_threshold"`  // Diet drift threshold for forced split
}

// ScreenConfig holds display settings.
type ScreenConfig struct {
	Width     int `yaml:"width"`
	Height    int `yaml:"height"`
	TargetFPS int `yaml:"target_fps"`
}

// WorldConfig holds simulation world dimensions.
// World can be larger than the screen; camera handles the viewport.
type WorldConfig struct {
	Width  int `yaml:"width"`  // World width in world units (0 = use screen width)
	Height int `yaml:"height"` // World height in world units (0 = use screen height)
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
	MaxEnergy     float64 `yaml:"max_energy"`
}

// CapabilitiesConfig holds shared entity capability defaults.
// Vision range and weights are now per-archetype (see ArchetypeConfig).
type CapabilitiesConfig struct {
	MinEffectiveness float64 `yaml:"min_effectiveness"` // Minimum vision weight floor
	MaxSpeed         float64 `yaml:"max_speed"`
	MaxAccel         float64 `yaml:"max_accel"`
	MaxTurnRate      float64 `yaml:"max_turn_rate"`
	Drag             float64 `yaml:"drag"`
	BiteRange        float64 `yaml:"bite_range"`
	ThrustDeadzone   float64 `yaml:"thrust_deadzone"` // Thrust below this = 0
}

// PopulationConfig holds population management parameters.
type PopulationConfig struct {
	Initial             int     `yaml:"initial"`
	MaxPrey             int     `yaml:"max_prey"`
	MaxPred             int     `yaml:"max_pred"`
	RespawnThreshold    int     `yaml:"respawn_threshold"`
	RespawnCount        int     `yaml:"respawn_count"`
	PredatorSpawnChance float64 `yaml:"predator_spawn_chance"`
	MinPredators        int     `yaml:"min_predators"` // Minimum predator count; respawn when below
	MinPrey             int     `yaml:"min_prey"`      // Minimum prey count; respawn when below
}

// ReproductionConfig holds reproduction parameters.
type ReproductionConfig struct {
	PreyThreshold        float64 `yaml:"prey_threshold"`
	PredThreshold        float64 `yaml:"pred_threshold"`
	MaturityAge          float64 `yaml:"maturity_age"`
	PreyCooldown         float64 `yaml:"prey_cooldown"`
	PredCooldown         float64 `yaml:"pred_cooldown"`
	CooldownJitter       float64 `yaml:"cooldown_jitter"`
	ParentEnergySplit    float64 `yaml:"parent_energy_split"`
	SpawnOffset          float64 `yaml:"spawn_offset"`
	HeadingJitter        float64 `yaml:"heading_jitter"`
	PredDensityK         float64 `yaml:"pred_density_k"`          // Density-dependent reproduction: p = prey / (prey + K)
	PreyDensityK         float64 `yaml:"prey_density_k"`          // Soft prey carrying capacity: p = K / (N + K)
	NewbornHuntCooldown  float64 `yaml:"newborn_hunt_cooldown"`   // Seconds before newborn predators can bite
}

// MutationConfig holds mutation parameters.
type MutationConfig struct {
	Rate              float64 `yaml:"rate"`
	Sigma             float64 `yaml:"sigma"`
	BigRate           float64 `yaml:"big_rate"`
	BigSigma          float64 `yaml:"big_sigma"`
	BiasRateMultiplier float64 `yaml:"bias_rate_multiplier"`
}

// ResourceConfig holds resource field parameters.
type ResourceConfig struct {
	GrazeRadius int     `yaml:"graze_radius"` // Grazing kernel radius in cells (1=3x3, 2=5x5)
	RegenRate   float64 `yaml:"regen_rate"`   // Regeneration rate when below capacity (per second)
	DecayRate   float64 `yaml:"decay_rate"`   // Decay rate when above capacity (per second)
}

// PotentialConfig holds potential field generation parameters.
type PotentialConfig struct {
	Scale          float64 `yaml:"scale"`           // Base noise frequency
	Octaves        int     `yaml:"octaves"`         // FBM octaves (detail level)
	Lacunarity     float64 `yaml:"lacunarity"`      // Frequency multiplier per octave
	Gain           float64 `yaml:"gain"`            // Amplitude multiplier per octave
	Contrast       float64 `yaml:"contrast"`        // FBM contrast exponent (higher = sparser patches)
	TimeSpeed      float64 `yaml:"time_speed"`      // Speed of noise animation (0 = static)
	UpdateInterval float64 `yaml:"update_interval"` // Seconds between capacity field updates
}

// EnergyConfig holds unified energy economics parameters.
// All costs are base values multiplied by the organism's metabolic_rate.
// Diet determines food source access: grazing scales by (1-diet), hunting by diet.
type EnergyConfig struct {
	// Base costs (multiplied by metabolic_rate)
	BaseCost  float64 `yaml:"base_cost"`  // Energy drain per second for existing
	MoveCost  float64 `yaml:"move_cost"`  // Movement cost multiplier
	AccelCost float64 `yaml:"accel_cost"` // Acceleration cost multiplier

	// Unified feeding
	FeedingRate       float64 `yaml:"feeding_rate"`       // Base intake rate (grazing per sec, bite damage)
	FeedingEfficiency float64 `yaml:"feeding_efficiency"` // Fraction of intake converted to energy
	CooldownFactor    float64 `yaml:"cooldown_factor"`    // Cooldown = energy_gained * cooldown_factor / metabolic_rate
}

// RefugiaConfig holds refugia mechanics parameters.
type RefugiaConfig struct {
	Strength float64 `yaml:"strength"`
}

// NeuralConfig holds neural network parameters.
type NeuralConfig struct {
	HiddenLayers []int `yaml:"hidden_layers"` // Sizes of hidden layers, e.g. [16, 8]
	NumOutputs   int   `yaml:"num_outputs"`
}

// SensorsConfig holds sensor parameters.
type SensorsConfig struct {
	NumSectors             int     `yaml:"num_sectors"`
	ResourceSampleDistance float64 `yaml:"resource_sample_distance"`
	DietThreshold          float64 `yaml:"diet_threshold"` // Min diet difference to register as food/threat
	KinRange               float64 `yaml:"kin_range"`      // Diet range for kin detection
}

// GPUConfig holds GPU rendering parameters.
type GPUConfig struct {
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

// HallOfFameConfig holds hall of fame settings for intelligent reseeding.
type HallOfFameConfig struct {
	Enabled         bool                   `yaml:"enabled"`
	Size            int                    `yaml:"size"`
	ReseedThreshold int                    `yaml:"reseed_threshold"`
	ReseedCount     int                    `yaml:"reseed_count"`
	ReseedEnergy    float64                `yaml:"reseed_energy"`
	Fitness         HallOfFameFitnessConfig `yaml:"fitness"`
	Entry           HallOfFameEntryConfig   `yaml:"entry"`
}

// HallOfFameFitnessConfig holds fitness calculation weights.
type HallOfFameFitnessConfig struct {
	ChildrenWeight float64 `yaml:"children_weight"`
	SurvivalWeight float64 `yaml:"survival_weight"`
	KillsWeight    float64 `yaml:"kills_weight"`
	ForageWeight   float64 `yaml:"forage_weight"`
}

// HallOfFameEntryConfig holds entry criteria thresholds.
type HallOfFameEntryConfig struct {
	MinChildren    int     `yaml:"min_children"`
	MinSurvivalSec float64 `yaml:"min_survival_sec"`
	MinForaging    float64 `yaml:"min_foraging"`
	MinKills       int     `yaml:"min_kills"`
}

// DetritusConfig holds detritus grid parameters for nutrient recycling.
type DetritusConfig struct {
	DecayRate       float64 `yaml:"decay_rate"`       // Fraction of detritus that decays per second
	DecayEfficiency float64 `yaml:"decay_efficiency"` // Fraction of decayed detritus converted to resource (rest is heat)
	CarcassFraction float64 `yaml:"carcass_fraction"` // Fraction of organism energy deposited as detritus on death
}


// DerivedConfig holds computed values derived from the loaded config.
type DerivedConfig struct {
	DT32           float32          // Physics.DT as float32
	NumInputs      int              // Sensors.NumSectors*3 + 3
	ScreenW32      float32          // Screen.Width as float32
	ScreenH32      float32          // Screen.Height as float32
	WorldW32       float32          // Effective world width as float32
	WorldH32       float32          // Effective world height as float32
	ArchetypeIndex map[string]uint8 // name -> index for archetype lookup
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
	c.Derived.NumInputs = c.Sensors.NumSectors*3 + 4 // food, threat, kin + energy, speed, diet, metabolic_rate
	c.Derived.ScreenW32 = float32(c.Screen.Width)
	c.Derived.ScreenH32 = float32(c.Screen.Height)

	// World dimensions default to screen size if not specified
	worldW := c.World.Width
	if worldW == 0 {
		worldW = c.Screen.Width
	}
	worldH := c.World.Height
	if worldH == 0 {
		worldH = c.Screen.Height
	}
	c.Derived.WorldW32 = float32(worldW)
	c.Derived.WorldH32 = float32(worldH)

	// Synthesize default archetypes if none specified
	if len(c.Archetypes) == 0 {
		c.Archetypes = []ArchetypeConfig{
			{
				Name:           "grazer",
				Diet:           0.0,
				MetabolicRate:  1.0,
				EnergyCapacity: c.Entity.MaxEnergy,
				VisionRange:    100.0,
				VisionWeights:  []float64{0.05, 0.6, 1.0, 0.6, 0.15, 0.6, 1.0, 0.6},
			},
			{
				Name:           "hunter",
				Diet:           1.0,
				MetabolicRate:  0.75,
				EnergyCapacity: c.Entity.MaxEnergy * 0.8,
				VisionRange:    140.0,
				VisionWeights:  []float64{0.0, 0.0, 0.0, 0.6, 1.0, 0.6, 0.0, 0.0},
			},
		}
	}

	// Apply defaults to archetypes that don't specify all fields
	for i := range c.Archetypes {
		arch := &c.Archetypes[i]
		if arch.MetabolicRate == 0 {
			arch.MetabolicRate = 1.0
		}
		if arch.EnergyCapacity == 0 {
			arch.EnergyCapacity = c.Entity.MaxEnergy
		}
		if arch.VisionRange == 0 {
			arch.VisionRange = 100.0
		}
		if len(arch.VisionWeights) == 0 {
			// Default to uniform weights
			arch.VisionWeights = make([]float64, c.Sensors.NumSectors)
			for j := range arch.VisionWeights {
				arch.VisionWeights[j] = 1.0
			}
		}
	}

	// Build archetype index for fast lookup
	c.Derived.ArchetypeIndex = make(map[string]uint8, len(c.Archetypes))
	for i, arch := range c.Archetypes {
		c.Derived.ArchetypeIndex[arch.Name] = uint8(i)
	}
}

// WriteYAML writes the configuration to a YAML file.
func (c *Config) WriteYAML(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}
	return nil
}

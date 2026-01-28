// Package traits defines organism behaviors and characteristics.
package traits

// Trait defines organism behavior.
type Trait uint32

const (
	// Diet traits
	Flora     Trait = 1 << iota // Photosynthesizes, releases spores
	Herbivore                   // Eats flora
	Carnivore                   // Eats fauna
	Carrion                     // Eats dead organisms

	// Behavior traits
	Herding  // Flocks with similar organisms
	Breeding // Can reproduce sexually

	// Flora-specific traits
	Rooted   // Anchored to bottom
	Floating // Drifts in space

	// Vision traits (fauna only)
	PredatorEyes // Narrow forward vision (90 deg), longer range
	PreyEyes     // Wide peripheral vision (270 deg), shorter range
	FarSight     // Increased perception radius

	// Physical traits (fauna only)
	Speed // Faster movement, overcomes size penalty

	// Gender (for breeding)
	Male
	Female
)

// Mutation defines cell mutations.
type Mutation uint8

const (
	NoMutation Mutation = iota
	Disease             // Energy drain, contagious
	Rage                // Faster, more aggressive, burns energy
	Growth              // Faster cell generation
	Splitting           // Can split off cells (self-propagating)
)

// Has checks if a trait set contains a trait.
func (t Trait) Has(other Trait) bool {
	return t&other != 0
}

// Add adds a trait to the set.
func (t Trait) Add(other Trait) Trait {
	return t | other
}

// Remove removes a trait from the set.
func (t Trait) Remove(other Trait) Trait {
	return t &^ other
}

// IsFlora checks if traits indicate flora.
func IsFlora(t Trait) bool {
	return t.Has(Flora) && !t.Has(Herbivore) && !t.Has(Carnivore)
}

// IsFauna checks if traits indicate fauna.
func IsFauna(t Trait) bool {
	return t.Has(Herbivore) || t.Has(Carnivore) || t.Has(Carrion)
}

// IsOmnivore checks if organism has both herbivore and carnivore.
func IsOmnivore(t Trait) bool {
	return t.Has(Herbivore) && t.Has(Carnivore)
}

// FloraOnlyTraits are traits that only apply to flora.
var FloraOnlyTraits = Flora | Rooted | Floating

// FaunaOnlyTraits are traits that only apply to fauna.
var FaunaOnlyTraits = Herbivore | Carnivore | Carrion | Herding | Breeding | PredatorEyes | PreyEyes | FarSight | Speed

// TraitWeights for random selection (higher = more common).
var TraitWeights = map[Trait]float32{
	Flora:        0.15,
	Herbivore:    0.25,
	Carnivore:    0.08,
	Carrion:      0.12,
	Herding:      0.15,
	Breeding:     0.10,
	Rooted:       0.05,
	Floating:     0.03,
	PredatorEyes: 0.06,
	PreyEyes:     0.08,
	FarSight:     0.05,
	Speed:        0.06,
}

// MutationWeights for random mutation selection.
var MutationWeights = map[Mutation]float32{
	Disease:   0.02,
	Rage:      0.03,
	Growth:    0.04,
	Splitting: 0.02,
}

// VisionParams holds vision parameters.
type VisionParams struct {
	FOV             float32 // Field of view in radians
	RangeMultiplier float32 // Perception radius multiplier
}

// GetVisionParams returns vision parameters based on traits.
func GetVisionParams(t Trait) VisionParams {
	const Pi = 3.14159265358979323846

	params := VisionParams{
		FOV:             Pi, // Default 180 degrees
		RangeMultiplier: 1.0,
	}

	if t.Has(PredatorEyes) {
		params.FOV = Pi / 2         // 90 degree narrow cone
		params.RangeMultiplier = 1.5 // See further forward
	} else if t.Has(PreyEyes) {
		params.FOV = Pi * 1.5       // 270 degree wide vision
		params.RangeMultiplier = 0.7 // Shorter range but wider
	}

	if t.Has(FarSight) {
		params.RangeMultiplier *= 1.5
	}

	return params
}

// TraitNames returns human-readable names for traits.
func TraitNames(t Trait) []string {
	var names []string
	if t.Has(Flora) {
		names = append(names, "Flora")
	}
	if t.Has(Herbivore) {
		names = append(names, "Herbivore")
	}
	if t.Has(Carnivore) {
		names = append(names, "Carnivore")
	}
	if t.Has(Carrion) {
		names = append(names, "Carrion")
	}
	if t.Has(Herding) {
		names = append(names, "Herding")
	}
	if t.Has(Breeding) {
		names = append(names, "Breeding")
	}
	if t.Has(Rooted) {
		names = append(names, "Rooted")
	}
	if t.Has(Floating) {
		names = append(names, "Floating")
	}
	if t.Has(PredatorEyes) {
		names = append(names, "Predator Eyes")
	}
	if t.Has(PreyEyes) {
		names = append(names, "Prey Eyes")
	}
	if t.Has(FarSight) {
		names = append(names, "Far Sight")
	}
	if t.Has(Speed) {
		names = append(names, "Speed")
	}
	if t.Has(Male) {
		names = append(names, "Male")
	}
	if t.Has(Female) {
		names = append(names, "Female")
	}
	return names
}

// MutationName returns the name of a mutation.
func MutationName(m Mutation) string {
	switch m {
	case Disease:
		return "Disease"
	case Rage:
		return "Rage"
	case Growth:
		return "Growth"
	case Splitting:
		return "Splitting"
	default:
		return ""
	}
}

// GetTraitColor returns RGB color based on traits.
func GetTraitColor(t Trait) (r, g, b uint8) {
	if IsFlora(t) {
		return 50, 180, 80 // Green
	}
	if IsOmnivore(t) {
		return 180, 100, 180 // Purple
	}
	if t.Has(Carnivore) {
		return 200, 80, 80 // Red
	}
	if t.Has(Herbivore) {
		return 80, 150, 200 // Blue
	}
	if t.Has(Carrion) {
		return 120, 100, 80 // Brown
	}
	return 150, 150, 150 // Gray default
}

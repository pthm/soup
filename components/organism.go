package components

// Kind identifies entity type for predator/prey interactions.
type Kind uint8

const (
	KindPrey Kind = iota
	KindPredator
)

// String returns a human-readable name for the kind.
func (k Kind) String() string {
	switch k {
	case KindPrey:
		return "prey"
	case KindPredator:
		return "predator"
	default:
		return "unknown"
	}
}

// Energy tracks an entity's metabolic state.
type Energy struct {
	Value      float32 `inspect:"bar"`           // 0..1
	Age        float32 `inspect:"label,fmt:%.1fs"` // seconds alive
	Alive      bool    `inspect:"bool"`
	LastThrust float32 // thrust from last tick for accel cost calculation
}

// Organism bundles identity, kind, and reproduction state.
type Organism struct {
	ID                 uint32  `inspect:"label"`
	Kind               Kind    `inspect:"label"`           // Derived from Diet for backwards compat
	FounderArchetypeID uint8   `inspect:"label"`           // Immutable founder template index
	Diet               float32 `inspect:"bar"`             // Heritable trait, 0..1 (0=herbivore, 1=carnivore)
	CladeID            uint64  `inspect:"label"`           // Lineage identifier
	ReproCooldown      float32 `inspect:"label,fmt:%.1fs"` // seconds until can reproduce again
	DigestCooldown     float32 `inspect:"label,fmt:%.1fs"` // seconds until can bite again (predators only)
}

// DerivedKind returns Kind based on current Diet for backwards compatibility.
func (o *Organism) DerivedKind() Kind {
	if o.Diet >= 0.5 {
		return KindPredator
	}
	return KindPrey
}

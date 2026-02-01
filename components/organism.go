package components

// Kind identifies entity type for predator/prey interactions.
type Kind uint8

const (
	KindPrey Kind = iota
	KindPredator
)

// Energy tracks an entity's metabolic state.
type Energy struct {
	Value      float32 `inspect:"bar"`           // 0..1
	Age        float32 `inspect:"label,fmt:%.1fs"` // seconds alive
	Alive      bool    `inspect:"bool"`
	LastThrust float32 // thrust from last tick for accel cost calculation
}

// Organism bundles identity, kind, and reproduction state.
type Organism struct {
	ID             uint32  `inspect:"label"`
	Kind           Kind    `inspect:"label"`
	ReproCooldown  float32 `inspect:"label,fmt:%.1fs"` // seconds until can reproduce again
	DigestCooldown float32 `inspect:"label,fmt:%.1fs"` // seconds until can bite again (predators only)
}

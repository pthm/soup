package components

// Kind identifies entity type for predator/prey interactions.
type Kind uint8

const (
	KindPrey Kind = iota
	KindPredator
)

// Energy tracks an entity's metabolic state.
type Energy struct {
	Value float32 // 0..1
	Age   float32 // seconds alive
	Alive bool
}

// Organism bundles identity, kind, and brain for an entity.
type Organism struct {
	ID   uint32
	Kind Kind
}

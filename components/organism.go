package components

// Energy tracks an entity's metabolic state.
// Value is in absolute energy units; Max is the per-organism capacity.
// Brain inputs and display use the ratio Value/Max for normalization.
type Energy struct {
	Value      float32 `inspect:"bar"`             // absolute energy
	Max        float32 `inspect:"label,fmt:%.2f"`  // maximum energy capacity
	Age        float32 `inspect:"label,fmt:%.1fs"` // seconds alive
	Alive      bool    `inspect:"bool"`
	LastThrust float32 // thrust from last tick for accel cost calculation
	LastBite   float32 // bite output from last tick for bite cost
}

// Organism bundles identity, diet, and reproduction state.
type Organism struct {
	ID                 uint32  `inspect:"label"`
	FounderArchetypeID uint8   `inspect:"label"`           // Immutable founder template index
	Diet               float32 `inspect:"bar"`             // Heritable trait, 0..1 (0=herbivore, 1=carnivore)
	CladeID            uint64  `inspect:"label"`           // Lineage identifier
	ReproCooldown      float32 `inspect:"label,fmt:%.1fs"` // seconds until can reproduce again
	DigestCooldown     float32 `inspect:"label,fmt:%.1fs"` // seconds until can bite again (predators only)
	HuntCooldown       float32 `inspect:"label,fmt:%.1fs"` // seconds until newborn can hunt (predators only)
}

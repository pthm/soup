package components

// Energy tracks an entity's metabolic state using a two-pool model.
//
// Met (metabolic) = short-term fuel from eating, used for movement/metabolism
// Bio (biomass) = structural body mass, grows from surplus Met up to BioCap
//
// Conservation: particles are the only external energy source.
// - Grazing: Res → Met (with efficiency loss to heat)
// - Growth: surplus Met → Bio (no loss, just pool transfer)
// - Metabolism: Met → Heat (base + bio_cost*Bio + movement)
// - Reproduction: Parent.Met → Child.Bio + Child.Met (with efficiency loss)
// - Kill: (Bio + Met) → Predator.Met + Heat
// - Starvation: (Bio + Met) → Detritus + Heat
type Energy struct {
	Met        float32 `inspect:"bar"`             // metabolic energy (fuel)
	Bio        float32 `inspect:"bar"`             // biomass (body structure)
	BioCap     float32 `inspect:"label,fmt:%.2f"`  // max biomass from archetype
	Age        float32 `inspect:"label,fmt:%.1fs"` // seconds alive
	Alive      bool    `inspect:"bool"`
	LastThrust float32 // thrust from last tick for accel cost calculation
	LastBite   float32 // bite output from last tick for bite cost
}

// MaxMet returns the metabolic energy capacity, derived from current biomass.
func (e *Energy) MaxMet(metPerBio float32) float32 {
	return e.Bio * metPerBio
}

// Organism bundles identity, capabilities, and reproduction state.
type Organism struct {
	ID                 uint32  `inspect:"label"`
	FounderArchetypeID uint8   `inspect:"label"`           // Immutable founder template index
	Diet               float32 `inspect:"bar"`             // Heritable trait, 0..1 (0=herbivore, 1=carnivore)
	MetabolicRate      float32 `inspect:"label,fmt:%.2f"`  // Scales all energy costs and intake (from archetype)
	CladeID            uint64  `inspect:"label"`           // Lineage identifier
	ReproCooldown      float32 `inspect:"label,fmt:%.1fs"` // seconds until can reproduce again
	DigestCooldown     float32 `inspect:"label,fmt:%.1fs"` // seconds until can bite again
	HuntCooldown       float32 `inspect:"label,fmt:%.1fs"` // seconds until newborn can hunt
}

package neural

// IODescriptor describes a brain input or output for UI display.
type IODescriptor struct {
	ID          string  // Unique identifier
	Label       string  // Display name
	Description string  // Tooltip/extended description
	Min         float32 // Minimum value
	Max         float32 // Maximum value
	IsCentered  bool    // True for centered bar display (e.g., -1 to +1)
	Group       string  // Logical grouping (e.g., "vision", "environment", "intent")
}

// BrainInputDescriptors returns metadata for all brain inputs.
// Order matches the indices used in SensoryInputs.ToInputs().
func BrainInputDescriptors() []IODescriptor {
	return []IODescriptor{
		// Self state (indices 0-1)
		{ID: "speed_norm", Label: "Speed", Description: "Current speed / max speed", Min: 0, Max: 1, Group: "self"},
		{ID: "energy_norm", Label: "Energy", Description: "Current energy / max energy", Min: 0, Max: 1, Group: "self"},

		// Body descriptor (indices 2-7)
		{ID: "size_norm", Label: "Size", Description: "Body radius normalized", Min: 0, Max: 1, Group: "body"},
		{ID: "speed_capacity", Label: "Speed Cap", Description: "Movement capability", Min: 0, Max: 1, Group: "body"},
		{ID: "agility_norm", Label: "Agility", Description: "Turn rate capability", Min: 0, Max: 1, Group: "body"},
		{ID: "sense_strength", Label: "Sense", Description: "Perception quality", Min: 0, Max: 1, Group: "body"},
		{ID: "bite_strength", Label: "Bite", Description: "Attack capability", Min: 0, Max: 1, Group: "body"},
		{ID: "armor_level", Label: "Armor", Description: "Damage resistance", Min: 0, Max: 1, Group: "body"},

		// Boid fields (indices 8-16)
		{ID: "cohesion_fwd", Label: "Coh Fwd", Description: "Flock center forward component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "cohesion_up", Label: "Coh Up", Description: "Flock center lateral component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "cohesion_mag", Label: "Coh Mag", Description: "Flock center distance", Min: 0, Max: 1, Group: "boid"},
		{ID: "alignment_fwd", Label: "Align Fwd", Description: "Flock heading forward component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "alignment_up", Label: "Align Up", Description: "Flock heading lateral component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "separation_fwd", Label: "Sep Fwd", Description: "Repulsion forward component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "separation_up", Label: "Sep Up", Description: "Repulsion lateral component", Min: -1, Max: 1, IsCentered: true, Group: "boid"},
		{ID: "separation_mag", Label: "Sep Mag", Description: "Separation urgency", Min: 0, Max: 1, Group: "boid"},
		{ID: "density_same", Label: "Density", Description: "Local same-species density", Min: 0, Max: 1, Group: "boid"},

		// Food fields (indices 17-22)
		{ID: "plant_fwd", Label: "Plant Fwd", Description: "Plant direction forward", Min: -1, Max: 1, IsCentered: true, Group: "food"},
		{ID: "plant_up", Label: "Plant Up", Description: "Plant direction lateral", Min: -1, Max: 1, IsCentered: true, Group: "food"},
		{ID: "plant_mag", Label: "Plant Mag", Description: "Plant attraction strength", Min: 0, Max: 1, Group: "food"},
		{ID: "meat_fwd", Label: "Meat Fwd", Description: "Prey direction forward", Min: -1, Max: 1, IsCentered: true, Group: "food"},
		{ID: "meat_up", Label: "Meat Up", Description: "Prey direction lateral", Min: -1, Max: 1, IsCentered: true, Group: "food"},
		{ID: "meat_mag", Label: "Meat Mag", Description: "Prey attraction strength", Min: 0, Max: 1, Group: "food"},

		// Threat info (indices 23-24)
		{ID: "threat_proximity", Label: "Threat Dist", Description: "Nearest predator distance (0=far, 1=close)", Min: 0, Max: 1, Group: "threat"},
		{ID: "threat_closing", Label: "Threat Speed", Description: "Predator approach rate", Min: -1, Max: 1, IsCentered: true, Group: "threat"},

		// Bias (index 25)
		{ID: "bias", Label: "Bias", Description: "Constant bias input (always 1.0)", Min: 0, Max: 1, Group: "internal"},
	}
}

// BrainOutputDescriptors returns metadata for all brain outputs.
// Order matches the indices used in DecodeOutputs().
func BrainOutputDescriptors() []IODescriptor {
	return []IODescriptor{
		{ID: "u_fwd", Label: "U Fwd", Description: "Desired forward velocity", Min: -1, Max: 1, IsCentered: true, Group: "movement"},
		{ID: "u_up", Label: "U Up", Description: "Desired lateral velocity", Min: -1, Max: 1, IsCentered: true, Group: "movement"},
		{ID: "attack_intent", Label: "Attack", Description: "Predation gate (>0.5=attack)", Min: 0, Max: 1, Group: "action"},
		{ID: "mate_intent", Label: "Mate", Description: "Mating gate (>0.5=ready)", Min: 0, Max: 1, Group: "action"},
	}
}

// InputGroups returns the logical groupings for inputs.
func InputGroups() []string {
	return []string{"self", "body", "boid", "food", "threat", "internal"}
}

// OutputGroups returns the logical groupings for outputs.
func OutputGroups() []string {
	return []string{"movement", "action"}
}

// InputByID returns the descriptor for a specific input by ID.
func InputByID(id string) (IODescriptor, bool) {
	for _, desc := range BrainInputDescriptors() {
		if desc.ID == id {
			return desc, true
		}
	}
	return IODescriptor{}, false
}

// OutputByID returns the descriptor for a specific output by ID.
func OutputByID(id string) (IODescriptor, bool) {
	for _, desc := range BrainOutputDescriptors() {
		if desc.ID == id {
			return desc, true
		}
	}
	return IODescriptor{}, false
}

// InputCount returns the number of brain inputs.
func InputCount() int {
	return BrainInputs
}

// OutputCount returns the number of brain outputs.
func OutputCount() int {
	return BrainOutputs
}

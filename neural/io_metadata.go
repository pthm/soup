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
		// Vision cones: Food (indices 0-3)
		{ID: "cone_food_front", Label: "Food Front", Description: "Food intensity ahead", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_food_right", Label: "Food Right", Description: "Food intensity to right", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_food_back", Label: "Food Back", Description: "Food intensity behind", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_food_left", Label: "Food Left", Description: "Food intensity to left", Min: 0, Max: 1, Group: "vision"},

		// Vision cones: Threat (indices 4-7)
		{ID: "cone_threat_front", Label: "Threat Front", Description: "Threat intensity ahead", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_threat_right", Label: "Threat Right", Description: "Threat intensity to right", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_threat_back", Label: "Threat Back", Description: "Threat intensity behind", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_threat_left", Label: "Threat Left", Description: "Threat intensity to left", Min: 0, Max: 1, Group: "vision"},

		// Vision cones: Friend (indices 8-11)
		{ID: "cone_friend_front", Label: "Friend Front", Description: "Friend (kin) intensity ahead", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_friend_right", Label: "Friend Right", Description: "Friend intensity to right", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_friend_back", Label: "Friend Back", Description: "Friend intensity behind", Min: 0, Max: 1, Group: "vision"},
		{ID: "cone_friend_left", Label: "Friend Left", Description: "Friend intensity to left", Min: 0, Max: 1, Group: "vision"},

		// Environment (indices 12-16)
		{ID: "energy_ratio", Label: "Energy", Description: "Current energy / max energy", Min: 0, Max: 1, Group: "internal"},
		{ID: "light_level", Label: "Light", Description: "Ambient light level from shadowmap", Min: 0, Max: 1, Group: "environment"},
		{ID: "flow_alignment", Label: "Flow Align", Description: "Alignment with flow field (-1=against, +1=with)", Min: -1, Max: 1, IsCentered: true, Group: "environment"},
		{ID: "light_fb", Label: "Light F/B", Description: "Light gradient front-back (>0=brighter ahead)", Min: -1, Max: 1, IsCentered: true, Group: "environment"},
		{ID: "light_lr", Label: "Light L/R", Description: "Light gradient left-right (>0=brighter right)", Min: -1, Max: 1, IsCentered: true, Group: "environment"},

		// Damage awareness (index 17)
		{ID: "being_eaten", Label: "Attacked", Description: "Currently being eaten (0=safe, 1=heavy attack)", Min: 0, Max: 1, Group: "internal"},

		// Bias (index 18)
		{ID: "bias", Label: "Bias", Description: "Constant bias input (always 1.0)", Min: 0, Max: 1, Group: "internal"},
	}
}

// BrainOutputDescriptors returns metadata for all brain outputs.
// Order matches the indices used in DecodeOutputs().
func BrainOutputDescriptors() []IODescriptor {
	return []IODescriptor{
		{ID: "desire_angle", Label: "Turn", Description: "Desired turn angle (-π to +π)", Min: -1, Max: 1, IsCentered: true, Group: "movement"},
		{ID: "desire_distance", Label: "Thrust", Description: "Movement urgency (0=stay, 1=max)", Min: 0, Max: 1, Group: "movement"},
		{ID: "eat", Label: "Eat", Description: "Feeding intent (>0.5=try to eat)", Min: 0, Max: 1, Group: "action"},
		{ID: "grow", Label: "Grow", Description: "Growth intent (allocate to new cells)", Min: 0, Max: 1, Group: "action"},
		{ID: "breed", Label: "Breed", Description: "Reproduction intent (>0.5=try to breed)", Min: 0, Max: 1, Group: "action"},
		{ID: "glow", Label: "Glow", Description: "Bioluminescence intensity", Min: 0, Max: 1, Group: "action"},
	}
}

// InputGroups returns the logical groupings for inputs.
func InputGroups() []string {
	return []string{"vision", "environment", "internal"}
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

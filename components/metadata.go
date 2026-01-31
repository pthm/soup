package components

// FieldDescriptor describes a component field for UI display.
type FieldDescriptor struct {
	ID          string  // Unique identifier
	Label       string  // Display name
	Format      string  // Printf format (e.g., "%.2f")
	Min         float32 // Minimum value (for bars)
	Max         float32 // Maximum value (for bars)
	IsCentered  bool    // True for centered bar display
	IsBar       bool    // True to render as progress bar
	ShowWhenZero bool   // Show even when value is zero
	Group       string  // Logical grouping
}

// String returns the display name for an AllocationMode.
func (m AllocationMode) String() string {
	names := AllocationModeNames()
	if int(m) < len(names) {
		return names[m]
	}
	return "Unknown"
}

// AllocationModeNames returns the display names for all allocation modes.
// The order matches the AllocationMode constants.
func AllocationModeNames() []string {
	return []string{"Survive", "Grow", "Breed", "Store"}
}

// AllocationModeCount returns the number of allocation modes.
func AllocationModeCount() int {
	return len(AllocationModeNames())
}

// OrganismFieldDescriptors returns metadata for Organism fields.
func OrganismFieldDescriptors() []FieldDescriptor {
	return []FieldDescriptor{
		{ID: "energy", Label: "Energy", Format: "%.0f/%.0f", Min: 0, Max: 1, IsBar: true, ShowWhenZero: true, Group: "stats"},
		{ID: "cells", Label: "Cells", Format: "%d", Group: "stats"},
		{ID: "max_speed", Label: "Max Speed", Format: "%.2f", Group: "stats"},
		{ID: "mode", Label: "Mode", Group: "stats"},
		{ID: "heading", Label: "Heading", Format: "%.2f", Min: -3.14159, Max: 3.14159, IsCentered: true, Group: "motion"},
		{ID: "active_thrust", Label: "Thrust", Format: "%.2f", Min: 0, Max: 1, IsBar: true, Group: "motion"},
	}
}

// CapabilityFieldDescriptors returns metadata for Capabilities fields.
func CapabilityFieldDescriptors() []FieldDescriptor {
	return []FieldDescriptor{
		{ID: "diet", Label: "Diet", Format: "%.2f", Min: 0, Max: 1, IsBar: true, ShowWhenZero: true, Group: "core"},
		{ID: "actuator_weight", Label: "Actuator", Format: "%.2f", Min: 0, Max: 5, IsBar: true, Group: "function"},
		{ID: "sensor_weight", Label: "Sensor", Format: "%.2f", Min: 0, Max: 5, IsBar: true, Group: "function"},
		{ID: "mouth_size", Label: "Mouth", Format: "%.2f", Min: 0, Max: 5, IsBar: true, Group: "function"},
		{ID: "armor", Label: "Armor", Format: "%.2f", Min: 0, Max: 1, IsBar: true, Group: "modifier"},
		{ID: "storage", Label: "Storage", Format: "%.2f", Min: 0, Max: 1, IsBar: true, Group: "modifier"},
		{ID: "reproductive", Label: "Repro", Format: "%.2f", Min: 0, Max: 5, IsBar: true, Group: "function"},
		{ID: "composition", Label: "Composition", Format: "%.2f", Min: 0, Max: 1, IsBar: true, ShowWhenZero: true, Group: "derived"},
		{ID: "repro_mode", Label: "Repro Mode", Format: "%.2f", Min: 0, Max: 1, IsBar: true, ShowWhenZero: true, Group: "derived"},
	}
}

// ShapeMetricsFieldDescriptors returns metadata for ShapeMetrics fields.
func ShapeMetricsFieldDescriptors() []FieldDescriptor {
	return []FieldDescriptor{
		{ID: "drag", Label: "Drag", Format: "%.2f", Min: 0.2, Max: 2.0, IsBar: true, Group: "shape"},
	}
}

// BrainOutputFieldDescriptors returns metadata for brain output fields on Organism.
// These correspond to the runtime motor outputs stored on the organism.
// Field IDs must match cases in GetOrganismValue().
func BrainOutputFieldDescriptors() []FieldDescriptor {
	return []FieldDescriptor{
		{ID: "u_fwd", Label: "Fwd Vel", Format: "%+.2f", Min: -1, Max: 1, IsCentered: true, IsBar: true, Group: "movement"},
		{ID: "u_up", Label: "Lat Vel", Format: "%+.2f", Min: -1, Max: 1, IsCentered: true, IsBar: true, Group: "movement"},
		{ID: "attack_intent", Label: "Attack", Format: "%.2f", Min: 0, Max: 1, IsBar: true, Group: "action"},
		{ID: "mate_intent", Label: "Mate", Format: "%.2f", Min: 0, Max: 1, IsBar: true, Group: "action"},
	}
}

// CapabilityGroups returns the logical groupings for capability fields.
func CapabilityGroups() []string {
	return []string{"core", "function", "modifier", "derived"}
}

// OrganismGroups returns the logical groupings for organism fields.
func OrganismGroups() []string {
	return []string{"stats", "motion"}
}

// GetCapabilityValue extracts a capability field value by ID.
func GetCapabilityValue(caps *Capabilities, fieldID string) float32 {
	switch fieldID {
	case "diet":
		return caps.DigestiveSpectrum()
	case "actuator_weight":
		return caps.ActuatorWeight
	case "sensor_weight":
		return caps.SensorWeight
	case "mouth_size":
		return caps.MouthSize
	case "armor":
		return caps.StructuralArmor
	case "storage":
		return caps.StorageCapacity
	case "reproductive":
		return caps.ReproductiveWeight
	case "composition":
		return caps.Composition()
	case "repro_mode":
		return caps.ReproductiveMode()
	default:
		return 0
	}
}

// GetOrganismValue extracts an organism field value by ID.
func GetOrganismValue(org *Organism, fieldID string) float32 {
	switch fieldID {
	case "energy":
		return org.Energy
	case "max_energy":
		return org.MaxEnergy
	case "max_speed":
		return org.MaxSpeed
	case "heading":
		return org.Heading
	case "active_thrust":
		return org.ActiveThrust
	case "u_fwd":
		return org.UFwd
	case "u_up":
		return org.UUp
	case "attack_intent":
		return org.AttackIntent
	case "mate_intent":
		return org.MateIntent
	case "eat_intent":
		return org.EatIntent
	case "breed_intent":
		return org.BreedIntent
	default:
		return 0
	}
}

// GetShapeMetricsValue extracts a shape metrics field value by ID.
func GetShapeMetricsValue(sm *ShapeMetrics, fieldID string) float32 {
	switch fieldID {
	case "drag":
		return sm.Drag
	default:
		return 0
	}
}

// GetBrainInput returns the brain input value at the given index.
// Index must be 0-25 (matching BrainInputs = 26).
func GetBrainInput(org *Organism, index int) float32 {
	if index < 0 || index >= 26 {
		return 0
	}
	return org.LastInputs[index]
}

package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// OverlayID uniquely identifies an overlay.
type OverlayID string

// Standard overlay IDs.
const (
	OverlaySpeciesColors    OverlayID = "species_colors"
	OverlayCapabilityColors OverlayID = "capability_colors"
	OverlayPerceptionCones  OverlayID = "perception_cones"
	OverlayLightMap         OverlayID = "light_map"
	OverlayFlowField        OverlayID = "flow_field"
	OverlayPathfinding      OverlayID = "pathfinding"
	OverlayCellTypes        OverlayID = "cell_types"
	OverlayCollisionBoxes   OverlayID = "collision_boxes"
	OverlayOrientation      OverlayID = "orientation"
)

// OverlayDescriptor defines an overlay that can be toggled.
type OverlayDescriptor struct {
	ID          OverlayID   // Unique identifier
	Name        string      // Display name
	Description string      // What this overlay shows
	Key         int32       // Keyboard key to toggle (0 = no key)
	KeyLabel    string      // Key label for display (e.g., "S", "V")
	Category    string      // Grouping (e.g., "visual", "debug", "ai")
	Exclusive   []OverlayID // Other overlays to disable when this is enabled
}

// OverlayRegistry manages overlay state and metadata.
type OverlayRegistry struct {
	descriptors []OverlayDescriptor
	byID        map[OverlayID]OverlayDescriptor
	enabled     map[OverlayID]bool
	order       []OverlayID // Maintains insertion order for display
}

// NewOverlayRegistry creates a registry with default overlays.
func NewOverlayRegistry() *OverlayRegistry {
	reg := &OverlayRegistry{
		byID:    make(map[OverlayID]OverlayDescriptor),
		enabled: make(map[OverlayID]bool),
	}
	reg.registerDefaults()
	return reg
}

// registerDefaults adds standard overlays.
func (r *OverlayRegistry) registerDefaults() {
	// Visual overlays
	r.Register(OverlayDescriptor{
		ID:          OverlaySpeciesColors,
		Name:        "Species Colors",
		Description: "Color organisms by NEAT species",
		Key:         rl.KeyS,
		KeyLabel:    "S",
		Category:    "visual",
		Exclusive:   []OverlayID{OverlayCapabilityColors},
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayCapabilityColors,
		Name:        "Capability Colors",
		Description: "Color organisms by diet spectrum",
		Key:         rl.KeyD,
		KeyLabel:    "D",
		Category:    "visual",
		Exclusive:   []OverlayID{OverlaySpeciesColors},
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayCellTypes,
		Name:        "Cell Types",
		Description: "Show cell function types with colors",
		Key:         rl.KeyT,
		KeyLabel:    "T",
		Category:    "visual",
	})

	// Perception overlays
	r.Register(OverlayDescriptor{
		ID:          OverlayPerceptionCones,
		Name:        "Vision Cones",
		Description: "Show perception cones for selected organism",
		Key:         rl.KeyV,
		KeyLabel:    "V",
		Category:    "perception",
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayLightMap,
		Name:        "Light Map",
		Description: "Visualize ambient light distribution",
		Key:         rl.KeyL,
		KeyLabel:    "L",
		Category:    "perception",
	})

	// Debug overlays
	r.Register(OverlayDescriptor{
		ID:          OverlayFlowField,
		Name:        "Flow Field",
		Description: "Show current/wind flow vectors",
		Key:         rl.KeyG,
		KeyLabel:    "G",
		Category:    "debug",
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayPathfinding,
		Name:        "Pathfinding",
		Description: "Show desire vs actual movement vectors",
		Key:         rl.KeyP,
		KeyLabel:    "P",
		Category:    "debug",
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayCollisionBoxes,
		Name:        "Collision Boxes",
		Description: "Show organism bounding boxes",
		Key:         rl.KeyB,
		KeyLabel:    "B",
		Category:    "debug",
	})

	r.Register(OverlayDescriptor{
		ID:          OverlayOrientation,
		Name:        "Orientation",
		Description: "Show forward/right axes and sensor/actuator cells",
		Key:         rl.KeyX,
		KeyLabel:    "X",
		Category:    "debug",
	})
}

// Register adds an overlay to the registry.
func (r *OverlayRegistry) Register(desc OverlayDescriptor) {
	r.descriptors = append(r.descriptors, desc)
	r.byID[desc.ID] = desc
	r.order = append(r.order, desc.ID)
	r.enabled[desc.ID] = false
}

// Toggle switches an overlay on/off and handles exclusivity.
func (r *OverlayRegistry) Toggle(id OverlayID) bool {
	desc, ok := r.byID[id]
	if !ok {
		return false
	}

	newState := !r.enabled[id]
	r.enabled[id] = newState

	// If enabling, disable exclusive overlays
	if newState {
		for _, excl := range desc.Exclusive {
			r.enabled[excl] = false
		}
	}

	return newState
}

// SetEnabled explicitly sets an overlay's state.
func (r *OverlayRegistry) SetEnabled(id OverlayID, enabled bool) {
	desc, ok := r.byID[id]
	if !ok {
		return
	}

	r.enabled[id] = enabled

	// If enabling, disable exclusive overlays
	if enabled {
		for _, excl := range desc.Exclusive {
			r.enabled[excl] = false
		}
	}
}

// IsEnabled returns whether an overlay is active.
func (r *OverlayRegistry) IsEnabled(id OverlayID) bool {
	return r.enabled[id]
}

// Get returns an overlay descriptor by ID.
func (r *OverlayRegistry) Get(id OverlayID) (OverlayDescriptor, bool) {
	desc, ok := r.byID[id]
	return desc, ok
}

// All returns all registered overlays in registration order.
func (r *OverlayRegistry) All() []OverlayDescriptor {
	return r.descriptors
}

// ByCategory returns overlays filtered by category.
func (r *OverlayRegistry) ByCategory(category string) []OverlayDescriptor {
	var result []OverlayDescriptor
	for _, desc := range r.descriptors {
		if desc.Category == category {
			result = append(result, desc)
		}
	}
	return result
}

// Categories returns all unique categories in order.
func (r *OverlayRegistry) Categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, desc := range r.descriptors {
		if !seen[desc.Category] {
			seen[desc.Category] = true
			cats = append(cats, desc.Category)
		}
	}
	return cats
}

// HandleKeyPress checks if a key corresponds to an overlay toggle.
// Returns the overlay ID and new state if a toggle occurred.
func (r *OverlayRegistry) HandleKeyPress(key int32) (OverlayID, bool, bool) {
	for _, desc := range r.descriptors {
		if desc.Key == key {
			newState := r.Toggle(desc.ID)
			return desc.ID, newState, true
		}
	}
	return "", false, false
}

// EnabledOverlays returns a list of currently enabled overlay IDs.
func (r *OverlayRegistry) EnabledOverlays() []OverlayID {
	var result []OverlayID
	for _, id := range r.order {
		if r.enabled[id] {
			result = append(result, id)
		}
	}
	return result
}

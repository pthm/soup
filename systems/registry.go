package systems

// SystemInfo describes a simulation system for UI display.
type SystemInfo struct {
	ID          string // Internal identifier (used for perf tracking)
	Name        string // Display name
	Description string // What this system does
	Category    string // Grouping (e.g., "core", "visual", "ai")
}

// SystemRegistry holds metadata about all systems.
// This centralizes system naming so the UI and perf tracker stay in sync.
type SystemRegistry struct {
	systems []SystemInfo
	byID    map[string]SystemInfo
}

// NewSystemRegistry creates a registry with all known systems.
func NewSystemRegistry() *SystemRegistry {
	reg := &SystemRegistry{
		byID: make(map[string]SystemInfo),
	}
	reg.registerDefaults()
	return reg
}

// registerDefaults adds all known systems to the registry.
// Update this when adding new systems.
func (r *SystemRegistry) registerDefaults() {
	// Core simulation systems
	r.Register(SystemInfo{ID: "dayNight", Name: "Day/Night", Description: "Updates light cycle", Category: "environment"})
	r.Register(SystemInfo{ID: "flowField", Name: "Flow Field", Description: "Updates current/wind patterns", Category: "environment"})
	r.Register(SystemInfo{ID: "shadowMap", Name: "Shadow Map", Description: "Computes light distribution", Category: "environment"})
	r.Register(SystemInfo{ID: "spatialGrid", Name: "Spatial Grid", Description: "Updates neighbor lookup grid", Category: "core"})

	// AI and behavior
	r.Register(SystemInfo{ID: "allocation", Name: "Allocation", Description: "Determines energy priorities", Category: "ai"})
	r.Register(SystemInfo{ID: "behavior", Name: "Behavior", Description: "Neural network evaluation and steering", Category: "ai"})
	r.Register(SystemInfo{ID: "pathfinding", Name: "Pathfinding", Description: "Computes navigation paths", Category: "ai"})

	// Physics and movement
	r.Register(SystemInfo{ID: "physics", Name: "Physics", Description: "Applies forces and updates positions", Category: "physics"})
	r.Register(SystemInfo{ID: "collision", Name: "Collision", Description: "Handles organism collisions", Category: "physics"})

	// Life cycle
	r.Register(SystemInfo{ID: "feeding", Name: "Feeding", Description: "Processes eating and digestion", Category: "lifecycle"})
	r.Register(SystemInfo{ID: "energy", Name: "Energy", Description: "Calculates metabolism and photosynthesis", Category: "lifecycle"})
	r.Register(SystemInfo{ID: "cells", Name: "Cells", Description: "Ages and maintains cells", Category: "lifecycle"})
	r.Register(SystemInfo{ID: "growth", Name: "Growth", Description: "Grows new cells", Category: "lifecycle"})
	r.Register(SystemInfo{ID: "breeding", Name: "Breeding", Description: "Handles reproduction and genetics", Category: "lifecycle"})

	// Flora-specific
	r.Register(SystemInfo{ID: "floraSystem", Name: "Flora", Description: "Updates plant organisms", Category: "flora"})
	r.Register(SystemInfo{ID: "spores", Name: "Spores", Description: "Manages spore dispersal", Category: "flora"})

	// Data collection (internal)
	r.Register(SystemInfo{ID: "collectOccluders", Name: "Collect Occluders", Description: "Gathers shadow casters", Category: "internal"})
	r.Register(SystemInfo{ID: "collectPositions", Name: "Collect Positions", Description: "Gathers entity positions", Category: "internal"})

	// Visual
	r.Register(SystemInfo{ID: "particles", Name: "Particles", Description: "Updates visual effects", Category: "visual"})

	// Cleanup
	r.Register(SystemInfo{ID: "cleanup", Name: "Cleanup", Description: "Removes dead entities", Category: "core"})
}

// Register adds a system to the registry.
func (r *SystemRegistry) Register(info SystemInfo) {
	r.systems = append(r.systems, info)
	r.byID[info.ID] = info
}

// Get returns system info by ID.
func (r *SystemRegistry) Get(id string) (SystemInfo, bool) {
	info, ok := r.byID[id]
	return info, ok
}

// GetName returns the display name for a system ID.
// Falls back to the ID itself if not found.
func (r *SystemRegistry) GetName(id string) string {
	if info, ok := r.byID[id]; ok {
		return info.Name
	}
	return id
}

// All returns all registered systems.
func (r *SystemRegistry) All() []SystemInfo {
	return r.systems
}

// ByCategory returns systems filtered by category.
func (r *SystemRegistry) ByCategory(category string) []SystemInfo {
	var result []SystemInfo
	for _, info := range r.systems {
		if info.Category == category {
			result = append(result, info)
		}
	}
	return result
}

// Categories returns all unique categories.
func (r *SystemRegistry) Categories() []string {
	seen := make(map[string]bool)
	var cats []string
	for _, info := range r.systems {
		if !seen[info.Category] {
			seen[info.Category] = true
			cats = append(cats, info.Category)
		}
	}
	return cats
}

// IDs returns all system IDs in registration order.
func (r *SystemRegistry) IDs() []string {
	ids := make([]string, len(r.systems))
	for i, info := range r.systems {
		ids[i] = info.ID
	}
	return ids
}

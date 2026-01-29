package systems

import (
	"math"
	"math/rand"
)

// Flora constants
const (
	// Energy/timing
	floraBasePhotoRate  = float32(0.3)  // Base photosynthesis rate per tick
	floraRootedMinLight = float32(0.3)  // Minimum effective light for rooted flora
	floraFloatMinLight  = float32(0.0)  // Minimum effective light for floating flora
	floraSporeInterval  = int32(400)    // Ticks between spore releases
	floraDeathThreshold = float32(0.10) // Die below 10% max energy
	floraDefaultArmor   = float32(0.1)  // Default structural armor for feeding calc

	// Population caps
	maxRootedFlora   = 500
	maxFloatingFlora = 300

	// Default values
	defaultFloraMaxEnergy = float32(150)
	defaultFloraSize      = float32(5)
	defaultFloraEnergy    = float32(80)
)

// RootedFlora represents a fixed-position flora organism (attached to terrain/seafloor).
type RootedFlora struct {
	X, Y       float32 // Fixed world position
	Energy     float32 // Current energy (0-MaxEnergy)
	MaxEnergy  float32 // Capacity (default 150)
	Size       float32 // Visual size (default 5)
	SporeTimer int32   // Ticks until next spore
	Dead       bool
}

// FloatingFlora represents a drifting flora organism in the water column.
type FloatingFlora struct {
	X, Y       float32 // World position
	VelX, VelY float32 // Velocity (driven by flow field)
	Energy     float32
	MaxEnergy  float32
	Size       float32
	SporeTimer int32
	Dead       bool
}

// FloraRef is a reference to a flora for spatial queries and feeding.
type FloraRef struct {
	Index    int
	IsRooted bool
	X, Y     float32
	Energy   float32
}

// FloraSystem manages lightweight flora outside the ECS.
type FloraSystem struct {
	Rooted   []RootedFlora
	Floating []FloatingFlora

	bounds    Bounds
	terrain   *TerrainSystem
	shadowMap *ShadowMap
	flowField *FlowFieldSystem
	noise     *PerlinNoise
	tick      int32
}

// NewFloraSystem creates a new flora management system.
func NewFloraSystem(bounds Bounds, terrain *TerrainSystem, shadowMap *ShadowMap, flowField *FlowFieldSystem) *FloraSystem {
	return &FloraSystem{
		Rooted:    make([]RootedFlora, 0, maxRootedFlora),
		Floating:  make([]FloatingFlora, 0, maxFloatingFlora),
		bounds:    bounds,
		terrain:   terrain,
		shadowMap: shadowMap,
		flowField: flowField,
		noise:     NewPerlinNoise(rand.Int63()),
	}
}

// Update processes photosynthesis, drift, spore timers, and death for all flora.
// spawnSpore callback is called when a flora is ready to release a spore.
func (fs *FloraSystem) Update(tick int32, spawnSpore func(x, y float32, isRooted bool)) {
	fs.tick = tick

	// Update rooted flora
	fs.updateRooted(spawnSpore)

	// Update floating flora
	fs.updateFloating(spawnSpore)
}

func (fs *FloraSystem) updateRooted(spawnSpore func(x, y float32, isRooted bool)) {
	alive := 0
	for i := range fs.Rooted {
		f := &fs.Rooted[i]

		if f.Dead {
			continue
		}

		// Photosynthesis
		light := fs.shadowMap.SampleLight(f.X, f.Y)
		if light < floraRootedMinLight {
			light = floraRootedMinLight // Rooted flora adapted to shade
		}
		f.Energy += floraBasePhotoRate * light

		// Clamp energy
		if f.Energy > f.MaxEnergy {
			f.Energy = f.MaxEnergy
		}

		// Death check
		if f.Energy < f.MaxEnergy*floraDeathThreshold {
			f.Dead = true
			continue
		}

		// Spore timer
		f.SporeTimer++
		if f.SporeTimer >= floraSporeInterval && f.Energy > 40 {
			f.SporeTimer = 0
			f.Energy -= 15 // Spore cost
			if spawnSpore != nil {
				spawnSpore(f.X, f.Y-f.Size, true)
			}
		}

		// Keep alive
		fs.Rooted[alive] = fs.Rooted[i]
		alive++
	}
	fs.Rooted = fs.Rooted[:alive]
}

func (fs *FloraSystem) updateFloating(spawnSpore func(x, y float32, isRooted bool)) {
	alive := 0
	for i := range fs.Floating {
		f := &fs.Floating[i]

		if f.Dead {
			continue
		}

		// Photosynthesis (no minimum light for floating flora)
		light := fs.shadowMap.SampleLight(f.X, f.Y)
		if light < floraFloatMinLight {
			light = floraFloatMinLight
		}
		f.Energy += floraBasePhotoRate * light

		// Clamp energy
		if f.Energy > f.MaxEnergy {
			f.Energy = f.MaxEnergy
		}

		// Death check
		if f.Energy < f.MaxEnergy*floraDeathThreshold {
			f.Dead = true
			continue
		}

		// Apply flow field drift
		fs.applyFloatDrift(f)

		// Spore timer
		f.SporeTimer++
		if f.SporeTimer >= floraSporeInterval && f.Energy > 40 {
			f.SporeTimer = 0
			f.Energy -= 15
			if spawnSpore != nil {
				spawnSpore(f.X, f.Y-f.Size, false)
			}
		}

		// Keep alive
		fs.Floating[alive] = fs.Floating[i]
		alive++
	}
	fs.Floating = fs.Floating[:alive]
}

func (fs *FloraSystem) applyFloatDrift(f *FloatingFlora) {
	const flowScale = 0.003
	const timeScale = 0.0001
	const baseStrength = 0.05 // Weak flow effect for flora

	// Get flow from Perlin noise (similar to behavior system)
	noiseX := fs.noise.Noise3D(float64(f.X)*flowScale, float64(f.Y)*flowScale, float64(fs.tick)*timeScale)
	noiseY := fs.noise.Noise3D(float64(f.X)*flowScale+100, float64(f.Y)*flowScale+100, float64(fs.tick)*timeScale)

	flowAngle := noiseX * math.Pi * 2
	flowMagnitude := (noiseY + 1) * 0.5
	flowX := float32(math.Cos(flowAngle) * flowMagnitude * baseStrength)
	flowY := float32(math.Sin(flowAngle) * flowMagnitude * baseStrength)

	// Slight downward drift
	flowY += 0.01

	// Apply to velocity
	f.VelX += flowX
	f.VelY += flowY

	// Drag
	f.VelX *= 0.95
	f.VelY *= 0.95

	// Cap velocity
	velMag := float32(math.Sqrt(float64(f.VelX*f.VelX + f.VelY*f.VelY)))
	if velMag > 0.5 {
		scale := 0.5 / velMag
		f.VelX *= scale
		f.VelY *= scale
	}

	// Update position
	newX := f.X + f.VelX
	newY := f.Y + f.VelY

	// Terrain collision check
	if fs.terrain != nil && fs.terrain.IsSolid(newX, newY) {
		// Bounce off terrain
		if fs.terrain.IsSolid(newX, f.Y) {
			f.VelX *= -0.5
			newX = f.X
		}
		if fs.terrain.IsSolid(f.X, newY) {
			f.VelY *= -0.3
			newY = f.Y
		}
	}

	f.X = newX
	f.Y = newY

	// Horizontal wrap-around
	if f.X < 0 {
		f.X = fs.bounds.Width
	}
	if f.X > fs.bounds.Width {
		f.X = 0
	}

	// Vertical bounds (bounce)
	if f.Y < 0 {
		f.Y = 0
		f.VelY = -f.VelY * 0.5
	}
	if f.Y > fs.bounds.Height {
		f.Y = fs.bounds.Height
		f.VelY = 0
	}
}

// GetNearbyFlora returns flora within radius of position for feeding queries.
func (fs *FloraSystem) GetNearbyFlora(x, y, radius float32) []FloraRef {
	radiusSq := radius * radius
	var refs []FloraRef

	// Check rooted flora
	for i := range fs.Rooted {
		f := &fs.Rooted[i]
		if f.Dead {
			continue
		}
		dx := f.X - x
		dy := f.Y - y
		if dx*dx+dy*dy <= radiusSq {
			refs = append(refs, FloraRef{
				Index:    i,
				IsRooted: true,
				X:        f.X,
				Y:        f.Y,
				Energy:   f.Energy,
			})
		}
	}

	// Check floating flora
	for i := range fs.Floating {
		f := &fs.Floating[i]
		if f.Dead {
			continue
		}
		dx := f.X - x
		dy := f.Y - y
		if dx*dx+dy*dy <= radiusSq {
			refs = append(refs, FloraRef{
				Index:    i,
				IsRooted: false,
				X:        f.X,
				Y:        f.Y,
				Energy:   f.Energy,
			})
		}
	}

	return refs
}

// GetAllFlora returns all living flora for behavior system vision.
func (fs *FloraSystem) GetAllFlora() []FloraRef {
	var refs []FloraRef

	for i := range fs.Rooted {
		f := &fs.Rooted[i]
		if !f.Dead {
			refs = append(refs, FloraRef{
				Index:    i,
				IsRooted: true,
				X:        f.X,
				Y:        f.Y,
				Energy:   f.Energy,
			})
		}
	}

	for i := range fs.Floating {
		f := &fs.Floating[i]
		if !f.Dead {
			refs = append(refs, FloraRef{
				Index:    i,
				IsRooted: false,
				X:        f.X,
				Y:        f.Y,
				Energy:   f.Energy,
			})
		}
	}

	return refs
}

// ApplyDamage damages a flora and returns energy extracted.
func (fs *FloraSystem) ApplyDamage(index int, isRooted bool, damage float32) float32 {
	if isRooted {
		if index < 0 || index >= len(fs.Rooted) {
			return 0
		}
		f := &fs.Rooted[index]
		if f.Dead {
			return 0
		}
		extracted := damage
		if extracted > f.Energy {
			extracted = f.Energy
		}
		f.Energy -= extracted
		if f.Energy <= 0 {
			f.Dead = true
		}
		return extracted
	}

	// Floating
	if index < 0 || index >= len(fs.Floating) {
		return 0
	}
	f := &fs.Floating[index]
	if f.Dead {
		return 0
	}
	extracted := damage
	if extracted > f.Energy {
		extracted = f.Energy
	}
	f.Energy -= extracted
	if f.Energy <= 0 {
		f.Dead = true
	}
	return extracted
}

// AddRooted adds a new rooted flora at position.
func (fs *FloraSystem) AddRooted(x, y, energy float32) bool {
	if len(fs.Rooted) >= maxRootedFlora {
		return false // At capacity
	}
	fs.Rooted = append(fs.Rooted, RootedFlora{
		X:          x,
		Y:          y,
		Energy:     energy,
		MaxEnergy:  defaultFloraMaxEnergy,
		Size:       defaultFloraSize,
		SporeTimer: rand.Int31n(floraSporeInterval / 2), // Stagger initial spore timing
		Dead:       false,
	})
	return true
}

// AddFloating adds a new floating flora at position.
func (fs *FloraSystem) AddFloating(x, y, energy float32) bool {
	if len(fs.Floating) >= maxFloatingFlora {
		return false // At capacity
	}
	fs.Floating = append(fs.Floating, FloatingFlora{
		X:          x,
		Y:          y,
		VelX:       (rand.Float32() - 0.5) * 0.1,
		VelY:       (rand.Float32() - 0.5) * 0.1,
		Energy:     energy,
		MaxEnergy:  defaultFloraMaxEnergy,
		Size:       defaultFloraSize,
		SporeTimer: rand.Int31n(floraSporeInterval / 2),
		Dead:       false,
	})
	return true
}

// RootedCount returns the number of living rooted flora.
func (fs *FloraSystem) RootedCount() int {
	count := 0
	for i := range fs.Rooted {
		if !fs.Rooted[i].Dead {
			count++
		}
	}
	return count
}

// FloatingCount returns the number of living floating flora.
func (fs *FloraSystem) FloatingCount() int {
	count := 0
	for i := range fs.Floating {
		if !fs.Floating[i].Dead {
			count++
		}
	}
	return count
}

// TotalCount returns the total number of living flora.
func (fs *FloraSystem) TotalCount() int {
	return fs.RootedCount() + fs.FloatingCount()
}

// TotalEnergy returns the total energy across all living flora.
func (fs *FloraSystem) TotalEnergy() float32 {
	var total float32
	for i := range fs.Rooted {
		if !fs.Rooted[i].Dead {
			total += fs.Rooted[i].Energy
		}
	}
	for i := range fs.Floating {
		if !fs.Floating[i].Dead {
			total += fs.Floating[i].Energy
		}
	}
	return total
}

// DefaultFloraArmor returns the default armor value for flora (used by feeding system).
func DefaultFloraArmor() float32 {
	return floraDefaultArmor
}

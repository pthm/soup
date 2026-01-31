package systems

import (
	"math"
	"math/rand"
	"sync"
)

// Flora constants
const (
	// Energy/timing
	floraBaseEnergyRate = float32(0.15) // Base energy gain rate per tick
	floraSporeInterval  = int32(400)    // Ticks between spore releases
	floraDeathThreshold = float32(0.10) // Die below 10% max energy
	floraDefaultArmor   = float32(0.1)  // Default structural armor for feeding calc

	// Population cap
	maxFlora = 800

	// Default values
	defaultFloraMaxEnergy = float32(150)
	defaultFloraSize      = float32(5)
	defaultFloraEnergy    = float32(80)

	// Flow field parameters
	floraFlowForce = float32(0.04) // How strongly flora responds to flow
	floraMaxSpeed  = float32(0.5)  // Maximum flora velocity
	floraDrag      = float32(0.97) // Velocity damping per tick

	// Fauna collision parameters
	floraCollisionRadius  = float32(8.0)  // Base collision distance
	floraPushForce        = float32(0.4)  // Force multiplier for pushing flora
	floraSpeedThreshold   = float32(0.8)  // Below this speed, no push occurs
	floraPushMaxSpeed     = float32(2.0)  // Flora max speed after being pushed
)

// Flora represents a floating flora organism in the water column.
type Flora struct {
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
	Index  int
	X, Y   float32
	Energy float32
}

// FloraSystem manages lightweight flora outside the ECS.
type FloraSystem struct {
	Flora []Flora

	bounds      Bounds
	flowSampler FlowSampler // Shared flow field (GPU-computed)
}

// NewFloraSystem creates a new flora management system.
func NewFloraSystem(bounds Bounds) *FloraSystem {
	return &FloraSystem{
		Flora:  make([]Flora, 0, maxFlora),
		bounds: bounds,
	}
}

// SetFlowSampler sets the shared flow field sampler.
func (fs *FloraSystem) SetFlowSampler(sampler FlowSampler) {
	fs.flowSampler = sampler
}

// sporeRequest holds data for deferred spore spawning.
type sporeRequest struct {
	x, y float32
}

// Update processes energy gain, drift, spore timers, and death for all flora.
// spawnSpore callback is called when a flora is ready to release a spore.
func (fs *FloraSystem) Update(tick int32, spawnSpore func(x, y float32)) {
	fs.updateFlora(spawnSpore)
}

// minFloraForParallel is the minimum flora count to use parallel processing.
const minFloraForParallel = 200

// UpdateParallel processes flora with parallel updates for large populations.
func (fs *FloraSystem) UpdateParallel(tick int32, spawnSpore func(x, y float32)) {
	// Use sequential for small populations
	if len(fs.Flora) < minFloraForParallel {
		fs.updateFlora(spawnSpore)
		return
	}

	// Parallel processing for large populations
	var wg sync.WaitGroup
	var spores []sporeRequest

	wg.Add(1)
	go func() {
		defer wg.Done()
		spores = fs.updateFloraParallel()
	}()
	wg.Wait()

	// Spawn spores sequentially (callback may not be thread-safe)
	if spawnSpore != nil {
		for _, s := range spores {
			spawnSpore(s.x, s.y)
		}
	}
}

func (fs *FloraSystem) updateFloraParallel() []sporeRequest {
	var spores []sporeRequest
	alive := 0
	for i := range fs.Flora {
		f := &fs.Flora[i]

		if f.Dead {
			continue
		}

		// Energy gain
		f.Energy += floraBaseEnergyRate
		if f.Energy > f.MaxEnergy {
			f.Energy = f.MaxEnergy
		}

		if f.Energy < f.MaxEnergy*floraDeathThreshold {
			f.Dead = true
			continue
		}

		fs.applyFlowField(f)

		f.SporeTimer++
		if f.SporeTimer >= floraSporeInterval && f.Energy > 40 {
			f.SporeTimer = 0
			f.Energy -= 15
			spores = append(spores, sporeRequest{f.X, f.Y - f.Size})
		}

		fs.Flora[alive] = fs.Flora[i]
		alive++
	}
	fs.Flora = fs.Flora[:alive]
	return spores
}

func (fs *FloraSystem) updateFlora(spawnSpore func(x, y float32)) {
	alive := 0
	for i := range fs.Flora {
		f := &fs.Flora[i]

		if f.Dead {
			continue
		}

		// Energy gain
		f.Energy += floraBaseEnergyRate
		if f.Energy > f.MaxEnergy {
			f.Energy = f.MaxEnergy
		}

		// Death check
		if f.Energy < f.MaxEnergy*floraDeathThreshold {
			f.Dead = true
			continue
		}

		// Apply flow field
		fs.applyFlowField(f)

		// Spore timer
		f.SporeTimer++
		if f.SporeTimer >= floraSporeInterval && f.Energy > 40 {
			f.SporeTimer = 0
			f.Energy -= 15
			if spawnSpore != nil {
				spawnSpore(f.X, f.Y-f.Size)
			}
		}

		// Keep alive
		fs.Flora[alive] = fs.Flora[i]
		alive++
	}
	fs.Flora = fs.Flora[:alive]
}

// applyFlowField samples the shared flow field and applies it to flora velocity.
func (fs *FloraSystem) applyFlowField(f *Flora) {
	if fs.flowSampler != nil {
		// Sample the unified GPU flow field
		flowX, flowY := fs.flowSampler.Sample(f.X, f.Y)

		// Apply flow force to velocity
		f.VelX += flowX * floraFlowForce
		f.VelY += flowY * floraFlowForce
	}

	// Drag
	f.VelX *= floraDrag
	f.VelY *= floraDrag

	// Limit speed
	speed := float32(math.Sqrt(float64(f.VelX*f.VelX + f.VelY*f.VelY)))
	if speed > floraMaxSpeed {
		f.VelX = f.VelX / speed * floraMaxSpeed
		f.VelY = f.VelY / speed * floraMaxSpeed
	}

	// Update position
	f.X += f.VelX
	f.Y += f.VelY

	// Wrap around edges (toroidal space)
	if f.X < 0 {
		f.X += fs.bounds.Width
	}
	if f.X > fs.bounds.Width {
		f.X -= fs.bounds.Width
	}
	if f.Y < 0 {
		f.Y += fs.bounds.Height
	}
	if f.Y > fs.bounds.Height {
		f.Y -= fs.bounds.Height
	}
}

// GetNearbyFlora returns flora within radius of position for feeding queries.
func (fs *FloraSystem) GetNearbyFlora(x, y, radius float32) []FloraRef {
	radiusSq := radius * radius
	var refs []FloraRef

	for i := range fs.Flora {
		f := &fs.Flora[i]
		if !f.Dead && distanceSq(f.X, f.Y, x, y) <= radiusSq {
			refs = append(refs, FloraRef{Index: i, X: f.X, Y: f.Y, Energy: f.Energy})
		}
	}

	return refs
}

// GetAllFlora returns all living flora for behavior system vision.
func (fs *FloraSystem) GetAllFlora() []FloraRef {
	refs := make([]FloraRef, 0, len(fs.Flora))

	for i := range fs.Flora {
		f := &fs.Flora[i]
		if !f.Dead {
			refs = append(refs, FloraRef{Index: i, X: f.X, Y: f.Y, Energy: f.Energy})
		}
	}

	return refs
}

// ApplyDamage damages a flora and returns energy extracted.
func (fs *FloraSystem) ApplyDamage(index int, damage float32) float32 {
	if index < 0 || index >= len(fs.Flora) {
		return 0
	}
	f := &fs.Flora[index]
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

// Add adds a new flora at position.
func (fs *FloraSystem) Add(x, y, energy float32) bool {
	if len(fs.Flora) >= maxFlora {
		return false
	}
	fs.Flora = append(fs.Flora, Flora{
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

// AddRooted is a compatibility alias for Add.
func (fs *FloraSystem) AddRooted(x, y, energy float32) bool {
	return fs.Add(x, y, energy)
}

// AddFloating is a compatibility alias for Add.
func (fs *FloraSystem) AddFloating(x, y, energy float32) bool {
	return fs.Add(x, y, energy)
}

// Count returns the number of living flora.
func (fs *FloraSystem) Count() int {
	count := 0
	for i := range fs.Flora {
		if !fs.Flora[i].Dead {
			count++
		}
	}
	return count
}

// RootedCount returns 0 (no rooted flora anymore).
func (fs *FloraSystem) RootedCount() int {
	return 0
}

// FloatingCount returns the count (all flora are floating now).
func (fs *FloraSystem) FloatingCount() int {
	return fs.Count()
}

// TotalCount returns the total number of living flora.
func (fs *FloraSystem) TotalCount() int {
	return fs.Count()
}

// TotalEnergy returns the total energy across all living flora.
func (fs *FloraSystem) TotalEnergy() float32 {
	var total float32
	for i := range fs.Flora {
		if !fs.Flora[i].Dead {
			total += fs.Flora[i].Energy
		}
	}
	return total
}

// DefaultFloraArmor returns the default armor value for flora.
func DefaultFloraArmor() float32 {
	return floraDefaultArmor
}

// FaunaCollider holds position and velocity data for fauna collision checks.
type FaunaCollider struct {
	X, Y       float32
	VelX, VelY float32
	Radius     float32
}

// ApplyFaunaCollisions pushes flora away from fast-moving fauna.
// This encourages organisms to approach flora slowly to feed.
func (fs *FloraSystem) ApplyFaunaCollisions(fauna []FaunaCollider) {
	for i := range fs.Flora {
		f := &fs.Flora[i]
		if f.Dead {
			continue
		}

		for j := range fauna {
			fc := &fauna[j]

			// Calculate distance (with toroidal wrapping)
			dx := f.X - fc.X
			dy := f.Y - fc.Y

			// Handle toroidal wrapping - check if shorter path is across boundary
			if dx > fs.bounds.Width/2 {
				dx -= fs.bounds.Width
			} else if dx < -fs.bounds.Width/2 {
				dx += fs.bounds.Width
			}
			if dy > fs.bounds.Height/2 {
				dy -= fs.bounds.Height
			} else if dy < -fs.bounds.Height/2 {
				dy += fs.bounds.Height
			}

			distSq := dx*dx + dy*dy
			collisionDist := floraCollisionRadius + f.Size + fc.Radius
			if distSq >= collisionDist*collisionDist {
				continue
			}

			// Calculate fauna speed
			faunaSpeed := float32(math.Sqrt(float64(fc.VelX*fc.VelX + fc.VelY*fc.VelY)))

			// Only push if fauna is moving above threshold
			if faunaSpeed <= floraSpeedThreshold {
				continue
			}

			// Push flora away - stronger push for faster movement
			dist := float32(math.Sqrt(float64(distSq)))
			if dist < 0.1 {
				dist = 0.1 // Avoid division by zero
			}

			// Normalized direction away from fauna
			normalX := dx / dist
			normalY := dy / dist

			// Push strength scales with speed above threshold
			excessSpeed := faunaSpeed - floraSpeedThreshold
			pushStrength := excessSpeed * floraPushForce

			// Apply push: away from fauna center + inherit some of fauna's momentum
			f.VelX += normalX*pushStrength + fc.VelX*0.15
			f.VelY += normalY*pushStrength + fc.VelY*0.15

			// Limit flora speed after push
			floraSpeed := float32(math.Sqrt(float64(f.VelX*f.VelX + f.VelY*f.VelY)))
			if floraSpeed > floraPushMaxSpeed {
				scale := floraPushMaxSpeed / floraSpeed
				f.VelX *= scale
				f.VelY *= scale
			}
		}
	}
}

package systems

import (
	"math"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/pthm-cable/soup/config"
)

// ParticleResourceField implements a mass-conserving resource system with:
// - Potential field P(x,y): slow-evolving FBM determining where new mass enters
// - Flow field (U,V): curl noise creating divergence-free currents
// - Resource grid R(x,y): mass density that organisms consume from
// - Particles: mass-carrying packets that drift with flow and exchange mass with grid
//
// Mass exchange: Particles deposit mass to grid, pick up mass from grid (entrainment).
// Organisms interact only with the grid via Graze().
type ParticleResourceField struct {
	// Particle data (SoA layout for cache efficiency)
	X, Y   []float32 // positions
	Mass   []float32 // mass carried by each particle
	Active []bool    // is this slot in use?

	// Pool management
	Count      int   // current active particle count
	MaxCount   int   // maximum particles
	FreeList   []int // recycled indices
	ActiveList []int // compact list of active particle indices

	// Potential field P (spawn rate map)
	Pot        []float32
	PotW, PotH int

	// Dual flow fields for smooth interpolation
	FlowU0, FlowV0 []float32 // Current "from" flow field
	FlowU1, FlowV1 []float32 // Target "to" flow field
	FlowW, FlowH   int
	FlowBlend      float32 // Interpolation factor: 0=from, 1=to

	// Blended flow field for rendering (updated each step)
	FlowUBlend, FlowVBlend []float32

	// Async flow field generation
	nextFlowU, nextFlowV []float32    // Buffer for async generation
	nextFlowTime         float32      // Time value for next field
	nextFlowReady        atomic.Bool  // Signals async generation complete
	flowGenMu            sync.Mutex   // Protects async buffer access
	generatingFlow       atomic.Bool  // Is generation in progress?

	// Resource grid R (mass density - what organisms consume)
	Res        []float32
	ResW, ResH int

	// World dimensions
	worldW, worldH float32

	// Timing
	Time          float32
	lastPotUpdate float32

	// Noise parameters
	Seed       uint32
	Scale      float32 // base noise scale
	Octaves    int
	Lacunarity float32
	Gain       float32
	Contrast   float32

	// Flow-specific
	FlowScale     float32
	FlowOctaves   int
	FlowEvolution float32
	FlowStrength  float32

	// Exchange rates
	SpawnRate   float32 // particles/sec base rate (scaled by P)
	DepositRate float32 // fraction of mass deposited to grid per sec
	PickupRate  float32 // mass pickup rate from grid per sec
	InitialMass float32 // mass of newly spawned particle

	// Update intervals
	FlowUpdateSec float32
	PotUpdateSec  float32

	rng *rand.Rand
}

// NewParticleResourceField creates a new particle-based resource field.
func NewParticleResourceField(gridW, gridH int, worldW, worldH float32, seed int64) *ParticleResourceField {
	cfg := config.Cfg().Particles

	maxCount := cfg.MaxCount
	if maxCount < 1 {
		maxCount = 8000
	}

	// Flow field uses separate (typically lower) resolution
	flowSize := cfg.FlowGridSize
	if flowSize < 1 {
		flowSize = 32 // Default to 32x32 if not configured
	}

	pf := &ParticleResourceField{
		// Particle arrays
		X:          make([]float32, maxCount),
		Y:          make([]float32, maxCount),
		Mass:       make([]float32, maxCount),
		Active:     make([]bool, maxCount),
		MaxCount:   maxCount,
		FreeList:   make([]int, 0, maxCount),
		ActiveList: make([]int, 0, maxCount),

		// Potential field (same resolution as resource grid)
		Pot:  make([]float32, gridW*gridH),
		PotW: gridW,
		PotH: gridH,

		// Dual flow fields for interpolation (lower resolution)
		FlowU0: make([]float32, flowSize*flowSize),
		FlowV0: make([]float32, flowSize*flowSize),
		FlowU1: make([]float32, flowSize*flowSize),
		FlowV1: make([]float32, flowSize*flowSize),
		FlowW:  flowSize,
		FlowH:  flowSize,

		// Blended flow field for rendering
		FlowUBlend: make([]float32, flowSize*flowSize),
		FlowVBlend: make([]float32, flowSize*flowSize),

		// Async generation buffers
		nextFlowU: make([]float32, flowSize*flowSize),
		nextFlowV: make([]float32, flowSize*flowSize),

		// Resource grid
		Res:  make([]float32, gridW*gridH),
		ResW: gridW,
		ResH: gridH,

		worldW: worldW,
		worldH: worldH,

		// Noise params (match CPU resource field for consistency)
		Seed:       uint32(seed),
		Scale:      4.0,
		Octaves:    4,
		Lacunarity: 2.0,
		Gain:       0.5,
		Contrast:   float32(config.Cfg().Resource.Contrast),

		// Flow params from config
		FlowScale:     float32(cfg.FlowScale),
		FlowOctaves:   cfg.FlowOctaves,
		FlowEvolution: float32(cfg.FlowEvolution),
		FlowStrength:  float32(cfg.FlowStrength),

		// Exchange rates from config
		SpawnRate:   float32(cfg.SpawnRate),
		DepositRate: float32(cfg.DepositRate),
		PickupRate:  float32(cfg.PickupRate),
		InitialMass: float32(cfg.InitialMass),

		// Update intervals
		FlowUpdateSec: float32(cfg.FlowUpdateSec),
		PotUpdateSec:  float32(cfg.PotUpdateSec),

		rng: rand.New(rand.NewSource(seed)),
	}

	// Initialize fields
	pf.rebuildPotential(0)

	// Initialize both flow fields (from and to) for smooth start
	pf.generateFlowFieldInto(pf.FlowU0, pf.FlowV0, 0)
	pf.generateFlowFieldInto(pf.FlowU1, pf.FlowV1, pf.FlowUpdateSec*pf.FlowEvolution)
	pf.FlowBlend = 0
	pf.updateFlowBlend() // Initialize blended arrays for rendering

	// Start async generation of the third field so it's ready when we transition
	pf.startAsyncFlowGeneration(pf.FlowUpdateSec * 2)

	// Initialize resource grid from potential (like CPUResourceField does with capacity)
	for i := range pf.Res {
		pf.Res[i] = pf.Pot[i]
	}

	return pf
}

// --- ResourceSampler interface ---

func (pf *ParticleResourceField) Sample(x, y float32) float32 {
	return pf.sampleGrid(pf.Res, pf.ResW, pf.ResH, x, y)
}

func (pf *ParticleResourceField) Width() float32  { return pf.worldW }
func (pf *ParticleResourceField) Height() float32 { return pf.worldH }

func (pf *ParticleResourceField) Graze(x, y float32, rate, dt float32, radiusCells int) float32 {
	return pf.grazeFromGrid(x, y, rate, dt, radiusCells)
}

func (pf *ParticleResourceField) Step(dt float32, evolve bool) {
	pf.Time += dt

	if evolve {
		// Smoothly interpolate flow field and trigger async generation
		pf.updateFlowInterpolation(dt)

		// Update potential field periodically
		if pf.Time-pf.lastPotUpdate >= pf.PotUpdateSec {
			pf.rebuildPotential(pf.Time)
			pf.lastPotUpdate = pf.Time
		}
	}

	// Update blended flow field for rendering
	pf.updateFlowBlend()

	// Particle dynamics - use compact active list for O(n) instead of O(MaxCount)
	pf.spawnParticles(dt)
	pf.advectParticlesCompact(dt)
	pf.depositCompact(dt)
	pf.pickupCompact(dt)
	pf.cleanupCompact()
}

func (pf *ParticleResourceField) ResData() []float32 {
	return pf.Res
}

func (pf *ParticleResourceField) GridSize() (int, int) {
	return pf.ResW, pf.ResH
}

// --- Flow Field (Curl Noise with Interpolation) ---

// generateFlowFieldInto computes a flow field at given time into the provided buffers.
func (pf *ParticleResourceField) generateFlowFieldInto(flowU, flowV []float32, tEvolved float32) {
	eps := float32(0.01)

	for y := 0; y < pf.FlowH; y++ {
		v := (float32(y) + 0.5) / float32(pf.FlowH)
		for x := 0; x < pf.FlowW; x++ {
			u := (float32(x) + 0.5) / float32(pf.FlowW)

			// Curl of scalar potential: (dpsi/dv, -dpsi/du)
			psi0 := pf.fbm3D(u, v, tEvolved, pf.FlowScale, pf.FlowOctaves)
			psiDu := pf.fbm3D(u+eps, v, tEvolved, pf.FlowScale, pf.FlowOctaves)
			psiDv := pf.fbm3D(u, v+eps, tEvolved, pf.FlowScale, pf.FlowOctaves)

			i := y*pf.FlowW + x
			flowU[i] = (psiDv - psi0) / eps * pf.FlowStrength
			flowV[i] = -(psiDu - psi0) / eps * pf.FlowStrength
		}
	}
}

// startAsyncFlowGeneration spawns a goroutine to generate the next flow field.
func (pf *ParticleResourceField) startAsyncFlowGeneration(targetTime float32) {
	if pf.generatingFlow.Load() {
		return // Already generating
	}
	pf.generatingFlow.Store(true)
	pf.nextFlowTime = targetTime

	go func() {
		tEvolved := targetTime * pf.FlowEvolution

		// Generate into temporary buffers
		pf.flowGenMu.Lock()
		pf.generateFlowFieldInto(pf.nextFlowU, pf.nextFlowV, tEvolved)
		pf.flowGenMu.Unlock()

		pf.nextFlowReady.Store(true)
		pf.generatingFlow.Store(false)
	}()
}

// updateFlowInterpolation advances flow field blending and handles transitions.
func (pf *ParticleResourceField) updateFlowInterpolation(dt float32) {
	// Advance blend factor
	pf.FlowBlend += dt / pf.FlowUpdateSec
	if pf.FlowBlend < 1.0 {
		return
	}

	// Transition complete: swap fields
	pf.FlowBlend = 0

	// Copy "to" into "from"
	copy(pf.FlowU0, pf.FlowU1)
	copy(pf.FlowV0, pf.FlowV1)

	// If async generation is ready, copy it to "to"
	if pf.nextFlowReady.Load() {
		pf.flowGenMu.Lock()
		copy(pf.FlowU1, pf.nextFlowU)
		copy(pf.FlowV1, pf.nextFlowV)
		pf.flowGenMu.Unlock()
		pf.nextFlowReady.Store(false)

		// Start generating the next field
		nextTime := pf.nextFlowTime + pf.FlowUpdateSec
		pf.startAsyncFlowGeneration(nextTime)
	} else {
		// Async not ready - generate synchronously as fallback
		// This shouldn't happen if FlowUpdateSec is long enough
		nextTime := pf.Time + pf.FlowUpdateSec
		tEvolved := nextTime * pf.FlowEvolution
		pf.generateFlowFieldInto(pf.FlowU1, pf.FlowV1, tEvolved)
		pf.startAsyncFlowGeneration(nextTime + pf.FlowUpdateSec)
	}
}

// sampleFlow returns interpolated flow at world position.
// Uses pre-blended flow arrays (computed once per tick in updateFlowBlend).
func (pf *ParticleResourceField) sampleFlow(x, y float32) (float32, float32) {
	// Compute grid coordinates
	u := pfFract(x / pf.worldW)
	v := pfFract(y / pf.worldH)

	fx := u * float32(pf.FlowW)
	fy := v * float32(pf.FlowH)

	x0 := int(fx)
	y0 := int(fy)
	if x0 >= pf.FlowW {
		x0 = 0
	}
	if y0 >= pf.FlowH {
		y0 = 0
	}

	x1 := x0 + 1
	if x1 >= pf.FlowW {
		x1 = 0
	}
	y1 := y0 + 1
	if y1 >= pf.FlowH {
		y1 = 0
	}

	tx := fx - float32(x0)
	ty := fy - float32(y0)

	// Precompute indices
	i00 := y0*pf.FlowW + x0
	i10 := y0*pf.FlowW + x1
	i01 := y1*pf.FlowW + x0
	i11 := y1*pf.FlowW + x1

	// Sample from pre-blended arrays (2 instead of 4)
	ub := pf.FlowUBlend[i00] + (pf.FlowUBlend[i10]-pf.FlowUBlend[i00])*tx
	ubb := pf.FlowUBlend[i01] + (pf.FlowUBlend[i11]-pf.FlowUBlend[i01])*tx

	vb := pf.FlowVBlend[i00] + (pf.FlowVBlend[i10]-pf.FlowVBlend[i00])*tx
	vbb := pf.FlowVBlend[i01] + (pf.FlowVBlend[i11]-pf.FlowVBlend[i01])*tx

	return ub + (ubb-ub)*ty, vb + (vbb-vb)*ty
}

// updateFlowBlend computes the interpolated flow field for rendering.
func (pf *ParticleResourceField) updateFlowBlend() {
	t := pf.FlowBlend // Linear interpolation for constant rate of change
	for i := range pf.FlowUBlend {
		pf.FlowUBlend[i] = pf.FlowU0[i] + (pf.FlowU1[i]-pf.FlowU0[i])*t
		pf.FlowVBlend[i] = pf.FlowV0[i] + (pf.FlowV1[i]-pf.FlowV0[i])*t
	}
}

// FlowData returns the current blended flow field for rendering.
func (pf *ParticleResourceField) FlowData() ([]float32, []float32) {
	return pf.FlowUBlend, pf.FlowVBlend
}

// --- Potential Field ---

func (pf *ParticleResourceField) rebuildPotential(t float32) {
	// Use time-based drift like CPUResourceField
	driftX := float32(0.02)
	driftY := float32(0.014)
	du := pfFract(t * driftX)
	dv := pfFract(t * driftY)

	for y := 0; y < pf.PotH; y++ {
		v := (float32(y) + 0.5) / float32(pf.PotH)
		v = pfFract(v + dv)
		for x := 0; x < pf.PotW; x++ {
			u := (float32(x) + 0.5) / float32(pf.PotW)
			u = pfFract(u + du)

			pf.Pot[y*pf.PotW+x] = pf.fbm2D(u, v)
		}
	}
}

func (pf *ParticleResourceField) samplePotential(x, y float32) float32 {
	return pf.sampleGrid(pf.Pot, pf.PotW, pf.PotH, x, y)
}

// --- Particle Advection (RK2 Midpoint Method) ---

func (pf *ParticleResourceField) advectParticles(dt float32) {
	for i := 0; i < pf.MaxCount; i++ {
		if !pf.Active[i] {
			continue
		}

		x0, y0 := pf.X[i], pf.Y[i]

		// k1 = flow at current position
		u1, v1 := pf.sampleFlow(x0, y0)

		// Midpoint
		xm := pfWrap(x0+u1*dt*0.5, pf.worldW)
		ym := pfWrap(y0+v1*dt*0.5, pf.worldH)

		// k2 = flow at midpoint
		u2, v2 := pf.sampleFlow(xm, ym)

		// Final position
		pf.X[i] = pfWrap(x0+u2*dt, pf.worldW)
		pf.Y[i] = pfWrap(y0+v2*dt, pf.worldH)
	}
}

// --- Mass Exchange ---

// deposit transfers mass from particles to grid
func (pf *ParticleResourceField) deposit(dt float32) {
	rate := pf.DepositRate * dt
	if rate > 1 {
		rate = 1 // Can't deposit more than 100%
	}

	for i := 0; i < pf.MaxCount; i++ {
		if !pf.Active[i] || pf.Mass[i] <= 0 {
			continue
		}

		// Mass to deposit
		dm := pf.Mass[i] * rate
		pf.Mass[i] -= dm

		// Splat to grid with tent kernel
		pf.splatToGrid(pf.X[i], pf.Y[i], dm)
	}
}

// pickup transfers mass from grid to particles (entrainment)
func (pf *ParticleResourceField) pickup(dt float32) {
	for i := 0; i < pf.MaxCount; i++ {
		if !pf.Active[i] {
			continue
		}

		// Sample local grid density
		r := pf.Sample(pf.X[i], pf.Y[i])
		if r < 0.001 {
			continue
		}

		// Mass to pick up
		dm := pf.PickupRate * r * dt
		if dm > r*0.5 {
			dm = r * 0.5 // Don't take more than half available
		}

		// Transfer: grid -> particle
		pf.removeFromGrid(pf.X[i], pf.Y[i], dm)
		pf.Mass[i] += dm
	}
}

// splatToGrid distributes mass to nearby cells with tent weighting
func (pf *ParticleResourceField) splatToGrid(x, y, mass float32) {
	if mass <= 0 {
		return
	}

	// Convert to grid coordinates
	gx := x / pf.worldW * float32(pf.ResW)
	gy := y / pf.worldH * float32(pf.ResH)

	cx := int(gx)
	cy := int(gy)

	// Fractional part for bilinear weighting
	fx := gx - float32(cx)
	fy := gy - float32(cy)

	// 2x2 bilinear splat (simpler and faster than 3x3 tent)
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	x0 := pfModInt(cx, pf.ResW)
	y0 := pfModInt(cy, pf.ResH)
	x1 := pfModInt(cx+1, pf.ResW)
	y1 := pfModInt(cy+1, pf.ResH)

	pf.Res[y0*pf.ResW+x0] += mass * w00
	pf.Res[y0*pf.ResW+x1] += mass * w10
	pf.Res[y1*pf.ResW+x0] += mass * w01
	pf.Res[y1*pf.ResW+x1] += mass * w11
}

// removeFromGrid removes mass from grid at position
func (pf *ParticleResourceField) removeFromGrid(x, y, mass float32) {
	if mass <= 0 {
		return
	}

	// Convert to grid coordinates
	gx := x / pf.worldW * float32(pf.ResW)
	gy := y / pf.worldH * float32(pf.ResH)

	cx := int(gx)
	cy := int(gy)

	fx := gx - float32(cx)
	fy := gy - float32(cy)

	// Bilinear removal
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	x0 := pfModInt(cx, pf.ResW)
	y0 := pfModInt(cy, pf.ResH)
	x1 := pfModInt(cx+1, pf.ResW)
	y1 := pfModInt(cy+1, pf.ResH)

	// Remove proportionally, clamping to zero
	pf.Res[y0*pf.ResW+x0] = maxf32(0, pf.Res[y0*pf.ResW+x0]-mass*w00)
	pf.Res[y0*pf.ResW+x1] = maxf32(0, pf.Res[y0*pf.ResW+x1]-mass*w10)
	pf.Res[y1*pf.ResW+x0] = maxf32(0, pf.Res[y1*pf.ResW+x0]-mass*w01)
	pf.Res[y1*pf.ResW+x1] = maxf32(0, pf.Res[y1*pf.ResW+x1]-mass*w11)
}

// --- Spawning from Potential ---

func (pf *ParticleResourceField) spawnParticles(dt float32) {
	// Target spawn count this tick
	baseSpawns := pf.SpawnRate * dt

	// Importance sampling from potential field
	// Try multiple samples, accept based on potential value
	maxAttempts := int(baseSpawns * 3)
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	spawned := 0
	targetSpawns := int(baseSpawns)
	if targetSpawns < 1 && pf.rng.Float32() < baseSpawns {
		targetSpawns = 1
	}

	for attempt := 0; attempt < maxAttempts && spawned < targetSpawns; attempt++ {
		if pf.Count >= pf.MaxCount {
			break
		}

		// Random position
		x := pf.rng.Float32() * pf.worldW
		y := pf.rng.Float32() * pf.worldH

		// Accept with probability proportional to potential
		pot := pf.samplePotential(x, y)
		if pf.rng.Float32() < pot {
			pf.spawn(x, y, pf.InitialMass)
			spawned++
		}
	}
}

func (pf *ParticleResourceField) spawn(x, y, mass float32) {
	var idx int
	if len(pf.FreeList) > 0 {
		idx = pf.FreeList[len(pf.FreeList)-1]
		pf.FreeList = pf.FreeList[:len(pf.FreeList)-1]
	} else if pf.Count < pf.MaxCount {
		idx = pf.Count
	} else {
		return // Pool full
	}

	pf.X[idx] = x
	pf.Y[idx] = y
	pf.Mass[idx] = mass
	pf.Active[idx] = true
	pf.ActiveList = append(pf.ActiveList, idx)
	pf.Count++
}

func (pf *ParticleResourceField) despawn(i int) {
	pf.Active[i] = false
	pf.Mass[i] = 0
	pf.FreeList = append(pf.FreeList, i)
	pf.Count--
	// Note: ActiveList is cleaned during cleanupCompact, not here
}

// --- Cleanup ---

func (pf *ParticleResourceField) cleanup() {
	const minMass = 0.0001

	for i := 0; i < pf.MaxCount; i++ {
		if pf.Active[i] && pf.Mass[i] < minMass {
			pf.despawn(i)
		}
	}

	// Clamp grid to non-negative
	for i := range pf.Res {
		if pf.Res[i] < 0 {
			pf.Res[i] = 0
		}
	}
}

// --- Compact Iteration Functions (O(active) instead of O(MaxCount)) ---

// advectParticlesCompact only iterates over active particles.
// Uses RK2 (midpoint) integration for smooth trajectories.
func (pf *ParticleResourceField) advectParticlesCompact(dt float32) {
	for _, i := range pf.ActiveList {
		if !pf.Active[i] {
			continue // Skip despawned (will be cleaned in cleanupCompact)
		}

		x0, y0 := pf.X[i], pf.Y[i]

		// k1 = flow at current position
		u1, v1 := pf.sampleFlow(x0, y0)

		// Midpoint
		xm := pfWrap(x0+u1*dt*0.5, pf.worldW)
		ym := pfWrap(y0+v1*dt*0.5, pf.worldH)

		// k2 = flow at midpoint
		u2, v2 := pf.sampleFlow(xm, ym)

		// Final position
		pf.X[i] = pfWrap(x0+u2*dt, pf.worldW)
		pf.Y[i] = pfWrap(y0+v2*dt, pf.worldH)
	}
}

// depositCompact transfers mass from particles to grid, only iterating active particles.
func (pf *ParticleResourceField) depositCompact(dt float32) {
	rate := pf.DepositRate * dt
	if rate > 1 {
		rate = 1
	}

	for _, i := range pf.ActiveList {
		if !pf.Active[i] || pf.Mass[i] <= 0 {
			continue
		}

		dm := pf.Mass[i] * rate
		pf.Mass[i] -= dm
		pf.splatToGrid(pf.X[i], pf.Y[i], dm)
	}
}

// pickupCompact transfers mass from grid to particles, only iterating active particles.
func (pf *ParticleResourceField) pickupCompact(dt float32) {
	for _, i := range pf.ActiveList {
		if !pf.Active[i] {
			continue
		}

		r := pf.Sample(pf.X[i], pf.Y[i])
		if r < 0.001 {
			continue
		}

		dm := pf.PickupRate * r * dt
		if dm > r*0.5 {
			dm = r * 0.5
		}

		pf.removeFromGrid(pf.X[i], pf.Y[i], dm)
		pf.Mass[i] += dm
	}
}

// cleanupCompact removes dead particles and compacts the ActiveList.
func (pf *ParticleResourceField) cleanupCompact() {
	const minMass = 0.0001

	// Mark low-mass particles as dead
	for _, i := range pf.ActiveList {
		if pf.Active[i] && pf.Mass[i] < minMass {
			pf.Active[i] = false
			pf.Mass[i] = 0
			pf.FreeList = append(pf.FreeList, i)
			pf.Count--
		}
	}

	// Compact the ActiveList by removing dead entries
	writeIdx := 0
	for _, i := range pf.ActiveList {
		if pf.Active[i] {
			pf.ActiveList[writeIdx] = i
			writeIdx++
		}
	}
	pf.ActiveList = pf.ActiveList[:writeIdx]

	// Note: Grid clamping removed - removeFromGrid already clamps with maxf32(0, ...)
}

// --- Grazing (same as CPUResourceField) ---

func (pf *ParticleResourceField) grazeFromGrid(x, y float32, rate, dt float32, radiusCells int) float32 {
	u := pfFract(x / pf.worldW)
	v := pfFract(y / pf.worldH)

	cx := int(u * float32(pf.ResW))
	cy := int(v * float32(pf.ResH))

	want := rate * dt
	if want <= 0 {
		return 0
	}

	// Compute kernel weights (tent function)
	var wsum float32
	for oy := -radiusCells; oy <= radiusCells; oy++ {
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			d := float32(pfAbsInt(ox) + pfAbsInt(oy))
			w := float32(radiusCells+1) - d
			if w > 0 {
				wsum += w
			}
		}
	}
	if wsum <= 0 {
		return 0
	}

	var removed float32
	for oy := -radiusCells; oy <= radiusCells; oy++ {
		yy := pfModInt(cy+oy, pf.ResH)
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			xx := pfModInt(cx+ox, pf.ResW)

			d := float32(pfAbsInt(ox) + pfAbsInt(oy))
			w := float32(radiusCells+1) - d
			if w <= 0 {
				continue
			}
			share := want * (w / wsum)

			i := yy*pf.ResW + xx
			avail := pf.Res[i]
			take := share
			if take > avail {
				take = avail
			}
			pf.Res[i] = avail - take
			removed += take
		}
	}
	return removed
}

// --- Grid Sampling (bilinear) ---

func (pf *ParticleResourceField) sampleGrid(grid []float32, w, h int, x, y float32) float32 {
	u := pfFract(x / pf.worldW)
	v := pfFract(y / pf.worldH)

	fx := u * float32(w)
	fy := v * float32(h)

	// Fast floor for positive values (truncation = floor for positive floats)
	x0 := int(fx)
	y0 := int(fy)

	// Wrap (coordinates are always positive so simple modulo works)
	if x0 >= w {
		x0 = 0
	}
	if y0 >= h {
		y0 = 0
	}

	x1 := x0 + 1
	if x1 >= w {
		x1 = 0
	}
	y1 := y0 + 1
	if y1 >= h {
		y1 = 0
	}

	// Fractional part
	tx := fx - float32(x0)
	ty := fy - float32(y0)

	i00 := y0*w + x0
	i10 := y0*w + x1
	i01 := y1*w + x0
	i11 := y1*w + x1

	a := grid[i00] + (grid[i10]-grid[i00])*tx
	b := grid[i01] + (grid[i11]-grid[i01])*tx
	return a + (b-a)*ty
}

// --- Noise Functions ---

// fbm2D generates tileable 2D FBM (same as CPUResourceField)
func (pf *ParticleResourceField) fbm2D(u, v float32) float32 {
	sum := float32(0)
	amp := float32(0.5)
	freq := pf.Scale

	for o := 0; o < pf.Octaves; o++ {
		sum += amp * pf.valueNoise2D(u, v, freq)
		freq *= pf.Lacunarity
		amp *= pf.Gain
	}

	// Contrast shaping
	return pfClamp01(float32(math.Pow(float64(sum), float64(pf.Contrast))))
}

// fbm3D generates tileable 3D FBM for flow field evolution
func (pf *ParticleResourceField) fbm3D(u, v, w float32, scale float32, octaves int) float32 {
	sum := float32(0)
	amp := float32(0.5)
	freq := scale

	for o := 0; o < octaves; o++ {
		sum += amp * pf.valueNoise3D(u, v, w, freq)
		freq *= pf.Lacunarity
		amp *= pf.Gain
	}

	return sum
}

// valueNoise2D generates tileable value noise at given frequency
func (pf *ParticleResourceField) valueNoise2D(u, v float32, freq float32) float32 {
	x := u * freq
	y := v * freq

	ix := int(math.Floor(float64(x)))
	iy := int(math.Floor(float64(y)))

	fx := x - float32(ix)
	fy := y - float32(iy)

	f := int(freq)
	if f < 1 {
		f = 1
	}

	i00x := pfModInt(ix, f)
	i10x := pfModInt(ix+1, f)
	i00y := pfModInt(iy, f)
	i01y := pfModInt(iy+1, f)

	a := pf.hash2D(i00x, i00y)
	b := pf.hash2D(i10x, i00y)
	c := pf.hash2D(i00x, i01y)
	d := pf.hash2D(i10x, i01y)

	ux := pfSmoothstep(fx)
	uy := pfSmoothstep(fy)

	ab := a + (b-a)*ux
	cd := c + (d-c)*ux
	return ab + (cd-ab)*uy
}

// valueNoise3D generates tileable 3D value noise
func (pf *ParticleResourceField) valueNoise3D(u, v, w float32, freq float32) float32 {
	x := u * freq
	y := v * freq
	z := w * freq

	ix := int(math.Floor(float64(x)))
	iy := int(math.Floor(float64(y)))
	iz := int(math.Floor(float64(z)))

	fx := x - float32(ix)
	fy := y - float32(iy)
	fz := z - float32(iz)

	f := int(freq)
	if f < 1 {
		f = 1
	}

	// For Z (time), we don't wrap - let it evolve continuously
	i00x := pfModInt(ix, f)
	i10x := pfModInt(ix+1, f)
	i00y := pfModInt(iy, f)
	i01y := pfModInt(iy+1, f)

	// 8 corners of the cube
	n000 := pf.hash3D(i00x, i00y, iz)
	n100 := pf.hash3D(i10x, i00y, iz)
	n010 := pf.hash3D(i00x, i01y, iz)
	n110 := pf.hash3D(i10x, i01y, iz)
	n001 := pf.hash3D(i00x, i00y, iz+1)
	n101 := pf.hash3D(i10x, i00y, iz+1)
	n011 := pf.hash3D(i00x, i01y, iz+1)
	n111 := pf.hash3D(i10x, i01y, iz+1)

	ux := pfSmoothstep(fx)
	uy := pfSmoothstep(fy)
	uz := pfSmoothstep(fz)

	// Trilinear interpolation
	n00 := n000 + (n100-n000)*ux
	n01 := n001 + (n101-n001)*ux
	n10 := n010 + (n110-n010)*ux
	n11 := n011 + (n111-n011)*ux

	n0 := n00 + (n10-n00)*uy
	n1 := n01 + (n11-n01)*uy

	return n0 + (n1-n0)*uz
}

func (pf *ParticleResourceField) hash2D(ix, iy int) float32 {
	x := uint32(ix)
	y := uint32(iy)
	h := x*374761393 + y*668265263 + pf.Seed*1442695041
	h = (h ^ (h >> 13)) * 1274126177
	h ^= (h >> 16)
	return float32(h&0x00FFFFFF) / float32(0x01000000)
}

func (pf *ParticleResourceField) hash3D(ix, iy, iz int) float32 {
	x := uint32(ix)
	y := uint32(iy)
	z := uint32(iz)
	h := x*374761393 + y*668265263 + z*1013904223 + pf.Seed*1442695041
	h = (h ^ (h >> 13)) * 1274126177
	h ^= (h >> 16)
	return float32(h&0x00FFFFFF) / float32(0x01000000)
}

// --- Helper functions (prefixed to avoid conflicts) ---

func pfFract(x float32) float32 {
	return x - float32(math.Floor(float64(x)))
}

func pfWrap(x, max float32) float32 {
	x = float32(math.Mod(float64(x), float64(max)))
	if x < 0 {
		x += max
	}
	return x
}

func pfModInt(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

func pfAbsInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func pfClamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func pfSmoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

func maxf32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// --- Telemetry Helpers ---

// ParticleCount returns the number of active particles.
func (pf *ParticleResourceField) ParticleCount() int {
	return pf.Count
}

// TotalMass returns the total mass in the system (particles + grid).
func (pf *ParticleResourceField) TotalMass() float32 {
	var total float32

	// Sum particle mass
	for i := 0; i < pf.MaxCount; i++ {
		if pf.Active[i] {
			total += pf.Mass[i]
		}
	}

	// Sum grid mass
	for _, r := range pf.Res {
		total += r
	}

	return total
}

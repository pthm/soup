package systems

import (
	"math"
	"math/rand"

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

	// Potential field P (spawn rate map)
	Pot        []float32
	PotW, PotH int

	// Flow field (curl of scalar potential)
	FlowU, FlowV []float32
	FlowW, FlowH int

	// Resource grid R (mass density - what organisms consume)
	Res        []float32
	ResW, ResH int

	// World dimensions
	worldW, worldH float32

	// Timing
	Time           float32
	lastFlowUpdate float32
	lastPotUpdate  float32

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

	pf := &ParticleResourceField{
		// Particle arrays
		X:        make([]float32, maxCount),
		Y:        make([]float32, maxCount),
		Mass:     make([]float32, maxCount),
		Active:   make([]bool, maxCount),
		MaxCount: maxCount,
		FreeList: make([]int, 0, maxCount),

		// Potential field (same resolution as resource grid)
		Pot:  make([]float32, gridW*gridH),
		PotW: gridW,
		PotH: gridH,

		// Flow field (same resolution)
		FlowU: make([]float32, gridW*gridH),
		FlowV: make([]float32, gridW*gridH),
		FlowW: gridW,
		FlowH: gridH,

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
	pf.rebuildFlowField(0)

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

	// Update fields periodically
	if evolve {
		if pf.Time-pf.lastFlowUpdate >= pf.FlowUpdateSec {
			pf.rebuildFlowField(pf.Time)
			pf.lastFlowUpdate = pf.Time
		}
		if pf.Time-pf.lastPotUpdate >= pf.PotUpdateSec {
			pf.rebuildPotential(pf.Time)
			pf.lastPotUpdate = pf.Time
		}
	}

	// Particle dynamics
	pf.spawnParticles(dt)
	pf.advectParticles(dt)
	pf.deposit(dt)
	pf.pickup(dt)
	pf.cleanup()
}

func (pf *ParticleResourceField) ResData() []float32 {
	return pf.Res
}

func (pf *ParticleResourceField) GridSize() (int, int) {
	return pf.ResW, pf.ResH
}

// --- Flow Field (Curl Noise) ---

func (pf *ParticleResourceField) rebuildFlowField(t float32) {
	tEvolved := t * pf.FlowEvolution
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
			pf.FlowU[i] = (psiDv - psi0) / eps * pf.FlowStrength
			pf.FlowV[i] = -(psiDu - psi0) / eps * pf.FlowStrength
		}
	}
}

func (pf *ParticleResourceField) sampleFlow(x, y float32) (float32, float32) {
	u := pf.sampleGrid(pf.FlowU, pf.FlowW, pf.FlowH, x, y)
	v := pf.sampleGrid(pf.FlowV, pf.FlowW, pf.FlowH, x, y)
	return u, v
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
	pf.Count++
}

func (pf *ParticleResourceField) despawn(i int) {
	pf.Active[i] = false
	pf.Mass[i] = 0
	pf.FreeList = append(pf.FreeList, i)
	pf.Count--
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

	x0 := int(math.Floor(float64(fx)))
	y0 := int(math.Floor(float64(fy)))
	x0 = pfModInt(x0, w)
	y0 = pfModInt(y0, h)

	x1 := pfModInt(x0+1, w)
	y1 := pfModInt(y0+1, h)

	tx := fx - float32(int(math.Floor(float64(fx))))
	ty := fy - float32(int(math.Floor(float64(fy))))

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

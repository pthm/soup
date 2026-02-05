package systems

import (
	"math"

	"github.com/pthm-cable/soup/config"
)

// ResourceField implements a resource grid with sparse updates.
// - Cap: immutable cell capacity (FBM-generated at startup)
// - Res: current resource (starts as copy of Cap)
// - Det: detritus from dead organisms
// - Only cells that have been disturbed are updated each tick
type ResourceField struct {
	// Cell capacity (immutable after init)
	Cap  []float32
	W, H int

	// Resource grid (what organisms consume)
	Res []float32

	// Detritus grid (dead biomass decaying back to resource)
	Det []float32


	// World dimensions
	worldW, worldH float32

	// Noise parameters (for FBM generation)
	Seed       uint32
	Scale      float32
	Octaves    int
	Lacunarity float32
	Gain       float32
	Contrast   float32

	// Regeneration rate (fraction per second towards capacity)
	RegenRate float32

	// Detritus parameters
	DetDecayRate float32
	DetDecayEff  float32

	// Epsilon for equilibrium check
	epsilon float32
}

// NewResourceField creates a new resource field with sparse updates.
func NewResourceField(gridW, gridH int, worldW, worldH float32, seed int64, cfg *config.Config) *ResourceField {
	potCfg := cfg.Potential
	detCfg := cfg.Detritus
	resCfg := cfg.Resource

	size := gridW * gridH
	rf := &ResourceField{
		Cap: make([]float32, size),
		W:   gridW,
		H:   gridH,

		Res: make([]float32, size),
		Det: make([]float32, size),

		worldW: worldW,
		worldH: worldH,

		Seed:       uint32(seed),
		Scale:      float32(potCfg.Scale),
		Octaves:    potCfg.Octaves,
		Lacunarity: float32(potCfg.Lacunarity),
		Gain:       float32(potCfg.Gain),
		Contrast:   float32(potCfg.Contrast),

		RegenRate:    float32(resCfg.RegenRate),
		DetDecayRate: float32(detCfg.DecayRate),
		DetDecayEff:  float32(detCfg.DecayEfficiency),

		epsilon: 1e-4, // Loose enough for float32 convergence
	}

	// Generate cell capacity using FBM
	rf.generateCapacity()

	// Initialize resource grid from capacity
	copy(rf.Res, rf.Cap)

	return rf
}

// --- ResourceSampler interface ---

func (rf *ResourceField) Sample(x, y float32) float32 {
	return rf.sampleGrid(rf.Res, x, y)
}

func (rf *ResourceField) Width() float32  { return rf.worldW }
func (rf *ResourceField) Height() float32 { return rf.worldH }

func (rf *ResourceField) Graze(x, y float32, rate, dt float32, radiusCells int) float32 {
	return rf.grazeFromGrid(x, y, rate, dt, radiusCells)
}

func (rf *ResourceField) Step(dt float32, _ bool) {
	regenRate := rf.RegenRate * dt
	if regenRate > 1 {
		regenRate = 1
	}

	detRate := rf.DetDecayRate * dt
	detEff := rf.DetDecayEff
	eps := rf.epsilon

	// Simple iteration with inline equilibrium skip
	for i := range rf.Res {
		cap := rf.Cap[i]
		res := rf.Res[i]
		det := rf.Det[i]

		// Skip cells at equilibrium (no detritus, resource at capacity)
		if det <= eps {
			diff := res - cap
			if diff < 0 {
				diff = -diff
			}
			if diff < eps {
				continue
			}
		}

		// Decay detritus
		if det > 0 {
			decayed := detRate * det
			rf.Det[i] = det - decayed
			res += detEff * decayed
		}

		// Regenerate towards capacity
		if regenRate > 0 {
			res = res + (cap-res)*regenRate
		}

		// Cap resource to capacity
		if res > cap {
			res = cap
		}

		rf.Res[i] = res
	}
}

func (rf *ResourceField) ResData() []float32 {
	return rf.Res
}

func (rf *ResourceField) GridSize() (int, int) {
	return rf.W, rf.H
}

// --- Detritus ---

func (rf *ResourceField) DepositDetritus(x, y, amount float32) float32 {
	if amount <= 0 {
		return 0
	}
	return rf.splatToDetGrid(x, y, amount)
}

func (rf *ResourceField) splatToDetGrid(x, y, mass float32) float32 {
	gx := x / rf.worldW * float32(rf.W)
	gy := y / rf.worldH * float32(rf.H)

	cx := int(gx)
	cy := int(gy)

	fx := gx - float32(cx)
	fy := gy - float32(cy)

	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy

	x0, y0 := cx, cy
	if x0 >= rf.W {
		x0 = 0
	}
	if y0 >= rf.H {
		y0 = 0
	}
	x1 := cx + 1
	if x1 >= rf.W {
		x1 = 0
	}
	y1 := cy + 1
	if y1 >= rf.H {
		y1 = 0
	}

	i00 := y0*rf.W + x0
	i10 := y0*rf.W + x1
	i01 := y1*rf.W + x0
	i11 := y1*rf.W + x1

	rf.Det[i00] += mass * w00
	rf.Det[i10] += mass * w10
	rf.Det[i01] += mass * w01
	rf.Det[i11] += mass * w11

	return mass
}

// --- Capacity Generation ---

func (rf *ResourceField) generateCapacity() {
	for y := 0; y < rf.H; y++ {
		v := (float32(y) + 0.5) / float32(rf.H)
		for x := 0; x < rf.W; x++ {
			u := (float32(x) + 0.5) / float32(rf.W)
			rf.Cap[y*rf.W+x] = rf.fbm2D(u, v)
		}
	}
}

// --- Grazing ---

func (rf *ResourceField) grazeFromGrid(x, y float32, rate, dt float32, radiusCells int) float32 {
	u := rfFract(x / rf.worldW)
	v := rfFract(y / rf.worldH)

	cx := int(u * float32(rf.W))
	cy := int(v * float32(rf.H))

	want := rate * dt
	if want <= 0 {
		return 0
	}

	// Compute kernel weights (tent function)
	var wsum float32
	for oy := -radiusCells; oy <= radiusCells; oy++ {
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			d := float32(rfAbsInt(ox) + rfAbsInt(oy))
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
		yy := rfModInt(cy+oy, rf.H)
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			xx := rfModInt(cx+ox, rf.W)

			d := float32(rfAbsInt(ox) + rfAbsInt(oy))
			w := float32(radiusCells+1) - d
			if w <= 0 {
				continue
			}
			share := want * (w / wsum)

			i := yy*rf.W + xx
			avail := rf.Res[i]
			take := share
			if take > avail {
				take = avail
			}
			if take > 0 {
				rf.Res[i] = avail - take
				removed += take
			}
		}
	}
	return removed
}

// --- Grid Sampling (bilinear) ---

func (rf *ResourceField) sampleGrid(grid []float32, x, y float32) float32 {
	u := rfFract(x / rf.worldW)
	v := rfFract(y / rf.worldH)

	fx := u * float32(rf.W)
	fy := v * float32(rf.H)

	x0 := int(fx)
	y0 := int(fy)

	if x0 >= rf.W {
		x0 = 0
	}
	if y0 >= rf.H {
		y0 = 0
	}

	x1 := x0 + 1
	if x1 >= rf.W {
		x1 = 0
	}
	y1 := y0 + 1
	if y1 >= rf.H {
		y1 = 0
	}

	tx := fx - float32(x0)
	ty := fy - float32(y0)

	i00 := y0*rf.W + x0
	i10 := y0*rf.W + x1
	i01 := y1*rf.W + x0
	i11 := y1*rf.W + x1

	a := grid[i00] + (grid[i10]-grid[i00])*tx
	b := grid[i01] + (grid[i11]-grid[i01])*tx
	return a + (b-a)*ty
}

// --- Noise Functions ---

func (rf *ResourceField) fbm2D(u, v float32) float32 {
	sum := float32(0)
	amp := float32(0.5)
	freq := rf.Scale

	for o := 0; o < rf.Octaves; o++ {
		sum += amp * rf.valueNoise2D(u, v, freq)
		freq *= rf.Lacunarity
		amp *= rf.Gain
	}

	return rfClamp01(float32(math.Pow(float64(sum), float64(rf.Contrast))))
}

func (rf *ResourceField) valueNoise2D(u, v float32, freq float32) float32 {
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

	i00x := rfModInt(ix, f)
	i10x := rfModInt(ix+1, f)
	i00y := rfModInt(iy, f)
	i01y := rfModInt(iy+1, f)

	a := rf.hash2D(i00x, i00y)
	b := rf.hash2D(i10x, i00y)
	c := rf.hash2D(i00x, i01y)
	d := rf.hash2D(i10x, i01y)

	ux := rfSmoothstep(fx)
	uy := rfSmoothstep(fy)

	ab := a + (b-a)*ux
	cd := c + (d-c)*ux
	return ab + (cd-ab)*uy
}

func (rf *ResourceField) hash2D(ix, iy int) float32 {
	x := uint32(ix)
	y := uint32(iy)
	h := x*374761393 + y*668265263 + rf.Seed*1442695041
	h = (h ^ (h >> 13)) * 1274126177
	h ^= (h >> 16)
	return float32(h&0x00FFFFFF) / float32(0x01000000)
}

// --- Telemetry Helpers ---

func (rf *ResourceField) TotalResMass() float32 {
	var total float32
	for _, r := range rf.Res {
		total += r
	}
	return total
}

func (rf *ResourceField) TotalDetMass() float32 {
	var total float32
	for _, d := range rf.Det {
		total += d
	}
	return total
}

func (rf *ResourceField) TotalMass() float32 {
	return rf.TotalResMass() + rf.TotalDetMass()
}

// SampleDetritus returns the detritus density at world coordinates.
func (rf *ResourceField) SampleDetritus(x, y float32) float32 {
	return rf.sampleGrid(rf.Det, x, y)
}

// DetData returns the detritus grid for visualization.
func (rf *ResourceField) DetData() []float32 {
	return rf.Det
}

// CapData returns the capacity field for visualization.
func (rf *ResourceField) CapData() []float32 {
	return rf.Cap
}

// --- Deprecated accessors for compatibility ---

// Pot returns the capacity field (deprecated, use Cap directly).
var _ = (*ResourceField)(nil).Pot // compile-time check

func (rf *ResourceField) Pot() []float32 {
	return rf.Cap
}

// PotW returns the grid width (deprecated, use W directly).
func (rf *ResourceField) PotW() int {
	return rf.W
}

// PotH returns the grid height (deprecated, use H directly).
func (rf *ResourceField) PotH() int {
	return rf.H
}

// ResW returns the grid width (deprecated, use W directly).
func (rf *ResourceField) ResW() int {
	return rf.W
}

// ResH returns the grid height (deprecated, use H directly).
func (rf *ResourceField) ResH() int {
	return rf.H
}

// --- Helper functions ---

func rfFract(x float32) float32 {
	if x >= 0 {
		return x - float32(int(x))
	}
	return x - float32(int(x)-1)
}

func rfModInt(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

func rfAbsInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func rfClamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func rfSmoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

// Ensure ResourceField implements ResourceSampler at compile time
var _ ResourceSampler = (*ResourceField)(nil)

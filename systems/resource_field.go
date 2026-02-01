package systems

import (
	"math"
)

// CPUResourceField is a CPU-based tileable resource grid with depletion,
// regrowth, and optional diffusion. Implements ResourceSampler.
type CPUResourceField struct {
	W, H int

	// Current resource [0,1] - what prey consume
	Res []float32
	// Capacity/target [0,1] - what Res regrows toward
	Cap []float32

	// World dimensions for coordinate mapping
	worldW, worldH float32

	// Evolution state
	Time float32

	// Parameters
	RegrowRate float32 // per second toward Cap
	Diffuse    float32 // diffusion strength per second (0 disables)
	DriftX     float32 // UV units per second
	DriftY     float32

	// Noise parameters
	Scale      float32
	Octaves    int
	Lacunarity float32
	Gain       float32
	Contrast   float32 // Exponent for contrast shaping (higher = sparser patches)
	Seed       uint32

	// Scratch buffer for diffusion
	tmp []float32

	// Evolution timing
	lastEvolveTime float32
	evolveInterval float32 // seconds between capacity rebuilds
}

// NewCPUResourceField creates a new CPU resource field.
func NewCPUResourceField(w, h int, worldW, worldH float32) *CPUResourceField {
	rf := &CPUResourceField{
		W: w, H: h,
		Res: make([]float32, w*h),
		Cap: make([]float32, w*h),
		tmp: make([]float32, w*h),

		worldW: worldW,
		worldH: worldH,

		// Default parameters - tune via SetParams
		RegrowRate: 0.25,
		Diffuse:    0.10,
		DriftX:     0.02,
		DriftY:     0.014,

		Scale:      4.0,
		Octaves:    4,
		Lacunarity: 2.0,
		Gain:       0.5,
		Contrast:   3.0, // Higher = sparser patches (only peaks stay high)
		Seed:       42,

		evolveInterval: 1.0, // rebuild capacity every 1 second
	}

	// Initialize capacity from FBM and set Res = Cap
	rf.rebuildCapacity(0)
	copy(rf.Res, rf.Cap)

	return rf
}

// SetParams configures resource field behavior.
func (rf *CPUResourceField) SetParams(regrowRate, diffuse, driftX, driftY, evolveInterval float32) {
	rf.RegrowRate = regrowRate
	rf.Diffuse = diffuse
	rf.DriftX = driftX
	rf.DriftY = driftY
	rf.evolveInterval = evolveInterval
}

// SetNoiseParams configures the FBM noise generation.
func (rf *CPUResourceField) SetNoiseParams(scale float32, octaves int, lacunarity, gain float32, seed uint32) {
	rf.Scale = scale
	rf.Octaves = octaves
	rf.Lacunarity = lacunarity
	rf.Gain = gain
	rf.Seed = seed
}

// Regenerate rebuilds the capacity grid and resets current resource to match.
// Call after changing noise parameters (Seed, Contrast, Scale, etc).
func (rf *CPUResourceField) Regenerate() {
	rf.rebuildCapacity(rf.Time)
	copy(rf.Res, rf.Cap)
}

// Width returns world width for ResourceSampler interface.
func (rf *CPUResourceField) Width() float32 { return rf.worldW }

// Height returns world height for ResourceSampler interface.
func (rf *CPUResourceField) Height() float32 { return rf.worldH }

// Sample returns the current resource level at world coordinates (bilinear interpolation).
func (rf *CPUResourceField) Sample(x, y float32) float32 {
	u := cpuFract(x / rf.worldW)
	v := cpuFract(y / rf.worldH)
	return rf.sampleBilinear(rf.Res, u, v)
}

// SampleCapacity returns the capacity (target) level at world coordinates.
func (rf *CPUResourceField) SampleCapacity(x, y float32) float32 {
	u := cpuFract(x / rf.worldW)
	v := cpuFract(y / rf.worldH)
	return rf.sampleBilinear(rf.Cap, u, v)
}

// Graze removes resource from the grid near (x,y) and returns the amount removed.
// The return value should be multiplied by efficiency to get actual energy gain.
// radiusCells=1 gives 3x3 kernel, radiusCells=2 gives 5x5.
func (rf *CPUResourceField) Graze(x, y float32, rate, dt float32, radiusCells int) float32 {
	u := cpuFract(x / rf.worldW)
	v := cpuFract(y / rf.worldH)

	cx := int(u * float32(rf.W))
	cy := int(v * float32(rf.H))

	// Total desired removal
	want := rate * dt
	if want <= 0 {
		return 0
	}

	// Compute kernel weights (tent function)
	var wsum float32
	for oy := -radiusCells; oy <= radiusCells; oy++ {
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			d := float32(cpuAbsInt(ox) + cpuAbsInt(oy))
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
		yy := cpuModInt(cy+oy, rf.H)
		for ox := -radiusCells; ox <= radiusCells; ox++ {
			xx := cpuModInt(cx+ox, rf.W)

			d := float32(cpuAbsInt(ox) + cpuAbsInt(oy))
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
			rf.Res[i] = avail - take
			removed += take
		}
	}
	return removed
}

// Step advances the resource field by dt seconds.
// Handles regrowth toward capacity and optional diffusion.
// evolve=true will rebuild capacity if enough time has passed.
func (rf *CPUResourceField) Step(dt float32, evolve bool) {
	rf.Time += dt

	// Evolve capacity periodically
	if evolve && rf.evolveInterval > 0 {
		if rf.Time-rf.lastEvolveTime >= rf.evolveInterval {
			rf.rebuildCapacity(rf.Time)
			rf.lastEvolveTime = rf.Time
		}
	}

	// Regrow Res toward Cap
	if rf.RegrowRate > 0 {
		k := rf.RegrowRate * dt
		for i := range rf.Res {
			target := rf.Cap[i]
			rf.Res[i] += (target - rf.Res[i]) * k
			rf.Res[i] = cpuClamp01(rf.Res[i])
		}
	}

	// Diffuse Res
	if rf.Diffuse > 0 {
		rf.diffuse(dt)
	}
}

// rebuildCapacity regenerates the capacity grid from FBM noise with time-based drift.
func (rf *CPUResourceField) rebuildCapacity(t float32) {
	du := cpuFract(t * rf.DriftX)
	dv := cpuFract(t * rf.DriftY)

	for y := 0; y < rf.H; y++ {
		v := (float32(y) + 0.5) / float32(rf.H)
		v = cpuFract(v + dv)
		for x := 0; x < rf.W; x++ {
			u := (float32(x) + 0.5) / float32(rf.W)
			u = cpuFract(u + du)

			rf.Cap[y*rf.W+x] = rf.fbm(u, v)
		}
	}
}

// diffuse applies 5-point stencil diffusion on the toroidal grid.
func (rf *CPUResourceField) diffuse(dt float32) {
	a := rf.Diffuse * dt
	if a <= 0 {
		return
	}
	// Stability clamp for explicit diffusion
	if a > 0.25 {
		a = 0.25
	}

	w, h := rf.W, rf.H
	src := rf.Res
	dst := rf.tmp

	for y := 0; y < h; y++ {
		yN := cpuModInt(y-1, h)
		yS := cpuModInt(y+1, h)
		for x := 0; x < w; x++ {
			xW := cpuModInt(x-1, w)
			xE := cpuModInt(x+1, w)

			i := y*w + x
			c := src[i]
			n := src[yN*w+x]
			s := src[yS*w+x]
			e := src[y*w+xE]
			wv := src[y*w+xW]

			// Laplacian diffusion
			dst[i] = c + a*(n+s+e+wv-4*c)
		}
	}

	// Swap and clamp
	copy(rf.Res, dst)
	for i := range rf.Res {
		rf.Res[i] = cpuClamp01(rf.Res[i])
	}
}

// fbm generates tileable Fractional Brownian Motion noise.
func (rf *CPUResourceField) fbm(u, v float32) float32 {
	sum := float32(0)
	amp := float32(0.5)
	freq := rf.Scale

	for o := 0; o < rf.Octaves; o++ {
		sum += amp * rf.valueNoiseTileable(u, v, freq)
		freq *= rf.Lacunarity
		amp *= rf.Gain
	}

	// Contrast shaping: higher exponent = sparser patches (mid-values become low)
	return cpuClamp01(float32(math.Pow(float64(sum), float64(rf.Contrast))))
}

// valueNoiseTileable generates tileable value noise at the given frequency.
func (rf *CPUResourceField) valueNoiseTileable(u, v float32, freq float32) float32 {
	x := u * freq
	y := v * freq

	ix := int(math.Floor(float64(x)))
	iy := int(math.Floor(float64(y)))

	fx := x - float32(ix)
	fy := y - float32(iy)

	// Wrap lattice coordinates for tiling
	f := int(freq)
	if f < 1 {
		f = 1
	}
	i00x := cpuModInt(ix, f)
	i10x := cpuModInt(ix+1, f)
	i00y := cpuModInt(iy, f)
	i01y := cpuModInt(iy+1, f)

	a := rf.hash(i00x, i00y)
	b := rf.hash(i10x, i00y)
	c := rf.hash(i00x, i01y)
	d := rf.hash(i10x, i01y)

	ux := cpuSmoothstep(fx)
	uy := cpuSmoothstep(fy)

	ab := a + (b-a)*ux
	cd := c + (d-c)*ux
	return ab + (cd-ab)*uy
}

// hash generates a pseudo-random float in [0,1) from integer coordinates.
func (rf *CPUResourceField) hash(ix, iy int) float32 {
	x := uint32(ix)
	y := uint32(iy)
	h := x*374761393 + y*668265263 + rf.Seed*1442695041
	h = (h ^ (h >> 13)) * 1274126177
	h ^= (h >> 16)
	return float32(h&0x00FFFFFF) / float32(0x01000000)
}

// sampleBilinear performs bilinear interpolation on a grid.
func (rf *CPUResourceField) sampleBilinear(grid []float32, u, v float32) float32 {
	fx := u * float32(rf.W)
	fy := v * float32(rf.H)

	x0 := int(math.Floor(float64(fx)))
	y0 := int(math.Floor(float64(fy)))
	x0 = cpuModInt(x0, rf.W)
	y0 = cpuModInt(y0, rf.H)

	x1 := cpuModInt(x0+1, rf.W)
	y1 := cpuModInt(y0+1, rf.H)

	tx := fx - float32(int(math.Floor(float64(fx))))
	ty := fy - float32(int(math.Floor(float64(fy))))

	i00 := y0*rf.W + x0
	i10 := y0*rf.W + x1
	i01 := y1*rf.W + x0
	i11 := y1*rf.W + x1

	a := grid[i00] + (grid[i10]-grid[i00])*tx
	b := grid[i01] + (grid[i11]-grid[i01])*tx
	return a + (b-a)*ty
}

// ResData returns the current resource grid for visualization.
func (rf *CPUResourceField) ResData() []float32 {
	return rf.Res
}

// GridSize returns the grid dimensions.
func (rf *CPUResourceField) GridSize() (int, int) {
	return rf.W, rf.H
}

// Helper functions with cpu prefix to avoid conflicts

func cpuSmoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

func cpuFract(x float32) float32 {
	return x - float32(math.Floor(float64(x)))
}

func cpuClamp01(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func cpuModInt(a, m int) int {
	r := a % m
	if r < 0 {
		r += m
	}
	return r
}

func cpuAbsInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

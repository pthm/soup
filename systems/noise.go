package systems

import (
	"math"
	"math/rand"
)

// PerlinNoise generates coherent noise values.
type PerlinNoise struct {
	perm [512]int
}

// NewPerlinNoise creates a new Perlin noise generator.
func NewPerlinNoise(seed int64) *PerlinNoise {
	p := &PerlinNoise{}
	rng := rand.New(rand.NewSource(seed))

	// Initialize permutation table
	var perm [256]int
	for i := range perm {
		perm[i] = i
	}

	// Shuffle
	for i := len(perm) - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		perm[i], perm[j] = perm[j], perm[i]
	}

	// Duplicate
	for i := 0; i < 256; i++ {
		p.perm[i] = perm[i]
		p.perm[i+256] = perm[i]
	}

	return p
}

// Noise3D returns a noise value for 3D coordinates.
func (p *PerlinNoise) Noise3D(x, y, z float64) float64 {
	// Find unit cube
	X := int(math.Floor(x)) & 255
	Y := int(math.Floor(y)) & 255
	Z := int(math.Floor(z)) & 255

	// Find relative position in cube
	x -= math.Floor(x)
	y -= math.Floor(y)
	z -= math.Floor(z)

	// Compute fade curves
	u := fade(x)
	v := fade(y)
	w := fade(z)

	// Hash coordinates of cube corners
	A := p.perm[X] + Y
	AA := p.perm[A] + Z
	AB := p.perm[A+1] + Z
	B := p.perm[X+1] + Y
	BA := p.perm[B] + Z
	BB := p.perm[B+1] + Z

	// Blend results from 8 corners
	return lerp(w, lerp(v, lerp(u, grad3D(p.perm[AA], x, y, z),
		grad3D(p.perm[BA], x-1, y, z)),
		lerp(u, grad3D(p.perm[AB], x, y-1, z),
			grad3D(p.perm[BB], x-1, y-1, z))),
		lerp(v, lerp(u, grad3D(p.perm[AA+1], x, y, z-1),
			grad3D(p.perm[BA+1], x-1, y, z-1)),
			lerp(u, grad3D(p.perm[AB+1], x, y-1, z-1),
				grad3D(p.perm[BB+1], x-1, y-1, z-1))))
}

// Noise2D returns a noise value for 2D coordinates.
func (p *PerlinNoise) Noise2D(x, y float64) float64 {
	return p.Noise3D(x, y, 0)
}

func fade(t float64) float64 {
	return t * t * t * (t*(t*6-15) + 10)
}

func lerp(t, a, b float64) float64 {
	return a + t*(b-a)
}

func grad3D(hash int, x, y, z float64) float64 {
	h := hash & 15
	u := x
	if h >= 8 {
		u = y
	}
	v := y
	if h >= 4 {
		if h == 12 || h == 14 {
			v = x
		} else {
			v = z
		}
	}
	if h&1 != 0 {
		u = -u
	}
	if h&2 != 0 {
		v = -v
	}
	return u + v
}

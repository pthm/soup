package systems

import (
	"testing"

	"github.com/pthm-cable/soup/config"
	"gonum.org/v1/gonum/blas/blas32"
)

// Benchmark flow blend with current scalar implementation
func BenchmarkFlowBlendScalar(b *testing.B) {
	size := 32 * 32 // Typical flow field size
	u0 := make([]float32, size)
	u1 := make([]float32, size)
	uBlend := make([]float32, size)

	// Initialize with some values
	for i := range u0 {
		u0[i] = float32(i) * 0.001
		u1[i] = float32(i) * 0.002
	}

	t := float32(0.5)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := range uBlend {
			uBlend[i] = u0[i] + (u1[i]-u0[i])*t
		}
	}
}

// Benchmark flow blend with blas32
func BenchmarkFlowBlendBLAS(b *testing.B) {
	size := 32 * 32
	u0 := make([]float32, size)
	u1 := make([]float32, size)
	uBlend := make([]float32, size)

	for i := range u0 {
		u0[i] = float32(i) * 0.001
		u1[i] = float32(i) * 0.002
	}

	t := float32(0.5)

	// Pre-create vectors (reused each iteration)
	v0 := blas32.Vector{N: size, Inc: 1, Data: u0}
	v1 := blas32.Vector{N: size, Inc: 1, Data: u1}
	vBlend := blas32.Vector{N: size, Inc: 1, Data: uBlend}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// dst = (1-t)*a + t*b
		blas32.Copy(v0, vBlend)      // uBlend = u0
		blas32.Scal(1-t, vBlend)     // uBlend = (1-t)*u0
		blas32.Axpy(t, v1, vBlend)   // uBlend = (1-t)*u0 + t*u1
	}
}

// Benchmark sum with scalar loop
func BenchmarkSumScalar(b *testing.B) {
	size := 128 * 128 // Typical resource grid size
	data := make([]float32, size)

	for i := range data {
		data[i] = float32(i) * 0.0001
	}

	b.ResetTimer()
	var total float32
	for n := 0; n < b.N; n++ {
		total = 0
		for _, v := range data {
			total += v
		}
	}
	_ = total
}

// Benchmark sum with blas32.Asum (works for positive values)
func BenchmarkSumBLAS(b *testing.B) {
	size := 128 * 128
	data := make([]float32, size)

	for i := range data {
		data[i] = float32(i) * 0.0001
	}

	v := blas32.Vector{N: size, Inc: 1, Data: data}

	b.ResetTimer()
	var total float32
	for n := 0; n < b.N; n++ {
		total = blas32.Asum(v)
	}
	_ = total
}

// Benchmark the full dual-array blend (U and V together) - scalar
func BenchmarkDualBlendScalar(b *testing.B) {
	size := 32 * 32
	u0 := make([]float32, size)
	u1 := make([]float32, size)
	v0 := make([]float32, size)
	v1 := make([]float32, size)
	uBlend := make([]float32, size)
	vBlend := make([]float32, size)

	for i := range u0 {
		u0[i] = float32(i) * 0.001
		u1[i] = float32(i) * 0.002
		v0[i] = float32(i) * 0.003
		v1[i] = float32(i) * 0.004
	}

	t := float32(0.5)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		for i := range uBlend {
			uBlend[i] = u0[i] + (u1[i]-u0[i])*t
			vBlend[i] = v0[i] + (v1[i]-v0[i])*t
		}
	}
}

// Benchmark the full dual-array blend (U and V together) - BLAS
func BenchmarkDualBlendBLAS(b *testing.B) {
	size := 32 * 32
	u0 := make([]float32, size)
	u1 := make([]float32, size)
	v0 := make([]float32, size)
	v1 := make([]float32, size)
	uBlend := make([]float32, size)
	vBlend := make([]float32, size)

	for i := range u0 {
		u0[i] = float32(i) * 0.001
		u1[i] = float32(i) * 0.002
		v0[i] = float32(i) * 0.003
		v1[i] = float32(i) * 0.004
	}

	t := float32(0.5)
	oneMinusT := 1 - t

	vu0 := blas32.Vector{N: size, Inc: 1, Data: u0}
	vu1 := blas32.Vector{N: size, Inc: 1, Data: u1}
	vv0 := blas32.Vector{N: size, Inc: 1, Data: v0}
	vv1 := blas32.Vector{N: size, Inc: 1, Data: v1}
	vuBlend := blas32.Vector{N: size, Inc: 1, Data: uBlend}
	vvBlend := blas32.Vector{N: size, Inc: 1, Data: vBlend}

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		// U blend
		blas32.Copy(vu0, vuBlend)
		blas32.Scal(oneMinusT, vuBlend)
		blas32.Axpy(t, vu1, vuBlend)

		// V blend
		blas32.Copy(vv0, vvBlend)
		blas32.Scal(oneMinusT, vvBlend)
		blas32.Axpy(t, vv1, vvBlend)
	}
}

// --- Phase-specific benchmarks ---

func setupResourceField(b *testing.B) *ResourceField {
	b.Helper()
	rf := NewResourceField(128, 128, 2560, 1440, 42, config.Cfg())
	return rf
}

func BenchmarkPhase_Graze(b *testing.B) {
	rf := setupResourceField(b)
	dt := float32(1.0 / 60.0)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate 200 prey grazing (typical population)
		for j := 0; j < 200; j++ {
			x := float32(j*13%2560)
			y := float32(j*17%1440)
			rf.Graze(x, y, 0.05, dt, 1)
		}
	}
}

// --- StepDetritus optimization experiments ---

// Current implementation (with conditional skip)
func stepDetritusOriginal(det, res []float32, rate, eff float32) float32 {
	var heat float32
	for i := range det {
		d := det[i]
		if d <= 0 {
			continue
		}
		decayed := rate * d
		det[i] = d - decayed
		res[i] += eff * decayed
		heat += (1 - eff) * decayed
	}
	return heat
}

// No conditional - always compute (may enable auto-vectorization)
func stepDetritusNoBranch(det, res []float32, rate, eff float32) float32 {
	heatFactor := 1 - eff
	var heat float32
	for i := range det {
		d := det[i]
		decayed := rate * d
		det[i] = d - decayed
		res[i] += eff * decayed
		heat += heatFactor * decayed
	}
	return heat
}

// 4x unrolled, no branch
func stepDetritus4xUnrolled(det, res []float32, rate, eff float32) float32 {
	heatFactor := 1 - eff
	var heat float32
	n := len(det)

	// Main loop - 4 at a time
	i := 0
	for ; i <= n-4; i += 4 {
		d0 := det[i]
		d1 := det[i+1]
		d2 := det[i+2]
		d3 := det[i+3]

		dec0 := rate * d0
		dec1 := rate * d1
		dec2 := rate * d2
		dec3 := rate * d3

		det[i] = d0 - dec0
		det[i+1] = d1 - dec1
		det[i+2] = d2 - dec2
		det[i+3] = d3 - dec3

		res[i] += eff * dec0
		res[i+1] += eff * dec1
		res[i+2] += eff * dec2
		res[i+3] += eff * dec3

		heat += heatFactor * (dec0 + dec1 + dec2 + dec3)
	}

	// Remainder
	for ; i < n; i++ {
		d := det[i]
		decayed := rate * d
		det[i] = d - decayed
		res[i] += eff * decayed
		heat += heatFactor * decayed
	}
	return heat
}

// 8x unrolled, no branch (better for SIMD widths)
func stepDetritus8xUnrolled(det, res []float32, rate, eff float32) float32 {
	heatFactor := 1 - eff
	var heat float32
	n := len(det)

	i := 0
	for ; i <= n-8; i += 8 {
		d0 := det[i]
		d1 := det[i+1]
		d2 := det[i+2]
		d3 := det[i+3]
		d4 := det[i+4]
		d5 := det[i+5]
		d6 := det[i+6]
		d7 := det[i+7]

		dec0 := rate * d0
		dec1 := rate * d1
		dec2 := rate * d2
		dec3 := rate * d3
		dec4 := rate * d4
		dec5 := rate * d5
		dec6 := rate * d6
		dec7 := rate * d7

		det[i] = d0 - dec0
		det[i+1] = d1 - dec1
		det[i+2] = d2 - dec2
		det[i+3] = d3 - dec3
		det[i+4] = d4 - dec4
		det[i+5] = d5 - dec5
		det[i+6] = d6 - dec6
		det[i+7] = d7 - dec7

		res[i] += eff * dec0
		res[i+1] += eff * dec1
		res[i+2] += eff * dec2
		res[i+3] += eff * dec3
		res[i+4] += eff * dec4
		res[i+5] += eff * dec5
		res[i+6] += eff * dec6
		res[i+7] += eff * dec7

		heat += heatFactor * (dec0 + dec1 + dec2 + dec3 + dec4 + dec5 + dec6 + dec7)
	}

	for ; i < n; i++ {
		d := det[i]
		decayed := rate * d
		det[i] = d - decayed
		res[i] += eff * decayed
		heat += heatFactor * decayed
	}
	return heat
}

func BenchmarkStepDetritus_Original(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	for i := range det {
		det[i] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritusOriginal(det, res, rate, eff)
	}
}

func BenchmarkStepDetritus_NoBranch(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	for i := range det {
		det[i] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritusNoBranch(det, res, rate, eff)
	}
}

func BenchmarkStepDetritus_4xUnrolled(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	for i := range det {
		det[i] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritus4xUnrolled(det, res, rate, eff)
	}
}

func BenchmarkStepDetritus_8xUnrolled(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	for i := range det {
		det[i] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritus8xUnrolled(det, res, rate, eff)
	}
}

// Also test with sparse data (many zeros) to see if branch helps
func BenchmarkStepDetritus_Original_Sparse(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	// Only 10% non-zero
	for i := 0; i < len(det)/10; i++ {
		det[i*10] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritusOriginal(det, res, rate, eff)
	}
}

func BenchmarkStepDetritus_NoBranch_Sparse(b *testing.B) {
	det := make([]float32, 128*128)
	res := make([]float32, 128*128)
	for i := 0; i < len(det)/10; i++ {
		det[i*10] = 0.1
	}
	rate := float32(0.05 / 60.0)
	eff := float32(0.8)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stepDetritusNoBranch(det, res, rate, eff)
	}
}

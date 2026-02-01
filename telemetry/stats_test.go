package telemetry

import (
	"math"
	"testing"
)

func TestPercentile(t *testing.T) {
	tests := []struct {
		name   string
		sorted []float64
		p      float64
		want   float64
	}{
		{"empty slice", []float64{}, 0.5, 0},
		{"single element", []float64{5.0}, 0.5, 5.0},
		{"p0", []float64{1, 2, 3, 4, 5}, 0.0, 1.0},
		{"p100", []float64{1, 2, 3, 4, 5}, 1.0, 5.0},
		{"p50 odd", []float64{1, 2, 3, 4, 5}, 0.5, 3.0},
		{"p50 even", []float64{1, 2, 3, 4}, 0.5, 2.5},
		{"p10", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.1, 1.9},
		{"p90", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0.9, 9.1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Percentile(tt.sorted, tt.p)
			if math.Abs(got-tt.want) > 0.001 {
				t.Errorf("Percentile(%v, %v) = %v, want %v", tt.sorted, tt.p, got, tt.want)
			}
		})
	}
}

func TestComputeEnergyStats(t *testing.T) {
	values := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0}
	mean, p10, p50, p90 := ComputeEnergyStats(values)

	// Mean should be 0.55
	if math.Abs(mean-0.55) > 0.001 {
		t.Errorf("mean = %v, want 0.55", mean)
	}

	// P10 should be around 0.19
	if math.Abs(p10-0.19) > 0.01 {
		t.Errorf("p10 = %v, want ~0.19", p10)
	}

	// P50 should be around 0.55
	if math.Abs(p50-0.55) > 0.01 {
		t.Errorf("p50 = %v, want ~0.55", p50)
	}

	// P90 should be around 0.91
	if math.Abs(p90-0.91) > 0.01 {
		t.Errorf("p90 = %v, want ~0.91", p90)
	}
}

func TestComputeEnergyStatsEmpty(t *testing.T) {
	mean, p10, p50, p90 := ComputeEnergyStats([]float64{})

	if mean != 0 || p10 != 0 || p50 != 0 || p90 != 0 {
		t.Error("empty slice should return all zeros")
	}
}

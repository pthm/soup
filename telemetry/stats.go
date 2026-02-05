package telemetry

import (
	"log/slog"
	"math"
	"sort"
)

// WindowStats holds aggregated statistics for a time window.
type WindowStats struct {
	WindowStartTick int32   `csv:"-"`
	WindowEndTick   int32   `csv:"window_end"`
	SimTimeSec      float64 `csv:"sim_time"`

	// Population counts at window end
	PreyCount int `csv:"prey"`
	PredCount int `csv:"pred"`

	// Events during window
	PreyBirths int `csv:"prey_births"`
	PredBirths int `csv:"pred_births"`
	PreyDeaths int `csv:"prey_deaths"`
	PredDeaths int `csv:"pred_deaths"`

	// Hunting
	BitesAttempted     int     `csv:"bites_attempted"`
	BitesHit           int     `csv:"bites_hit"`
	Kills              int     `csv:"kills"`
	BitesBlockedDigest int     `csv:"bites_blocked_digest"`
	BitesMissedRefugia int     `csv:"bites_missed_refugia"`
	HitRate            float64 `csv:"hit_rate"`
	KillRate           float64 `csv:"kill_rate"`

	// Energy distribution (sampled at window end)
	PreyEnergyMean float64 `csv:"prey_energy_mean"`
	PreyEnergyP10  float64 `csv:"prey_energy_p10"`
	PreyEnergyP50  float64 `csv:"prey_energy_p50"`
	PreyEnergyP90  float64 `csv:"prey_energy_p90"`

	PredEnergyMean float64 `csv:"pred_energy_mean"`
	PredEnergyP10  float64 `csv:"pred_energy_p10"`
	PredEnergyP50  float64 `csv:"pred_energy_p50"`
	PredEnergyP90  float64 `csv:"pred_energy_p90"`

	// Resource utilization
	MeanResourceAtPreyPos float64 `csv:"resource_util"`

	// Energy pools (for conservation validation)
	TotalRes       float64 `csv:"total_res"`        // Total resource grid energy
	TotalDet       float64 `csv:"total_det"`        // Total detritus grid energy
	TotalOrganisms float64 `csv:"total_organisms"`  // Total energy in living organisms
	InTransit      float64 `csv:"in_transit"`       // Energy carried by in-transit particles (unused)
	HeatLossAccum  float64 `csv:"heat_loss_accum"`  // Cumulative energy lost to heat
	EnergyInput    float64 `csv:"energy_input"`     // Cumulative energy input from regeneration

	// Diet distribution
	DietMean float64 `csv:"diet_mean"`
	DietStd  float64 `csv:"diet_std"`
	DietP10  float64 `csv:"diet_p10"`
	DietP50  float64 `csv:"diet_p50"`
	DietP90  float64 `csv:"diet_p90"`

	// Clade tracking
	ActiveClades int `csv:"active_clades"`
}

// Percentile calculates the p-th percentile of a sorted slice.
// p should be in [0, 1]. Returns 0 if slice is empty.
func Percentile(sorted []float64, p float64) float64 {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[n-1]
	}

	// Linear interpolation
	idx := p * float64(n-1)
	lo := int(idx)
	hi := lo + 1
	if hi >= n {
		return sorted[n-1]
	}

	frac := idx - float64(lo)
	return sorted[lo]*(1-frac) + sorted[hi]*frac
}

// ComputeEnergyStats calculates mean and percentiles from energy values.
func ComputeEnergyStats(values []float64) (mean, p10, p50, p90 float64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0, 0
	}

	// Calculate mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(n)

	// Sort for percentiles
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	p10 = Percentile(sorted, 0.10)
	p50 = Percentile(sorted, 0.50)
	p90 = Percentile(sorted, 0.90)

	return mean, p10, p50, p90
}

// ComputeDietStats calculates mean, std, and percentiles from diet values.
func ComputeDietStats(values []float64) (mean, std, p10, p50, p90 float64) {
	n := len(values)
	if n == 0 {
		return 0, 0, 0, 0, 0
	}

	// Mean
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean = sum / float64(n)

	// Standard deviation
	var sqDiffSum float64
	for _, v := range values {
		d := v - mean
		sqDiffSum += d * d
	}
	std = math.Sqrt(sqDiffSum / float64(n))

	// Sort for percentiles
	sorted := make([]float64, n)
	copy(sorted, values)
	sort.Float64s(sorted)

	p10 = Percentile(sorted, 0.10)
	p50 = Percentile(sorted, 0.50)
	p90 = Percentile(sorted, 0.90)

	return mean, std, p10, p50, p90
}

// LogValue implements slog.LogValuer for structured logging.
func (s WindowStats) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Int("window_start", int(s.WindowStartTick)),
		slog.Int("window_end", int(s.WindowEndTick)),
		slog.Float64("sim_time", s.SimTimeSec),
		slog.Int("prey", s.PreyCount),
		slog.Int("pred", s.PredCount),
		slog.Int("prey_births", s.PreyBirths),
		slog.Int("pred_births", s.PredBirths),
		slog.Int("prey_deaths", s.PreyDeaths),
		slog.Int("pred_deaths", s.PredDeaths),
		slog.Int("bites_attempted", s.BitesAttempted),
		slog.Int("bites_hit", s.BitesHit),
		slog.Int("kills", s.Kills),
		slog.Int("bites_blocked_digest", s.BitesBlockedDigest),
		slog.Int("bites_missed_refugia", s.BitesMissedRefugia),
		slog.Float64("hit_rate", s.HitRate),
		slog.Float64("kill_rate", s.KillRate),
		slog.Float64("prey_energy_mean", s.PreyEnergyMean),
		slog.Float64("prey_energy_p10", s.PreyEnergyP10),
		slog.Float64("prey_energy_p50", s.PreyEnergyP50),
		slog.Float64("prey_energy_p90", s.PreyEnergyP90),
		slog.Float64("pred_energy_mean", s.PredEnergyMean),
		slog.Float64("pred_energy_p10", s.PredEnergyP10),
		slog.Float64("pred_energy_p50", s.PredEnergyP50),
		slog.Float64("pred_energy_p90", s.PredEnergyP90),
		slog.Float64("resource_util", s.MeanResourceAtPreyPos),
		slog.Float64("total_res", s.TotalRes),
		slog.Float64("total_det", s.TotalDet),
		slog.Float64("total_organisms", s.TotalOrganisms),
		slog.Float64("in_transit", s.InTransit),
		slog.Float64("heat_loss_accum", s.HeatLossAccum),
		slog.Float64("energy_input", s.EnergyInput),
		slog.Float64("diet_mean", s.DietMean),
		slog.Float64("diet_std", s.DietStd),
		slog.Float64("diet_p10", s.DietP10),
		slog.Float64("diet_p50", s.DietP50),
		slog.Float64("diet_p90", s.DietP90),
		slog.Int("active_clades", s.ActiveClades),
	)
}

// LogStats logs the window stats using slog.
func (s WindowStats) LogStats() {
	slog.Info("stats",
		"window_end", s.WindowEndTick,
		"sim_time", s.SimTimeSec,
		"prey", s.PreyCount,
		"pred", s.PredCount,
		"prey_births", s.PreyBirths,
		"pred_births", s.PredBirths,
		"prey_deaths", s.PreyDeaths,
		"pred_deaths", s.PredDeaths,
		"bites_attempted", s.BitesAttempted,
		"bites_hit", s.BitesHit,
		"kills", s.Kills,
		"bites_blocked_digest", s.BitesBlockedDigest,
		"bites_missed_refugia", s.BitesMissedRefugia,
		"hit_rate", s.HitRate,
		"kill_rate", s.KillRate,
		"prey_energy_mean", s.PreyEnergyMean,
		"prey_energy_p10", s.PreyEnergyP10,
		"prey_energy_p50", s.PreyEnergyP50,
		"prey_energy_p90", s.PreyEnergyP90,
		"pred_energy_mean", s.PredEnergyMean,
		"pred_energy_p10", s.PredEnergyP10,
		"pred_energy_p50", s.PredEnergyP50,
		"pred_energy_p90", s.PredEnergyP90,
		"resource_util", s.MeanResourceAtPreyPos,
		"total_res", s.TotalRes,
		"total_det", s.TotalDet,
		"total_organisms", s.TotalOrganisms,
		"in_transit", s.InTransit,
		"heat_loss_accum", s.HeatLossAccum,
		"energy_input", s.EnergyInput,
		"diet_mean", s.DietMean,
		"diet_std", s.DietStd,
		"diet_p10", s.DietP10,
		"diet_p50", s.DietP50,
		"diet_p90", s.DietP90,
		"active_clades", s.ActiveClades,
	)
}


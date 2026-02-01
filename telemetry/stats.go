package telemetry

import (
	"log/slog"
	"sort"
)

// WindowStats holds aggregated statistics for a time window.
type WindowStats struct {
	WindowStartTick int32
	WindowEndTick   int32
	SimTimeSec      float64

	// Population counts at window end
	PreyCount int
	PredCount int

	// Events during window
	PreyBirths  int
	PredBirths  int
	PreyDeaths  int
	PredDeaths  int

	// Hunting
	BitesAttempted     int
	BitesHit           int
	Kills              int
	BitesBlockedDigest int     // Bites blocked by digestion cooldown
	BitesMissedRefugia int     // Bites that missed due to refugia protection
	HitRate            float64 // BitesHit / BitesAttempted (0 if no attempts)
	KillRate           float64 // Kills / BitesHit (0 if no hits)

	// Energy distribution (sampled at window end)
	PreyEnergyMean float64
	PreyEnergyP10  float64
	PreyEnergyP50  float64
	PreyEnergyP90  float64

	PredEnergyMean float64
	PredEnergyP10  float64
	PredEnergyP50  float64
	PredEnergyP90  float64

	// Resource utilization
	MeanResourceAtPreyPos float64
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
	)
}

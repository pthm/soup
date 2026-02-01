package telemetry

import (
	"log/slog"
	"time"
)

// Phase names for the simulation step.
const (
	PhaseResourceField   = "resource_field"
	PhaseSpatialGrid     = "spatial_grid"
	PhaseBehaviorPhysics = "behavior_physics"
	PhaseFeeding         = "feeding"
	PhaseEnergy          = "energy"
	PhaseCooldowns       = "cooldowns"
	PhaseReproduction    = "reproduction"
	PhaseCleanup         = "cleanup"
	PhaseTelemetry       = "telemetry"
)

// PerfSample holds timing data for a single tick.
type PerfSample struct {
	TickDuration time.Duration
	Phases       map[string]time.Duration
}

// PerfCollector tracks performance metrics over a rolling window.
type PerfCollector struct {
	windowSize    int
	samples       []PerfSample
	writeIndex    int
	sampleCount   int
	currentPhases map[string]time.Duration
	tickStart     time.Time
	phaseStart    time.Time
	lastPhase     string

	// Frame timing (for graphics mode)
	lastFrameTime time.Time
	frameDuration time.Duration
}

// NewPerfCollector creates a new performance collector.
// windowSize: number of ticks to average over (e.g., 60 for 1 second at 60fps).
func NewPerfCollector(windowSize int) *PerfCollector {
	if windowSize < 1 {
		windowSize = 60
	}
	return &PerfCollector{
		windowSize:    windowSize,
		samples:       make([]PerfSample, windowSize),
		currentPhases: make(map[string]time.Duration),
	}
}

// StartTick begins timing a new simulation tick.
func (p *PerfCollector) StartTick() {
	p.tickStart = time.Now()
	p.currentPhases = make(map[string]time.Duration)
	p.lastPhase = ""
}

// StartPhase begins timing a specific phase.
func (p *PerfCollector) StartPhase(phase string) {
	now := time.Now()
	// End previous phase if any
	if p.lastPhase != "" {
		p.currentPhases[p.lastPhase] += now.Sub(p.phaseStart)
	}
	p.phaseStart = now
	p.lastPhase = phase
}

// EndTick finishes timing the current tick and records the sample.
func (p *PerfCollector) EndTick() {
	now := time.Now()
	// End final phase
	if p.lastPhase != "" {
		p.currentPhases[p.lastPhase] += now.Sub(p.phaseStart)
	}

	sample := PerfSample{
		TickDuration: now.Sub(p.tickStart),
		Phases:       p.currentPhases,
	}

	p.samples[p.writeIndex] = sample
	p.writeIndex = (p.writeIndex + 1) % p.windowSize
	if p.sampleCount < p.windowSize {
		p.sampleCount++
	}
}

// RecordFrame records frame timing for graphics mode.
func (p *PerfCollector) RecordFrame() {
	now := time.Now()
	if !p.lastFrameTime.IsZero() {
		p.frameDuration = now.Sub(p.lastFrameTime)
	}
	p.lastFrameTime = now
}

// PerfStats holds aggregated performance statistics.
type PerfStats struct {
	// Tick timing
	AvgTickDuration time.Duration
	MinTickDuration time.Duration
	MaxTickDuration time.Duration

	// Phase breakdown (average durations)
	PhaseAvg map[string]time.Duration

	// Phase percentages of total tick time
	PhasePct map[string]float64

	// Throughput
	TicksPerSecond float64

	// Frame timing (graphics mode)
	FrameDuration time.Duration
	FPS           float64
}

// Stats computes aggregated statistics over the current window.
func (p *PerfCollector) Stats() PerfStats {
	// Frame timing is always available (independent of tick samples)
	var fps float64
	if p.frameDuration > 0 {
		fps = float64(time.Second) / float64(p.frameDuration)
	}

	if p.sampleCount == 0 {
		return PerfStats{
			PhaseAvg:      make(map[string]time.Duration),
			PhasePct:      make(map[string]float64),
			FrameDuration: p.frameDuration,
			FPS:           fps,
		}
	}

	var totalTick time.Duration
	var minTick, maxTick time.Duration
	phaseSum := make(map[string]time.Duration)

	// Iterate over valid samples
	for i := 0; i < p.sampleCount; i++ {
		s := p.samples[i]
		totalTick += s.TickDuration

		if i == 0 || s.TickDuration < minTick {
			minTick = s.TickDuration
		}
		if s.TickDuration > maxTick {
			maxTick = s.TickDuration
		}

		for phase, dur := range s.Phases {
			phaseSum[phase] += dur
		}
	}

	avgTick := totalTick / time.Duration(p.sampleCount)

	// Calculate phase averages and percentages
	phaseAvg := make(map[string]time.Duration)
	phasePct := make(map[string]float64)
	for phase, sum := range phaseSum {
		phaseAvg[phase] = sum / time.Duration(p.sampleCount)
		if avgTick > 0 {
			phasePct[phase] = float64(phaseAvg[phase]) / float64(avgTick) * 100
		}
	}

	// Calculate throughput
	var ticksPerSec float64
	if avgTick > 0 {
		ticksPerSec = float64(time.Second) / float64(avgTick)
	}

	return PerfStats{
		AvgTickDuration: avgTick,
		MinTickDuration: minTick,
		MaxTickDuration: maxTick,
		PhaseAvg:        phaseAvg,
		PhasePct:        phasePct,
		TicksPerSecond:  ticksPerSec,
		FrameDuration:   p.frameDuration,
		FPS:             fps,
	}
}

// LogStats logs performance statistics.
func (s PerfStats) LogStats() {
	attrs := []any{
		"avg_tick_us", s.AvgTickDuration.Microseconds(),
		"min_tick_us", s.MinTickDuration.Microseconds(),
		"max_tick_us", s.MaxTickDuration.Microseconds(),
		"ticks_per_sec", int(s.TicksPerSecond),
	}

	if s.FPS > 0 {
		attrs = append(attrs, "fps", int(s.FPS))
	}

	// Add phase breakdowns
	phases := []string{
		PhaseResourceField, PhaseSpatialGrid, PhaseBehaviorPhysics,
		PhaseFeeding, PhaseEnergy, PhaseCooldowns,
		PhaseReproduction, PhaseCleanup, PhaseTelemetry,
	}

	for _, phase := range phases {
		if pct, ok := s.PhasePct[phase]; ok && pct > 0.1 {
			attrs = append(attrs, phase+"_pct", int(pct*10)/10.0)
		}
	}

	slog.Info("perf", attrs...)
}

// LogValue implements slog.LogValuer for structured logging.
func (s PerfStats) LogValue() slog.Value {
	attrs := []slog.Attr{
		slog.Int64("avg_tick_us", s.AvgTickDuration.Microseconds()),
		slog.Int64("min_tick_us", s.MinTickDuration.Microseconds()),
		slog.Int64("max_tick_us", s.MaxTickDuration.Microseconds()),
		slog.Float64("ticks_per_sec", s.TicksPerSecond),
	}

	if s.FPS > 0 {
		attrs = append(attrs, slog.Float64("fps", s.FPS))
	}

	for phase, pct := range s.PhasePct {
		attrs = append(attrs, slog.Float64(phase+"_pct", pct))
	}

	return slog.GroupValue(attrs...)
}

// PerfStatsCSV is a flat struct for CSV export of performance stats.
type PerfStatsCSV struct {
	WindowEnd          int32   `csv:"window_end"`
	AvgTickUS          int64   `csv:"avg_tick_us"`
	MinTickUS          int64   `csv:"min_tick_us"`
	MaxTickUS          int64   `csv:"max_tick_us"`
	TicksPerSec        float64 `csv:"ticks_per_sec"`
	FPS                float64 `csv:"fps"`
	ResourceFieldPct   float64 `csv:"resource_field_pct"`
	SpatialGridPct     float64 `csv:"spatial_grid_pct"`
	BehaviorPhysicsPct float64 `csv:"behavior_physics_pct"`
	FeedingPct         float64 `csv:"feeding_pct"`
	EnergyPct          float64 `csv:"energy_pct"`
	CooldownsPct       float64 `csv:"cooldowns_pct"`
	ReproductionPct    float64 `csv:"reproduction_pct"`
	CleanupPct         float64 `csv:"cleanup_pct"`
	TelemetryPct       float64 `csv:"telemetry_pct"`
}

// ToCSV converts PerfStats to a flat CSV-friendly struct.
func (s PerfStats) ToCSV(windowEnd int32) PerfStatsCSV {
	return PerfStatsCSV{
		WindowEnd:          windowEnd,
		AvgTickUS:          s.AvgTickDuration.Microseconds(),
		MinTickUS:          s.MinTickDuration.Microseconds(),
		MaxTickUS:          s.MaxTickDuration.Microseconds(),
		TicksPerSec:        s.TicksPerSecond,
		FPS:                s.FPS,
		ResourceFieldPct:   s.PhasePct[PhaseResourceField],
		SpatialGridPct:     s.PhasePct[PhaseSpatialGrid],
		BehaviorPhysicsPct: s.PhasePct[PhaseBehaviorPhysics],
		FeedingPct:         s.PhasePct[PhaseFeeding],
		EnergyPct:          s.PhasePct[PhaseEnergy],
		CooldownsPct:       s.PhasePct[PhaseCooldowns],
		ReproductionPct:    s.PhasePct[PhaseReproduction],
		CleanupPct:         s.PhasePct[PhaseCleanup],
		TelemetryPct:       s.PhasePct[PhaseTelemetry],
	}
}

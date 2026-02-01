package telemetry

import (
	"testing"
	"time"
)

func TestPerfCollector_BasicTiming(t *testing.T) {
	pc := NewPerfCollector(10)

	// Simulate a few ticks
	for i := 0; i < 5; i++ {
		pc.StartTick()
		pc.StartPhase(PhaseSpatialGrid)
		time.Sleep(100 * time.Microsecond)
		pc.StartPhase(PhaseBehaviorPhysics)
		time.Sleep(200 * time.Microsecond)
		pc.EndTick()
	}

	stats := pc.Stats()

	// Verify we got timing data
	if stats.AvgTickDuration <= 0 {
		t.Error("expected positive average tick duration")
	}

	// Verify phases are tracked
	if len(stats.PhaseAvg) == 0 {
		t.Error("expected phase averages to be populated")
	}

	if _, ok := stats.PhaseAvg[PhaseSpatialGrid]; !ok {
		t.Error("expected spatial_grid phase to be tracked")
	}

	if _, ok := stats.PhaseAvg[PhaseBehaviorPhysics]; !ok {
		t.Error("expected behavior_physics phase to be tracked")
	}
}

func TestPerfCollector_RollingWindow(t *testing.T) {
	pc := NewPerfCollector(5) // Small window

	// Fill window completely
	for i := 0; i < 10; i++ {
		pc.StartTick()
		pc.StartPhase(PhaseSpatialGrid)
		pc.EndTick()
	}

	stats := pc.Stats()

	// Should have data
	if stats.AvgTickDuration <= 0 {
		t.Error("expected positive average tick duration after window filled")
	}

	if stats.TicksPerSecond <= 0 {
		t.Error("expected positive ticks per second")
	}
}

func TestPerfCollector_PhasePercentages(t *testing.T) {
	pc := NewPerfCollector(10)

	// Simulate with uneven phase durations
	for i := 0; i < 5; i++ {
		pc.StartTick()
		pc.StartPhase("fast")
		time.Sleep(10 * time.Microsecond)
		pc.StartPhase("slow")
		time.Sleep(100 * time.Microsecond)
		pc.EndTick()
	}

	stats := pc.Stats()

	fastPct := stats.PhasePct["fast"]
	slowPct := stats.PhasePct["slow"]

	// Slow phase should take more % than fast
	if slowPct <= fastPct {
		t.Errorf("expected slow phase (%v%%) > fast phase (%v%%)", slowPct, fastPct)
	}
}

func TestPerfCollector_EmptyStats(t *testing.T) {
	pc := NewPerfCollector(10)

	stats := pc.Stats()

	// Empty collector should return zero values without panicking
	if stats.AvgTickDuration != 0 {
		t.Error("expected zero avg tick duration for empty collector")
	}

	if stats.PhaseAvg == nil {
		t.Error("expected non-nil PhaseAvg map")
	}

	if stats.PhasePct == nil {
		t.Error("expected non-nil PhasePct map")
	}
}

func TestPerfCollector_FrameTiming(t *testing.T) {
	pc := NewPerfCollector(10)

	// First call establishes baseline
	pc.RecordFrame()
	time.Sleep(16 * time.Millisecond) // ~60fps frame time
	// Second call measures duration
	pc.RecordFrame()

	stats := pc.Stats()

	if stats.FrameDuration < 15*time.Millisecond {
		t.Errorf("expected frame duration >= 15ms, got %v", stats.FrameDuration)
	}

	if stats.FPS <= 0 {
		t.Error("expected positive FPS")
	}

	// With 16ms frames, expect ~60 FPS (allow range 40-80)
	if stats.FPS < 40 || stats.FPS > 80 {
		t.Errorf("expected FPS between 40-80 with 16ms frame time, got %v", stats.FPS)
	}
}

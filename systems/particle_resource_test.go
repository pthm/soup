package systems

import (
	"testing"

	"github.com/pthm-cable/soup/config"
)

func init() {
	// Initialize config for tests
	config.MustInit("")
}

func TestParticleResourceFieldCreation(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42)

	if pf == nil {
		t.Fatal("expected non-nil particle field")
	}

	// Check dimensions
	w, h := pf.GridSize()
	if w != 64 || h != 64 {
		t.Errorf("expected grid size 64x64, got %dx%d", w, h)
	}

	// Check world dimensions
	if pf.Width() != 1280 || pf.Height() != 720 {
		t.Errorf("expected world 1280x720, got %.0fx%.0f", pf.Width(), pf.Height())
	}

	// Check initial state
	if pf.Count != 0 {
		t.Errorf("expected 0 particles, got %d", pf.Count)
	}
}

func TestParticleResourceFieldSampling(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42)

	// Sample at center
	v := pf.Sample(640, 360)
	if v < 0 || v > 1 {
		t.Errorf("expected sample in [0,1], got %f", v)
	}

	// Sample at edges (should wrap)
	v1 := pf.Sample(0, 0)
	v2 := pf.Sample(1280, 720)
	// Both should be valid
	if v1 < 0 || v1 > 1 || v2 < 0 || v2 > 1 {
		t.Errorf("expected edge samples in [0,1], got %f, %f", v1, v2)
	}
}

func TestParticleResourceFieldStep(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42)

	initialMass := pf.TotalMass()

	// Run a few steps
	dt := float32(1.0 / 60.0)
	for i := 0; i < 100; i++ {
		pf.Step(dt, true)
	}

	// Should have spawned some particles
	if pf.Count == 0 {
		t.Error("expected some particles after 100 steps")
	}

	// Mass should have changed due to spawning
	finalMass := pf.TotalMass()
	if finalMass <= initialMass {
		t.Errorf("expected mass to increase from spawning, got initial=%.4f, final=%.4f", initialMass, finalMass)
	}
}

func TestParticleResourceFieldGraze(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42)

	// Run a few steps to establish initial state
	dt := float32(1.0 / 60.0)
	for i := 0; i < 10; i++ {
		pf.Step(dt, true)
	}

	// Sample before graze
	before := pf.Sample(640, 360)

	// Graze
	removed := pf.Graze(640, 360, 0.5, dt, 1)

	// Sample after graze
	after := pf.Sample(640, 360)

	// Resource should have decreased
	if removed <= 0 && before > 0.01 {
		t.Errorf("expected positive removal when resource available, got %.6f (before=%.4f)", removed, before)
	}
	if after > before && removed > 0 {
		t.Errorf("expected resource to decrease after grazing, before=%.4f, after=%.4f", before, after)
	}
}

func TestParticleResourceFieldMassConservation(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42)

	// Set spawn rate to 0 to test mass conservation without new mass entering
	pf.SpawnRate = 0

	// Run a few steps to let particles deposit/pickup
	dt := float32(1.0 / 60.0)
	initialMass := pf.TotalMass()

	for i := 0; i < 100; i++ {
		pf.Step(dt, true)
	}

	finalMass := pf.TotalMass()

	// Mass should be conserved (no spawning, no consumption)
	// Allow small tolerance for floating point
	tolerance := initialMass * 0.001
	if diff := finalMass - initialMass; diff > tolerance || diff < -tolerance {
		t.Errorf("mass not conserved: initial=%.6f, final=%.6f, diff=%.6f", initialMass, finalMass, diff)
	}
}

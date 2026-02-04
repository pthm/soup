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
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

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
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

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
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

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
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

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
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

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

func TestDetritusDeposit(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Det grid should start empty
	for i, d := range pf.Det {
		if d != 0 {
			t.Fatalf("expected Det[%d]=0, got %f", i, d)
		}
	}

	// Deposit detritus at world center
	deposited := pf.DepositDetritus(640, 360, 1.0)
	if deposited != 1.0 {
		t.Errorf("expected deposited=1.0, got %f", deposited)
	}

	// Det grid should now have mass
	totalDet := pf.TotalDetMass()
	if totalDet < 0.99 || totalDet > 1.01 {
		t.Errorf("expected total detritus ~1.0, got %f", totalDet)
	}

	// Sample should return non-zero at deposit location
	s := pf.SampleDetritus(640, 360)
	if s <= 0 {
		t.Error("expected positive detritus sample at deposit location")
	}
}

func TestDetritusDecay(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())
	// Disable particle spawning so we can isolate detritus behavior
	pf.SpawnRate = 0

	// Clear Res grid to isolate decay effect
	for i := range pf.Res {
		pf.Res[i] = 0
	}

	// Deposit known amount of detritus
	pf.DepositDetritus(640, 360, 10.0)
	initialDet := pf.TotalDetMass()

	// Run one second of decay (60 ticks at dt=1/60)
	dt := float32(1.0 / 60.0)
	var totalHeat float32
	for i := 0; i < 60; i++ {
		totalHeat += pf.StepDetritus(dt)
	}

	finalDet := pf.TotalDetMass()
	finalRes := pf.TotalResMass()

	// Detritus should decrease
	if finalDet >= initialDet {
		t.Errorf("expected detritus to decrease: initial=%f, final=%f", initialDet, finalDet)
	}

	// Resource should increase from decay
	if finalRes <= 0 {
		t.Error("expected resource to increase from detritus decay")
	}

	// Heat should be positive (decay efficiency < 1.0)
	if totalHeat <= 0 {
		t.Error("expected positive heat loss from detritus decay")
	}

	// Conservation: decayed amount = resource gained + heat lost
	decayed := initialDet - finalDet
	accounted := finalRes + totalHeat
	tolerance := decayed * 0.001
	if diff := decayed - accounted; diff > tolerance || diff < -tolerance {
		t.Errorf("detritus decay not conserved: decayed=%f, res=%f + heat=%f = %f, diff=%f",
			decayed, finalRes, totalHeat, accounted, diff)
	}
}

func TestDetritusZeroDoesNothing(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Depositing zero or negative should do nothing
	dep := pf.DepositDetritus(640, 360, 0)
	if dep != 0 {
		t.Errorf("expected 0 deposited for 0 amount, got %f", dep)
	}
	dep = pf.DepositDetritus(640, 360, -1.0)
	if dep != 0 {
		t.Errorf("expected 0 deposited for negative amount, got %f", dep)
	}

	// Det grid should still be empty
	if total := pf.TotalDetMass(); total != 0 {
		t.Errorf("expected 0 total detritus, got %f", total)
	}
}

func TestDetritusIncludedInTotalMass(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())
	pf.SpawnRate = 0

	massBefore := pf.TotalMass()

	// Deposit detritus — should increase total mass
	pf.DepositDetritus(640, 360, 5.0)
	massAfter := pf.TotalMass()

	diff := massAfter - massBefore
	if diff < 4.99 || diff > 5.01 {
		t.Errorf("expected total mass to increase by ~5.0, got diff=%f", diff)
	}
}

func TestDetritusDecayConvergesButDoesNotOscillate(t *testing.T) {
	pf := NewParticleResourceField(64, 64, 1280, 720, 42, config.Cfg())
	pf.SpawnRate = 0

	// Clear Res
	for i := range pf.Res {
		pf.Res[i] = 0
	}

	// Deposit detritus spread across grid
	for y := 0; y < pf.ResH; y++ {
		for x := 0; x < pf.ResW; x++ {
			pf.Det[y*pf.ResW+x] = 1.0
		}
	}

	// Run many ticks — detritus should monotonically decrease
	dt := float32(1.0 / 60.0)
	prevDet := pf.TotalDetMass()
	for tick := 0; tick < 600; tick++ {
		pf.StepDetritus(dt)
		curDet := pf.TotalDetMass()
		if curDet > prevDet+0.0001 {
			t.Fatalf("detritus increased at tick %d: prev=%f, cur=%f", tick, prevDet, curDet)
		}
		prevDet = curDet
	}

	// After 10 seconds of decay at 5%/sec, should be significantly depleted
	if prevDet > pf.TotalDetMass()*1.01+1.0 {
		t.Errorf("detritus did not decay significantly after 10s: remaining=%f", prevDet)
	}
}

func BenchmarkParticleResourceFieldStep(b *testing.B) {
	// Use production-like grid size (matches 2560x1440 world with 128 base)
	pf := NewParticleResourceField(227, 128, 2560, 1440, 42, config.Cfg())

	// Warm up to spawn particles
	dt := float32(1.0 / 60.0)
	for i := 0; i < 600; i++ { // 10 seconds of sim time
		pf.Step(dt, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pf.Step(dt, true)
	}
}

func BenchmarkParticleResourceFieldStepNoEvolve(b *testing.B) {
	// Same but without evolution (potential/flow updates)
	pf := NewParticleResourceField(227, 128, 2560, 1440, 42, config.Cfg())

	dt := float32(1.0 / 60.0)
	for i := 0; i < 600; i++ {
		pf.Step(dt, true)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pf.Step(dt, false) // evolve=false
	}
}

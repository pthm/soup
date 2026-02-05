package systems

import (
	"testing"

	"github.com/pthm-cable/soup/config"
)

func init() {
	// Initialize config for tests
	config.MustInit("")
}

func TestResourceFieldCreation(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	if rf == nil {
		t.Fatal("expected non-nil resource field")
	}

	// Check dimensions
	w, h := rf.GridSize()
	if w != 64 || h != 64 {
		t.Errorf("expected grid size 64x64, got %dx%d", w, h)
	}

	// Check world dimensions
	if rf.Width() != 1280 || rf.Height() != 720 {
		t.Errorf("expected world 1280x720, got %.0fx%.0f", rf.Width(), rf.Height())
	}
}

func TestResourceFieldSampling(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Sample at center
	v := rf.Sample(640, 360)
	if v < 0 || v > 1 {
		t.Errorf("expected sample in [0,1], got %f", v)
	}

	// Sample at edges (should wrap)
	v1 := rf.Sample(0, 0)
	v2 := rf.Sample(1280, 720)
	if v1 < 0 || v1 > 1 || v2 < 0 || v2 > 1 {
		t.Errorf("expected edge samples in [0,1], got %f, %f", v1, v2)
	}
}

func TestResourceFieldGraze(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Sample before graze
	before := rf.Sample(640, 360)

	// Graze
	dt := float32(1.0 / 60.0)
	removed := rf.Graze(640, 360, 0.5, dt, 1)

	// Sample after graze
	after := rf.Sample(640, 360)

	// Resource should have decreased
	if removed <= 0 && before > 0.01 {
		t.Errorf("expected positive removal when resource available, got %.6f (before=%.4f)", removed, before)
	}
	if after > before && removed > 0 {
		t.Errorf("expected resource to decrease after grazing, before=%.4f, after=%.4f", before, after)
	}
}

func TestResourceFieldRegeneration(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())
	rf.RegenRate = 0.1 // 10% per second for faster testing

	// Deplete some resource
	dt := float32(1.0 / 60.0)
	for i := 0; i < 60; i++ {
		rf.Graze(640, 360, 1.0, dt, 1) // Heavy grazing
	}

	depletedValue := rf.Sample(640, 360)

	// Get the capacity at this location
	u := 640.0 / rf.Width()
	v := 360.0 / rf.Height()
	cx := int(u * float32(rf.W))
	cy := int(v * float32(rf.H))
	targetValue := rf.Cap[cy*rf.W+cx]

	// Run regeneration steps
	for i := 0; i < 600; i++ { // 10 seconds
		rf.Step(dt, true)
	}

	regeneratedValue := rf.Sample(640, 360)

	// Resource should have moved towards capacity
	if regeneratedValue <= depletedValue {
		t.Errorf("expected resource to regenerate towards capacity, depleted=%.4f, regenerated=%.4f",
			depletedValue, regeneratedValue)
	}
	if regeneratedValue > targetValue*1.01 {
		t.Errorf("resource exceeded capacity target: value=%.4f, target=%.4f",
			regeneratedValue, targetValue)
	}
}

func TestDetritusDeposit(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Det grid should start empty
	for i, d := range rf.Det {
		if d != 0 {
			t.Fatalf("expected Det[%d]=0, got %f", i, d)
		}
	}

	// Deposit detritus at world center
	deposited := rf.DepositDetritus(640, 360, 1.0)
	if deposited != 1.0 {
		t.Errorf("expected deposited=1.0, got %f", deposited)
	}

	// Det grid should now have mass
	totalDet := rf.TotalDetMass()
	if totalDet < 0.99 || totalDet > 1.01 {
		t.Errorf("expected total detritus ~1.0, got %f", totalDet)
	}

	// Sample should return non-zero at deposit location
	s := rf.SampleDetritus(640, 360)
	if s <= 0 {
		t.Error("expected positive detritus sample at deposit location")
	}
}

func TestDetritusDecay(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())
	rf.RegenRate = 0 // Disable regen to isolate decay

	// Clear Res grid to isolate decay effect
	for i := range rf.Res {
		rf.Res[i] = 0
	}

	// Deposit known amount of detritus
	rf.DepositDetritus(640, 360, 10.0)
	initialDet := rf.TotalDetMass()

	// Run one second of decay (60 ticks at dt=1/60)
	dt := float32(1.0 / 60.0)
	for i := 0; i < 60; i++ {
		rf.Step(dt, true)
	}

	finalDet := rf.TotalDetMass()
	finalRes := rf.TotalResMass()

	// Detritus should decrease
	if finalDet >= initialDet {
		t.Errorf("expected detritus to decrease: initial=%f, final=%f", initialDet, finalDet)
	}

	// Resource should increase from decay
	if finalRes <= 0 {
		t.Error("expected resource to increase from detritus decay")
	}
}

func TestDetritusZeroDoesNothing(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Depositing zero or negative should do nothing
	dep := rf.DepositDetritus(640, 360, 0)
	if dep != 0 {
		t.Errorf("expected 0 deposited for 0 amount, got %f", dep)
	}
	dep = rf.DepositDetritus(640, 360, -1.0)
	if dep != 0 {
		t.Errorf("expected 0 deposited for negative amount, got %f", dep)
	}

	// Det grid should still be empty
	if total := rf.TotalDetMass(); total != 0 {
		t.Errorf("expected 0 total detritus, got %f", total)
	}
}

func TestEquilibriumSkip(t *testing.T) {
	rf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())
	rf.RegenRate = 1.0 // Fast regen for testing
	rf.TimeSpeed = 0   // Disable animation for this test

	// Graze to disturb a cell
	dt := float32(1.0 / 60.0)
	rf.Graze(640, 360, 0.5, dt, 1)
	depletedValue := rf.Sample(640, 360)

	// Get capacity at that location
	u := 640.0 / rf.Width()
	v := 360.0 / rf.Height()
	cx := int(u * float32(rf.W))
	cy := int(v * float32(rf.H))
	capValue := rf.Cap[cy*rf.W+cx]

	// After enough steps, resource should return to capacity
	for i := 0; i < 1000; i++ {
		rf.Step(dt, true)
	}

	finalValue := rf.Sample(640, 360)

	// Should have regenerated close to capacity
	diff := finalValue - capValue
	if diff < 0 {
		diff = -diff
	}
	if diff > 0.001 {
		t.Errorf("expected resource to return to capacity: final=%.6f, cap=%.6f, depleted=%.6f",
			finalValue, capValue, depletedValue)
	}
}

func BenchmarkResourceFieldStep(b *testing.B) {
	rf := NewResourceField(128, 128, 2560, 1440, 42, config.Cfg())
	dt := float32(1.0 / 60.0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rf.Step(dt, true)
	}
}

func BenchmarkResourceFieldStepWithActivity(b *testing.B) {
	rf := NewResourceField(128, 128, 2560, 1440, 42, config.Cfg())
	dt := float32(1.0 / 60.0)

	// Disturb some cells by grazing
	for j := 0; j < 200; j++ {
		x := float32(j*13%2560)
		y := float32(j*17%1440)
		rf.Graze(x, y, 0.5, dt, 1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Simulate ongoing activity: graze then step
		for j := 0; j < 200; j++ {
			x := float32((i*7 + j*13) % 2560)
			y := float32((i*11 + j*17) % 1440)
			rf.Graze(x, y, 0.01, dt, 1)
		}
		rf.Step(dt, true)
	}
}

func BenchmarkResourceFieldGraze(b *testing.B) {
	rf := NewResourceField(128, 128, 2560, 1440, 42, config.Cfg())
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

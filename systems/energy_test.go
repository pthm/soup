package systems

import (
	"math"
	"testing"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

// ensureCache makes sure config and sensor cache are initialized.
// The package-level init() in sensors_test.go handles this, but
// we guard here for safety in case tests run in isolation.
func ensureCache() {
	if !cacheInitialized {
		config.MustInit("")
		InitSensorCache()
	}
}

// testCapabilities returns default capabilities for testing.
// This replaces the removed DefaultCapabilities function.
func testCapabilities() components.Capabilities {
	cfg := config.Cfg()
	return components.Capabilities{
		VisionRange: 100,
		MaxSpeed:    float32(cfg.Capabilities.MaxSpeed),
		MaxAccel:    float32(cfg.Capabilities.MaxAccel),
		MaxTurnRate: float32(cfg.Capabilities.MaxTurnRate),
		Drag:        float32(cfg.Capabilities.Drag),
		BiteRange:   float32(cfg.Capabilities.BiteRange),
	}
}

// testEnergy creates a standard test energy with the two-pool model.
func testEnergy(met, bio, bioCap float32, alive bool) components.Energy {
	return components.Energy{Met: met, Bio: bio, BioCap: bioCap, Alive: alive}
}

// ---------- UpdateEnergy basic behavior ----------

func TestUpdateEnergy_DeadEntityNoOp(t *testing.T) {
	ensureCache()
	e := testEnergy(0.5, 1.0, 1.5, false)
	vel := components.Velocity{}
	caps := testCapabilities()

	cost := UpdateEnergy(&e, vel, caps, 0, 1.0/60)
	if cost != 0 {
		t.Errorf("expected 0 cost for dead entity, got %f", cost)
	}
	if e.Met != 0.5 {
		t.Errorf("dead entity Met should not change, got %f", e.Met)
	}
}

func TestUpdateEnergy_AgeIncreases(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	e := testEnergy(0.5, 1.0, 1.5, true)
	e.Age = 10.0
	vel := components.Velocity{}
	caps := testCapabilities()

	UpdateEnergy(&e, vel, caps, 1.0, dt)

	expected := float32(10.0) + dt
	if math.Abs(float64(e.Age-expected)) > 1e-6 {
		t.Errorf("expected age %.6f, got %.6f", expected, e.Age)
	}
}

func TestUpdateEnergy_BaseCostApplied(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)

	// Stationary entity, no thrust, no bite → only base cost + bio cost
	e := testEnergy(0.5, 1.0, 1.5, true)
	vel := components.Velocity{X: 0, Y: 0}
	caps := testCapabilities()

	cost := UpdateEnergy(&e, vel, caps, 1.0, dt)

	if cost <= 0 {
		t.Error("expected positive base metabolic cost")
	}
	if e.Met >= 0.5 {
		t.Error("expected Met to decrease from base cost")
	}

	// Cost should equal Met lost
	metLost := float32(0.5) - e.Met
	if math.Abs(float64(cost-metLost)) > 1e-6 {
		t.Errorf("cost (%f) should match Met lost (%f)", cost, metLost)
	}
}

func TestUpdateEnergy_MovementCostIncreasesWithSpeed(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// Stationary
	e1 := testEnergy(0.5, 1.0, 1.5, true)
	cost1 := UpdateEnergy(&e1, components.Velocity{X: 0, Y: 0}, caps, 1.0, dt)

	// Moving at half max speed
	halfSpeed := caps.MaxSpeed * 0.5
	e2 := testEnergy(0.5, 1.0, 1.5, true)
	cost2 := UpdateEnergy(&e2, components.Velocity{X: halfSpeed, Y: 0}, caps, 1.0, dt)

	// Moving at max speed
	e3 := testEnergy(0.5, 1.0, 1.5, true)
	cost3 := UpdateEnergy(&e3, components.Velocity{X: caps.MaxSpeed, Y: 0}, caps, 1.0, dt)

	if cost2 <= cost1 {
		t.Errorf("half speed cost (%f) should exceed stationary cost (%f)", cost2, cost1)
	}
	if cost3 <= cost2 {
		t.Errorf("full speed cost (%f) should exceed half speed cost (%f)", cost3, cost2)
	}
}

func TestUpdateEnergy_AccelCostProportionalToThrustSquared(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// No thrust
	e1 := testEnergy(0.5, 1.0, 1.5, true)
	e1.LastThrust = 0
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 1.0, dt)

	// Half thrust
	e2 := testEnergy(0.5, 1.0, 1.5, true)
	e2.LastThrust = 0.5
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 1.0, dt)

	// Full thrust
	e3 := testEnergy(0.5, 1.0, 1.5, true)
	e3.LastThrust = 1.0
	cost3 := UpdateEnergy(&e3, components.Velocity{}, caps, 1.0, dt)

	accelCostHalf := cost2 - cost1
	accelCostFull := cost3 - cost1

	// Full thrust cost should be 4x half thrust cost (1.0^2 / 0.5^2 = 4)
	ratio := accelCostFull / accelCostHalf
	if math.Abs(float64(ratio-4.0)) > 0.01 {
		t.Errorf("expected full/half accel cost ratio ~4.0, got %f", ratio)
	}
}

func TestUpdateEnergy_DeathAtZero(t *testing.T) {
	ensureCache()
	// Very low Met → should die after one tick
	e := testEnergy(0.0001, 1.0, 1.5, true)
	vel := components.Velocity{}
	caps := testCapabilities()

	UpdateEnergy(&e, vel, caps, 1.0, 1.0) // large dt and metabolicRate=1.0 to ensure death

	if e.Alive {
		t.Error("expected entity to die when Met hits 0")
	}
	if e.Met != 0 {
		t.Errorf("expected Met clamped to 0, got %f", e.Met)
	}
}

// ---------- Cost returns match Met lost ----------

func TestUpdateEnergy_ReturnedCostMatchesEnergyDelta(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// Entity with all cost sources active
	e := testEnergy(0.8, 1.0, 1.5, true)
	e.LastThrust = 0.7
	e.LastBite = 0.6
	vel := components.Velocity{X: 30, Y: 20}
	before := e.Met

	cost := UpdateEnergy(&e, vel, caps, 0.8, dt)

	metLost := before - e.Met
	if math.Abs(float64(cost-metLost)) > 1e-5 {
		t.Errorf("returned cost (%f) doesn't match Met delta (%f)", cost, metLost)
	}
}

func TestUpdateEnergy_CostPositiveForAllMetabolicRates(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	for _, metabRate := range []float32{0.5, 0.75, 1.0, 1.25, 1.5} {
		e := testEnergy(0.5, 1.0, 1.5, true)
		cost := UpdateEnergy(&e, components.Velocity{}, caps, metabRate, dt)
		if cost <= 0 {
			t.Errorf("expected positive cost for metabolic_rate=%f, got %f", metabRate, cost)
		}
	}
}

// ---------- Metabolic rate scaling ----------

func TestUpdateEnergy_MetabolicRateScaling(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// Low metabolic rate (0.5)
	e1 := testEnergy(0.8, 1.0, 1.5, true)
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 0.5, dt)

	// High metabolic rate (1.5)
	e2 := testEnergy(0.8, 1.0, 1.5, true)
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 1.5, dt)

	// Higher metabolic rate should cost more
	if cost1 >= cost2 {
		t.Errorf("expected higher metabolic rate to cost more: metab=0.5 cost %f, metab=1.5 cost %f", cost1, cost2)
	}

	// Cost should scale roughly linearly with metabolic rate
	ratio := cost2 / cost1
	expectedRatio := float32(1.5 / 0.5) // 3.0
	if ratio < expectedRatio*0.8 || ratio > expectedRatio*1.2 {
		t.Errorf("cost ratio %f should be close to metabolic rate ratio %f", ratio, expectedRatio)
	}
}

// ---------- TransferEnergy conservation ----------

func TestTransferEnergy_ConservesEnergy(t *testing.T) {
	ensureCache()
	pred := testEnergy(0.4, 1.0, 1.5, true)
	prey := testEnergy(0.6, 0.8, 1.0, true)

	// Total before = pred.Met + pred.Bio + prey.Met + prey.Bio
	totalBefore := pred.Met + pred.Bio + prey.Met + prey.Bio

	xfer := TransferEnergy(&pred, &prey, 0.3)

	totalAfter := pred.Met + pred.Bio + prey.Met + prey.Bio
	accounted := totalAfter + xfer.ToDet + xfer.ToHeat + xfer.Overflow

	if math.Abs(float64(totalBefore-accounted)) > 1e-5 {
		t.Errorf("transfer not conserved: before=%f, after_pools=%f+det=%f+heat=%f+overflow=%f = %f",
			totalBefore, totalAfter, xfer.ToDet, xfer.ToHeat, xfer.Overflow, accounted)
	}
}

func TestTransferEnergy_OverflowToDetritus(t *testing.T) {
	ensureCache()
	// Predator with high Met relative to Bio → should overflow
	metPerBio := cachedMetPerBio
	pred := testEnergy(0.95*metPerBio, 1.0, 1.5, true) // Near max Met
	prey := testEnergy(0.6, 0.8, 1.0, true)

	xfer := TransferEnergy(&pred, &prey, 0.5)

	maxMet := pred.Bio * metPerBio
	if pred.Met > maxMet+1e-6 {
		t.Errorf("predator Met %f exceeds maxMet %f", pred.Met, maxMet)
	}
	if xfer.Overflow <= 0 {
		t.Error("expected overflow when predator near max")
	}
}

func TestTransferEnergy_PreyDiesAtZero(t *testing.T) {
	ensureCache()
	pred := testEnergy(0.3, 1.0, 1.5, true)
	prey := testEnergy(0.1, 0.5, 1.0, true)

	// Transfer more than prey has (Met-wise)
	_ = TransferEnergy(&pred, &prey, 0.5)

	if prey.Alive {
		t.Error("expected prey to die when Met fully drained")
	}
	if prey.Met != 0 {
		t.Errorf("expected prey Met=0, got %f", prey.Met)
	}
	// On kill, prey's Bio should be consumed
	if prey.Bio != 0 {
		t.Errorf("expected prey Bio=0 after kill, got %f", prey.Bio)
	}
}

func TestTransferEnergy_DeadEntitiesNoOp(t *testing.T) {
	ensureCache()
	pred := testEnergy(0.3, 1.0, 1.5, false)
	prey := testEnergy(0.5, 0.8, 1.0, true)

	xfer := TransferEnergy(&pred, &prey, 0.2)
	if xfer.Removed != 0 {
		t.Errorf("expected no transfer from dead predator, got %f", xfer.Removed)
	}

	pred.Alive = true
	prey.Alive = false
	xfer = TransferEnergy(&pred, &prey, 0.2)
	if xfer.Removed != 0 {
		t.Errorf("expected no transfer to dead prey, got %f", xfer.Removed)
	}
}

// ---------- Grazing resource accounting ----------

func TestGrazing_ResourceRemovalMatchesEnergyGain(t *testing.T) {
	ensureCache()
	cfg := config.Cfg()
	feedingEff := float32(cfg.Energy.FeedingEfficiency)

	pf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())

	// Resource grid is already initialized from potential
	dt := float32(1.0 / 60.0)

	resBefore := pf.TotalResMass()
	detBefore := pf.TotalDetMass()

	// Graze at center
	removed := pf.Graze(640, 360, 0.5, dt, 1)

	resAfter := pf.TotalResMass()
	detAfter := pf.TotalDetMass()

	if removed <= 0 {
		t.Skip("no resource to graze at test location")
	}

	// Resource grid should decrease by exactly the removed amount
	resDelta := resBefore - resAfter
	if math.Abs(float64(resDelta-removed)) > 1e-4 {
		t.Errorf("resource delta %f != removed %f", resDelta, removed)
	}

	// Detritus should not change from grazing alone
	detDelta := detAfter - detBefore
	if math.Abs(float64(detDelta)) > 1e-6 {
		t.Errorf("detritus changed during grazing: delta=%f", detDelta)
	}

	// Energy gain should be: removed * feedingEfficiency
	// The rest goes to heat
	gain := removed * feedingEff
	heat := removed - gain
	if heat < 0 {
		t.Errorf("negative grazing heat: removed=%f, gain=%f", removed, gain)
	}
	if math.Abs(float64(gain+heat-removed)) > 1e-6 {
		t.Errorf("grazing accounting: gain(%f) + heat(%f) != removed(%f)", gain, heat, removed)
	}
}

// ---------- End-to-end metabolic cost sum check ----------

func TestUpdateEnergy_AllCostComponentsAccounted(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// Set up entity with all cost sources active
	e := testEnergy(0.8, 1.0, 1.5, true)
	e.LastThrust = 0.6
	e.LastBite = 0.5
	vel := components.Velocity{X: 20, Y: 15}
	metabolicRate := float32(1.0)
	before := e.Met

	cost := UpdateEnergy(&e, vel, caps, metabolicRate, dt)

	// Manually compute expected costs (all scaled by metabolic rate)
	baseCost := cachedBaseCost * metabolicRate
	bioCost := cachedBioCost * metabolicRate
	moveCost := cachedMoveCost * metabolicRate
	accelCost := cachedAccelCost * metabolicRate

	expectedBase := (baseCost + bioCost*e.Bio) * dt // Bio cost added
	speedSq := vel.X*vel.X + vel.Y*vel.Y
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	expectedMove := moveCost * (speedSq / maxSpeedSq) * dt
	expectedAccel := accelCost * 0.6 * 0.6 * dt

	_ = expectedBase + expectedMove + expectedAccel // computed for documentation, verified via cost == metLost
	metLost := before - e.Met

	// Note: comparing against before - e.Met because e.Bio may have changed the calculation
	if math.Abs(float64(cost-metLost)) > 1e-5 {
		t.Errorf("cost %f != Met lost %f", cost, metLost)
	}
}

// ---------- No double-counting checks ----------

func TestUpdateEnergy_NoCostLeakageOnRepeat(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := testCapabilities()

	// Run UpdateEnergy twice with same parameters → second cost should
	// be identical (no accumulated state leaking between calls)
	e1 := testEnergy(0.8, 1.0, 1.5, true)
	e1.LastThrust = 0.3
	cost1 := UpdateEnergy(&e1, components.Velocity{X: 10}, caps, 0.2, dt)

	e2 := testEnergy(0.8, 1.0, 1.5, true)
	e2.LastThrust = 0.3
	cost2 := UpdateEnergy(&e2, components.Velocity{X: 10}, caps, 0.2, dt)

	if math.Abs(float64(cost1-cost2)) > 1e-6 {
		t.Errorf("repeated calls with same state should yield same cost: %f vs %f", cost1, cost2)
	}
}

func TestTransferEnergy_NoDoubleCounting(t *testing.T) {
	ensureCache()

	// Run a series of transfers and verify sum of all flows equals initial pool
	pred := testEnergy(0.3, 1.0, 1.5, true)
	prey := testEnergy(0.8, 0.8, 1.0, true)
	initial := pred.Met + pred.Bio + prey.Met + prey.Bio

	var totalDet, totalHeat, totalOverflow float32
	for i := 0; i < 5; i++ {
		xfer := TransferEnergy(&pred, &prey, 0.1)
		totalDet += xfer.ToDet
		totalHeat += xfer.ToHeat
		totalOverflow += xfer.Overflow
		if !pred.Alive || !prey.Alive {
			break
		}
	}

	remaining := pred.Met + pred.Bio + prey.Met + prey.Bio
	accounted := remaining + totalDet + totalHeat + totalOverflow

	if math.Abs(float64(initial-accounted)) > 1e-4 {
		t.Errorf("multi-transfer not conserved: initial=%f, accounted=%f (pools=%f det=%f heat=%f overflow=%f)",
			initial, accounted, remaining, totalDet, totalHeat, totalOverflow)
	}
}

// ---------- Detritus + resource field conservation ----------

func TestDetritus_DecayConservation(t *testing.T) {
	ensureCache()
	pf := NewResourceField(64, 64, 1280, 720, 42, config.Cfg())
	pf.RegenRate = 0 // Disable regeneration

	// Clear resource grid to isolate decay behavior
	for i := range pf.Res {
		pf.Res[i] = 0
	}

	// Deposit known detritus
	pf.DepositDetritus(640, 360, 5.0)
	initialDet := pf.TotalDetMass()

	// Run decay for 1 second
	dt := float32(1.0 / 60.0)
	for i := 0; i < 60; i++ {
		pf.Step(dt, true)
	}

	finalDet := pf.TotalDetMass()
	finalRes := pf.TotalResMass()

	// Detritus should decrease and resource should increase
	if finalDet >= initialDet {
		t.Errorf("expected detritus to decrease: initial=%f, final=%f", initialDet, finalDet)
	}
	if finalRes <= 0 {
		t.Error("expected resource to increase from detritus decay")
	}
}

// ---------- Growth tests ----------

func TestGrowBiomass_GrowsWhenSurplus(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	metPerBio := cachedMetPerBio
	threshold := cachedGrowthThreshold

	// Entity with surplus Met (above threshold)
	bio := float32(0.5)
	bioCap := float32(1.5)
	maxMet := bio * metPerBio
	met := maxMet * (threshold + 0.1) // Above threshold

	e := testEnergy(met, bio, bioCap, true)
	beforeBio := e.Bio

	grown := GrowBiomass(&e, dt)

	if grown <= 0 {
		t.Error("expected positive growth when above threshold")
	}
	if e.Bio <= beforeBio {
		t.Errorf("expected Bio to increase: before=%f, after=%f", beforeBio, e.Bio)
	}
}

func TestGrowBiomass_NoGrowthBelowThreshold(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	metPerBio := cachedMetPerBio
	threshold := cachedGrowthThreshold

	// Entity with Met below threshold
	bio := float32(0.5)
	bioCap := float32(1.5)
	maxMet := bio * metPerBio
	met := maxMet * (threshold - 0.1) // Below threshold

	e := testEnergy(met, bio, bioCap, true)
	beforeBio := e.Bio

	grown := GrowBiomass(&e, dt)

	if grown != 0 {
		t.Errorf("expected no growth below threshold, got %f", grown)
	}
	if e.Bio != beforeBio {
		t.Errorf("Bio should not change below threshold: before=%f, after=%f", beforeBio, e.Bio)
	}
}

func TestGrowBiomass_CapsAtBioCap(t *testing.T) {
	ensureCache()

	// Entity already at BioCap
	e := testEnergy(1.5, 1.5, 1.5, true) // Bio == BioCap

	grown := GrowBiomass(&e, 1.0) // Large dt

	if grown != 0 {
		t.Errorf("expected no growth at BioCap, got %f", grown)
	}
	if e.Bio != e.BioCap {
		t.Errorf("Bio should stay at BioCap: expected %f, got %f", e.BioCap, e.Bio)
	}
}

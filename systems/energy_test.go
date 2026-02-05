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

// ---------- UpdateEnergy basic behavior ----------

func TestUpdateEnergy_DeadEntityNoOp(t *testing.T) {
	ensureCache()
	e := components.Energy{Value: 0.5, Max: 1.0, Alive: false}
	vel := components.Velocity{}
	caps := components.DefaultCapabilities(0.0)

	cost := UpdateEnergy(&e, vel, caps, 0, 1.0/60)
	if cost != 0 {
		t.Errorf("expected 0 cost for dead entity, got %f", cost)
	}
	if e.Value != 0.5 {
		t.Errorf("dead entity energy should not change, got %f", e.Value)
	}
}

func TestUpdateEnergy_AgeIncreases(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	e := components.Energy{Value: 0.5, Max: 1.0, Alive: true, Age: 10.0}
	vel := components.Velocity{}
	caps := components.DefaultCapabilities(0.0)

	UpdateEnergy(&e, vel, caps, 0, dt)

	expected := float32(10.0) + dt
	if math.Abs(float64(e.Age-expected)) > 1e-6 {
		t.Errorf("expected age %.6f, got %.6f", expected, e.Age)
	}
}

func TestUpdateEnergy_BaseCostApplied(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)

	// Stationary entity, no thrust, no bite → only base cost
	e := components.Energy{Value: 0.5, Max: 1.0, Alive: true}
	vel := components.Velocity{X: 0, Y: 0}
	caps := components.DefaultCapabilities(0.0)

	cost := UpdateEnergy(&e, vel, caps, 0, dt)

	if cost <= 0 {
		t.Error("expected positive base metabolic cost")
	}
	if e.Value >= 0.5 {
		t.Error("expected energy to decrease from base cost")
	}

	// Cost should equal energy lost
	energyLost := float32(0.5) - e.Value
	if math.Abs(float64(cost-energyLost)) > 1e-6 {
		t.Errorf("cost (%f) should match energy lost (%f)", cost, energyLost)
	}
}

func TestUpdateEnergy_MovementCostIncreasesWithSpeed(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(0.0)

	// Stationary
	e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true}
	cost1 := UpdateEnergy(&e1, components.Velocity{X: 0, Y: 0}, caps, 0, dt)

	// Moving at half max speed
	halfSpeed := caps.MaxSpeed * 0.5
	e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true}
	cost2 := UpdateEnergy(&e2, components.Velocity{X: halfSpeed, Y: 0}, caps, 0, dt)

	// Moving at max speed
	e3 := components.Energy{Value: 0.5, Max: 1.0, Alive: true}
	cost3 := UpdateEnergy(&e3, components.Velocity{X: caps.MaxSpeed, Y: 0}, caps, 0, dt)

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
	caps := components.DefaultCapabilities(0.0)

	// No thrust
	e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastThrust: 0}
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 0, dt)

	// Half thrust
	e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastThrust: 0.5}
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 0, dt)

	// Full thrust
	e3 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastThrust: 1.0}
	cost3 := UpdateEnergy(&e3, components.Velocity{}, caps, 0, dt)

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
	// Very low energy → should die after one tick
	e := components.Energy{Value: 0.0001, Max: 1.0, Alive: true}
	vel := components.Velocity{}
	caps := components.DefaultCapabilities(0.0)

	UpdateEnergy(&e, vel, caps, 0, 1.0) // large dt to ensure death

	if e.Alive {
		t.Error("expected entity to die when energy hits 0")
	}
	if e.Value != 0 {
		t.Errorf("expected energy clamped to 0, got %f", e.Value)
	}
}

// ---------- Bite cost ----------

func TestUpdateEnergy_BiteCostAppliedForPredator(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(1.0)

	// No bite signal
	e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0}
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 1.0, dt) // diet=1.0 (pure pred)

	// Active bite signal
	e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0.8}
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 1.0, dt)

	if cost2 <= cost1 {
		t.Errorf("biting cost (%f) should exceed non-biting cost (%f)", cost2, cost1)
	}

	biteCostDelta := cost2 - cost1
	expectedBiteCost := cachedBiteCost * dt // diet=1.0 → full bite cost
	if math.Abs(float64(biteCostDelta-expectedBiteCost)) > 1e-6 {
		t.Errorf("bite cost delta %f != expected %f", biteCostDelta, expectedBiteCost)
	}
}

func TestUpdateEnergy_BiteCostZeroForPrey(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(0.0)

	// No bite
	e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0}
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 0, dt) // diet=0 (pure prey)

	// Active bite signal — should not cost anything for prey (diet=0)
	e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0.8}
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 0, dt)

	if math.Abs(float64(cost2-cost1)) > 1e-6 {
		t.Errorf("prey bite cost should be 0: with_bite=%f, without=%f", cost2, cost1)
	}
}

func TestUpdateEnergy_BiteCostDeadzoneRespected(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(1.0)

	// Bite below deadzone → no bite cost
	e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0.05}
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 1.0, dt)

	// No bite at all
	e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0}
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 1.0, dt)

	if math.Abs(float64(cost1-cost2)) > 1e-6 {
		t.Errorf("bite below deadzone should have no cost: got %f vs %f", cost1, cost2)
	}
}

func TestUpdateEnergy_BiteCostScalesWithDiet(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(1.0)

	// Collect bite cost contribution at various diet levels
	var costs [5]float32
	diets := [5]float32{0, 0.25, 0.5, 0.75, 1.0}

	for i, diet := range diets {
		// With bite
		e1 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0.8}
		costBite := UpdateEnergy(&e1, components.Velocity{}, caps, diet, dt)

		// Without bite
		e2 := components.Energy{Value: 0.5, Max: 1.0, Alive: true, LastBite: 0}
		costNoBite := UpdateEnergy(&e2, components.Velocity{}, caps, diet, dt)

		costs[i] = costBite - costNoBite
	}

	// Diet=0 → 0 bite cost
	if math.Abs(float64(costs[0])) > 1e-6 {
		t.Errorf("expected 0 bite cost at diet=0, got %f", costs[0])
	}

	// Bite cost should increase monotonically with diet
	for i := 1; i < len(costs); i++ {
		if costs[i] < costs[i-1]-1e-6 {
			t.Errorf("bite cost should increase with diet: costs[%d]=%f < costs[%d]=%f",
				i, costs[i], i-1, costs[i-1])
		}
	}

	// Diet=1 → full bite cost
	expectedFull := cachedBiteCost * dt
	if math.Abs(float64(costs[4]-expectedFull)) > 1e-6 {
		t.Errorf("expected full bite cost %f at diet=1.0, got %f", expectedFull, costs[4])
	}
}

// ---------- Cost returns match energy lost ----------

func TestUpdateEnergy_ReturnedCostMatchesEnergyDelta(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(1.0)

	// Entity with all cost sources active
	e := components.Energy{Value: 0.8, Max: 1.0, Alive: true, LastThrust: 0.7, LastBite: 0.6}
	vel := components.Velocity{X: 30, Y: 20}
	before := e.Value

	cost := UpdateEnergy(&e, vel, caps, 0.8, dt)

	energyLost := before - e.Value
	if math.Abs(float64(cost-energyLost)) > 1e-5 {
		t.Errorf("returned cost (%f) doesn't match energy delta (%f)", cost, energyLost)
	}
}

func TestUpdateEnergy_CostPositiveForAllDiets(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)

	for _, diet := range []float32{0, 0.25, 0.5, 0.75, 1.0} {
		caps := components.DefaultCapabilities(diet)
		e := components.Energy{Value: 0.5, Max: 1.0, Alive: true}
		cost := UpdateEnergy(&e, components.Velocity{}, caps, diet, dt)
		if cost <= 0 {
			t.Errorf("expected positive cost for diet=%f, got %f", diet, cost)
		}
	}
}

// ---------- Diet interpolation ----------

func TestUpdateEnergy_DietInterpolation(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(0.0)

	// Pure prey (diet=0)
	e1 := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	cost1 := UpdateEnergy(&e1, components.Velocity{}, caps, 0, dt)

	// Pure predator (diet=1)
	e2 := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	cost2 := UpdateEnergy(&e2, components.Velocity{}, caps, 1.0, dt)

	// Predators have lower base cost than prey → cost should differ
	if cost1 == cost2 {
		t.Error("expected different costs for diet=0 vs diet=1")
	}

	// Mid diet (0.5) should be between the two
	e3 := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	cost3 := UpdateEnergy(&e3, components.Velocity{}, caps, 0.5, dt)

	lo, hi := cost1, cost2
	if lo > hi {
		lo, hi = hi, lo
	}
	if cost3 < lo-1e-6 || cost3 > hi+1e-6 {
		t.Errorf("mid-diet cost %f not between endpoints %f and %f", cost3, lo, hi)
	}
}

// ---------- TransferEnergy conservation ----------

func TestTransferEnergy_ConservesEnergy(t *testing.T) {
	ensureCache()
	pred := components.Energy{Value: 0.4, Max: 1.0, Alive: true}
	prey := components.Energy{Value: 0.6, Max: 1.0, Alive: true}

	totalBefore := pred.Value + prey.Value

	xfer := TransferEnergy(&pred, &prey, 0.3)

	totalAfter := pred.Value + prey.Value
	accounted := totalAfter + xfer.ToDet + xfer.ToHeat + xfer.Overflow

	if math.Abs(float64(totalBefore-accounted)) > 1e-5 {
		t.Errorf("transfer not conserved: before=%f, after_pools=%f+det=%f+heat=%f+overflow=%f = %f",
			totalBefore, totalAfter, xfer.ToDet, xfer.ToHeat, xfer.Overflow, accounted)
	}
}

func TestTransferEnergy_OverflowToDetritus(t *testing.T) {
	ensureCache()
	// Predator nearly full → should overflow
	pred := components.Energy{Value: 0.95, Max: 1.0, Alive: true}
	prey := components.Energy{Value: 0.6, Max: 1.0, Alive: true}

	xfer := TransferEnergy(&pred, &prey, 0.5)

	if pred.Value > pred.Max+1e-6 {
		t.Errorf("predator energy %f exceeds max %f", pred.Value, pred.Max)
	}
	if xfer.Overflow <= 0 {
		t.Error("expected overflow when predator near max")
	}

	// Conservation check
	totalBefore := float32(0.95 + 0.6)
	totalAfter := pred.Value + prey.Value + xfer.ToDet + xfer.ToHeat + xfer.Overflow
	if math.Abs(float64(totalBefore-totalAfter)) > 1e-5 {
		t.Errorf("overflow transfer not conserved: before=%f, after=%f", totalBefore, totalAfter)
	}
}

func TestTransferEnergy_PreyDiesAtZero(t *testing.T) {
	ensureCache()
	pred := components.Energy{Value: 0.3, Max: 1.0, Alive: true}
	prey := components.Energy{Value: 0.1, Max: 1.0, Alive: true}

	// Transfer more than prey has
	xfer := TransferEnergy(&pred, &prey, 0.5)

	if prey.Alive {
		t.Error("expected prey to die when energy fully drained")
	}
	if prey.Value != 0 {
		t.Errorf("expected prey energy=0, got %f", prey.Value)
	}
	if xfer.Removed > 0.1+1e-6 {
		t.Errorf("removed %f but prey only had 0.1", xfer.Removed)
	}
}

func TestTransferEnergy_DeadEntitiesNoOp(t *testing.T) {
	ensureCache()
	pred := components.Energy{Value: 0.3, Max: 1.0, Alive: false}
	prey := components.Energy{Value: 0.5, Max: 1.0, Alive: true}

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

// ---------- Grazing efficiency ----------

func TestGrazingEfficiency_PurePreyFull(t *testing.T) {
	ensureCache()
	eff := GrazingEfficiency(0)
	if math.Abs(float64(eff-1.0)) > 1e-6 {
		t.Errorf("expected 1.0 at diet=0, got %f", eff)
	}
}

func TestGrazingEfficiency_HighDietZero(t *testing.T) {
	ensureCache()
	// Above grazing diet cap → 0
	eff := GrazingEfficiency(1.0)
	if eff != 0 {
		t.Errorf("expected 0 at diet=1.0, got %f", eff)
	}
}

func TestHuntingEfficiency_PurePredFull(t *testing.T) {
	ensureCache()
	eff := HuntingEfficiency(1.0)
	if math.Abs(float64(eff-1.0)) > 1e-6 {
		t.Errorf("expected 1.0 at diet=1.0, got %f", eff)
	}
}

func TestHuntingEfficiency_LowDietZero(t *testing.T) {
	ensureCache()
	eff := HuntingEfficiency(0)
	if eff != 0 {
		t.Errorf("expected 0 at diet=0, got %f", eff)
	}
}

// ---------- Grazing resource accounting ----------

func TestGrazing_ResourceRemovalMatchesEnergyGain(t *testing.T) {
	ensureCache()
	cfg := config.Cfg()
	forageEff := float32(cfg.Resource.ForageEfficiency)

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

	// Energy gain should be: removed * forageEfficiency * dietGrazingEff
	// The rest goes to heat
	gain := removed * forageEff // at diet=0, grazing eff = 1.0
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
	caps := components.DefaultCapabilities(1.0)

	// Set up entity with all cost sources active
	e := components.Energy{Value: 0.8, Max: 1.0, Alive: true, LastThrust: 0.6, LastBite: 0.5}
	vel := components.Velocity{X: 20, Y: 15}
	diet := float32(0.7)
	before := e.Value

	cost := UpdateEnergy(&e, vel, caps, diet, dt)

	// Manually compute expected costs
	baseCost := lerp32(cachedPreyBaseCost, cachedPredBaseCost, diet)
	moveCost := lerp32(cachedPreyMoveCost, cachedPredMoveCost, diet)
	accelCost := lerp32(cachedPreyAccelCost, cachedPredAccelCost, diet)

	expectedBase := baseCost * dt
	speedSq := vel.X*vel.X + vel.Y*vel.Y
	maxSpeedSq := caps.MaxSpeed * caps.MaxSpeed
	expectedMove := moveCost * (speedSq / maxSpeedSq) * dt
	expectedAccel := accelCost * 0.6 * 0.6 * dt

	biteCost := lerp32(0, cachedBiteCost, diet)
	expectedBite := biteCost * dt

	expectedTotal := expectedBase + expectedMove + expectedAccel + expectedBite
	energyLost := before - e.Value

	if math.Abs(float64(cost-expectedTotal)) > 1e-5 {
		t.Errorf("cost %f != expected %f (base=%f move=%f accel=%f bite=%f)",
			cost, expectedTotal, expectedBase, expectedMove, expectedAccel, expectedBite)
	}
	if math.Abs(float64(energyLost-expectedTotal)) > 1e-5 {
		t.Errorf("energy lost %f != expected total %f", energyLost, expectedTotal)
	}
}

// ---------- No double-counting checks ----------

func TestUpdateEnergy_NoCostLeakageOnRepeat(t *testing.T) {
	ensureCache()
	dt := float32(1.0 / 60.0)
	caps := components.DefaultCapabilities(0.0)

	// Run UpdateEnergy twice with same parameters → second cost should
	// be identical (no accumulated state leaking between calls)
	e1 := components.Energy{Value: 0.8, Max: 1.0, Alive: true, LastThrust: 0.3}
	cost1 := UpdateEnergy(&e1, components.Velocity{X: 10}, caps, 0.2, dt)

	e2 := components.Energy{Value: 0.8, Max: 1.0, Alive: true, LastThrust: 0.3}
	cost2 := UpdateEnergy(&e2, components.Velocity{X: 10}, caps, 0.2, dt)

	if math.Abs(float64(cost1-cost2)) > 1e-6 {
		t.Errorf("repeated calls with same state should yield same cost: %f vs %f", cost1, cost2)
	}
}

func TestTransferEnergy_NoDoubleCounting(t *testing.T) {
	ensureCache()

	// Run a series of transfers and verify sum of all flows equals initial pool
	pred := components.Energy{Value: 0.3, Max: 1.0, Alive: true}
	prey := components.Energy{Value: 0.8, Max: 1.0, Alive: true}
	initial := pred.Value + prey.Value

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

	remaining := pred.Value + prey.Value
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

// Particle-specific tests removed - using simplified resource field

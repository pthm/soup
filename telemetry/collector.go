package telemetry

import "github.com/pthm-cable/soup/components"

// Collector accumulates events within time windows and produces WindowStats.
type Collector struct {
	windowDurationSec float64
	windowDurationTicks int32
	dt                float32

	// Current window tracking
	windowStartTick int32

	// Event counters for current window
	preyBirths          int
	predBirths          int
	preyDeaths          int
	predDeaths          int
	bitesAttempted      int
	bitesHit            int
	kills               int
	bitesBlockedDigest  int
	bitesMissedRefugia  int
}

// NewCollector creates a new stats collector.
// windowDurationSec: how long each stats window lasts in simulation seconds
// dt: seconds per tick (used for tick-to-time conversion)
func NewCollector(windowDurationSec float64, dt float32) *Collector {
	ticksPerWindow := int32(windowDurationSec / float64(dt))
	if ticksPerWindow < 1 {
		ticksPerWindow = 1
	}

	return &Collector{
		windowDurationSec:   windowDurationSec,
		windowDurationTicks: ticksPerWindow,
		dt:                  dt,
		windowStartTick:     0,
	}
}

// RecordBiteAttempt records a bite attempt.
func (c *Collector) RecordBiteAttempt() {
	c.bitesAttempted++
}

// RecordBiteHit records a successful bite.
func (c *Collector) RecordBiteHit() {
	c.bitesHit++
}

// RecordKill records a kill.
func (c *Collector) RecordKill() {
	c.kills++
}

// RecordBiteBlockedByDigest records a bite blocked by digestion cooldown.
func (c *Collector) RecordBiteBlockedByDigest() {
	c.bitesBlockedDigest++
}

// RecordBiteMissedRefugia records a bite that missed due to refugia protection.
func (c *Collector) RecordBiteMissedRefugia() {
	c.bitesMissedRefugia++
}

// RecordBirth records a birth event.
func (c *Collector) RecordBirth(kind components.Kind) {
	if kind == components.KindPrey {
		c.preyBirths++
	} else {
		c.predBirths++
	}
}

// RecordDeath records a death event.
func (c *Collector) RecordDeath(kind components.Kind) {
	if kind == components.KindPrey {
		c.preyDeaths++
	} else {
		c.predDeaths++
	}
}

// ShouldFlush returns true if enough ticks have passed to flush the window.
func (c *Collector) ShouldFlush(currentTick int32) bool {
	return currentTick-c.windowStartTick >= c.windowDurationTicks
}

// EnergyPools holds energy pool totals for conservation tracking.
type EnergyPools struct {
	TotalRes       float64 // Total resource grid energy
	TotalDet       float64 // Total detritus grid energy
	TotalOrganisms float64 // Total energy in living organisms
	HeatLossAccum  float64 // Cumulative energy lost to heat
	ParticleInput  float64 // Cumulative energy injected by particles
}

// Flush produces a WindowStats and resets counters for the next window.
// The caller must provide:
// - currentTick: the current simulation tick
// - preyCount, predCount: current population counts
// - preyEnergies, predEnergies: energy values for percentile calculation
// - meanResourceAtPrey: average resource value at prey positions
// - activeClades: number of unique clades among living entities
// - pools: energy pool totals for conservation tracking
func (c *Collector) Flush(
	currentTick int32,
	preyCount, predCount int,
	preyEnergies, predEnergies []float64,
	meanResourceAtPrey float64,
	activeClades int,
	pools EnergyPools,
) WindowStats {
	// Calculate rates
	var hitRate, killRate float64
	if c.bitesAttempted > 0 {
		hitRate = float64(c.bitesHit) / float64(c.bitesAttempted)
	}
	if c.bitesHit > 0 {
		killRate = float64(c.kills) / float64(c.bitesHit)
	}

	// Compute energy stats
	preyMean, preyP10, preyP50, preyP90 := ComputeEnergyStats(preyEnergies)
	predMean, predP10, predP50, predP90 := ComputeEnergyStats(predEnergies)

	stats := WindowStats{
		WindowStartTick: c.windowStartTick,
		WindowEndTick:   currentTick,
		SimTimeSec:      float64(currentTick) * float64(c.dt),

		PreyCount: preyCount,
		PredCount: predCount,

		PreyBirths: c.preyBirths,
		PredBirths: c.predBirths,
		PreyDeaths: c.preyDeaths,
		PredDeaths: c.predDeaths,

		BitesAttempted:     c.bitesAttempted,
		BitesHit:           c.bitesHit,
		Kills:              c.kills,
		BitesBlockedDigest: c.bitesBlockedDigest,
		BitesMissedRefugia: c.bitesMissedRefugia,
		HitRate:            hitRate,
		KillRate:           killRate,

		PreyEnergyMean: preyMean,
		PreyEnergyP10:  preyP10,
		PreyEnergyP50:  preyP50,
		PreyEnergyP90:  preyP90,

		PredEnergyMean: predMean,
		PredEnergyP10:  predP10,
		PredEnergyP50:  predP50,
		PredEnergyP90:  predP90,

		MeanResourceAtPreyPos: meanResourceAtPrey,

		TotalRes:       pools.TotalRes,
		TotalDet:       pools.TotalDet,
		TotalOrganisms: pools.TotalOrganisms,
		HeatLossAccum:  pools.HeatLossAccum,
		ParticleInput:  pools.ParticleInput,

		ActiveClades: activeClades,
	}

	// Reset for next window
	c.windowStartTick = currentTick
	c.preyBirths = 0
	c.predBirths = 0
	c.preyDeaths = 0
	c.predDeaths = 0
	c.bitesAttempted = 0
	c.bitesHit = 0
	c.kills = 0
	c.bitesBlockedDigest = 0
	c.bitesMissedRefugia = 0

	return stats
}

// WindowDurationTicks returns the number of ticks per window.
func (c *Collector) WindowDurationTicks() int32 {
	return c.windowDurationTicks
}

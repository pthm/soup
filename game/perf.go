package game

import (
	"sort"
	"time"
)

// PerfStats tracks execution time for each system.
type PerfStats struct {
	samples    map[string][]time.Duration
	maxSamples int
}

// NewPerfStats creates a new performance stats tracker.
func NewPerfStats() *PerfStats {
	return &PerfStats{
		samples:    make(map[string][]time.Duration),
		maxSamples: 120, // ~2 seconds of samples at 60fps
	}
}

// Record adds a duration sample for the named system.
func (p *PerfStats) Record(name string, d time.Duration) {
	p.samples[name] = append(p.samples[name], d)
	if len(p.samples[name]) > p.maxSamples {
		p.samples[name] = p.samples[name][1:]
	}
}

// Avg returns the average duration for the named system.
func (p *PerfStats) Avg(name string) time.Duration {
	s := p.samples[name]
	if len(s) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range s {
		total += d
	}
	return total / time.Duration(len(s))
}

// Total returns the sum of all average durations.
func (p *PerfStats) Total() time.Duration {
	var total time.Duration
	for name := range p.samples {
		total += p.Avg(name)
	}
	return total
}

// SortedNames returns system names sorted by average duration (descending).
func (p *PerfStats) SortedNames() []string {
	names := make([]string, 0, len(p.samples))
	for name := range p.samples {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return p.Avg(names[i]) > p.Avg(names[j])
	})
	return names
}

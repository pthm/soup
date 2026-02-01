package telemetry

import (
	"fmt"
	"log/slog"
)

// BookmarkType identifies the type of bookmark.
type BookmarkType string

const (
	BookmarkHuntBreakthrough    BookmarkType = "hunt_breakthrough"
	BookmarkForageBreakthrough  BookmarkType = "forage_breakthrough"
	BookmarkPredatorRecovery    BookmarkType = "predator_recovery"
	BookmarkPreyCrash           BookmarkType = "prey_crash"
	BookmarkStableEcosystem     BookmarkType = "stable_ecosystem"
)

// Bookmark represents an automatically triggered bookmark.
type Bookmark struct {
	Type        BookmarkType
	Tick        int32
	Description string
}

// LogBookmark logs the bookmark using slog.
func (b Bookmark) LogBookmark() {
	slog.Info("bookmark",
		"type", string(b.Type),
		"tick", b.Tick,
		"description", b.Description,
	)
}

// BookmarkDetector detects interesting moments in the simulation.
type BookmarkDetector struct {
	// Rolling history (circular buffer)
	history     []WindowStats
	historySize int
	historyIdx  int
	historyFull bool

	// State tracking
	recentPredMin      int     // minimum predator count in recent history
	recentPreyPeak     int     // peak prey count in recent history
	stableWindowsCount int     // consecutive windows with stable populations
}

// NewBookmarkDetector creates a detector with the given history size.
func NewBookmarkDetector(historySize int) *BookmarkDetector {
	if historySize < 5 {
		historySize = 5 // minimum for stable ecosystem detection
	}
	return &BookmarkDetector{
		history:     make([]WindowStats, historySize),
		historySize: historySize,
	}
}

// Check analyzes the latest stats and returns any triggered bookmarks.
func (bd *BookmarkDetector) Check(stats WindowStats) []Bookmark {
	var bookmarks []Bookmark

	if bd.historyFull || bd.historyIdx > 0 {
		// Hunt breakthrough: kill rate > 2x rolling average
		if b := bd.checkHuntBreakthrough(stats); b != nil {
			bookmarks = append(bookmarks, *b)
		}

		// Forage breakthrough: mean resource at prey > 2x rolling average
		if b := bd.checkForageBreakthrough(stats); b != nil {
			bookmarks = append(bookmarks, *b)
		}

		// Predator recovery: was ≤3, now ≥3x that
		if b := bd.checkPredatorRecovery(stats); b != nil {
			bookmarks = append(bookmarks, *b)
		}

		// Prey crash: dropped >30% from recent peak
		if b := bd.checkPreyCrash(stats); b != nil {
			bookmarks = append(bookmarks, *b)
		}

		// Stable ecosystem: both populations present with low variance over 5+ windows
		if b := bd.checkStableEcosystem(stats); b != nil {
			bookmarks = append(bookmarks, *b)
		}
	}

	// Update history
	bd.addToHistory(stats)

	// Track predator minimum and prey peak
	if stats.PredCount < bd.recentPredMin || bd.recentPredMin == 0 {
		bd.recentPredMin = stats.PredCount
	}
	if stats.PreyCount > bd.recentPreyPeak {
		bd.recentPreyPeak = stats.PreyCount
	}

	return bookmarks
}

func (bd *BookmarkDetector) addToHistory(stats WindowStats) {
	bd.history[bd.historyIdx] = stats
	bd.historyIdx = (bd.historyIdx + 1) % bd.historySize
	if bd.historyIdx == 0 {
		bd.historyFull = true
	}
}

func (bd *BookmarkDetector) getHistory() []WindowStats {
	if bd.historyFull {
		return bd.history
	}
	return bd.history[:bd.historyIdx]
}

func (bd *BookmarkDetector) checkHuntBreakthrough(stats WindowStats) *Bookmark {
	history := bd.getHistory()
	if len(history) < 3 {
		return nil
	}

	// Calculate rolling average kill rate
	var totalKills, totalHits int
	for _, h := range history {
		totalKills += h.Kills
		totalHits += h.BitesHit
	}

	if totalHits == 0 || stats.BitesHit == 0 {
		return nil
	}

	avgKillRate := float64(totalKills) / float64(totalHits)
	if avgKillRate == 0 {
		return nil
	}

	currentKillRate := stats.KillRate
	if currentKillRate > avgKillRate*2.0 && stats.Kills >= 3 {
		return &Bookmark{
			Type:        BookmarkHuntBreakthrough,
			Tick:        stats.WindowEndTick,
			Description: fmt.Sprintf("Kill rate %.2f is %.1fx average (%.2f)", currentKillRate, currentKillRate/avgKillRate, avgKillRate),
		}
	}

	return nil
}

func (bd *BookmarkDetector) checkForageBreakthrough(stats WindowStats) *Bookmark {
	history := bd.getHistory()
	if len(history) < 3 {
		return nil
	}

	// Calculate rolling average resource utilization
	var totalResource float64
	for _, h := range history {
		totalResource += h.MeanResourceAtPreyPos
	}
	avgResource := totalResource / float64(len(history))

	if avgResource == 0 {
		return nil
	}

	if stats.MeanResourceAtPreyPos > avgResource*2.0 && stats.MeanResourceAtPreyPos > 0.3 {
		return &Bookmark{
			Type:        BookmarkForageBreakthrough,
			Tick:        stats.WindowEndTick,
			Description: fmt.Sprintf("Resource util %.2f is %.1fx average (%.2f)", stats.MeanResourceAtPreyPos, stats.MeanResourceAtPreyPos/avgResource, avgResource),
		}
	}

	return nil
}

func (bd *BookmarkDetector) checkPredatorRecovery(stats WindowStats) *Bookmark {
	if bd.recentPredMin == 0 || bd.recentPredMin > 3 {
		return nil
	}

	threshold := bd.recentPredMin * 3
	if stats.PredCount >= threshold && stats.PredCount >= 6 {
		// Reset the minimum after triggering
		oldMin := bd.recentPredMin
		bd.recentPredMin = stats.PredCount

		return &Bookmark{
			Type:        BookmarkPredatorRecovery,
			Tick:        stats.WindowEndTick,
			Description: fmt.Sprintf("Predator population recovered from %d to %d", oldMin, stats.PredCount),
		}
	}

	return nil
}

func (bd *BookmarkDetector) checkPreyCrash(stats WindowStats) *Bookmark {
	if bd.recentPreyPeak == 0 {
		return nil
	}

	dropPercent := 1.0 - float64(stats.PreyCount)/float64(bd.recentPreyPeak)
	if dropPercent > 0.30 && stats.PreyCount < bd.recentPreyPeak-10 {
		// Reset peak after crash
		oldPeak := bd.recentPreyPeak
		bd.recentPreyPeak = stats.PreyCount

		return &Bookmark{
			Type:        BookmarkPreyCrash,
			Tick:        stats.WindowEndTick,
			Description: fmt.Sprintf("Prey crashed %.0f%% from peak %d to %d", dropPercent*100, oldPeak, stats.PreyCount),
		}
	}

	return nil
}

func (bd *BookmarkDetector) checkStableEcosystem(stats WindowStats) *Bookmark {
	// Need both populations present
	if stats.PreyCount < 10 || stats.PredCount < 3 {
		bd.stableWindowsCount = 0
		return nil
	}

	history := bd.getHistory()
	if len(history) < 4 {
		return nil
	}

	// Check variance in recent windows
	var preySum, predSum float64
	for _, h := range history[len(history)-4:] {
		preySum += float64(h.PreyCount)
		predSum += float64(h.PredCount)
	}
	preyMean := preySum / 4
	predMean := predSum / 4

	var preyVar, predVar float64
	for _, h := range history[len(history)-4:] {
		preyDiff := float64(h.PreyCount) - preyMean
		predDiff := float64(h.PredCount) - predMean
		preyVar += preyDiff * preyDiff
		predVar += predDiff * predDiff
	}
	preyVar /= 4
	predVar /= 4

	// Low variance: coefficient of variation < 20%
	preyCV := 0.0
	if preyMean > 0 {
		preyCV = (preyVar / (preyMean * preyMean))
	}
	predCV := 0.0
	if predMean > 0 {
		predCV = (predVar / (predMean * predMean))
	}

	if preyCV < 0.04 && predCV < 0.04 { // CV^2 < 0.04 means CV < 0.2
		bd.stableWindowsCount++
	} else {
		bd.stableWindowsCount = 0
	}

	if bd.stableWindowsCount == 5 { // trigger exactly once at 5 windows
		return &Bookmark{
			Type:        BookmarkStableEcosystem,
			Tick:        stats.WindowEndTick,
			Description: fmt.Sprintf("Stable ecosystem with %d prey, %d predators over 5+ windows", stats.PreyCount, stats.PredCount),
		}
	}

	return nil
}

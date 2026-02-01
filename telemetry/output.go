package telemetry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gocarina/gocsv"
	"github.com/pthm-cable/soup/config"
)

// OutputManager handles structured experiment output with CSV logging.
type OutputManager struct {
	dir           string
	telemetryFile *os.File
	perfFile      *os.File
	bookmarkFile  *os.File

	// Track if headers have been written
	telemetryHeaderWritten bool
	perfHeaderWritten      bool
	bookmarkHeaderWritten  bool
}

// NewOutputManager creates a new output manager and initializes the output directory.
// Returns nil if dir is empty (output disabled).
func NewOutputManager(dir string) (*OutputManager, error) {
	if dir == "" {
		return nil, nil
	}

	// Create output directory
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	om := &OutputManager{dir: dir}

	// Open telemetry.csv
	telemetryPath := filepath.Join(dir, "telemetry.csv")
	f, err := os.Create(telemetryPath)
	if err != nil {
		return nil, fmt.Errorf("creating telemetry.csv: %w", err)
	}
	om.telemetryFile = f

	// Open perf.csv
	perfPath := filepath.Join(dir, "perf.csv")
	f, err = os.Create(perfPath)
	if err != nil {
		om.telemetryFile.Close()
		return nil, fmt.Errorf("creating perf.csv: %w", err)
	}
	om.perfFile = f

	// Open bookmarks.csv
	bookmarkPath := filepath.Join(dir, "bookmarks.csv")
	f, err = os.Create(bookmarkPath)
	if err != nil {
		om.telemetryFile.Close()
		om.perfFile.Close()
		return nil, fmt.Errorf("creating bookmarks.csv: %w", err)
	}
	om.bookmarkFile = f

	return om, nil
}

// WriteConfig saves the current configuration as YAML.
func (om *OutputManager) WriteConfig(cfg *config.Config) error {
	if om == nil {
		return nil
	}
	configPath := filepath.Join(om.dir, "config.yaml")
	return cfg.WriteYAML(configPath)
}

// WriteTelemetry writes a window stats record to telemetry.csv.
func (om *OutputManager) WriteTelemetry(stats WindowStats) error {
	if om == nil {
		return nil
	}

	records := []WindowStats{stats}

	if !om.telemetryHeaderWritten {
		// First write includes headers
		if err := gocsv.Marshal(records, om.telemetryFile); err != nil {
			return fmt.Errorf("writing telemetry: %w", err)
		}
		om.telemetryHeaderWritten = true
	} else {
		// Subsequent writes skip headers
		if err := gocsv.MarshalWithoutHeaders(records, om.telemetryFile); err != nil {
			return fmt.Errorf("writing telemetry: %w", err)
		}
	}

	return nil
}

// WritePerf writes a performance stats record to perf.csv.
func (om *OutputManager) WritePerf(stats PerfStats, windowEnd int32) error {
	if om == nil {
		return nil
	}

	csvRecord := stats.ToCSV(windowEnd)
	records := []PerfStatsCSV{csvRecord}

	if !om.perfHeaderWritten {
		if err := gocsv.Marshal(records, om.perfFile); err != nil {
			return fmt.Errorf("writing perf: %w", err)
		}
		om.perfHeaderWritten = true
	} else {
		if err := gocsv.MarshalWithoutHeaders(records, om.perfFile); err != nil {
			return fmt.Errorf("writing perf: %w", err)
		}
	}

	return nil
}

// WriteBookmark writes a bookmark record to bookmarks.csv.
func (om *OutputManager) WriteBookmark(b Bookmark) error {
	if om == nil {
		return nil
	}

	records := []Bookmark{b}

	if !om.bookmarkHeaderWritten {
		if err := gocsv.Marshal(records, om.bookmarkFile); err != nil {
			return fmt.Errorf("writing bookmark: %w", err)
		}
		om.bookmarkHeaderWritten = true
	} else {
		if err := gocsv.MarshalWithoutHeaders(records, om.bookmarkFile); err != nil {
			return fmt.Errorf("writing bookmark: %w", err)
		}
	}

	return nil
}

// WriteHallOfFame saves the hall of fame as JSON.
func (om *OutputManager) WriteHallOfFame(hof *HallOfFame) error {
	if om == nil || hof == nil {
		return nil
	}

	hofPath := filepath.Join(om.dir, "hall_of_fame.json")
	data, err := hof.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshaling hall of fame: %w", err)
	}

	if err := os.WriteFile(hofPath, data, 0644); err != nil {
		return fmt.Errorf("writing hall_of_fame.json: %w", err)
	}

	return nil
}

// Dir returns the output directory path.
func (om *OutputManager) Dir() string {
	if om == nil {
		return ""
	}
	return om.dir
}

// Close flushes and closes all output files.
func (om *OutputManager) Close() error {
	if om == nil {
		return nil
	}

	var firstErr error

	if om.telemetryFile != nil {
		if err := om.telemetryFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if om.perfFile != nil {
		if err := om.perfFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if om.bookmarkFile != nil {
		if err := om.bookmarkFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

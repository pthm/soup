// Package ui provides a descriptor-driven UI system for the simulation.
// Instead of hard-coding field names and layouts, UI elements are defined
// through metadata that can be updated alongside the underlying systems.
package ui

import rl "github.com/gen2brain/raylib-go/raylib"

// WidgetType specifies how a field should be rendered.
type WidgetType int

const (
	WidgetText WidgetType = iota // Plain text with format string
	WidgetBar                    // Progress bar [0, 1]
	WidgetCenteredBar            // Centered bar [-1, +1] or custom range
	WidgetColorSwatch            // Color preview square
	WidgetEnergyBar              // Energy bar with color thresholds
	WidgetSection                // Section header
	WidgetSpacer                 // Vertical spacing
)

// FieldRange defines the value range for bar widgets.
type FieldRange struct {
	Min float32
	Max float32
}

// DefaultRange returns a [0, 1] range.
func DefaultRange() FieldRange {
	return FieldRange{Min: 0, Max: 1}
}

// CenteredRange returns a [-1, +1] range.
func CenteredRange() FieldRange {
	return FieldRange{Min: -1, Max: 1}
}

// FieldDescriptor defines how to display a single piece of data.
type FieldDescriptor struct {
	ID      string            // Unique identifier for the field
	Label   string            // Display label
	Widget  WidgetType        // How to render
	Format  string            // Printf format for text (e.g., "%.2f")
	Range   FieldRange        // Value range for bars
	Color   rl.Color          // Optional color override
	Visible func(any) bool    // Optional visibility check (nil = always visible)
	Getter  func(any) float32 // Value extractor (for numeric fields)
	TextGetter func(any) string // Value extractor (for text fields)
	ColorGetter func(any) rl.Color // Color extractor (for color swatches)
}

// SectionDescriptor defines a group of fields with a header.
type SectionDescriptor struct {
	ID      string            // Unique identifier
	Title   string            // Section header text
	Fields  []FieldDescriptor // Fields in this section
	Visible func(any) bool    // Optional visibility check for entire section
}

// PanelDescriptor defines a complete panel layout.
type PanelDescriptor struct {
	ID       string              // Unique identifier
	Title    string              // Panel title (optional)
	Sections []SectionDescriptor // Sections in order
	Width    int32               // Panel width (0 = auto)
	Anchor   PanelAnchor         // Where to position
}

// PanelAnchor specifies where a panel is anchored on screen.
type PanelAnchor int

const (
	AnchorTopLeft PanelAnchor = iota
	AnchorTopRight
	AnchorBottomLeft
	AnchorBottomRight
	AnchorCenter
)

// Theme holds UI styling constants.
type Theme struct {
	PanelBg        rl.Color
	PanelBorder    rl.Color
	SectionHeader  rl.Color
	LabelColor     rl.Color
	ValueColor     rl.Color
	BarBg          rl.Color
	BarFill        rl.Color
	BarFillLow     rl.Color
	BarFillMedium  rl.Color
	BarFillHigh    rl.Color
	BarFillNegative rl.Color
	BarFillPositive rl.Color
	Padding        int32
	LineHeight     int32
	LabelWidth     int32
	BarHeight      int32
	FontSize       int32
	HeaderFontSize int32
}

// DefaultTheme returns the default UI theme.
func DefaultTheme() Theme {
	return Theme{
		PanelBg:        rl.Color{R: 20, G: 25, B: 30, A: 240},
		PanelBorder:    rl.Color{R: 60, G: 70, B: 80, A: 255},
		SectionHeader:  rl.Yellow,
		LabelColor:     rl.LightGray,
		ValueColor:     rl.LightGray,
		BarBg:          rl.Color{R: 40, G: 40, B: 40, A: 255},
		BarFill:        rl.Color{R: 100, G: 150, B: 200, A: 255},
		BarFillLow:     rl.Color{R: 200, G: 100, B: 100, A: 255},
		BarFillMedium:  rl.Color{R: 200, G: 180, B: 100, A: 255},
		BarFillHigh:    rl.Color{R: 100, G: 200, B: 100, A: 255},
		BarFillNegative: rl.Color{R: 200, G: 100, B: 100, A: 255},
		BarFillPositive: rl.Color{R: 100, G: 200, B: 100, A: 255},
		Padding:        10,
		LineHeight:     16,
		LabelWidth:     60,
		BarHeight:      12,
		FontSize:       12,
		HeaderFontSize: 14,
	}
}

// InspectableData is an interface for types that provide inspection data.
// Types can implement this to provide custom field values.
type InspectableData interface {
	// GetFieldValue returns the value for a field by ID.
	// Returns (value, exists).
	GetFieldValue(fieldID string) (any, bool)
}

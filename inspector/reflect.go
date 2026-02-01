package inspector

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// Widget types for rendering fields.
type Widget int

const (
	WidgetAuto Widget = iota
	WidgetLabel
	WidgetBar
	WidgetAngle
	WidgetBool
	WidgetSkip
)

// Field represents a component field with rendering hints.
type Field struct {
	Name    string
	Value   interface{}
	Widget  Widget
	Options map[string]string
}

// ParseTag parses an inspect struct tag.
// Format: `inspect:"widget[,option:value...]"`
// Examples:
//
//	`inspect:"bar"`
//	`inspect:"bar,max:200"`
//	`inspect:"angle"`
//	`inspect:"label,fmt:%.1f"`
//	`inspect:"skip"`
func ParseTag(tag string) (Widget, map[string]string) {
	options := make(map[string]string)

	if tag == "" {
		return WidgetAuto, options
	}

	parts := strings.Split(tag, ",")
	widgetStr := strings.TrimSpace(parts[0])

	var widget Widget
	switch widgetStr {
	case "label":
		widget = WidgetLabel
	case "bar":
		widget = WidgetBar
	case "angle":
		widget = WidgetAngle
	case "bool":
		widget = WidgetBool
	case "skip":
		widget = WidgetSkip
	default:
		widget = WidgetAuto
	}

	// Parse options
	for _, part := range parts[1:] {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) == 2 {
			options[kv[0]] = kv[1]
		}
	}

	return widget, options
}

// ExtractFields uses reflection to extract all fields from a component.
func ExtractFields(component interface{}) []Field {
	v := reflect.ValueOf(component)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	var fields []Field

	for i := 0; i < v.NumField(); i++ {
		sf := t.Field(i)
		fv := v.Field(i)

		// Skip unexported fields
		if !sf.IsExported() {
			continue
		}

		tag := sf.Tag.Get("inspect")
		widget, options := ParseTag(tag)

		if widget == WidgetSkip {
			continue
		}

		// Auto-detect widget if not specified
		if widget == WidgetAuto {
			widget = autoDetectWidget(fv)
		}

		fields = append(fields, Field{
			Name:    sf.Name,
			Value:   fv.Interface(),
			Widget:  widget,
			Options: options,
		})
	}

	return fields
}

// autoDetectWidget chooses a widget based on the field type.
func autoDetectWidget(v reflect.Value) Widget {
	switch v.Kind() {
	case reflect.Bool:
		return WidgetBool
	case reflect.Float32, reflect.Float64:
		return WidgetLabel
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return WidgetLabel
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return WidgetLabel
	case reflect.Array, reflect.Slice:
		// Arrays of floats become bar groups
		return WidgetBar
	default:
		return WidgetLabel
	}
}

// FormatValue formats a field value as a string.
func FormatValue(value interface{}, fmtStr string) string {
	if fmtStr == "" {
		switch v := value.(type) {
		case float32:
			return fmt.Sprintf("%.2f", v)
		case float64:
			return fmt.Sprintf("%.2f", v)
		default:
			return fmt.Sprintf("%v", value)
		}
	}
	return fmt.Sprintf(fmtStr, value)
}

// GetMax returns the max option as a float, defaulting to 1.0.
func GetMax(options map[string]string) float32 {
	if maxStr, ok := options["max"]; ok {
		if max, err := strconv.ParseFloat(maxStr, 32); err == nil {
			return float32(max)
		}
	}
	return 1.0
}

// GetFloatValue extracts a float32 from various types.
func GetFloatValue(value interface{}) (float32, bool) {
	switch v := value.(type) {
	case float32:
		return v, true
	case float64:
		return float32(v), true
	case int:
		return float32(v), true
	case int32:
		return float32(v), true
	case int64:
		return float32(v), true
	case uint32:
		return float32(v), true
	default:
		return 0, false
	}
}

// GetFloatSlice extracts a slice of float32 from arrays.
func GetFloatSlice(value interface{}) ([]float32, bool) {
	v := reflect.ValueOf(value)
	if v.Kind() != reflect.Array && v.Kind() != reflect.Slice {
		return nil, false
	}

	result := make([]float32, v.Len())
	for i := 0; i < v.Len(); i++ {
		elem := v.Index(i)
		switch e := elem.Interface().(type) {
		case float32:
			result[i] = e
		case float64:
			result[i] = float32(e)
		default:
			return nil, false
		}
	}
	return result, true
}

package camera

import (
	"math"
	"testing"
)

func TestNew(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)

	// Should be centered on world
	if cam.X != 1280 || cam.Y != 720 {
		t.Errorf("expected camera at (1280, 720), got (%f, %f)", cam.X, cam.Y)
	}
	if cam.Zoom != 1.0 {
		t.Errorf("expected zoom 1.0, got %f", cam.Zoom)
	}
}

func TestWorldToScreenCentered(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)

	// Camera center should map to screen center
	sx, sy := cam.WorldToScreen(1280, 720)
	if math.Abs(float64(sx-640)) > 0.01 || math.Abs(float64(sy-360)) > 0.01 {
		t.Errorf("expected screen center (640, 360), got (%f, %f)", sx, sy)
	}
}

func TestScreenToWorldRoundtrip(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)

	// Test roundtrip at various positions
	testCases := []struct{ sx, sy float32 }{
		{640, 360},  // center
		{100, 100},  // top-left
		{1200, 600}, // near bottom-right
	}

	for _, tc := range testCases {
		wx, wy := cam.ScreenToWorld(tc.sx, tc.sy)
		sx, sy := cam.WorldToScreen(wx, wy)
		if math.Abs(float64(sx-tc.sx)) > 0.01 || math.Abs(float64(sy-tc.sy)) > 0.01 {
			t.Errorf("roundtrip failed: (%f,%f) -> (%f,%f) -> (%f,%f)",
				tc.sx, tc.sy, wx, wy, sx, sy)
		}
	}
}

func TestToroidalWrap(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)
	cam.X = 100 // Near left edge

	// Entity at world right edge should appear on the left side of screen
	// (closer via toroidal distance)
	sx, _ := cam.WorldToScreen(2500, 720)

	// Should be on left side of screen (negative offset from center)
	if sx >= 640 {
		t.Errorf("expected entity on left of screen, got x=%f", sx)
	}
}

func TestPanWraps(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)
	cam.X = 100

	// Pan left should wrap to right side of world
	cam.Pan(-200, 0)

	if cam.X < 2000 {
		t.Errorf("expected X to wrap around, got %f", cam.X)
	}
}

func TestZoomClamp(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)

	// MinZoom should be max(1280/2560, 720/1440) = max(0.5, 0.5) = 0.5
	if cam.MinZoom != 0.5 {
		t.Errorf("expected MinZoom 0.5, got %f", cam.MinZoom)
	}

	cam.SetZoom(0.1) // Below min
	if cam.Zoom != 0.5 {
		t.Errorf("expected zoom clamped to 0.5, got %f", cam.Zoom)
	}

	cam.SetZoom(10.0) // Above max
	if cam.Zoom != 4.0 {
		t.Errorf("expected zoom clamped to 4.0, got %f", cam.Zoom)
	}
}

func TestMinZoomPreventsDeadSpace(t *testing.T) {
	// Test with asymmetric world/viewport ratios
	cam := New(800, 600, 1600, 800)

	// MinZoom should be max(800/1600, 600/800) = max(0.5, 0.75) = 0.75
	if math.Abs(float64(cam.MinZoom-0.75)) > 0.001 {
		t.Errorf("expected MinZoom 0.75, got %f", cam.MinZoom)
	}

	// At min zoom, visible area should exactly fit world in limiting dimension
	cam.SetZoom(cam.MinZoom)
	visibleH := cam.ViewportH / cam.Zoom // 600 / 0.75 = 800 = worldH
	if math.Abs(float64(visibleH-cam.WorldH)) > 0.01 {
		t.Errorf("at min zoom, visible height %f should equal world height %f", visibleH, cam.WorldH)
	}
}

func TestIsVisible(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)

	// Camera centered at (1280, 720), viewport 1280x720
	// Visible range in world coords: (1280-640, 720-360) to (1280+640, 720+360)
	// = (640, 360) to (1920, 1080)

	// Point at camera center should be visible
	if !cam.IsVisible(1280, 720, 10) {
		t.Error("center should be visible")
	}

	// Point far outside should not be visible
	if cam.IsVisible(2400, 1300, 10) {
		t.Error("far point should not be visible")
	}

	// Point near edge with large radius should be visible
	if !cam.IsVisible(600, 720, 100) {
		t.Error("edge point with large radius should be visible")
	}
}

func TestReset(t *testing.T) {
	cam := New(1280, 720, 2560, 1440)
	cam.X = 500
	cam.Y = 500
	cam.Zoom = 2.5

	cam.Reset()

	if cam.X != 1280 || cam.Y != 720 {
		t.Errorf("expected position (1280, 720), got (%f, %f)", cam.X, cam.Y)
	}
	if cam.Zoom != 1.0 {
		t.Errorf("expected zoom 1.0, got %f", cam.Zoom)
	}
}

package systems

import (
	"testing"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/config"
)

func init() {
	config.MustInit("")
	InitSensorCache()
}

func TestSensorInputsAsSlice(t *testing.T) {
	inputs := SensorInputs{
		Prey:     [NumSectors]float32{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8},
		Pred:     [NumSectors]float32{0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1},
		Resource: [NumSectors]float32{},
		Energy:   0.8,
		Speed:    0.5,
	}

	slice := inputs.AsSlice()

	if len(slice) != NumSectors*3+2 {
		t.Errorf("AsSlice wrong length: got %d, want %d", len(slice), NumSectors*3+2)
	}

	// Check prey values
	for i := 0; i < NumSectors; i++ {
		if slice[i] != inputs.Prey[i] {
			t.Errorf("Prey[%d] mismatch: got %f, want %f", i, slice[i], inputs.Prey[i])
		}
	}

	// Check energy and speed
	if slice[NumSectors*3] != inputs.Energy {
		t.Errorf("Energy mismatch: got %f, want %f", slice[NumSectors*3], inputs.Energy)
	}
	if slice[NumSectors*3+1] != inputs.Speed {
		t.Errorf("Speed mismatch: got %f, want %f", slice[NumSectors*3+1], inputs.Speed)
	}
}

func TestComputeSensorsNoNeighbors(t *testing.T) {
	pos := components.Position{X: 100, Y: 100}
	vel := components.Velocity{X: 1, Y: 0}
	rot := components.Rotation{Heading: 0}
	energy := components.Energy{Value: 0.8, Alive: true}
	caps := components.DefaultCapabilities(components.KindPrey)

	inputs := ComputeSensors(
		pos, vel, rot, energy, caps, components.KindPrey,
		nil, nil, nil, nil, // neighbors, posMap, orgMap, resourceField
		1280, 720,
	)

	// With no neighbors, prey and pred should be zero
	for i := 0; i < NumSectors; i++ {
		if inputs.Prey[i] != 0 {
			t.Errorf("Prey[%d] should be 0, got %f", i, inputs.Prey[i])
		}
		if inputs.Pred[i] != 0 {
			t.Errorf("Pred[%d] should be 0, got %f", i, inputs.Pred[i])
		}
	}

	// Energy and speed should be set
	if inputs.Energy != 0.8 {
		t.Errorf("Energy wrong: got %f, want 0.8", inputs.Energy)
	}

	expectedSpeed := 1.0 / caps.MaxSpeed
	if inputs.Speed != expectedSpeed {
		t.Errorf("Speed wrong: got %f, want %f", inputs.Speed, expectedSpeed)
	}
}

func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		input    float32
		expected float32
	}{
		{0, 0},
		{3.14159, 3.14159},
		{-3.14159, -3.14159},
		{6.28318, 0}, // 2pi -> 0
	}

	for _, tt := range tests {
		got := normalizeAngle(tt.input)
		// Allow some floating point tolerance
		if got < tt.expected-0.01 || got > tt.expected+0.01 {
			t.Errorf("normalizeAngle(%f) = %f, want ~%f", tt.input, got, tt.expected)
		}
	}

	// Special case: 3pi should normalize to +pi or -pi (both valid)
	got := normalizeAngle(9.42478) // 3pi
	if got < 3.13 && got > -3.13 {
		t.Errorf("normalizeAngle(3pi) = %f, want ~pi or ~-pi", got)
	}
}

func TestSmoothSaturate(t *testing.T) {
	// smoothSaturate(0) should be 0
	if smoothSaturate(0) != 0 {
		t.Errorf("smoothSaturate(0) = %f, want 0", smoothSaturate(0))
	}

	// smoothSaturate should approach 1 for large values
	if smoothSaturate(10) < 0.99 {
		t.Errorf("smoothSaturate(10) = %f, want ~1", smoothSaturate(10))
	}

	// smoothSaturate should be monotonic
	prev := float32(0)
	for i := 0; i <= 10; i++ {
		x := float32(i) * 0.5
		v := smoothSaturate(x)
		if v < prev {
			t.Errorf("smoothSaturate not monotonic at %f", x)
		}
		prev = v
	}
}

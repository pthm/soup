package components

// Position represents an entity's world position.
type Position struct {
	X float32 `inspect:"label,fmt:%.0f"`
	Y float32 `inspect:"label,fmt:%.0f"`
}

// Velocity represents an entity's velocity.
type Velocity struct {
	X float32 `inspect:"label,fmt:%.1f"`
	Y float32 `inspect:"label,fmt:%.1f"`
}

// Rotation represents an entity's heading and angular velocity.
type Rotation struct {
	Heading float32 `inspect:"angle"` // radians
	AngVel  float32 `inspect:"label,fmt:%.2f"` // angular velocity (radians per tick)
}

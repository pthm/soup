// Package components defines ECS components for the simulation.
package components

// Position represents an entity's world position.
type Position struct {
	X, Y float32
}

// Velocity represents an entity's velocity.
type Velocity struct {
	X, Y float32
}

// Rotation represents an entity's heading and angular velocity.
type Rotation struct {
	Heading float32 // radians
	AngVel  float32 // angular velocity (radians per tick)
}

// Body holds physical properties of an entity.
type Body struct {
	Radius float32
}

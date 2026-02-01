package game

import (
	"fmt"
	"math"

	"github.com/mlange-42/ark/ecs"
	rl "github.com/gen2brain/raylib-go/raylib"

	"github.com/pthm-cable/soup/components"
	"github.com/pthm-cable/soup/renderer"
)

// Screen dimensions
const (
	ScreenWidth  = 1280
	ScreenHeight = 720
)

// Physics constants
const (
	MaxSpeed     = 3.0
	Acceleration = 0.1
	TurnRateMax  = 0.15 // radians per tick
)

// Controller defines the interface for entity control.
// Brain will implement this later.
type Controller interface {
	Update(pos components.Position, vel components.Velocity, rot components.Rotation) (turnRate, throttle float32)
}

// FlowController follows the GPU flow field.
type FlowController struct {
	flow *renderer.GPUFlowField
}

// NewFlowController creates a controller that follows the flow field.
func NewFlowController(flow *renderer.GPUFlowField) *FlowController {
	return &FlowController{flow: flow}
}

// Update returns turn rate and throttle to follow the flow field.
func (c *FlowController) Update(pos components.Position, vel components.Velocity, rot components.Rotation) (float32, float32) {
	flowX, flowY := c.flow.Sample(pos.X, pos.Y)

	// Target heading from flow direction
	targetHeading := float32(math.Atan2(float64(flowY), float64(flowX)))

	// Compute angle difference
	diff := normalizeAngle(targetHeading - rot.Heading)

	// Proportional turn rate
	turnRate := diff * 0.1
	if turnRate > TurnRateMax {
		turnRate = TurnRateMax
	} else if turnRate < -TurnRateMax {
		turnRate = -TurnRateMax
	}

	throttle := float32(0.5)
	return turnRate, throttle
}

// normalizeAngle wraps angle to [-pi, pi].
func normalizeAngle(a float32) float32 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a < -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// Game holds the complete game state.
type Game struct {
	world  *ecs.World
	entity ecs.Entity

	// Mappers
	mapper *ecs.Map4[components.Position, components.Velocity, components.Rotation, components.Body]
	filter *ecs.Filter4[components.Position, components.Velocity, components.Rotation, components.Body]

	// Rendering
	water *renderer.WaterBackground
	flow  *renderer.GPUFlowField

	// Control
	controller Controller

	// State
	tick   int32
	paused bool

	// Window dimensions
	width, height float32
}

// NewGame creates a new game instance.
func NewGame() *Game {
	world := ecs.NewWorld()

	g := &Game{
		world:  world,
		width:  float32(ScreenWidth),
		height: float32(ScreenHeight),
		mapper: ecs.NewMap4[components.Position, components.Velocity, components.Rotation, components.Body](world),
		filter: ecs.NewFilter4[components.Position, components.Velocity, components.Rotation, components.Body](world),
	}

	// GPU flow field
	g.flow = renderer.NewGPUFlowField(g.width, g.height)

	// Water background
	g.water = renderer.NewWaterBackground(int32(g.width), int32(g.height))

	// Controller (follows flow field)
	g.controller = NewFlowController(g.flow)

	// Create single entity
	g.createEntity()

	return g
}

// createEntity spawns a single entity in the center of the world.
func (g *Game) createEntity() {
	pos := components.Position{X: g.width / 2, Y: g.height / 2}
	vel := components.Velocity{X: 0, Y: 0}
	rot := components.Rotation{Heading: 0, AngVel: 0}
	body := components.Body{Radius: 10}

	g.entity = g.mapper.NewEntity(&pos, &vel, &rot, &body)
}

// Update runs one simulation step.
func (g *Game) Update() {
	// Handle input
	g.handleInput()

	if g.paused {
		return
	}

	// Update flow field
	g.flow.Update(g.tick, float32(g.tick)*0.01)

	// Run simulation step
	g.updatePhysics()

	g.tick++
}

// handleInput processes keyboard input.
func (g *Game) handleInput() {
	if rl.IsKeyPressed(rl.KeySpace) {
		g.paused = !g.paused
	}
}

// updatePhysics applies physics to all entities.
func (g *Game) updatePhysics() {
	query := g.filter.Query()
	for query.Next() {
		pos, vel, rot, _ := query.Get()

		// Get control inputs
		turnRate, throttle := g.controller.Update(*pos, *vel, *rot)

		// Apply angular velocity to heading (heading-as-state)
		// Minimum turn rate (30%) even at zero throttle enables arrival behavior
		effectiveTurnRate := turnRate * max(throttle, 0.3)
		rot.Heading += effectiveTurnRate
		rot.Heading = normalizeAngle(rot.Heading)

		// Compute desired velocity from heading
		desiredVelX := float32(math.Cos(float64(rot.Heading))) * throttle * MaxSpeed
		desiredVelY := float32(math.Sin(float64(rot.Heading))) * throttle * MaxSpeed

		// Smooth velocity change
		vel.X += (desiredVelX - vel.X) * Acceleration
		vel.Y += (desiredVelY - vel.Y) * Acceleration

		// Update position
		pos.X += vel.X
		pos.Y += vel.Y

		// Toroidal wrap
		pos.X = mod(pos.X, g.width)
		pos.Y = mod(pos.Y, g.height)
	}
}

// mod returns positive modulo (Go's % can return negative).
func mod(a, b float32) float32 {
	return float32(math.Mod(float64(a)+float64(b), float64(b)))
}

// Draw renders the game.
func (g *Game) Draw() {
	rl.BeginDrawing()
	rl.ClearBackground(rl.Black)

	// Water background
	g.water.Draw(float32(g.tick) * 0.01)

	// Draw entity
	g.drawEntities()

	// Draw tick count
	rl.DrawText(fmt.Sprintf("Tick: %d", g.tick), 10, 10, 20, rl.White)
	if g.paused {
		rl.DrawText("PAUSED", 10, 35, 20, rl.Yellow)
	}

	rl.EndDrawing()
}

// drawEntities renders all entities as oriented triangles.
func (g *Game) drawEntities() {
	query := g.filter.Query()
	for query.Next() {
		pos, _, rot, body := query.Get()

		// Draw oriented triangle
		drawOrientedTriangle(pos.X, pos.Y, rot.Heading, body.Radius)
	}
}

// drawOrientedTriangle draws a triangle pointing in the heading direction.
func drawOrientedTriangle(x, y, heading, radius float32) {
	// Triangle vertices relative to center
	// Point in heading direction
	cos := float32(math.Cos(float64(heading)))
	sin := float32(math.Sin(float64(heading)))

	// Front point
	frontX := x + cos*radius*1.5
	frontY := y + sin*radius*1.5

	// Back left
	backAngle := heading + math.Pi*0.8
	backLeftX := x + float32(math.Cos(float64(backAngle)))*radius
	backLeftY := y + float32(math.Sin(float64(backAngle)))*radius

	// Back right
	backAngle = heading - math.Pi*0.8
	backRightX := x + float32(math.Cos(float64(backAngle)))*radius
	backRightY := y + float32(math.Sin(float64(backAngle)))*radius

	v1 := rl.Vector2{X: frontX, Y: frontY}
	v2 := rl.Vector2{X: backLeftX, Y: backLeftY}
	v3 := rl.Vector2{X: backRightX, Y: backRightY}

	rl.DrawTriangle(v1, v2, v3, rl.Green)
	rl.DrawTriangleLines(v1, v2, v3, rl.White)
}

// Unload releases all resources.
func (g *Game) Unload() {
	if g.water != nil {
		g.water.Unload()
	}
	if g.flow != nil {
		g.flow.Unload()
	}
}

// Tick returns the current simulation tick.
func (g *Game) Tick() int32 {
	return g.tick
}

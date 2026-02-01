// Package telemetry provides ecosystem health tracking, bookmarking, and snapshots.
package telemetry

import "github.com/pthm-cable/soup/components"

// EventType identifies telemetry events.
type EventType uint8

const (
	EventBiteAttempt EventType = iota
	EventBiteHit
	EventKill
	EventBirth
	EventDeath
	EventForage
)

// Event represents a single telemetry event.
type Event struct {
	Type     EventType
	Tick     int32
	EntityID uint32
	Kind     components.Kind

	// Optional fields depending on event type
	TargetID uint32  // for bite/kill events
	Amount   float32 // energy transferred (bite) or foraging gain
}

// NewBiteAttemptEvent creates a bite attempt event.
func NewBiteAttemptEvent(tick int32, predatorID uint32) Event {
	return Event{
		Type:     EventBiteAttempt,
		Tick:     tick,
		EntityID: predatorID,
		Kind:     components.KindPredator,
	}
}

// NewBiteHitEvent creates a successful bite event.
func NewBiteHitEvent(tick int32, predatorID, preyID uint32, energyTransferred float32) Event {
	return Event{
		Type:     EventBiteHit,
		Tick:     tick,
		EntityID: predatorID,
		Kind:     components.KindPredator,
		TargetID: preyID,
		Amount:   energyTransferred,
	}
}

// NewKillEvent creates a kill event (prey died from bite).
func NewKillEvent(tick int32, predatorID, preyID uint32) Event {
	return Event{
		Type:     EventKill,
		Tick:     tick,
		EntityID: predatorID,
		Kind:     components.KindPredator,
		TargetID: preyID,
	}
}

// NewBirthEvent creates a birth event.
func NewBirthEvent(tick int32, childID, parentID uint32, kind components.Kind) Event {
	return Event{
		Type:     EventBirth,
		Tick:     tick,
		EntityID: childID,
		Kind:     kind,
		TargetID: parentID, // parent ID stored in TargetID
	}
}

// NewDeathEvent creates a death event.
func NewDeathEvent(tick int32, entityID uint32, kind components.Kind) Event {
	return Event{
		Type:     EventDeath,
		Tick:     tick,
		EntityID: entityID,
		Kind:     kind,
	}
}

// NewForageEvent creates a foraging event (prey gaining energy from resources).
func NewForageEvent(tick int32, preyID uint32, amount float32) Event {
	return Event{
		Type:     EventForage,
		Tick:     tick,
		EntityID: preyID,
		Kind:     components.KindPrey,
		Amount:   amount,
	}
}

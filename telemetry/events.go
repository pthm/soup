// Package telemetry provides ecosystem health tracking, bookmarking, and snapshots.
package telemetry

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
	Diet     float32

	// Optional fields depending on event type
	TargetID uint32  // for bite/kill events
	Amount   float32 // energy transferred (bite) or foraging gain
}

// NewBiteAttemptEvent creates a bite attempt event.
func NewBiteAttemptEvent(tick int32, predatorID uint32, diet float32) Event {
	return Event{
		Type:     EventBiteAttempt,
		Tick:     tick,
		EntityID: predatorID,
		Diet:     diet,
	}
}

// NewBiteHitEvent creates a successful bite event.
func NewBiteHitEvent(tick int32, predatorID, preyID uint32, diet, energyTransferred float32) Event {
	return Event{
		Type:     EventBiteHit,
		Tick:     tick,
		EntityID: predatorID,
		Diet:     diet,
		TargetID: preyID,
		Amount:   energyTransferred,
	}
}

// NewKillEvent creates a kill event (prey died from bite).
func NewKillEvent(tick int32, predatorID, preyID uint32, diet float32) Event {
	return Event{
		Type:     EventKill,
		Tick:     tick,
		EntityID: predatorID,
		Diet:     diet,
		TargetID: preyID,
	}
}

// NewBirthEvent creates a birth event.
func NewBirthEvent(tick int32, childID, parentID uint32, diet float32) Event {
	return Event{
		Type:     EventBirth,
		Tick:     tick,
		EntityID: childID,
		Diet:     diet,
		TargetID: parentID, // parent ID stored in TargetID
	}
}

// NewDeathEvent creates a death event.
func NewDeathEvent(tick int32, entityID uint32, diet float32) Event {
	return Event{
		Type:     EventDeath,
		Tick:     tick,
		EntityID: entityID,
		Diet:     diet,
	}
}

// NewForageEvent creates a foraging event (organism gaining energy from resources).
func NewForageEvent(tick int32, entityID uint32, diet, amount float32) Event {
	return Event{
		Type:     EventForage,
		Tick:     tick,
		EntityID: entityID,
		Diet:     diet,
		Amount:   amount,
	}
}

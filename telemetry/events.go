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


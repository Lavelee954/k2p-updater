package models

// Event represents events that can trigger state transitions
type Event string

// State machine events
const (
	EventInitialize        Event = "Initialize"
	EventCooldownEnded     Event = "CooldownEnded"
	EventThresholdExceeded Event = "ThresholdExceeded"
	EventSpecUpRequested   Event = "SpecUpRequested"
	EventSpecUpCompleted   Event = "SpecUpCompleted"
	EventSpecUpFailed      Event = "SpecUpFailed"
	EventHealthCheckPassed Event = "HealthCheckPassed"
	EventHealthCheckFailed Event = "HealthCheckFailed"
	EventEnterCooldown     Event = "EnterCooldown"
	EventRecoveryAttempt   Event = "RecoveryAttempt"
	EventCooldownStatus    Event = "CooldownStatus"
	EventMonitoringStatus  Event = "MonitoringStatus"
)

package domain

import (
	"context"
)

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

// StateMachine defines the interface for the VM spec up state machine
type StateMachine interface {
	// GetCurrentState returns the current state of the state machine
	GetCurrentState(ctx context.Context, nodeName string) (State, error)

	// HandleEvent processes an event and performs the appropriate state transition
	HandleEvent(ctx context.Context, nodeName string, event Event, data map[string]interface{}) error

	// GetStatus returns the full status of a control plane node
	GetStatus(ctx context.Context, nodeName string) (*ControlPlaneStatus, error)

	// UpdateStatus updates the status in the custom resource
	UpdateStatus(ctx context.Context, nodeName string, status *ControlPlaneStatus) error
}

// StateHandler defines the interface for individual state handlers
type StateHandler interface {
	// Handle processes events for a specific state
	Handle(ctx context.Context, status *ControlPlaneStatus, event Event, data map[string]interface{}) (*ControlPlaneStatus, error)

	// OnEnter is called when entering this state
	OnEnter(ctx context.Context, status *ControlPlaneStatus) (*ControlPlaneStatus, error)

	// OnExit is called when exiting this state
	OnExit(ctx context.Context, status *ControlPlaneStatus) (*ControlPlaneStatus, error)
}

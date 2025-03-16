// state_dispatcher.go
package state

import (
	"context"
	"fmt"

	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// EventDispatcher handles dispatching events to appropriate handlers
type EventDispatcher struct {
	transitionMatrix map[models.State]map[models.Event]models.State
	stateHandlers    map[models.State]interfaces.StateHandler
}

// NewEventDispatcher creates a new event dispatcher
func NewEventDispatcher(handlers map[models.State]interfaces.StateHandler) *EventDispatcher {
	return &EventDispatcher{
		stateHandlers:    handlers,
		transitionMatrix: buildTransitionMatrix(),
	}
}

// buildTransitionMatrix defines the allowed state transitions
func buildTransitionMatrix() map[models.State]map[models.Event]models.State {
	matrix := make(map[models.State]map[models.Event]models.State)

	// Define transitions for PendingVmSpecUp state
	matrix[models.StatePendingVmSpecUp] = map[models.Event]models.State{
		models.EventCooldownEnded: models.StateMonitoring,
		models.EventInitialize:    models.StatePendingVmSpecUp, // Self-transition
	}

	// Define transitions for Monitoring state
	matrix[models.StateMonitoring] = map[models.Event]models.State{
		models.EventThresholdExceeded: models.StateInProgressVmSpecUp,
		models.EventMonitoringStatus:  models.StateMonitoring, // Self-transition
		models.EventInitialize:        models.StateMonitoring, // Self-transition
	}

	// Define transitions for InProgressVmSpecUp state
	matrix[models.StateInProgressVmSpecUp] = map[models.Event]models.State{
		models.EventSpecUpRequested:   models.StateInProgressVmSpecUp, // Self-transition
		models.EventSpecUpCompleted:   models.StateInProgressVmSpecUp, // Self-transition
		models.EventHealthCheckPassed: models.StateCompletedVmSpecUp,
		models.EventHealthCheckFailed: models.StateFailedVmSpecUp,
		models.EventSpecUpFailed:      models.StateFailedVmSpecUp,
	}

	// Define transitions for CompletedVmSpecUp state
	matrix[models.StateCompletedVmSpecUp] = map[models.Event]models.State{
		models.EventEnterCooldown: models.StateCoolDown,
	}

	// Define transitions for FailedVmSpecUp state
	matrix[models.StateFailedVmSpecUp] = map[models.Event]models.State{
		models.EventRecoveryAttempt: models.StatePendingVmSpecUp,
	}

	// Define transitions for CoolDown state
	matrix[models.StateCoolDown] = map[models.Event]models.State{
		models.EventCooldownEnded:  models.StateMonitoring,
		models.EventCooldownStatus: models.StateCoolDown, // Self-transition
		models.EventInitialize:     models.StateCoolDown, // Self-transition
	}

	return matrix
}

// DispatchEvent dispatches an event to the appropriate handler
func (d *EventDispatcher) DispatchEvent(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("context error before dispatching event: %w", err)
	}

	currentState := status.CurrentState

	// Validate event against transition matrix
	if err := d.validateEvent(currentState, event); err != nil {
		return nil, err
	}

	// Get the handler for the current state
	handler, exists := d.stateHandlers[currentState]
	if !exists {
		return nil, fmt.Errorf("no handler found for state %s", currentState)
	}

	// Handle the event
	newStatus, err := handler.Handle(ctx, status, event, data)
	if err != nil {
		return nil, fmt.Errorf("error handling event %s in state %s: %w", event, currentState, err)
	}

	// Validate the resulting state transition
	if newStatus.CurrentState != currentState {
		expectedTargetState := d.transitionMatrix[currentState][event]
		if newStatus.CurrentState != expectedTargetState {
			return nil, fmt.Errorf("invalid state transition from %s to %s triggered by event %s (expected %s)",
				currentState, newStatus.CurrentState, event, expectedTargetState)
		}
	}

	return newStatus, nil
}

// validateEvent checks if the event is valid for the current state
func (d *EventDispatcher) validateEvent(currentState models.State, event models.Event) error {
	stateTransitions, stateExists := d.transitionMatrix[currentState]
	if !stateExists {
		return fmt.Errorf("no transitions defined for state %s", currentState)
	}

	_, eventExists := stateTransitions[event]
	if !eventExists {
		return fmt.Errorf("event %s is not valid for state %s", event, currentState)
	}

	return nil
}

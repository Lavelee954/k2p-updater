package state

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
)

// EventDispatcher handles dispatching events to the appropriate handler
type EventDispatcher struct {
	transitionMatrix map[models.State]map[models.Event]models.State
	stateHandlers    map[models.State]interfaces.StateHandler
}

// NewEventDispatcher creates a new event dispatcher
func NewEventDispatcher(handlers map[models.State]interfaces.StateHandler) *EventDispatcher {
	dispatcher := &EventDispatcher{
		stateHandlers:    handlers,
		transitionMatrix: buildTransitionMatrix(),
	}
	return dispatcher
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

	// Check context early
	if err := common.CheckContext(ctx); err != nil {
		return nil, fmt.Errorf("context error before dispatching event: %w", err)
	}

	currentState := status.CurrentState
	handler, exists := d.stateHandlers[currentState]
	if !exists {
		return nil, fmt.Errorf("no handler found for state %s", currentState)
	}

	// Handle the event with the current state handler
	newStatus, err := handler.Handle(ctx, status, event, data)
	if err != nil {
		return nil, fmt.Errorf("error handling event %s in state %s: %w", event, currentState, err)
	}

	// Check if the resulting state is allowed by the transition matrix
	if newStatus.CurrentState != currentState {
		allowed, exists := d.isTransitionAllowed(currentState, event, newStatus.CurrentState)
		if !exists || !allowed {
			return nil, fmt.Errorf("invalid state transition from %s to %s triggered by event %s",
				currentState, newStatus.CurrentState, event)
		}
	}

	return newStatus, nil
}

// isTransitionAllowed checks if a transition is allowed by the matrix
func (d *EventDispatcher) isTransitionAllowed(fromState models.State, event models.Event,
	toState models.State) (bool, bool) {
	stateTransitions, exists := d.transitionMatrix[fromState]
	if !exists {
		return false, false
	}

	allowedState, exists := stateTransitions[event]
	return allowedState == toState, exists
}

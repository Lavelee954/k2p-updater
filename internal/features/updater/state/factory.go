package state

import (
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/internal/features/updater/state/handlers"
	"k2p-updater/pkg/resource"
)

// CreateStateHandlers creates the default set of state handlers
func CreateStateHandlers(resourceFactory *resource.Factory) map[models.State]interfaces.StateHandler {
	return map[models.State]interfaces.StateHandler{
		models.StatePendingVmSpecUp:    handlers.NewPendingHandler(resourceFactory),
		models.StateMonitoring:         handlers.NewMonitoringHandler(resourceFactory),
		models.StateInProgressVmSpecUp: handlers.NewInProgressHandler(resourceFactory),
		models.StateCompletedVmSpecUp:  handlers.NewCompletedHandler(resourceFactory),
		models.StateFailedVmSpecUp:     handlers.NewFailedHandler(resourceFactory),
		models.StateCoolDown:           handlers.NewCoolDownHandler(resourceFactory),
	}
}

// NewStateMachineWithDefaultHandlers creates a new state machine with default handlers
func NewStateMachineWithDefaultHandlers(resourceFactory *resource.Factory) interfaces.StateMachine {
	handlers := CreateStateHandlers(resourceFactory)
	return NewStateMachine(resourceFactory, handlers)
}

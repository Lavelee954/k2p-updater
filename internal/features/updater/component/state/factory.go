package state

import (
	handlers2 "k2p-updater/internal/features/updater/component/state/handlers"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
)

// CreateStateHandlers creates the default set of state handlers
func CreateStateHandlers(resourceFactory *resource.Factory) map[models.State]interfaces.StateHandler {
	return map[models.State]interfaces.StateHandler{
		models.StatePendingVmSpecUp:    handlers2.NewPendingHandler(resourceFactory),
		models.StateMonitoring:         handlers2.NewMonitoringHandler(resourceFactory),
		models.StateInProgressVmSpecUp: handlers2.NewInProgressHandler(resourceFactory),
		models.StateCompletedVmSpecUp:  handlers2.NewCompletedHandler(resourceFactory),
		models.StateFailedVmSpecUp:     handlers2.NewFailedHandler(resourceFactory),
		models.StateCoolDown:           handlers2.NewCoolDownHandler(resourceFactory),
	}
}

// NewStateMachineWithDefaultHandlers creates a new state machine with default handlers
func NewStateMachineWithDefaultHandlers(resourceFactory *resource.Factory) interfaces.StateMachine {
	handlers := CreateStateHandlers(resourceFactory)
	return NewStateMachine(resourceFactory, handlers)
}

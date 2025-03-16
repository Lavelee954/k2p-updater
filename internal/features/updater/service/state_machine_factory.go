package service

import (
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
)

// StateMachineFactory creates instances of the state machine with appropriate handlers
type StateMachineFactory struct {
	resourceFactory *resource.Factory
}

// NewStateMachineFactory creates a new state machine factory
func NewStateMachineFactory(resourceFactory *resource.Factory) *StateMachineFactory {
	return &StateMachineFactory{
		resourceFactory: resourceFactory,
	}
}

// CreateStateMachine creates a new state machine with all handlers properly initialized
func (f *StateMachineFactory) CreateStateMachine() domain.StateMachine {
	sm := &stateMachine{
		statusMap:       make(map[string]*domain.ControlPlaneStatus),
		stateHandlers:   make(map[domain.State]domain.StateHandler),
		resourceFactory: f.resourceFactory,
	}

	// Register state handlers
	sm.stateHandlers[domain.StatePendingVmSpecUp] = newPendingHandler(f.resourceFactory)
	sm.stateHandlers[domain.StateMonitoring] = newMonitoringHandler(f.resourceFactory)
	sm.stateHandlers[domain.StateInProgressVmSpecUp] = newInProgressHandler(f.resourceFactory)
	sm.stateHandlers[domain.StateCompletedVmSpecUp] = newCompletedHandler(f.resourceFactory)
	sm.stateHandlers[domain.StateFailedVmSpecUp] = newFailedHandler(f.resourceFactory)
	sm.stateHandlers[domain.StateCoolDown] = newCoolDownHandler(f.resourceFactory)

	return sm
}

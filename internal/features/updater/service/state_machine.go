package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	updaterDomain "k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// stateMachine implements the domain.StateMachine interface
type stateMachine struct {
	// statusMap stores the current status for each node
	statusMap map[string]*updaterDomain.ControlPlaneStatus

	// stateHandlers maps states to their handlers
	stateHandlers map[updaterDomain.State]updaterDomain.StateHandler

	// resourceFactory is used to update the custom resource status
	resourceFactory *resource.Factory

	// mu protects concurrent access to the status map
	mu sync.RWMutex
}

// NewStateMachine creates a new state machine
func NewStateMachine(resourceFactory *resource.Factory) updaterDomain.StateMachine {
	sm := &stateMachine{
		statusMap:       make(map[string]*updaterDomain.ControlPlaneStatus),
		stateHandlers:   make(map[updaterDomain.State]updaterDomain.StateHandler),
		resourceFactory: resourceFactory,
	}

	// Register state handlers
	sm.stateHandlers[updaterDomain.StatePendingVmSpecUp] = newPendingHandler(resourceFactory)
	sm.stateHandlers[updaterDomain.StateMonitoring] = newMonitoringHandler(resourceFactory)
	sm.stateHandlers[updaterDomain.StateInProgressVmSpecUp] = newInProgressHandler(resourceFactory)
	sm.stateHandlers[updaterDomain.StateCompletedVmSpecUp] = newCompletedHandler(resourceFactory)
	sm.stateHandlers[updaterDomain.StateFailedVmSpecUp] = newFailedHandler(resourceFactory)
	sm.stateHandlers[updaterDomain.StateCoolDown] = newCoolDownHandler(resourceFactory)

	return sm
}

// GetCurrentState returns the current state for a node
func (sm *stateMachine) GetCurrentState(ctx context.Context, nodeName string) (updaterDomain.State, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status, exists := sm.statusMap[nodeName]
	if !exists {
		return "", common.NotFoundError("node %s not found in state machine", nodeName)
	}

	return status.CurrentState, nil
}

// HandleEvent processes an event and transitions to the next state if needed
func (sm *stateMachine) HandleEvent(ctx context.Context, nodeName string, event updaterDomain.Event, data map[string]interface{}) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get or initialize node status
	status, exists := sm.statusMap[nodeName]
	if !exists {
		// Initialize with default state
		status = &updaterDomain.ControlPlaneStatus{
			NodeName:           nodeName,
			CurrentState:       updaterDomain.StatePendingVmSpecUp,
			LastTransitionTime: time.Now(),
			Message:            "Initializing",
		}
		sm.statusMap[nodeName] = status
	}

	// Get current state handler
	handler, exists := sm.stateHandlers[status.CurrentState]
	if !exists {
		return fmt.Errorf("no handler found for state %s", status.CurrentState)
	}

	// Handle the event
	newStatus, err := handler.Handle(ctx, status, event, data)
	if err != nil {
		return fmt.Errorf("error handling event %s in state %s: %w", event, status.CurrentState, err)
	}

	// If state has changed, perform transition
	if newStatus.CurrentState != status.CurrentState {
		// Call exit handler for the current state
		exitHandler := sm.stateHandlers[status.CurrentState]
		if exitHandler != nil {
			if _, err := exitHandler.OnExit(ctx, status); err != nil {
				log.Printf("Error during state exit: %v", err)
			}
		}

		// Update timestamp and perform transition
		newStatus.LastTransitionTime = time.Now()

		// Call enter handler for the new state
		enterHandler := sm.stateHandlers[newStatus.CurrentState]
		if enterHandler != nil {
			if updatedStatus, err := enterHandler.OnEnter(ctx, newStatus); err != nil {
				log.Printf("Error during state entry: %v", err)
			} else {
				newStatus = updatedStatus
			}
		}

		log.Printf("State transition for node %s: %s -> %s",
			nodeName, status.CurrentState, newStatus.CurrentState)
	}

	// Update status in map
	sm.statusMap[nodeName] = newStatus

	// Update the custom resource status
	if err := sm.updateCRStatus(ctx, nodeName, newStatus); err != nil {
		log.Printf("Failed to update CR status: %v", err)
	}

	return nil
}

// GetStatus returns the current status for a node
func (sm *stateMachine) GetStatus(ctx context.Context, nodeName string) (*updaterDomain.ControlPlaneStatus, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status, exists := sm.statusMap[nodeName]
	if !exists {
		return nil, common.NotFoundError("node %s not found in state machine", nodeName)
	}

	// Return a copy to prevent modifications outside of the state machine
	statusCopy := *status
	return &statusCopy, nil
}

// UpdateStatus updates the status for a node
func (sm *stateMachine) UpdateStatus(ctx context.Context, nodeName string, status *updaterDomain.ControlPlaneStatus) error {
	if status == nil {
		return common.InvalidInputError("status cannot be nil")
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Store the updated status
	sm.statusMap[nodeName] = status

	// Update the custom resource status
	return sm.updateCRStatus(ctx, nodeName, status)
}

// updateCRStatus updates the control plane status in the custom resource
func (sm *stateMachine) updateCRStatus(ctx context.Context, nodeName string, status *updaterDomain.ControlPlaneStatus) error {
	// Convert status to a map for the CR update
	statusData := map[string]interface{}{
		"controlPlaneNodeName":     nodeName,
		"currentState":             string(status.CurrentState),
		"lastTransitionTime":       status.LastTransitionTime.Format(time.RFC3339),
		"message":                  status.Message,
		"cpuUtilization":           status.CPUUtilization,
		"windowAverageUtilization": status.WindowAverageUtilization,
		"specUpRequested":          status.SpecUpRequested,
		"specUpCompleted":          status.SpecUpCompleted,
		"healthCheckPassed":        status.HealthCheckPassed,
	}

	if !status.CoolDownEndTime.IsZero() {
		statusData["coolDownEndTime"] = status.CoolDownEndTime.Format(time.RFC3339)
	}

	// Update the CR status using the resource factory
	return sm.resourceFactory.Status().UpdateGenericWithNode(ctx, "updater", nodeName, statusData)
}

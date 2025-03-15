package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// stateMachine implements the domain.StateMachine interface
type stateMachine struct {
	// statusMap stores the current status for each node
	statusMap map[string]*domain.ControlPlaneStatus

	// stateHandlers maps states to their handlers
	stateHandlers map[domain.State]domain.StateHandler

	// resourceFactory is used to update the custom resource status
	resourceFactory *resource.Factory

	// metricsCollector collects metrics about the state machine
	metricsCollector *MetricsCollector

	// mu protects concurrent access to the status map
	mu sync.RWMutex
}

// NewStateMachine creates a new state machine
func NewStateMachine(resourceFactory *resource.Factory, metricsCollector ...*MetricsCollector) domain.StateMachine {
	sm := &stateMachine{
		statusMap:       make(map[string]*domain.ControlPlaneStatus),
		stateHandlers:   make(map[domain.State]domain.StateHandler),
		resourceFactory: resourceFactory,
	}

	// Assign metrics collector if provided
	if len(metricsCollector) > 0 && metricsCollector[0] != nil {
		sm.metricsCollector = metricsCollector[0]
	}

	// Register state handlers
	sm.stateHandlers[domain.StatePendingVmSpecUp] = newPendingHandler(resourceFactory)
	sm.stateHandlers[domain.StateMonitoring] = newMonitoringHandler(resourceFactory)
	sm.stateHandlers[domain.StateInProgressVmSpecUp] = newInProgressHandler(resourceFactory)
	sm.stateHandlers[domain.StateCompletedVmSpecUp] = newCompletedHandler(resourceFactory)
	sm.stateHandlers[domain.StateFailedVmSpecUp] = newFailedHandler(resourceFactory)
	sm.stateHandlers[domain.StateCoolDown] = newCoolDownHandler(resourceFactory)

	return sm
}

// GetCurrentState returns the current state for a node
func (sm *stateMachine) GetCurrentState(ctx context.Context, nodeName string) (domain.State, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status, exists := sm.statusMap[nodeName]
	if !exists {
		return "", common.NotFoundError("node %s not found in state machine", nodeName)
	}

	return status.CurrentState, nil
}

// HandleEvent processes an event and transitions to the next state if needed
func (sm *stateMachine) HandleEvent(ctx context.Context, nodeName string, event domain.Event, data map[string]interface{}) error {
	// Log at the beginning of event handling
	log.Printf("STATE MACHINE: Handling event %s for node %s", event, nodeName)
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get or initialize node status
	status, exists := sm.statusMap[nodeName]
	if !exists {
		// Initialize with default state
		status = &domain.ControlPlaneStatus{
			NodeName:           nodeName,
			CurrentState:       domain.StatePendingVmSpecUp,
			LastTransitionTime: time.Now(),
			Message:            "Initializing",
		}
		sm.statusMap[nodeName] = status
		log.Printf("DEBUG: Initializing new node %s in state %s", nodeName, status.CurrentState)
	}

	// Record original state for metrics and logging
	originalState := status.CurrentState
	log.Printf("STATE MACHINE: Handling event originalState %s ", originalState)

	// Get current state handler
	handler, exists := sm.stateHandlers[status.CurrentState]
	if !exists {
		log.Printf("ERROR: No handler found for state %s", status.CurrentState)
		return fmt.Errorf("no handler found for state %s", status.CurrentState)
	}

	// Handle the event
	log.Printf("DEBUG: Calling handle for node %s in state %s with event %s",
		nodeName, status.CurrentState, event)
	newStatus, err := handler.Handle(ctx, status, event, data)
	if err != nil {
		log.Printf("ERROR: Handler error for node %s: %v", nodeName, err)
		return fmt.Errorf("error handling event %s in state %s: %w", event, status.CurrentState, err)
	}

	// If state has changed, perform transition
	if newStatus.CurrentState != status.CurrentState {
		log.Printf("DEBUG: State transition for node %s: %s -> %s",
			nodeName, status.CurrentState, newStatus.CurrentState)

		// Call exit handler for the current state
		exitHandler := sm.stateHandlers[status.CurrentState]
		if exitHandler != nil {
			log.Printf("DEBUG: Calling OnExit for node %s state %s",
				nodeName, status.CurrentState)
			if _, err := exitHandler.OnExit(ctx, status); err != nil {
				log.Printf("ERROR: Exit handler error for node %s: %v", nodeName, err)
			}
		}

		// Update timestamp and perform transition
		newStatus.LastTransitionTime = time.Now()

		// Call enter handler for the new state
		enterHandler := sm.stateHandlers[newStatus.CurrentState]
		if enterHandler != nil {
			log.Printf("DEBUG: Calling OnEnter for node %s state %s",
				nodeName, newStatus.CurrentState)
			if updatedStatus, err := enterHandler.OnEnter(ctx, newStatus); err != nil {
				log.Printf("ERROR: Enter handler error for node %s: %v", nodeName, err)
			} else {
				newStatus = updatedStatus
			}
		}
	} else {
		log.Printf("DEBUG: Node %s remains in state %s after event %s",
			nodeName, status.CurrentState, event)
	}

	// Update CPU metrics if available and metrics collector is configured
	if sm.metricsCollector != nil && data != nil {
		var current, windowAvg float64

		if cpu, ok := data["cpuUtilization"].(float64); ok {
			current = cpu
			newStatus.CPUUtilization = cpu
		}

		if avg, ok := data["windowAverageUtilization"].(float64); ok {
			windowAvg = avg
			newStatus.WindowAverageUtilization = avg
		}

		sm.metricsCollector.UpdateCPUMetrics(nodeName, current, windowAvg)
	}

	// Copy message from data if provided
	if data != nil {
		if message, ok := data["message"].(string); ok && message != "" {
			newStatus.Message = message
		}

		// Copy cooldown end time if provided
		if cooldownTime, ok := data["coolDownEndTime"].(time.Time); ok {
			newStatus.CoolDownEndTime = cooldownTime
		}

		// Copy other status flags if provided
		if requested, ok := data["specUpRequested"].(bool); ok {
			newStatus.SpecUpRequested = requested
		}

		if completed, ok := data["specUpCompleted"].(bool); ok {
			newStatus.SpecUpCompleted = completed
		}

		if healthCheck, ok := data["healthCheckPassed"].(bool); ok {
			newStatus.HealthCheckPassed = healthCheck
		}
	}

	// Update status in map
	sm.statusMap[nodeName] = newStatus

	// Update the custom resource status
	if err := sm.updateCRStatus(ctx, nodeName, newStatus); err != nil {
		log.Printf("ERROR: Failed to update CR status for node %s: %v", nodeName, err)
	} else {
		log.Printf("DEBUG: Successfully updated CR status for node %s", nodeName)
	}

	return nil
}

// GetStatus returns the current status for a node
func (sm *stateMachine) GetStatus(ctx context.Context, nodeName string) (*domain.ControlPlaneStatus, error) {
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
func (sm *stateMachine) UpdateStatus(ctx context.Context, nodeName string, status *domain.ControlPlaneStatus) error {
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
func (sm *stateMachine) updateCRStatus(ctx context.Context, nodeName string, status *domain.ControlPlaneStatus) error {
	// Log what we're trying to update
	log.Printf("State machine updating CR status for node %s: state=%s, CPU=%.2f%%, window=%.2f%%",
		nodeName, status.CurrentState, status.CPUUtilization, status.WindowAverageUtilization)

	// Convert domain status to CRD-compatible format
	statusData := map[string]interface{}{
		"controlPlaneNodeName": nodeName,
		"cpuWinUsage":          status.WindowAverageUtilization,
		"coolDown":             status.CurrentState == domain.StateCoolDown,
		"updateStatus":         string(status.CurrentState),
		"message":              status.Message,
		"lastUpdateTime":       status.LastTransitionTime.Format(time.RFC3339),
	}

	// Always use "master" as the node name for the updater resource
	err := sm.resourceFactory.Status().UpdateGenericWithNode(ctx, "updater", "master", statusData)
	if err != nil {
		log.Printf("Failed to update CR status for node %s: %v", nodeName, err)
		return err
	}

	log.Printf("Successfully updated CR status for node %s via state machine", nodeName)
	return nil
}

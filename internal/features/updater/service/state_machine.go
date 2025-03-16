package service

import (
	"context"
	"errors"
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
	// Check for context cancellation at the beginning
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Log at the beginning of event handling
	log.Printf("STATE MACHINE: Handling event %s for node %s", event, nodeName)
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get or initialize node status
	status, exists := sm.statusMap[nodeName]
	if !exists {
		// Check if an initial state was provided in the data
		initialState := domain.StatePendingVmSpecUp
		if data != nil {
			if state, ok := data["initialState"].(domain.State); ok && state != "" {
				initialState = state
				log.Printf("DEBUG: Using provided initial state %s for node %s", initialState, nodeName)
			}
		}

		// Initialize with the determined state
		status = &domain.ControlPlaneStatus{
			NodeName:           nodeName,
			CurrentState:       initialState,
			LastTransitionTime: time.Now(),
			Message:            fmt.Sprintf("Initializing in %s state", initialState),
		}
		sm.statusMap[nodeName] = status
		log.Printf("DEBUG: Initializing new node %s in state %s", nodeName, status.CurrentState)
	}

	// Check for context cancellation again before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Record original state for metrics and logging
	originalState := status.CurrentState
	log.Printf("STATE MACHINE: Current state before handling event: %s", originalState)

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
		if errors.Is(err, context.Canceled) {
			log.Printf("Context canceled during event handling: %v", err)
			return err
		}
		log.Printf("ERROR: Handler error for node %s: %v", nodeName, err)
		return fmt.Errorf("error handling event %s in state %s: %w", event, status.CurrentState, err)
	}

	// Check for context cancellation after handling
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Log the potential new state
	log.Printf("DEBUG: After handler.Handle, potential new state is: %s (was %s)",
		newStatus.CurrentState, status.CurrentState)

	// If state has changed, perform transition
	if newStatus.CurrentState != status.CurrentState {
		log.Printf("STATE TRANSITION: Node %s: %s -> %s triggered by event %s",
			nodeName, status.CurrentState, newStatus.CurrentState, event)

		// Call exit handler for the current state
		exitHandler := sm.stateHandlers[status.CurrentState]
		if exitHandler != nil {
			log.Printf("DEBUG: Calling OnExit for node %s state %s",
				nodeName, status.CurrentState)
			if _, err := exitHandler.OnExit(ctx, status); err != nil {
				if errors.Is(err, context.Canceled) {
					log.Printf("Context canceled during exit handler: %v", err)
					return err
				}
				log.Printf("ERROR: Exit handler error for node %s: %v", nodeName, err)
			}
		}

		// Check for context cancellation before entering new state
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Update timestamp and perform transition
		newStatus.LastTransitionTime = time.Now()

		// Call enter handler for the new state
		enterHandler := sm.stateHandlers[newStatus.CurrentState]
		if enterHandler != nil {
			log.Printf("DEBUG: Calling OnEnter for node %s state %s",
				nodeName, newStatus.CurrentState)
			if updatedStatus, err := enterHandler.OnEnter(ctx, newStatus); err != nil {
				if errors.Is(err, context.Canceled) {
					log.Printf("Context canceled during enter handler: %v", err)
					return err
				}
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

	// Check for context cancellation before updating status
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Update status in map
	sm.statusMap[nodeName] = newStatus
	log.Printf("DEBUG: Final state after event handling: %s", newStatus.CurrentState)

	// Update the custom resource status
	if err := sm.updateCRStatus(ctx, nodeName, newStatus); err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("Context canceled during CR status update: %v", err)
			return err
		}
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
	// Check for context cancellation
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Log what we're trying to update
	log.Printf("State machine updating CR status for node %s: state=%s, CPU=%.2f%%, window=%.2f%%",
		nodeName, status.CurrentState, status.CPUUtilization, status.WindowAverageUtilization)

	// Convert domain status to CRD-compatible format
	statusData := map[string]interface{}{
		"controlPlaneNodeName": nodeName,
		"cpuWinUsage":          status.WindowAverageUtilization,
		"coolDown":             status.CurrentState == domain.StateCoolDown,
		//"updateStatus":         string(status.CurrentState),
		"message":        status.Message,
		"lastUpdateTime": status.LastTransitionTime.Format(time.RFC3339),
	}

	// Use "k2pupdater-master" as the resource name (from the CR snippet we can see the name format)
	err := sm.resourceFactory.Status().UpdateGenericWithNode(ctx, domain.UpdateKey, domain.ResourceName, statusData)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			log.Printf("Context canceled during CR status update: %v", err)
			return err
		}
		log.Printf("Failed to update CR status for node %s: %v", nodeName, err)
		return err
	}

	log.Printf("Successfully updated CR status for node %s via state machine", nodeName)
	return nil
}

package state

import (
	"context"
	"errors"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"math"
	"sync"
	"time"
)

// stateMachine implements the interfaces.StateMachine interface
type stateMachine struct {
	// statusMap stores the current status for each node
	statusMap map[string]*models.ControlPlaneStatus

	// stateHandlers maps states to their handlers
	stateHandlers map[models.State]interfaces.StateHandler

	// resourceFactory is used to update the custom resource status
	resourceFactory *resource.Factory

	// mu protects concurrent access to the status map
	mu sync.RWMutex
}

// NewStateMachine creates a new state machine
func NewStateMachine(
	resourceFactory *resource.Factory,
	stateHandlers map[models.State]interfaces.StateHandler,
) interfaces.StateMachine {
	sm := &stateMachine{
		statusMap:       make(map[string]*models.ControlPlaneStatus),
		stateHandlers:   make(map[models.State]interfaces.StateHandler),
		resourceFactory: resourceFactory,
	}

	// Use provided handlers if available, otherwise create defaults
	if stateHandlers != nil && len(stateHandlers) > 0 {
		for state, handler := range stateHandlers {
			sm.stateHandlers[state] = handler
		}
	} else {
		// Register default state handlers - will be implemented in factory
		sm.stateHandlers = createDefaultStateHandlers(resourceFactory)
	}

	return sm
}

// GetCurrentState returns the current state for a node
func (sm *stateMachine) GetCurrentState(ctx context.Context, nodeName string) (models.State, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status, exists := sm.statusMap[nodeName]
	if !exists {
		return "", common.NotFoundError("node %s not found in state machine", nodeName)
	}

	return status.CurrentState, nil
}

// HandleEvent processes an event and transitions to the next state if needed
func (sm *stateMachine) HandleEvent(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error {
	// Check for context cancellation at the beginning
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Log at the beginning of event handling
	log.Printf("STATE MACHINE: Handling event %s for node %s", event, nodeName)

	// First create a transaction object to hold the entire state transition
	transaction := &stateTransaction{
		nodeName:      nodeName,
		event:         event,
		data:          data,
		initialState:  "",
		targetState:   "",
		currentStatus: nil,
		newStatus:     nil,
		executed:      false,
	}

	// Prepare the transaction (read current state, determine new state)
	if err := sm.prepareTransaction(ctx, transaction); err != nil {
		return fmt.Errorf("failed to prepare transaction: %w", err)
	}

	// No state change needed, just update data
	if transaction.initialState == transaction.targetState {
		return sm.executeSimpleUpdate(ctx, transaction)
	}

	// Execute the full state transition as an atomic operation
	return sm.executeStateTransition(ctx, transaction)
}

// GetStatus returns the current status for a node
func (sm *stateMachine) GetStatus(ctx context.Context, nodeName string) (*models.ControlPlaneStatus, error) {
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
func (sm *stateMachine) UpdateStatus(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) error {
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
func (sm *stateMachine) updateCRStatus(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) error {
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
		"cpuWinUsage":          math.Round(status.WindowAverageUtilization*10) / 10,

		"coolDown": status.CurrentState == models.StateCoolDown,
		//"updateStatus":         string(status.CurrentState),
		"message":        status.Message,
		"lastUpdateTime": status.LastTransitionTime.Format(time.RFC3339),
	}

	// Use "k2pupdater-master" as the resource name (from the CR snippet we can see the name format)
	err := sm.resourceFactory.Status().UpdateGenericWithNode(ctx, models.UpdateKey, models.ResourceName, statusData)
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

// createDefaultStateHandlers creates the default set of state handlers
func createDefaultStateHandlers(resourceFactory *resource.Factory) map[models.State]interfaces.StateHandler {
	// Just call the CreateStateHandlers function from factory.go
	return CreateStateHandlers(resourceFactory)
}

// prepareTransaction reads current state and prepares the transaction
func (sm *stateMachine) prepareTransaction(ctx context.Context, tx *stateTransaction) error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Get or initialize node status
	status, exists := sm.statusMap[tx.nodeName]
	if !exists {
		// Initialize with default state
		initialState := models.StatePendingVmSpecUp
		if tx.data != nil {
			if state, ok := tx.data["initialState"].(models.State); ok && state != "" {
				initialState = state
			}
		}

		status = &models.ControlPlaneStatus{
			NodeName:           tx.nodeName,
			CurrentState:       initialState,
			LastTransitionTime: time.Now(),
			Message:            fmt.Sprintf("Initializing in %s state", initialState),
		}
	}

	// Make a deep copy of the status
	tx.currentStatus = &models.ControlPlaneStatus{}
	*tx.currentStatus = *status

	// Record initial state
	tx.initialState = status.CurrentState

	return nil
}

// executeSimpleUpdate handles updates that don't change state
func (sm *stateMachine) executeSimpleUpdate(ctx context.Context, tx *stateTransaction) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Get current state handler
	handler, exists := sm.stateHandlers[tx.initialState]
	if !exists {
		return fmt.Errorf("no handler found for state %s", tx.initialState)
	}

	// Handle the event
	newStatus, err := handler.Handle(ctx, tx.currentStatus, tx.event, tx.data)
	if err != nil {
		return err
	}

	// Update in memory and CR status
	sm.statusMap[tx.nodeName] = newStatus
	return sm.updateCRStatus(ctx, tx.nodeName, newStatus)
}

// executeStateTransition executes a full state transition atomically
func (sm *stateMachine) executeStateTransition(ctx context.Context, tx *stateTransaction) error {
	// Lock for the entire state transition to prevent race conditions
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check context cancellation again before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Verify current state hasn't changed since we prepared the transaction
	currentStatus, exists := sm.statusMap[tx.nodeName]
	if !exists || currentStatus.CurrentState != tx.initialState {
		return fmt.Errorf("state changed since transaction preparation (was %s, now %s)",
			tx.initialState, currentStatus.CurrentState)
	}

	// Get current state handler
	fromHandler, exists := sm.stateHandlers[tx.initialState]
	if !exists {
		return fmt.Errorf("no handler found for initial state %s", tx.initialState)
	}

	// Handle the event to determine target state
	newStatus, err := fromHandler.Handle(ctx, currentStatus, tx.event, tx.data)
	if err != nil {
		return fmt.Errorf("event handler error: %w", err)
	}

	// If state unchanged, just update and return
	if newStatus.CurrentState == tx.initialState {
		sm.statusMap[tx.nodeName] = newStatus
		return sm.updateCRStatus(ctx, tx.nodeName, newStatus)
	}

	// We're changing state, call exit handler
	if _, err := fromHandler.OnExit(ctx, currentStatus); err != nil {
		return fmt.Errorf("exit handler error: %w", err)
	}

	// Call enter handler for new state
	toHandler, exists := sm.stateHandlers[newStatus.CurrentState]
	if !exists {
		return fmt.Errorf("no handler found for target state %s", newStatus.CurrentState)
	}

	// Update transition time
	newStatus.LastTransitionTime = time.Now()

	// Call enter handler
	finalStatus, err := toHandler.OnEnter(ctx, newStatus)
	if err != nil {
		// We're in an inconsistent state! Attempt to revert
		log.Printf("ERROR: Enter handler failed, attempting to revert to previous state: %v", err)
		revertStatus := currentStatus
		revertStatus.Message = fmt.Sprintf("Failed to transition to %s: %v",
			newStatus.CurrentState, err)

		sm.statusMap[tx.nodeName] = revertStatus
		sm.updateCRStatus(ctx, tx.nodeName, revertStatus)
		return fmt.Errorf("enter handler error: %w", err)
	}

	// Update the status map and CR status
	sm.statusMap[tx.nodeName] = finalStatus
	return sm.updateCRStatus(ctx, tx.nodeName, finalStatus)
}

// New type for tracking state transitions
type stateTransaction struct {
	nodeName      string
	event         models.Event
	data          map[string]interface{}
	initialState  models.State
	targetState   models.State
	currentStatus *models.ControlPlaneStatus
	newStatus     *models.ControlPlaneStatus
	executed      bool
}

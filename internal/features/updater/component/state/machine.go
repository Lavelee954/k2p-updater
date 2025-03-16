package state

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/component/queue"
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

	// eventDispatcher handles events according to the transition matrix
	eventDispatcher *EventDispatcher

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
	// Create state machine instance
	sm := &stateMachine{
		statusMap:       make(map[string]*models.ControlPlaneStatus),
		stateHandlers:   stateHandlers,
		resourceFactory: resourceFactory,
	}

	// Create event dispatcher with transition matrix
	sm.eventDispatcher = NewEventDispatcher(stateHandlers)

	return sm
}

// initUpdateQueue initializes the update queue with this state machine as processor
func (sm *stateMachine) initUpdateQueue() {
	// Create the update queue with this state machine as the processor
	sm.updateQueue = queue.NewUpdateQueue(
		// Use an adapter to satisfy the NodeProcessor interface
		&nodeProcessorAdapter{sm: sm},
		queue.WithMaxQueueSize(200),
		queue.WithMaxRetries(3),
		queue.WithProcessingDelay(50*time.Millisecond),
	)
}

// nodeProcessorAdapter adapts the state machine to the NodeProcessor interface
type nodeProcessorAdapter struct {
	sm *stateMachine
}

// HandleEvent processes an event and performs the appropriate state transition
func (sm *stateMachine) HandleEvent(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error {
	// Check context early
	if err := common.CheckContextWithOp(ctx, "handling event"); err != nil {
		return err
	}

	// Log at the beginning of event handling
	log.Printf("STATE MACHINE: Handling event %s for node %s", event, nodeName)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get current status
	status, exists := sm.statusMap[nodeName]
	if !exists {
		// Initialize with default state if needed
		initialState := models.StatePendingVmSpecUp
		if data != nil {
			if state, ok := data["initialState"].(models.State); ok && state != "" {
				initialState = state
			}
		}

		status = &models.ControlPlaneStatus{
			NodeName:           nodeName,
			CurrentState:       initialState,
			LastTransitionTime: time.Now(),
			Message:            fmt.Sprintf("Initializing in %s state", initialState),
		}
		sm.statusMap[nodeName] = status
	}

	// Make a copy of the status
	currentStatus := &models.ControlPlaneStatus{}
	*currentStatus = *status

	// Record the initial state
	initialState := currentStatus.CurrentState

	// Dispatch the event
	newStatus, err := sm.eventDispatcher.DispatchEvent(ctx, currentStatus, event, data)
	if err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during event handling: %w", err)
		}
		return fmt.Errorf("error dispatching event: %w", err)
	}

	// Check if state has changed
	if newStatus.CurrentState != initialState {
		// We need to perform a state transition with exit and enter handlers

		// Call exit handler for current state
		handler, exists := sm.stateHandlers[initialState]
		if !exists {
			return fmt.Errorf("no handler found for state %s", initialState)
		}

		if _, err := handler.OnExit(ctx, currentStatus); err != nil {
			if common.IsContextCanceled(err) {
				return fmt.Errorf("context canceled during exit handler: %w", err)
			}
			return fmt.Errorf("exit handler error: %w", err)
		}

		// Update transition time
		newStatus.LastTransitionTime = time.Now()

		// Call enter handler for new state
		handler, exists = sm.stateHandlers[newStatus.CurrentState]
		if !exists {
			return fmt.Errorf("no handler found for target state %s", newStatus.CurrentState)
		}

		finalStatus, err := handler.OnEnter(ctx, newStatus)
		if err != nil {
			if common.IsContextCanceled(err) {
				return fmt.Errorf("context canceled during enter handler: %w", err)
			}

			// We're in an inconsistent state! Log error and attempt to revert
			log.Printf("ERROR: Enter handler failed, attempting to revert to previous state: %v", err)
			revertStatus := currentStatus
			revertStatus.Message = fmt.Sprintf("Failed to transition to %s: %v",
				newStatus.CurrentState, err)

			sm.statusMap[nodeName] = revertStatus
			sm.updateCRStatus(ctx, nodeName, revertStatus)
			return fmt.Errorf("enter handler error: %w", err)
		}

		// Update the status
		sm.statusMap[nodeName] = finalStatus
		return sm.updateCRStatus(ctx, nodeName, finalStatus)
	}

	// No state change, just update the status
	sm.statusMap[nodeName] = newStatus
	return sm.updateCRStatus(ctx, nodeName, newStatus)
}

// GetCurrentState returns the current state for a node
func (sm *stateMachine) GetCurrentState(ctx context.Context, nodeName string) (models.State, error) {
	// Check context before accessing state
	if err := common.CheckContext(ctx); err != nil {
		return "", fmt.Errorf("context error before getting current state: %w", err)
	}

	sm.mu.RLock()
	defer sm.mu.RUnlock()

	status, exists := sm.statusMap[nodeName]
	if !exists {
		return "", common.NotFoundError("node %s not found in state machine", nodeName)
	}

	return status.CurrentState, nil
}

// GetStatus returns the current status for a node
func (sm *stateMachine) GetStatus(ctx context.Context, nodeName string) (*models.ControlPlaneStatus, error) {
	// Check context before accessing status
	if err := common.CheckContext(ctx); err != nil {
		return nil, fmt.Errorf("context error before getting status: %w", err)
	}

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
	// Check context before updating status
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error before updating status: %w", err)
	}

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

// prepareTransaction reads current state and prepares the transaction
func (sm *stateMachine) prepareTransaction(ctx context.Context, tx *stateTransaction) error {
	// Check context before preparing transaction
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error before preparing transaction: %w", err)
	}

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

	// Check context after preparing transaction
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after preparing transaction: %w", err)
	}

	return nil
}

// executeSimpleUpdate handles updates that don't change state
func (sm *stateMachine) executeSimpleUpdate(ctx context.Context, tx *stateTransaction) error {
	// Check context before executing update
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error before executing simple update: %w", err)
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get current state handler
	handler, exists := sm.stateHandlers[tx.initialState]
	if !exists {
		return fmt.Errorf("no handler found for state %s", tx.initialState)
	}

	// Handle the event
	newStatus, err := handler.Handle(ctx, tx.currentStatus, tx.event, tx.data)
	if err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during event handling: %w", err)
		}
		return fmt.Errorf("error handling event: %w", err)
	}

	// Check context after handling event
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after handling event: %w", err)
	}

	// Update in memory and CR status
	sm.statusMap[tx.nodeName] = newStatus
	return sm.updateCRStatus(ctx, tx.nodeName, newStatus)
}

// executeStateTransition executes a full state transition atomically
func (sm *stateMachine) executeStateTransition(ctx context.Context, tx *stateTransaction) error {
	// Check context before executing state transition
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error before executing state transition: %w", err)
	}

	// Lock for the entire state transition to prevent race conditions
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Verify current state hasn't changed since we prepared the transaction
	currentStatus, exists := sm.statusMap[tx.nodeName]
	if !exists {
		return fmt.Errorf("node %s no longer exists in state machine", tx.nodeName)
	}

	// Store the current state for comparison
	currentState := currentStatus.CurrentState

	// Check if state has changed since transaction preparation
	if currentState != tx.initialState {
		return fmt.Errorf("state changed since transaction preparation (was %s, now %s)",
			tx.initialState, currentState)
	}

	// Get current state handler
	fromHandler, exists := sm.stateHandlers[tx.initialState]
	if !exists {
		return fmt.Errorf("no handler found for initial state %s", tx.initialState)
	}

	// Handle the event to determine target state
	newStatus, err := fromHandler.Handle(ctx, currentStatus, tx.event, tx.data)
	if err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during event handling: %w", err)
		}
		return fmt.Errorf("event handler error: %w", err)
	}

	// Check context after handling event
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after handling event: %w", err)
	}

	// If state unchanged, just update and return
	if newStatus.CurrentState == tx.initialState {
		sm.statusMap[tx.nodeName] = newStatus
		return sm.updateCRStatus(ctx, tx.nodeName, newStatus)
	}

	// We're changing state, call exit handler
	if _, err := fromHandler.OnExit(ctx, currentStatus); err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during exit handler: %w", err)
		}
		return fmt.Errorf("exit handler error: %w", err)
	}

	// Check context after exit handler
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after exit handler: %w", err)
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
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during enter handler: %w", err)
		}

		// We're in an inconsistent state! Attempt to revert
		log.Printf("ERROR: Enter handler failed, attempting to revert to previous state: %v", err)
		revertStatus := currentStatus
		revertStatus.Message = fmt.Sprintf("Failed to transition to %s: %v",
			newStatus.CurrentState, err)

		sm.statusMap[tx.nodeName] = revertStatus
		sm.updateCRStatus(ctx, tx.nodeName, revertStatus)
		return fmt.Errorf("enter handler error: %w", err)
	}

	// Check context after enter handler
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after enter handler: %w", err)
	}

	// Update the status map and CR status
	sm.statusMap[tx.nodeName] = finalStatus
	return sm.updateCRStatus(ctx, tx.nodeName, finalStatus)
}

// updateCRStatus updates the control plane status in the custom resource
func (sm *stateMachine) updateCRStatus(ctx context.Context, nodeName string, status *models.ControlPlaneStatus) error {
	// Convert domain status to CRD-compatible format
	statusData := map[string]interface{}{
		"controlPlaneNodeName": nodeName,
		"cpuWinUsage":          math.Round(status.WindowAverageUtilization*10) / 10,
		"coolDown":             status.CurrentState == models.StateCoolDown,
		"message":              status.Message,
		"lastUpdateTime":       status.LastTransitionTime.Format(time.RFC3339),
	}

	// Update the CR status
	err := sm.resourceFactory.Status().UpdateGenericWithNode(ctx, models.UpdateKey, models.ResourceName, statusData)
	if err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during CR status update: %w", err)
		}
		log.Printf("Failed to update CR status for node %s: %v", nodeName, err)
		return fmt.Errorf("failed to update CR status: %w", err)
	}

	log.Printf("Successfully updated CR status for node %s", nodeName)
	return nil
}

// Close shuts down the state machine
func (sm *stateMachine) Close() {
	if sm.updateQueue != nil {
		sm.updateQueue.Shutdown()
	}
}

// createDefaultStateHandlers creates the default set of state handlers
func createDefaultStateHandlers(resourceFactory *resource.Factory) map[models.State]interfaces.StateHandler {
	// Just call the CreateStateHandlers function from factory.go
	return CreateStateHandlers(resourceFactory)
}

// ProcessNodeOperation implements the NodeProcessor interface
func (a *nodeProcessorAdapter) ProcessNodeOperation(
	ctx context.Context,
	nodeName string,
	event models.Event,
	data map[string]interface{},
) error {
	// Create a transaction object to hold the state transition
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

	// Prepare the transaction
	if err := a.sm.prepareTransaction(ctx, transaction); err != nil {
		return fmt.Errorf("failed to prepare transaction: %w", err)
	}

	// Check context after preparation
	if err := common.CheckContext(ctx); err != nil {
		return fmt.Errorf("context error after preparing transaction: %w", err)
	}

	// No state change needed, just update data
	if transaction.targetState == "" || transaction.initialState == transaction.targetState {
		return a.sm.executeSimpleUpdate(ctx, transaction)
	}

	// Execute the full state transition
	return a.sm.executeStateTransition(ctx, transaction)
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

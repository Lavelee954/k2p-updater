package state

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/component/queue"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
)

// stateMachine implements interfaces.StateMachine
type stateMachine struct {
	statusMap       map[string]*models.ControlPlaneStatus
	stateHandlers   map[models.State]interfaces.StateHandler
	eventDispatcher *EventDispatcher
	resourceFactory *resource.Factory
	updateQueue     *queue.UpdateQueue
	mu              sync.RWMutex
}

// NewStateMachine creates a new state machine
func NewStateMachine(
	resourceFactory *resource.Factory,
	stateHandlers map[models.State]interfaces.StateHandler,
) interfaces.StateMachine {
	sm := &stateMachine{
		statusMap:       make(map[string]*models.ControlPlaneStatus),
		stateHandlers:   stateHandlers,
		resourceFactory: resourceFactory,
	}

	sm.eventDispatcher = NewEventDispatcher(stateHandlers)
	sm.initUpdateQueue()

	return sm
}

// HandleEvent processes an event and performs the appropriate state transition
func (sm *stateMachine) HandleEvent(ctx context.Context, nodeName string, event models.Event, data map[string]interface{}) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("context error before handling event: %w", err)
	}

	log.Printf("STATE MACHINE: Handling event %s for node %s", event, nodeName)

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Get or initialize current status
	status, exists := sm.statusMap[nodeName]
	if !exists {
		status = sm.initializeNodeStatus(nodeName, data)
		sm.statusMap[nodeName] = status
	}

	// Make a copy of the status
	currentStatus := &models.ControlPlaneStatus{}
	*currentStatus = *status

	// Record the initial state for transition tracking
	initialState := currentStatus.CurrentState

	// Process the event
	newStatus, err := sm.eventDispatcher.DispatchEvent(ctx, currentStatus, event, data)
	if err != nil {
		if common.IsContextCanceled(err) {
			return fmt.Errorf("context canceled during event handling: %w", err)
		}
		return fmt.Errorf("error dispatching event: %w", err)
	}

	// Check if state has changed
	if newStatus.CurrentState != initialState {
		return sm.handleStateTransition(ctx, nodeName, initialState, currentStatus, newStatus)
	}

	// No state change, just update the status
	sm.statusMap[nodeName] = newStatus
	return sm.updateCRStatus(ctx, nodeName, newStatus)
}

// handleStateTransition manages state transitions including exit and enter handlers
func (sm *stateMachine) handleStateTransition(
	ctx context.Context,
	nodeName string,
	initialState models.State,
	currentStatus *models.ControlPlaneStatus,
	newStatus *models.ControlPlaneStatus,
) error {
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

// GetCurrentState returns the current state for a node
func (sm *stateMachine) GetCurrentState(ctx context.Context, nodeName string) (models.State, error) {
	if err := ctx.Err(); err != nil {
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
	if err := ctx.Err(); err != nil {
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
	if err := ctx.Err(); err != nil {
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

// nodeProcessorAdapter adapts the state machine to the NodeProcessor interface
type nodeProcessorAdapter struct {
	sm *stateMachine
}

// ProcessNodeOperation implements the NodeProcessor interface
func (a *nodeProcessorAdapter) ProcessNodeOperation(
	ctx context.Context,
	nodeName string,
	event models.Event,
	data map[string]interface{},
) error {
	return a.sm.HandleEvent(ctx, nodeName, event, data)
}

// initializeNodeStatus creates a new node status with default values
func (sm *stateMachine) initializeNodeStatus(nodeName string, data map[string]interface{}) *models.ControlPlaneStatus {
	initialState := models.StatePendingVmSpecUp
	if data != nil {
		if state, ok := data["initialState"].(models.State); ok && state != "" {
			initialState = state
		}
	}

	return &models.ControlPlaneStatus{
		NodeName:           nodeName,
		CurrentState:       initialState,
		LastTransitionTime: time.Now(),
		Message:            fmt.Sprintf("Initializing in %s state", initialState),
	}
}

// initUpdateQueue initializes the update queue
func (sm *stateMachine) initUpdateQueue() {
	sm.updateQueue = queue.NewUpdateQueue(
		&nodeProcessorAdapter{sm: sm},
		queue.WithMaxQueueSize(200),
		queue.WithMaxRetries(3),
		queue.WithProcessingDelay(50*time.Millisecond),
	)
}

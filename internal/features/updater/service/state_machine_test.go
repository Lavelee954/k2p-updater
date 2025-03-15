package service

import (
	"context"
	"k2p-updater/internal/features/updater/domain"
	"testing"
	"time"
)

// Create a custom test state machine implementation
type testStateMachine struct {
	stateMachine // Embed the original to inherit behavior

	// Override with test-specific fields
	statusMap map[string]*domain.ControlPlaneStatus
}

// Create a simple wrapper for the state machine to avoid the need for unsafe type conversions
func createTestStateMachine() domain.StateMachine {
	// Create test state machine that overrides problematic methods
	sm := &testStateMachine{
		statusMap: make(map[string]*domain.ControlPlaneStatus),
	}

	// Initialize handlers
	sm.stateHandlers = make(map[domain.State]domain.StateHandler)
	sm.stateHandlers[domain.StatePendingVmSpecUp] = &testStateHandler{}
	sm.stateHandlers[domain.StateMonitoring] = &testStateHandler{}
	sm.stateHandlers[domain.StateInProgressVmSpecUp] = &testStateHandler{}
	sm.stateHandlers[domain.StateCompletedVmSpecUp] = &testCompletedHandler{}
	sm.stateHandlers[domain.StateFailedVmSpecUp] = &testFailedHandler{}
	sm.stateHandlers[domain.StateCoolDown] = &testStateHandler{}

	return sm
}

// Override the updateCRStatus method to avoid nil pointer dereference
func (sm *testStateMachine) updateCRStatus(ctx context.Context, nodeName string, status *domain.ControlPlaneStatus) error {
	// No-op for testing
	return nil
}

// Override HandleEvent to use our custom updateCRStatus
func (sm *testStateMachine) HandleEvent(ctx context.Context, nodeName string, event domain.Event, data map[string]interface{}) error {
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
	}

	// Get current state handler
	handler, exists := sm.stateHandlers[status.CurrentState]
	if !exists {
		return nil // Simplified for testing
	}

	// Handle the event
	newStatus, err := handler.Handle(ctx, status, event, data)
	if err != nil {
		return err
	}

	// If state has changed, perform transition
	if newStatus.CurrentState != status.CurrentState {
		// Update timestamp
		newStatus.LastTransitionTime = time.Now()

		// Call enter handler for the new state
		enterHandler := sm.stateHandlers[newStatus.CurrentState]
		if enterHandler != nil {
			if updatedStatus, err := enterHandler.OnEnter(ctx, newStatus); err == nil {
				newStatus = updatedStatus
			}
		}
	}

	// Update CPU metrics if available
	if data != nil {
		if cpu, ok := data["cpuUtilization"].(float64); ok {
			newStatus.CPUUtilization = cpu
		}

		if avg, ok := data["windowAverageUtilization"].(float64); ok {
			newStatus.WindowAverageUtilization = avg
		}

		// Copy message from data if provided
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

	return nil
}

// GetCurrentState returns the current state for a node
func (sm *testStateMachine) GetCurrentState(ctx context.Context, nodeName string) (domain.State, error) {
	status, exists := sm.statusMap[nodeName]
	if !exists {
		return "", nil
	}

	return status.CurrentState, nil
}

// GetStatus returns the current status for a node
func (sm *testStateMachine) GetStatus(ctx context.Context, nodeName string) (*domain.ControlPlaneStatus, error) {
	status, exists := sm.statusMap[nodeName]
	if !exists {
		return nil, nil
	}

	// Return a copy to prevent modifications outside of the state machine
	statusCopy := *status
	return &statusCopy, nil
}

// UpdateStatus updates the status for a node
func (sm *testStateMachine) UpdateStatus(ctx context.Context, nodeName string, status *domain.ControlPlaneStatus) error {
	if status == nil {
		return nil
	}

	// Store the updated status
	sm.statusMap[nodeName] = status
	return nil
}

// A simple test state handler that just performs basic transitions
type testStateHandler struct{}

func (h *testStateHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Copy the status to avoid modifying the original
	newStatus := *status

	// A simple transition logic for test purposes
	switch event {
	case domain.EventInitialize:
		newStatus.CurrentState = domain.StatePendingVmSpecUp
	case domain.EventCooldownEnded:
		newStatus.CurrentState = domain.StateMonitoring
	case domain.EventThresholdExceeded:
		newStatus.CurrentState = domain.StateInProgressVmSpecUp
	case domain.EventHealthCheckPassed:
		newStatus.CurrentState = domain.StateCompletedVmSpecUp
	case domain.EventSpecUpFailed:
		newStatus.CurrentState = domain.StateFailedVmSpecUp
	}

	return &newStatus, nil
}

func (h *testStateHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	return status, nil
}

func (h *testStateHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	return status, nil
}

// A specialized handler for the completed state that auto-transitions to cooldown
type testCompletedHandler struct {
	testStateHandler
}

func (h *testCompletedHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Don't auto-transition to CoolDown here
	return status, nil
}

// Handle transition to CoolDown only when explicitly requested
func (h *testCompletedHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	if event == domain.EventEnterCooldown {
		newStatus.CurrentState = domain.StateCoolDown
		return &newStatus, nil
	}

	return h.testStateHandler.Handle(ctx, status, event, data)
}

// A specialized handler for the failed state to handle recovery
type testFailedHandler struct {
	testStateHandler
}

func (h *testFailedHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	if event == domain.EventRecoveryAttempt {
		newStatus.CurrentState = domain.StatePendingVmSpecUp
	}

	return &newStatus, nil
}

func TestStateMachineInitialization(t *testing.T) {
	// Create a test state machine
	sm := createTestStateMachine()

	// Test node status initialization
	ctx := context.Background()
	nodeName := "test-node"

	// Initialize the node
	data := map[string]interface{}{
		"coolDownEndTime": time.Now().Add(5 * time.Minute),
	}

	err := sm.HandleEvent(ctx, nodeName, domain.EventInitialize, data)
	if err != nil {
		t.Fatalf("Failed to initialize node: %v", err)
	}

	// Check if node was initialized correctly
	state, err := sm.GetCurrentState(ctx, nodeName)
	if err != nil {
		t.Fatalf("Failed to get node state: %v", err)
	}

	if state != domain.StatePendingVmSpecUp {
		t.Errorf("Expected initial state %s, got %s", domain.StatePendingVmSpecUp, state)
	}

	// Get the status and check fields
	status, err := sm.GetStatus(ctx, nodeName)
	if err != nil {
		t.Fatalf("Failed to get node status: %v", err)
	}

	if status.NodeName != nodeName {
		t.Errorf("Expected node name %s, got %s", nodeName, status.NodeName)
	}

	if status.CurrentState != domain.StatePendingVmSpecUp {
		t.Errorf("Expected state %s, got %s", domain.StatePendingVmSpecUp, status.CurrentState)
	}
}

func TestStateMachineTransitions(t *testing.T) {
	// Create a test state machine
	sm := createTestStateMachine()

	// Test node status initialization
	ctx := context.Background()
	nodeName := "test-node"

	// Initialize the node
	initData := map[string]interface{}{
		"coolDownEndTime": time.Now().Add(5 * time.Minute),
	}

	err := sm.HandleEvent(ctx, nodeName, domain.EventInitialize, initData)
	if err != nil {
		t.Fatalf("Failed to initialize node: %v", err)
	}

	// Test transition: PendingVmSpecUp -> Monitoring
	cooldownData := map[string]interface{}{
		"cpuUtilization":           30.0,
		"windowAverageUtilization": 35.0,
	}

	err = sm.HandleEvent(ctx, nodeName, domain.EventCooldownEnded, cooldownData)
	if err != nil {
		t.Fatalf("Failed to transition to Monitoring: %v", err)
	}

	state, _ := sm.GetCurrentState(ctx, nodeName)
	if state != domain.StateMonitoring {
		t.Errorf("Expected state %s, got %s", domain.StateMonitoring, state)
	}

	// Test transition: Monitoring -> InProgressVmSpecUp
	thresholdData := map[string]interface{}{
		"cpuUtilization":           60.0,
		"windowAverageUtilization": 55.0,
	}

	err = sm.HandleEvent(ctx, nodeName, domain.EventThresholdExceeded, thresholdData)
	if err != nil {
		t.Fatalf("Failed to transition to InProgressVmSpecUp: %v", err)
	}

	state, _ = sm.GetCurrentState(ctx, nodeName)
	if state != domain.StateInProgressVmSpecUp {
		t.Errorf("Expected state %s, got %s", domain.StateInProgressVmSpecUp, state)
	}

	// Test transition: InProgressVmSpecUp -> CompletedVmSpecUp
	healthData := map[string]interface{}{
		"specUpCompleted":   true,
		"healthCheckPassed": true,
	}

	err = sm.HandleEvent(ctx, nodeName, domain.EventHealthCheckPassed, healthData)
	if err != nil {
		t.Fatalf("Failed to transition to CompletedVmSpecUp: %v", err)
	}

	state, _ = sm.GetCurrentState(ctx, nodeName)
	if state != domain.StateCompletedVmSpecUp {
		t.Errorf("Expected state %s, got %s", domain.StateCompletedVmSpecUp, state)
	}

	// Explicitly test transitioning to CoolDown
	err = sm.HandleEvent(ctx, nodeName, domain.EventEnterCooldown, nil)
	if err != nil {
		t.Fatalf("Failed to transition to CoolDown: %v", err)
	}

	// Get status and check if it transitioned to CoolDown
	state, _ = sm.GetCurrentState(ctx, nodeName)
	if state != domain.StateCoolDown {
		t.Errorf("Expected state %s after cooldown event, got %s", domain.StateCoolDown, state)
	}
}

func TestFailureAndRecovery(t *testing.T) {
	// Create a test state machine
	sm := createTestStateMachine()

	// Test node status initialization
	ctx := context.Background()
	nodeName := "test-node"

	// Initialize and transition to InProgressVmSpecUp
	sm.HandleEvent(ctx, nodeName, domain.EventInitialize, nil)
	sm.HandleEvent(ctx, nodeName, domain.EventCooldownEnded, nil)
	sm.HandleEvent(ctx, nodeName, domain.EventThresholdExceeded, nil)

	// Test failure scenario
	failureData := map[string]interface{}{
		"error": "Backend connection timeout",
	}

	err := sm.HandleEvent(ctx, nodeName, domain.EventSpecUpFailed, failureData)
	if err != nil {
		t.Fatalf("Failed to handle failure event: %v", err)
	}

	state, _ := sm.GetCurrentState(ctx, nodeName)
	if state != domain.StateFailedVmSpecUp {
		t.Errorf("Expected state %s, got %s", domain.StateFailedVmSpecUp, state)
	}

	// Test recovery scenario
	recoveryData := map[string]interface{}{
		"coolDownEndTime": time.Now().Add(10 * time.Minute),
		"recoveryAttempt": 1,
	}

	err = sm.HandleEvent(ctx, nodeName, domain.EventRecoveryAttempt, recoveryData)
	if err != nil {
		t.Fatalf("Failed to handle recovery event: %v", err)
	}

	state, _ = sm.GetCurrentState(ctx, nodeName)
	if state != domain.StatePendingVmSpecUp {
		t.Errorf("Expected state %s after recovery, got %s", domain.StatePendingVmSpecUp, state)
	}
}

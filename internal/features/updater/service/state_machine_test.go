package service

import (
	"context"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"testing"
	"time"
)

// mockResourceFactory implements minimal functionality for testing
type mockResourceFactory struct{}

func (m *mockResourceFactory) Status() resource.Status {
	return &mockStatus{}
}

func (m *mockResourceFactory) Event() resource.Event {
	return &mockEvent{}
}

type mockStatus struct{}

func (m *mockStatus) UpdateGenericWithNode(ctx context.Context, resourceKey string, nodeName string, newStatusData interface{}) error {
	return nil
}

func (m *mockStatus) Create(ctx context.Context, resourceKey, message, status string) error {
	return nil
}

func (m *mockStatus) Read(ctx context.Context, resourceKey, message, status string) error {
	return nil
}

func (m *mockStatus) Update(ctx context.Context, resourceKey, message, status string) error {
	return nil
}

func (m *mockStatus) UpdateGeneric(ctx context.Context, resourceKey string, newStatusData interface{}) error {
	return nil
}

type mockEvent struct{}

func (m *mockEvent) NormalRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
	return nil
}

func (m *mockEvent) WarningRecord(ctx context.Context, resourceKey, reason, message string, args ...interface{}) error {
	return nil
}

func (m *mockEvent) NormalRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	return nil
}

func (m *mockEvent) WarningRecordWithNode(ctx context.Context, resourceKey, nodeName, reason, message string, args ...interface{}) error {
	return nil
}

func TestStateMachineInitialization(t *testing.T) {
	// Create a mock resource factory
	mockFactory := &mockResourceFactory{}

	// Create a mock metrics collector
	mockMetrics := NewMockMetricsCollector()

	// Create state machine with both mocks
	sm := NewStateMachine(mockFactory, mockMetrics)

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
	// Create a mock resource factory
	mockFactory := &mockResourceFactory{}

	// Create state machine
	sm := NewStateMachine(mockFactory)

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

	// CompletedVmSpecUp should auto-transition to CoolDown
	// Get status and check if it transitioned automatically
	status, _ := sm.GetStatus(ctx, nodeName)
	if status.CurrentState != domain.StateCoolDown {
		t.Errorf("Expected auto-transition to %s, got %s", domain.StateCoolDown, status.CurrentState)
	}
}

func TestFailureAndRecovery(t *testing.T) {
	// Create a mock resource factory
	mockFactory := &mockResourceFactory{}

	// Create state machine
	sm := NewStateMachine(mockFactory)

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

// mockMetricsCollector implements minimal functionality for testing
type mockMetricsCollector struct{}

func NewMockMetricsCollector() *mockMetricsCollector {
	return &mockMetricsCollector{}
}

// Implementation of all the required methods with empty bodies
func (m *mockMetricsCollector) RecordTransitionLatency(nodeName string, fromState, toState domain.State, durationSeconds float64) {
	// No-op for testing
}

func (m *mockMetricsCollector) UpdateNodeState(nodeName string, oldState, newState domain.State) {
	// No-op for testing
}

func (m *mockMetricsCollector) UpdateCPUMetrics(nodeName string, current, windowAvg float64) {
	// No-op for testing
}

func (m *mockMetricsCollector) Register() {
	// No-op for testing
}

func (m *mockMetricsCollector) RecordRecoveryAttempt(nodeName string, successful bool) {
	// No-op for testing
}

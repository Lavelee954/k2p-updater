package handlers

import (
	"context"
	"testing"
	"time"

	"k2p-updater/internal/features/updater/domain/models"
)

// mockEventRecorder implements a test version of EventRecorder
type mockEventRecorder struct {
	events []string // Stores event information for verification
}

func (m *mockEventRecorder) NormalRecordWithNode(
	ctx context.Context,
	component,
	nodeName,
	reason,
	messageFmt string,
	args ...interface{},
) error {
	// Just record that an event happened, don't actually do anything
	m.events = append(m.events, reason)
	return nil
}

func (m *mockEventRecorder) WarningRecordWithNode(
	ctx context.Context,
	component,
	nodeName,
	reason,
	messageFmt string,
	args ...interface{},
) error {
	// Just record that an event happened, don't actually do anything
	m.events = append(m.events, "WARNING-"+reason)
	return nil
}

// mockStatusUpdater implements a test version of StatusUpdater
type mockStatusUpdater struct{}

func (m *mockStatusUpdater) UpdateGenericWithNode(
	ctx context.Context,
	component,
	nodeName string,
	statusData map[string]interface{},
) error {
	// Do nothing in the test
	return nil
}

// mockResourceFactory creates a properly mocked resource factory
type mockResourceFactory struct {
	eventRecorder *mockEventRecorder
	statusUpdater *mockStatusUpdater
}

func (m *mockResourceFactory) Event() interface{} {
	return m.eventRecorder
}

func (m *mockResourceFactory) Status() interface{} {
	return m.statusUpdater
}

// createMockResourceFactory creates a properly initialized mock factory
func createMockResourceFactory() *mockResourceFactory {
	return &mockResourceFactory{
		eventRecorder: &mockEventRecorder{events: make([]string, 0)},
		statusUpdater: &mockStatusUpdater{},
	}
}

// mockBaseStateHandler creates a BaseStateHandler with test implementation
func mockBaseStateHandler(events []models.Event) *BaseStateHandler {
	return &BaseStateHandler{
		resourceFactory: nil, // We'll override methods that use this
		supportedEvents: events,
	}
}

// monitoring_handler_test.go

func TestMonitoringHandler_Handle(t *testing.T) {
	// Create test handler
	handler := NewTestMonitoringHandler()

	// Create test status
	status := &models.ControlPlaneStatus{
		NodeName:                 "test-node",
		CurrentState:             models.StateMonitoring,
		LastTransitionTime:       time.Now(),
		Message:                  "Test status",
		CPUUtilization:           70.0,
		WindowAverageUtilization: 75.0,
	}

	// Test ThresholdExceeded event
	result, err := handler.Handle(context.Background(), status, models.EventThresholdExceeded, nil)
	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}

	if result.CurrentState != models.StateInProgressVmSpecUp {
		t.Errorf("Expected state to be %s, got %s", models.StateInProgressVmSpecUp, result.CurrentState)
	}

	// Test MonitoringStatus event
	data := map[string]interface{}{
		"cpuUtilization":           85.0,
		"windowAverageUtilization": 80.0,
	}

	result, err = handler.Handle(context.Background(), status, models.EventMonitoringStatus, data)
	if err != nil {
		t.Errorf("Handle returned error: %v", err)
	}

	if result.CurrentState != models.StateMonitoring {
		t.Errorf("Expected state to remain %s, got %s", models.StateMonitoring, result.CurrentState)
	}

	if result.CPUUtilization != 85.0 {
		t.Errorf("Expected CPU utilization to be %.1f, got %.1f", 85.0, result.CPUUtilization)
	}

	// Test unsupported event
	if handler.IsEventSupported(models.EventSpecUpRequested) {
		t.Error("EventSpecUpRequested should not be supported but was reported as supported")
	}
}

func TestMonitoringHandler_OnEnter(t *testing.T) {
	// Create test handler
	handler := NewTestMonitoringHandler()

	// Create test status
	status := &models.ControlPlaneStatus{
		NodeName:                 "test-node",
		CurrentState:             models.StateMonitoring,
		LastTransitionTime:       time.Now(),
		Message:                  "Test status",
		CPUUtilization:           70.0,
		WindowAverageUtilization: 75.0,
	}

	// Test OnEnter
	result, err := handler.OnEnter(context.Background(), status)
	if err != nil {
		t.Errorf("OnEnter returned error: %v", err)
	}

	if result.Message != "Entered monitoring state" {
		t.Errorf("Expected message to be 'Entered monitoring state', got '%s'", result.Message)
	}
}

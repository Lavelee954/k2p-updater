package state

import (
	"context"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"testing"
	"time"

	"k2p-updater/internal/features/updater/domain/models"
)

// mockStateHandler implements the interfaces.StateHandler interface for testing
type mockStateHandler struct {
	handleFunc     func(ctx context.Context, status *models.ControlPlaneStatus, event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error)
	onEnterFunc    func(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error)
	onExitFunc     func(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error)
	supportedEvent []models.Event
}

func (m *mockStateHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {
	return m.handleFunc(ctx, status, event, data)
}

func (m *mockStateHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	return m.onEnterFunc(ctx, status)
}

func (m *mockStateHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	return m.onExitFunc(ctx, status)
}

func (m *mockStateHandler) SupportedEvents() []models.Event {
	return m.supportedEvent
}

func TestEventDispatcher_DispatchEvent(t *testing.T) {
	// Create handlers map
	handlers := make(map[models.State]interfaces.StateHandler)

	// Create mock handler for Monitoring state
	monitoringHandler := &mockStateHandler{
		handleFunc: func(ctx context.Context, status *models.ControlPlaneStatus,
			event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {
			newStatus := *status
			if event == models.EventThresholdExceeded {
				newStatus.CurrentState = models.StateInProgressVmSpecUp
				newStatus.Message = "Threshold exceeded"
			}
			return &newStatus, nil
		},
		onEnterFunc: func(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
			return status, nil
		},
		onExitFunc: func(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
			return status, nil
		},
		supportedEvent: []models.Event{
			models.EventThresholdExceeded,
			models.EventMonitoringStatus,
		},
	}

	handlers[models.StateMonitoring] = monitoringHandler

	// Create dispatcher
	dispatcher := NewEventDispatcher(handlers)

	// Create test status
	status := &models.ControlPlaneStatus{
		NodeName:           "test-node",
		CurrentState:       models.StateMonitoring,
		LastTransitionTime: time.Now(),
		Message:            "Test status",
		CPUUtilization:     80.0,
	}

	// Test valid transition
	result, err := dispatcher.DispatchEvent(context.Background(), status, models.EventThresholdExceeded, nil)
	if err != nil {
		t.Errorf("DispatchEvent returned error: %v", err)
	}

	if result.CurrentState != models.StateInProgressVmSpecUp {
		t.Errorf("Expected state to be %s, got %s", models.StateInProgressVmSpecUp, result.CurrentState)
	}

	// Test invalid event
	_, err = dispatcher.DispatchEvent(context.Background(), status, models.EventSpecUpRequested, nil)
	if err == nil {
		t.Error("Expected error for invalid event, got nil")
	}
}

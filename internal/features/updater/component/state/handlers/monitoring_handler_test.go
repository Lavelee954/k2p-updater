package handlers

import (
	"context"
	"testing"
	"time"

	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
)

// mockResourceFactory creates a resource factory that doesn't do anything
func mockResourceFactory() *resource.Factory {
	// In a real implementation, you would create a mock of resource.Factory
	// For simplicity, we'll create a stub that doesn't do anything
	return &resource.Factory{}
}

func TestMonitoringHandler_Handle(t *testing.T) {
	// Create handler
	handler := NewMonitoringHandler(mockResourceFactory())

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
	_, err = handler.Handle(context.Background(), status, models.EventSpecUpRequested, nil)
	if err == nil {
		t.Error("Expected error for unsupported event, got nil")
	}
}

func TestMonitoringHandler_OnEnter(t *testing.T) {
	// Create handler
	handler := NewMonitoringHandler(mockResourceFactory())

	// Create test status
	status := &models.ControlPlaneStatus{
		NodeName:                 "test-node",
		CurrentState:             models.StatePendingVmSpecUp, // Previous state
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

	if result.Message == status.Message {
		t.Error("Expected message to be updated")
	}
}

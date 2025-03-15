package service

import (
	"context"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
)

// inProgressHandler handles the InProgressVmSpecUp state
type inProgressHandler struct {
	resourceFactory *resource.Factory
}

// newInProgressHandler creates a new handler for InProgressVmSpecUp state
func newInProgressHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &inProgressHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the InProgressVmSpecUp state
func (h *inProgressHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventSpecUpRequested:
		// Update status to indicate spec up was requested successfully
		newStatus.SpecUpRequested = true
		newStatus.Message = "VM spec up request sent to backend successfully"

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
		}

		return &newStatus, nil

	case domain.EventSpecUpCompleted:
		// Backend says spec up is complete, now waiting for health check
		newStatus.SpecUpCompleted = true
		newStatus.Message = "VM spec up completed, performing health check"
		return &newStatus, nil

	case domain.EventHealthCheckPassed:
		// When health check passes, transition to completed state
		newStatus.CurrentState = domain.StateCompletedVmSpecUp
		newStatus.HealthCheckPassed = true
		newStatus.Message = "VM spec up successful, health check passed"
		return &newStatus, nil

	case domain.EventHealthCheckFailed:
		// When health check fails, transition to failed state
		newStatus.CurrentState = domain.StateFailedVmSpecUp
		newStatus.HealthCheckPassed = false
		newStatus.Message = "VM spec up failed health check"
		return &newStatus, nil

	case domain.EventSpecUpFailed:
		// When spec up process fails for any reason
		newStatus.CurrentState = domain.StateFailedVmSpecUp
		newStatus.Message = "VM spec up failed"

		// Include error message if available
		if data != nil {
			if errMsg, ok := data["error"].(string); ok {
				newStatus.Message = "VM spec up failed: " + errMsg
			}
		}

		return &newStatus, nil
	}

	// Default: no state change for other events
	return &newStatus, nil
}

// OnEnter is called when entering the InProgressVmSpecUp state
func (h *inProgressHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	log.Printf("CRITICAL: OnEnter called for InProgressHandler - nodeName: %s", status.NodeName)
	newStatus := *status

	// Record the event
	log.Printf("CRITICAL: About to call resourceFactory.Event().NormalRecordWithNode")
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"InProgressVmSpecUp",
		"Node %s started VM spec up process with CPU utilization at %.2f%% (window avg: %.2f%%)",
		status.NodeName,
		status.CPUUtilization,
		status.WindowAverageUtilization,
	)
	log.Printf("CRITICAL: After calling NormalRecordWithNode, err: %v", err)

	if err != nil {
		log.Printf("Failed to record completion event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded completion event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

// OnExit is called when exiting the InProgressVmSpecUp state
func (h *inProgressHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
)

// inProgressHandler handles the InProgressVmSpecUp state
type inProgressHandler struct {
	*BaseStateHandler
}

// NewInProgressHandler creates a new handler for InProgressVmSpecUp state
func NewInProgressHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventSpecUpRequested,
		models.EventSpecUpCompleted,
		models.EventHealthCheckPassed,
		models.EventHealthCheckFailed,
		models.EventSpecUpFailed,
	}

	return &inProgressHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
	}
}

// Handle processes events for the InProgressVmSpecUp state
func (h *inProgressHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling in-progress state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in InProgressVmSpecUp state", event)
	}

	// Create a copy of the status to work with
	newStatus := *status

	log.Printf("IN_PROGRESS HANDLER: Processing event %s for node %s", event, status.NodeName)

	switch event {
	case models.EventSpecUpRequested:
		log.Printf("IN_PROGRESS HANDLER: Spec up requested for node %s", status.NodeName)
		// Update status to indicate spec up was requested successfully
		newStatus.SpecUpRequested = true
		newStatus.Message = "VM spec up request sent to backend successfully"

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
				log.Printf("IN_PROGRESS HANDLER: CPU utilization for node %s: %.2f%%", status.NodeName, cpu)
			}
		}

	case models.EventSpecUpCompleted:
		log.Printf("IN_PROGRESS HANDLER: Spec up completed for node %s, waiting for health check", status.NodeName)
		// Backend says spec up is complete, now waiting for health check
		newStatus.SpecUpCompleted = true
		newStatus.Message = "VM spec up completed, performing health check"

	case models.EventHealthCheckPassed:
		log.Printf("IN_PROGRESS HANDLER: Health check passed for node %s, transitioning to CompletedVmSpecUp", status.NodeName)
		// When health check passes, transition to completed state
		newStatus.CurrentState = models.StateCompletedVmSpecUp
		newStatus.HealthCheckPassed = true
		newStatus.Message = "VM spec up successful, health check passed"

	case models.EventHealthCheckFailed:
		log.Printf("IN_PROGRESS HANDLER: Health check failed for node %s, transitioning to FailedVmSpecUp", status.NodeName)
		// When health check fails, transition to failed state
		newStatus.CurrentState = models.StateFailedVmSpecUp
		newStatus.HealthCheckPassed = false
		newStatus.Message = "VM spec up failed health check"

	case models.EventSpecUpFailed:
		log.Printf("IN_PROGRESS HANDLER: Spec up failed for node %s, transitioning to FailedVmSpecUp", status.NodeName)
		// When spec up process fails for any reason
		newStatus.CurrentState = models.StateFailedVmSpecUp
		newStatus.Message = "VM spec up failed"

		// Include error message if available
		if data != nil {
			if errMsg, ok := data["error"].(string); ok {
				newStatus.Message = "VM spec up failed: " + errMsg
				log.Printf("IN_PROGRESS HANDLER: Failure reason for node %s: %s", status.NodeName, errMsg)
			}
		}
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing in-progress handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the InProgressVmSpecUp state
func (h *inProgressHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnEnter(ctx, status); err != nil {
		return nil, err
	}

	log.Printf("VM state transition: Node %s transitioning from %s to InProgressVmSpecUp",
		status.NodeName, status.CurrentState)

	newStatus := *status

	// Record the VM state transition event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		string(models.StateInProgressVmSpecUp),
		"Node %s VM state changed from %s to InProgressVmSpecUp with CPU utilization at %.2f%% (window avg: %.2f%%)",
		status.NodeName,
		status.CurrentState,
		status.CPUUtilization,
		status.WindowAverageUtilization,
	)

	if err != nil {
		log.Printf("Failed to record VM state transition event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded VM state transition event for node %s", status.NodeName)
	}

	newStatus.Message = fmt.Sprintf("Starting VM spec up operation with CPU at %.2f%%",
		status.WindowAverageUtilization)

	return &newStatus, nil
}

// OnExit is called when exiting the InProgressVmSpecUp state
func (h *inProgressHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Record the transition event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		string(models.StateInProgressVmSpecUp),
		"Node %s completed in-progress processing with outcome: %s",
		status.NodeName,
		status.Message,
	)

	if err != nil {
		log.Printf("Failed to record exit event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	}

	return status, nil
}

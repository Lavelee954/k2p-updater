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
	// Check for context cancellation at the beginning
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Create a copy of the status to work with
	newStatus := *status

	log.Printf("IN_PROGRESS HANDLER: Processing event %s for node %s", event, status.NodeName)

	switch event {
	case domain.EventSpecUpRequested:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

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

		return &newStatus, nil

	case domain.EventSpecUpCompleted:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Printf("IN_PROGRESS HANDLER: Spec up completed for node %s, waiting for health check", status.NodeName)
		// Backend says spec up is complete, now waiting for health check
		newStatus.SpecUpCompleted = true
		newStatus.Message = "VM spec up completed, performing health check"
		return &newStatus, nil

	case domain.EventHealthCheckPassed:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Printf("IN_PROGRESS HANDLER: Health check passed for node %s, transitioning to CompletedVmSpecUp", status.NodeName)
		// When health check passes, transition to completed state
		newStatus.CurrentState = domain.StateCompletedVmSpecUp
		newStatus.HealthCheckPassed = true
		newStatus.Message = "VM spec up successful, health check passed"
		return &newStatus, nil

	case domain.EventHealthCheckFailed:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Printf("IN_PROGRESS HANDLER: Health check failed for node %s, transitioning to FailedVmSpecUp", status.NodeName)
		// When health check fails, transition to failed state
		newStatus.CurrentState = domain.StateFailedVmSpecUp
		newStatus.HealthCheckPassed = false
		newStatus.Message = "VM spec up failed health check"
		return &newStatus, nil

	case domain.EventSpecUpFailed:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Printf("IN_PROGRESS HANDLER: Spec up failed for node %s, transitioning to FailedVmSpecUp", status.NodeName)
		// When spec up process fails for any reason
		newStatus.CurrentState = domain.StateFailedVmSpecUp
		newStatus.Message = "VM spec up failed"

		// Include error message if available
		if data != nil {
			if errMsg, ok := data["error"].(string); ok {
				newStatus.Message = "VM spec up failed: " + errMsg
				log.Printf("IN_PROGRESS HANDLER: Failure reason for node %s: %s", status.NodeName, errMsg)
			}
		}

		return &newStatus, nil
	default:
		// Check for context cancellation
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		log.Printf("IN_PROGRESS HANDLER: Unhandled event %s for node %s, no state change", event, status.NodeName)
	}

	// Final context check before returning the default response
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Default: no state change for other events
	return &newStatus, nil
}

// OnEnter is called when entering the InProgressVmSpecUp state
func (h *inProgressHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	log.Printf("VM state transition: Node %s transitioning from %s to InProgressVmSpecUp",
		status.NodeName, status.CurrentState)
	newStatus := *status

	// Record the VM state transition event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		domain.UpdateKey,
		status.NodeName,
		"VMStateTransition",
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

	return &newStatus, nil
}

// OnExit is called when exiting the InProgressVmSpecUp state
func (h *inProgressHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Check for context cancellation
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Nothing special to do on exit
	return status, nil
}

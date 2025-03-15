package service

import (
	"context"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
)

// failedHandler handles the FailedVmSpecUp state
type failedHandler struct {
	resourceFactory *resource.Factory
}

// newFailedHandler creates a new handler for FailedVmSpecUp state
func newFailedHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &failedHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the FailedVmSpecUp state
func (h *failedHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	// Failed state is a terminal state for this iteration
	// We don't expect transitions out of this state for now

	// However, in a real-world scenario, you might want to add recovery logic
	// or a retry mechanism here

	return &newStatus, nil
}

// OnEnter is called when entering the FailedVmSpecUp state
func (h *failedHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	// Get error message if available
	errorReason := "Unknown error"
	if status.Message != "" && status.Message != "VM spec up failed" {
		errorReason = status.Message
	}

	// Record the failed event
	h.resourceFactory.Event().WarningRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"FailedVmSpecUp",
		"Node %s failed VM spec up process: %s",
		status.NodeName,
		errorReason,
	)

	return &newStatus, nil
}

// OnExit is called when exiting the FailedVmSpecUp state
func (h *failedHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

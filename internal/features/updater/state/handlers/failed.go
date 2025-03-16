package handlers

import (
	"context"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// failedHandler handles the FailedVmSpecUp state
type failedHandler struct {
	resourceFactory *resource.Factory
}

// NewFailedHandler creates a new handler for FailedVmSpecUp state
func NewFailedHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	return &failedHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the FailedVmSpecUp state
func (h *failedHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus, event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case models.EventRecoveryAttempt:
		// Transition back to pending state for another attempt
		newStatus.CurrentState = models.StatePendingVmSpecUp
		newStatus.Message = "Attempting recovery from failed state"

		// Reset failure flags
		newStatus.SpecUpRequested = false
		newStatus.SpecUpCompleted = false
		newStatus.HealthCheckPassed = false

		// Set cooldown time if provided
		if data != nil {
			if cooldownTime, ok := data["coolDownEndTime"].(time.Time); ok {
				newStatus.CoolDownEndTime = cooldownTime
			} else {
				// Default recovery cooldown
				newStatus.CoolDownEndTime = time.Now().Add(10 * time.Minute)
			}
		}

		return &newStatus, nil
	}

	// Default: no state change for other events
	return &newStatus, nil
}

// OnEnter is called when entering the FailedVmSpecUp state
func (h *failedHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	newStatus := *status

	// Get error message if available
	errorReason := "Unknown error"
	if status.Message != "" && status.Message != "VM spec up failed" {
		errorReason = status.Message
	}

	// Record the failed event
	err := h.resourceFactory.Event().WarningRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"FailedVmSpecUp",
		"Node %s failed VM spec up process: %s",
		status.NodeName,
		errorReason,
	)

	if err != nil {
		log.Printf("Failed to record completion event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded completion event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

// OnExit is called when exiting the FailedVmSpecUp state
func (h *failedHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

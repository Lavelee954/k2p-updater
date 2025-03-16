package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// failedHandler handles the FailedVmSpecUp state
type failedHandler struct {
	*BaseStateHandler
}

// NewFailedHandler creates a new handler for FailedVmSpecUp state
func NewFailedHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventRecoveryAttempt,
	}

	return &failedHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
	}
}

// Handle processes events for the FailedVmSpecUp state
func (h *failedHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling failed state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in FailedVmSpecUp state", event)
	}

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

		log.Printf("Recovery attempt initiated for node %s with cooldown until %s",
			status.NodeName, newStatus.CoolDownEndTime.Format(time.RFC3339))
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing failed handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the FailedVmSpecUp state
func (h *failedHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnEnter(ctx, status); err != nil {
		return nil, err
	}

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
		log.Printf("Failed to record failure event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded failure event for node %s", status.NodeName)
	}

	// Set an appropriate message if not already set
	if newStatus.Message == "" || newStatus.Message == "VM spec up failed" {
		newStatus.Message = fmt.Sprintf("VM spec up failed: %s", errorReason)
	}

	return &newStatus, nil
}

// OnExit is called when exiting the FailedVmSpecUp state
func (h *failedHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Record the recovery event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"RecoveryFromFailure",
		"Node %s recovering from failed state, transitioning to PendingVmSpecUp",
		status.NodeName,
	)

	if err != nil {
		log.Printf("Failed to record recovery exit event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	}

	return status, nil
}

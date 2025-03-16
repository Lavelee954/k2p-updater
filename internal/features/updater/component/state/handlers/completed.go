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

// completedHandler handles the CompletedVmSpecUp state
type completedHandler struct {
	*BaseStateHandler
}

// NewCompletedHandler creates a new handler for CompletedVmSpecUp state
func NewCompletedHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventEnterCooldown,
	}

	return &completedHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
	}
}

// Handle processes events for the CompletedVmSpecUp state
func (h *completedHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling completed state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in CompletedVmSpecUp state", event)
	}

	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case models.EventEnterCooldown:
		// Transition to cooldown state
		newStatus.CurrentState = models.StateCoolDown

		// Set cooldown end time if provided
		if data != nil {
			if endTime, ok := data["coolDownEndTime"].(time.Time); ok {
				newStatus.CoolDownEndTime = endTime
			} else {
				// Default cooldown period if not specified (should be from config)
				cooldownPeriod := 5 * time.Minute
				if period, ok := data["cooldownPeriod"].(time.Duration); ok {
					cooldownPeriod = period
				}
				newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
			}
		}

		// Calculate cooldown duration and include it in the message
		cooldownMinutes := time.Until(newStatus.CoolDownEndTime).Minutes()
		newStatus.Message = fmt.Sprintf("VM spec up completed, entering cooldown period for %.1f minutes", cooldownMinutes)

		log.Printf("Node %s entering cooldown state for %.1f minutes after successful spec up",
			status.NodeName, cooldownMinutes)
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing completed handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the CompletedVmSpecUp state
func (h *completedHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnEnter(ctx, status); err != nil {
		return nil, err
	}

	newStatus := *status

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		"CompletedVmSpecUp",
		"Node %s completed VM spec up process successfully",
		status.NodeName,
	)

	if err != nil {
		log.Printf("Failed to record completion event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded completion event for node %s", status.NodeName)
	}

	// Set cooldown parameters and directly transition to CoolDown state
	cooldownPeriod := 10 * time.Minute // Set default cooldown period
	newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
	newStatus.Message = fmt.Sprintf("VM spec up completed, entering cooldown period for %.1f minutes", cooldownPeriod.Minutes())
	newStatus.CurrentState = models.StateCoolDown

	log.Printf("Node %s directly transitioned to CoolDown state with cooldown period of %.1f minutes",
		status.NodeName, cooldownPeriod.Minutes())

	return &newStatus, nil
}

// OnExit is called when exiting the CompletedVmSpecUp state
func (h *completedHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Since we're rarely in this state (direct transition to CoolDown in OnEnter),
	// just log the exit event
	log.Printf("Node %s exiting CompletedVmSpecUp state, transitioning to CoolDown", status.NodeName)

	return status, nil
}

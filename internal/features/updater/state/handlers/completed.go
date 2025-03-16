package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// completedHandler handles the CompletedVmSpecUp state
type completedHandler struct {
	resourceFactory *resource.Factory
}

// NewCompletedHandler creates a new handler for CompletedVmSpecUp state
func NewCompletedHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	return &completedHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the CompletedVmSpecUp state
func (h *completedHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus, event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {
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

		return &newStatus, nil
	}

	// Default: no state change for other events
	return &newStatus, nil
}

// OnEnter is called when entering the CompletedVmSpecUp state
func (h *completedHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
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
	// Nothing special to do on exit
	return status, nil
}

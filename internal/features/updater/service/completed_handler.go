package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"time"
)

// completedHandler handles the CompletedVmSpecUp state
type completedHandler struct {
	resourceFactory *resource.Factory
}

// newCompletedHandler creates a new handler for CompletedVmSpecUp state
func newCompletedHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &completedHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the CompletedVmSpecUp state
func (h *completedHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventEnterCooldown:
		// Transition to cooldown state
		newStatus.CurrentState = domain.StateCoolDown

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
func (h *completedHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	// Set up automatic transition to cooldown state
	// In a real implementation, we might want to trigger this via a goroutine
	// or scheduler rather than doing it immediately

	// Record the completion event
	h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"CompletedVmSpecUp",
		"Node %s successfully completed VM spec up process",
		status.NodeName,
	)

	// Automatically enter cooldown after recording completion
	cooldownPeriod := 5 * time.Minute
	data := map[string]interface{}{
		"cooldownPeriod": cooldownPeriod,
	}

	tempStatus, _ := h.Handle(ctx, &newStatus, domain.EventEnterCooldown, data)
	return tempStatus, nil
}

// OnExit is called when exiting the CompletedVmSpecUp state
func (h *completedHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

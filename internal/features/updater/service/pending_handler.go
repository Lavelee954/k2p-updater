package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
	"time"
)

// pendingHandler handles the PendingVmSpecUp state
type pendingHandler struct {
	resourceFactory *resource.Factory
}

// newPendingHandler creates a new handler for PendingVmSpecUp state
func newPendingHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &pendingHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the PendingVmSpecUp state
func (h *pendingHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventInitialize:
		// Stay in the same state, just update the message
		newStatus.Message = "Pending VM spec up, in cooldown period"
		return &newStatus, nil

	case domain.EventCooldownEnded:
		// Transition to Monitoring state when cooldown ends
		newStatus.CurrentState = domain.StateMonitoring
		newStatus.Message = "Monitoring CPU utilization"

		// Record CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		// Record transition in events
		h.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			"updater",
			status.NodeName,
			string(domain.StatePendingVmSpecUp),
			"Node %s exited cooldown period, starting CPU monitoring",
			status.NodeName,
		)

		return &newStatus, nil
	}

	// Default: no state change
	return &newStatus, nil
}

// OnEnter is called when entering the PendingVmSpecUp state
func (h *pendingHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"PendingVmSpecUp",
		"Node %s is in cooldown period, pending VM spec up",
		status.NodeName,
	)

	if err != nil {
		log.Printf("Failed to record event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded event for node %s", status.NodeName)
	}

	// If this is the first run and CoolDown is true (as per requirements)
	// Set CoolDown to true on initial startup of the application
	if newStatus.CoolDownEndTime.IsZero() {
		// Default cooldown period of 10 minutes
		cooldownPeriod := 10 * time.Minute
		newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
		newStatus.Message = fmt.Sprintf("Initial startup cooldown period for %.1f minutes",
			cooldownPeriod.Minutes())
	}

	return &newStatus, nil
}

// OnExit is called when exiting the PendingVmSpecUp state
func (h *pendingHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

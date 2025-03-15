package service

import (
	"context"
	"k2p-updater/internal/features/updater/domian"
	"k2p-updater/pkg/resource"
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
			"CooldownEnded",
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
	h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"PendingVmSpecUp",
		"Node %s is in cooldown period, pending VM spec up",
		status.NodeName,
	)

	return &newStatus, nil
}

// OnExit is called when exiting the PendingVmSpecUp state
func (h *pendingHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

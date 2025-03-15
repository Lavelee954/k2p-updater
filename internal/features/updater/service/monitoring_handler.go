package service

import (
	"context"
	updaterDomain "k2p-updater/internal/features/updater/domian"
	"k2p-updater/pkg/resource"
)

// monitoringHandler handles the Monitoring state
type monitoringHandler struct {
	resourceFactory *resource.Factory
}

// newMonitoringHandler creates a new handler for Monitoring state
func newMonitoringHandler(resourceFactory *resource.Factory) updaterDomain.StateHandler {
	return &monitoringHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the Monitoring state
func (h *monitoringHandler) Handle(ctx context.Context, status *updaterDomain.ControlPlaneStatus, event updaterDomain.Event, data map[string]interface{}) (*updaterDomain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case updaterDomain.EventThresholdExceeded:
		// Transition to InProgressVmSpecUp when CPU threshold is exceeded
		newStatus.CurrentState = updaterDomain.StateInProgressVmSpecUp
		newStatus.Message = "CPU threshold exceeded, initiating VM spec up"

		// Record CPU metrics
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		return &newStatus, nil

	case updaterDomain.EventInitialize:
		// Stay in Monitoring state, update metrics
		newStatus.Message = "Monitoring CPU utilization"

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		return &newStatus, nil
	}

	// Default: no state change
	return &newStatus, nil
}

// OnEnter is called when entering the Monitoring state
func (h *monitoringHandler) OnEnter(ctx context.Context, status *updaterDomain.ControlPlaneStatus) (*updaterDomain.ControlPlaneStatus, error) {
	newStatus := *status

	// Record the event
	h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"MonitoringStarted",
		"Node %s is now monitoring CPU utilization (current: %.2f%%, window avg: %.2f%%)",
		status.NodeName,
		status.CPUUtilization,
		status.WindowAverageUtilization,
	)

	return &newStatus, nil
}

// OnExit is called when exiting the Monitoring state
func (h *monitoringHandler) OnExit(ctx context.Context, status *updaterDomain.ControlPlaneStatus) (*updaterDomain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
)

// monitoringHandler handles the Monitoring state
type monitoringHandler struct {
	resourceFactory *resource.Factory
}

// newMonitoringHandler creates a new handler for Monitoring state
func newMonitoringHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &monitoringHandler{
		resourceFactory: resourceFactory,
	}
}

// Handle processes events for the Monitoring state
func (h *monitoringHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventThresholdExceeded:
		// Transition to InProgressVmSpecUp when CPU threshold is exceeded
		newStatus.CurrentState = domain.StateInProgressVmSpecUp
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

	case domain.EventInitialize:
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

	case domain.EventMonitoringStatus:
		// Update monitoring status with current CPU metrics
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		// Update message with current CPU utilization
		newStatus.Message = fmt.Sprintf("Monitoring CPU utilization: current %.2f%%, window avg %.2f%%",
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		// Record the event in CR message
		h.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			"updater",
			status.NodeName,
			"CPUMetricsUpdate",
			"Node %s CPU metrics: current %.2f%%, window avg %.2f%%",
			status.NodeName,
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization,
		)

		return &newStatus, nil
	}

	// Default: no state change
	return &newStatus, nil
}

// OnEnter is called when entering the Monitoring state
func (h *monitoringHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"MonitoringStarted",
		"Node %s is now monitoring CPU utilization (current: %.2f%%, window avg: %.2f%%)",
		status.NodeName,
		status.CPUUtilization,
		status.WindowAverageUtilization,
	)

	if err != nil {
		log.Printf("Failed to record completion event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded completion event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

// OnExit is called when exiting the Monitoring state
func (h *monitoringHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	log.Printf("DEBUG: MonitoringHandler.OnEnter called for node %s", status.NodeName)

	newStatus := *status

	// Record the event with detailed debugging
	log.Printf("DEBUG: About to create event for node %s via NormalRecordWithNode", status.NodeName)

	// Format parameters for logging
	eventMsg := fmt.Sprintf("Node %s is now monitoring CPU utilization (current: %.2f%%, window avg: %.2f%%)",
		status.NodeName,
		status.CPUUtilization,
		status.WindowAverageUtilization)
	log.Printf("DEBUG: Event message will be: %s", eventMsg)

	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		"updater",
		status.NodeName,
		"MonitoringStarted",
		eventMsg,
	)

	if err != nil {
		log.Printf("ERROR: Failed to create event for node %s: %v", status.NodeName, err)
	} else {
		log.Printf("SUCCESS: Created event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

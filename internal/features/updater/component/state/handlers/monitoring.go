package handlers

import (
	"context"
	"fmt"
	"k2p-updater/internal/common"
	"k2p-updater/internal/features/updater/domain/interfaces"
	"k2p-updater/internal/features/updater/domain/models"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// monitoringHandler handles the Monitoring state
type monitoringHandler struct {
	*BaseStateHandler
	lastEventTime map[string]time.Time
	eventMutex    sync.RWMutex
	eventInterval time.Duration // Minimum interval between events
}

// NewMonitoringHandler creates a new handler for Monitoring state
func NewMonitoringHandler(resourceFactory *resource.Factory) interfaces.StateHandler {
	// Define supported events for this state
	supportedEvents := []models.Event{
		models.EventThresholdExceeded,
		models.EventMonitoringStatus,
		models.EventInitialize,
	}

	return &monitoringHandler{
		BaseStateHandler: NewBaseStateHandler(resourceFactory, supportedEvents),
		lastEventTime:    make(map[string]time.Time),
		eventInterval:    10 * time.Minute,
	}
}

// Handle processes events for the Monitoring state
func (h *monitoringHandler) Handle(ctx context.Context, status *models.ControlPlaneStatus,
	event models.Event, data map[string]interface{}) (*models.ControlPlaneStatus, error) {

	// Check context first
	if err := common.CheckContextWithOp(ctx, "handling monitoring state event"); err != nil {
		return nil, err
	}

	// Verify the event is supported
	if !h.IsEventSupported(event) {
		return nil, fmt.Errorf("event %s is not supported in Monitoring state", event)
	}

	// Create a copy of the status to work with
	newStatus := *status

	log.Printf("MONITORING HANDLER: Processing event %s for node %s", event, status.NodeName)

	switch event {
	case models.EventThresholdExceeded:
		log.Printf("MONITORING HANDLER: CPU threshold exceeded for node %s, transitioning to InProgressVmSpecUp",
			status.NodeName)

		// Transition to InProgressVmSpecUp when CPU threshold is exceeded
		newStatus.CurrentState = models.StateInProgressVmSpecUp
		newStatus.Message = "CPU threshold exceeded, initiating VM spec up"

		// Record CPU metrics
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
				log.Printf("MONITORING HANDLER: CPU utilization for node %s: %.2f%%", status.NodeName, cpu)
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
				log.Printf("MONITORING HANDLER: Window average for node %s: %.2f%%", status.NodeName, windowAvg)
			}
		}

	case models.EventMonitoringStatus:
		log.Printf("MONITORING HANDLER: Status update event for node %s", status.NodeName)
		// Update monitoring status with current CPU metrics
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
				log.Printf("MONITORING HANDLER: CPU utilization for node %s: %.2f%%", status.NodeName, cpu)
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
				log.Printf("MONITORING HANDLER: Window average for node %s: %.2f%%", status.NodeName, windowAvg)
			}
		}

		// Update message with current CPU utilization
		newStatus.Message = fmt.Sprintf("Monitoring CPU utilization: current %.2f%%, window avg %.2f%%",
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		// Update the last transition time to ensure the CR message gets updated
		newStatus.LastTransitionTime = time.Now()

		// Log the status update
		log.Printf("MONITORING STATUS: Node %s monitoring status updated: CPU: %.2f%%, Avg: %.2f%%",
			status.NodeName,
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		// Check if enough time has passed since the last event
		h.eventMutex.RLock()
		lastTime, exists := h.lastEventTime[status.NodeName]
		h.eventMutex.RUnlock()

		now := time.Now()
		if !exists || now.Sub(lastTime) > h.eventInterval {
			// Record the periodic monitoring event
			err := h.resourceFactory.Event().NormalRecordWithNode(
				ctx,
				models.UpdateKey,
				status.NodeName,
				string(models.StatePendingVmSpecUp),
				"Node %s periodic monitoring report (10min): CPU current %.2f%%, window avg %.2f%%",
				status.NodeName,
				newStatus.CPUUtilization,
				newStatus.WindowAverageUtilization,
			)

			if err != nil {
				log.Printf("Failed to record monitoring event: %v", err)
				// Not returning error as this shouldn't prevent state update
			} else {
				// Update the last event time
				h.eventMutex.Lock()
				h.lastEventTime[status.NodeName] = now
				h.eventMutex.Unlock()

				log.Printf("Created 10-minute periodic monitoring event for node %s", status.NodeName)
			}
		}

	case models.EventInitialize:
		// Just update metrics without state change
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		// Keep existing message or set default
		if newStatus.Message == "" {
			newStatus.Message = fmt.Sprintf("Monitoring CPU utilization: current %.2f%%, window avg %.2f%%",
				newStatus.CPUUtilization,
				newStatus.WindowAverageUtilization)
		}
	}

	// Final context check
	if err := common.CheckContextWithOp(ctx, "completing monitoring handler"); err != nil {
		return nil, err
	}

	return &newStatus, nil
}

// OnEnter is called when entering the Monitoring state
func (h *monitoringHandler) OnEnter(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
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
		string(models.StateMonitoring),
		"Node %s is now monitoring CPU utilization (current: %.2f%%, window avg: %.2f%%)",
		status.NodeName,
		status.CPUUtilization,
		status.WindowAverageUtilization,
	)

	if err != nil {
		log.Printf("Failed to record monitoring entry event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded monitoring entry event for node %s", status.NodeName)
	}

	// Set an appropriate message
	newStatus.Message = fmt.Sprintf("Monitoring CPU utilization: current %.2f%%, window avg %.2f%%",
		newStatus.CPUUtilization,
		newStatus.WindowAverageUtilization)

	return &newStatus, nil
}

// OnExit is called when exiting the Monitoring state
func (h *monitoringHandler) OnExit(ctx context.Context, status *models.ControlPlaneStatus) (*models.ControlPlaneStatus, error) {
	// Call base method first for context check
	if _, err := h.BaseStateHandler.OnExit(ctx, status); err != nil {
		return nil, err
	}

	// Record the event with target state in the reason (will be filled later)
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		models.UpdateKey,
		status.NodeName,
		string(models.StatePendingVmSpecUp),
		"Node %s transitioning from Monitoring state with CPU utilization at %.2f%%",
		status.NodeName,
		status.WindowAverageUtilization,
	)

	if err != nil {
		log.Printf("Failed to record monitoring exit event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	}

	return status, nil
}

package service

import (
	"context"
	"fmt"
	"k2p-updater/internal/features/updater/domain"
	"k2p-updater/pkg/resource"
	"log"
	"sync"
	"time"
)

// monitoringHandler handles the Monitoring state
type monitoringHandler struct {
	resourceFactory *resource.Factory
	lastEventTime   map[string]time.Time
	eventMutex      sync.RWMutex
	eventInterval   time.Duration // Minimum interval between events
}

// newMonitoringHandler creates a new handler for Monitoring state
func newMonitoringHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &monitoringHandler{
		resourceFactory: resourceFactory,
		lastEventTime:   make(map[string]time.Time),
		eventInterval:   10 * time.Minute, // Changed to 10 minutes as required
	}
}

// Handle processes events for the Monitoring state
func (h *monitoringHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	log.Printf("MONITORING HANDLER: Processing event %s for node %s", event, status.NodeName)

	switch event {
	case domain.EventThresholdExceeded:
		log.Printf("MONITORING HANDLER: CPU threshold exceeded for node %s, transitioning to InProgressVmSpecUp", status.NodeName)
		// Transition to InProgressVmSpecUp when CPU threshold is exceeded
		newStatus.CurrentState = domain.StateInProgressVmSpecUp
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

		return &newStatus, nil

	case domain.EventMonitoringStatus:
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
			h.resourceFactory.Event().NormalRecordWithNode(
				ctx,
				"updater",
				status.NodeName,
				"PeriodicMonitoring",
				"Node %s periodic monitoring report (10min): CPU current %.2f%%, window avg %.2f%%",
				status.NodeName,
				newStatus.CPUUtilization,
				newStatus.WindowAverageUtilization,
			)

			// Update the last event time
			h.eventMutex.Lock()
			h.lastEventTime[status.NodeName] = now
			h.eventMutex.Unlock()

			log.Printf("Created 10-minute periodic monitoring event for node %s", status.NodeName)
		}

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
		string(domain.StatePendingVmSpecUp),
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
		string(domain.StatePendingVmSpecUp),
		eventMsg,
	)

	if err != nil {
		log.Printf("ERROR: Failed to create event for node %s: %v", status.NodeName, err)
	} else {
		log.Printf("SUCCESS: Created event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

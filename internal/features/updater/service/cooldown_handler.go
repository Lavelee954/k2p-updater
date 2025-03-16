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

// coolDownHandler handles the CoolDown state
type coolDownHandler struct {
	resourceFactory *resource.Factory
	lastEventTime   map[string]time.Time
	lastStatusTime  map[string]time.Time
	eventMutex      sync.RWMutex
	eventInterval   time.Duration // Interval between events
	statusInterval  time.Duration // Interval between CR status updates
}

// newCoolDownHandler creates a new handler for CoolDown state
func newCoolDownHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &coolDownHandler{
		resourceFactory: resourceFactory,
		lastEventTime:   make(map[string]time.Time),
		lastStatusTime:  make(map[string]time.Time),
		eventInterval:   5 * time.Minute, // Event interval
		statusInterval:  1 * time.Minute, // CR update interval set to 1 minute
	}
}

// Handle processes events for the CoolDown state
func (h *coolDownHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventInitialize:
		// Stay in the same state, just update the message
		if !newStatus.CoolDownEndTime.IsZero() {
			remaining := time.Until(newStatus.CoolDownEndTime)
			if remaining < 0 {
				remaining = 0
			}
			newStatus.Message = fmt.Sprintf("In cooldown period, %.1f minutes remaining",
				remaining.Minutes())
		} else {
			newStatus.Message = "In cooldown period"
		}

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

	case domain.EventCooldownStatus:
		// Update cooldown remaining message
		if !newStatus.CoolDownEndTime.IsZero() {
			remaining := time.Until(newStatus.CoolDownEndTime)
			if remaining < 0 {
				remaining = 0
			}
			newStatus.Message = fmt.Sprintf("In cooldown period, %.1f minutes remaining",
				remaining.Minutes())
		}

		// Update CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		// Check if it's time to update CR status (every 1 minute)
		h.eventMutex.RLock()
		lastStatusUpdate, statusExists := h.lastStatusTime[status.NodeName]
		h.eventMutex.RUnlock()

		now := time.Now()
		if !statusExists || now.Sub(lastStatusUpdate) > h.statusInterval {
			// Update the last transition time to ensure the CR message gets updated
			newStatus.LastTransitionTime = now

			// Update the last status update time
			h.eventMutex.Lock()
			h.lastStatusTime[status.NodeName] = now
			h.eventMutex.Unlock()

			log.Printf("Updating CR status for node %s (1-minute interval): %.1f minutes cooldown remaining",
				status.NodeName, time.Until(newStatus.CoolDownEndTime).Minutes())
		}

		// Log the status update
		log.Printf("COOLDOWN STATUS: Node %s cooldown status updated: %.1f minutes remaining, CPU: %.2f%%, Avg: %.2f%%",
			status.NodeName,
			time.Until(newStatus.CoolDownEndTime).Minutes(),
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		return &newStatus, nil

	case domain.EventCooldownEnded:
		// Transition DIRECTLY to Monitoring state when cooldown ends
		// Skip the PendingVmSpecUp state completely to avoid race conditions
		newStatus.CurrentState = domain.StateMonitoring
		newStatus.Message = "Cooldown period ended, now monitoring CPU utilization"

		// Record CPU metrics if available
		if data != nil {
			if cpu, ok := data["cpuUtilization"].(float64); ok {
				newStatus.CPUUtilization = cpu
			}
			if windowAvg, ok := data["windowAverageUtilization"].(float64); ok {
				newStatus.WindowAverageUtilization = windowAvg
			}
		}

		log.Printf("COOLDOWN ENDED: Node %s transitioning directly from CoolDown to Monitoring",
			status.NodeName)

		// Create a state transition event
		h.resourceFactory.Event().NormalRecordWithNode(
			ctx,
			domain.UpdateKey,
			status.NodeName,
			string(domain.StatePendingVmSpecUp),
			"Node %s state changed from CoolDown to Monitoring, cooldown period completed",
			status.NodeName,
		)

		return &newStatus, nil
	}

	// Default: no state change for other events
	return &newStatus, nil
}

// OnEnter is called when entering the CoolDown state
func (h *coolDownHandler) OnEnter(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	newStatus := *status

	// Make sure CoolDownEndTime is set
	if newStatus.CoolDownEndTime.IsZero() {
		// Default cooldown period (should come from config)
		cooldownPeriod := 5 * time.Minute
		newStatus.CoolDownEndTime = time.Now().Add(cooldownPeriod)
	}

	// Calculate cooldown duration
	cooldownMinutes := time.Until(newStatus.CoolDownEndTime).Minutes()

	// Record the event
	err := h.resourceFactory.Event().NormalRecordWithNode(
		ctx,
		domain.UpdateKey,
		status.NodeName,
		"CoolDown",
		"Node %s entered cooldown period for %.1f minutes after VM spec up",
		status.NodeName,
		cooldownMinutes,
	)
	if err != nil {
		log.Printf("Failed to record completion event for node %s: %v", status.NodeName, err)
		// Don't return error as we don't want to prevent state transition
	} else {
		log.Printf("Successfully recorded completion event for node %s", status.NodeName)
	}

	return &newStatus, nil
}

// OnExit is called when exiting the CoolDown state
func (h *coolDownHandler) OnExit(ctx context.Context, status *domain.ControlPlaneStatus) (*domain.ControlPlaneStatus, error) {
	// Nothing special to do on exit
	return status, nil
}

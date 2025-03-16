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
	eventMutex      sync.RWMutex
	eventInterval   time.Duration // Minimum interval between events
}

// newCoolDownHandler creates a new handler for CoolDown state
func newCoolDownHandler(resourceFactory *resource.Factory) domain.StateHandler {
	return &coolDownHandler{
		resourceFactory: resourceFactory,
		lastEventTime:   make(map[string]time.Time),
		eventInterval:   30 * time.Second, // Only create events every 30 seconds at most
	}
}

// Handle processes events for the CoolDown state
func (h *coolDownHandler) Handle(ctx context.Context, status *domain.ControlPlaneStatus, event domain.Event, data map[string]interface{}) (*domain.ControlPlaneStatus, error) {
	// Create a copy of the status to work with
	newStatus := *status

	switch event {
	case domain.EventInitialize:
		// ... existing initialize code ...
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

		// Update the last transition time to ensure the CR message gets updated
		newStatus.LastTransitionTime = time.Now()

		// Log the status update
		log.Printf("COOLDOWN STATUS: Node %s cooldown status updated: %.1f minutes remaining, CPU: %.2f%%, Avg: %.2f%%",
			status.NodeName,
			time.Until(newStatus.CoolDownEndTime).Minutes(),
			newStatus.CPUUtilization,
			newStatus.WindowAverageUtilization)

		// Check if enough time has passed since the last event
		h.eventMutex.RLock()
		lastTime, exists := h.lastEventTime[status.NodeName]
		h.eventMutex.RUnlock()

		now := time.Now()
		if !exists || now.Sub(lastTime) > h.eventInterval {
			// Create a status message event
			h.resourceFactory.Event().NormalRecordWithNode(
				ctx,
				"updater",
				status.NodeName,
				string(domain.StatePendingVmSpecUp),
				"Node %s cooldown update: %.1f minutes remaining, CPU: %.2f%%, Avg: %.2f%%",
				status.NodeName,
				time.Until(newStatus.CoolDownEndTime).Minutes(),
				newStatus.CPUUtilization,
				newStatus.WindowAverageUtilization,
			)

			// Update the last event time
			h.eventMutex.Lock()
			h.lastEventTime[status.NodeName] = now
			h.eventMutex.Unlock()
		}

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
		"updater",
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
